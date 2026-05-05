// Package wizard hosts multi-step interactive flows that the chat REPL
// can hand control to. Each wizard implements:
//
//   Begin(reply func(string))         called when wizard starts
//   Step(input string, reply func(string)) (done bool, err error)
//                                     called for each subsequent user input
//                                     until done=true
//
// install_brain.go — guide the user through the brain install. UX contract
// is plug-and-play: zero paths, zero URLs, zero terminal commands shown to
// the user. The wizard:
//   1. locates the bundled llama-server binary (or PATH fallback)
//   2. downloads the Bonsai GGUF over plain HTTPS with progress
//   3. spawns the brain as a managed subprocess and waits for /health
//
// On success the wizard stashes a *Brain handle that the REPL retrieves
// via RunningBrain(); the REPL is responsible for Stop()ing the brain on
// exit. The brain is a child of CHEW: when CHEW dies, the brain dies.
//
// All side-effecting behaviour (download, spawn, runtime lookup) is
// pluggable through fields on InstallBrain so tests don't touch the
// network or spawn processes.

package wizard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// InstallBrain is the brain-install state machine.
//
// Once the user says 'yes', all three steps run inline (no per-step
// keypress required) — that's the right shape for non-tech users.
type InstallBrain struct {
	step      installStep
	chewHome  string
	modelDir  string
	modelPath string
	platform  string

	// Hooks — replace in tests to avoid network/spawn side effects.
	acquireRuntimeFn func(brainDir string, progress func(done, total int64)) (string, error)
	downloadFn       func(url, dest string, progress func(done, total int64)) error
	startBrainFn     func(cfg BrainConfig) (*Brain, error)
	waitHealthyFn    func(ctx context.Context, b *Brain) error
	healthDeadline   time.Duration

	// Result of a successful run; nil otherwise.
	brain *Brain
}

type installStep int

const (
	stepShowPlan installStep = iota
	stepShowDetails
	stepDone
	stepAborted
)

// NewInstallBrain creates a fresh wizard. The chat REPL calls Begin to
// start; subsequent user inputs go to Step until done=true.
//
// Brain storage location is `<repo>/brain/` — the directory that contains
// the running CHEW binary, or the current working directory if we're
// invoked via `go run` (which puts the temp binary in TMPDIR). The goal is
// "delete the CHEWAgent folder to fully uninstall" — a hidden ~/.chew
// dotfile dir would defeat that for non-tech users.
func NewInstallBrain() *InstallBrain {
	root, _ := repoAnchor()
	brainDir := filepath.Join(root, "brain")
	return &InstallBrain{
		step:             stepShowPlan,
		chewHome:         brainDir,
		modelDir:         brainDir,
		modelPath:        filepath.Join(brainDir, installBrainModelFile),
		platform:         runtime.GOOS + "-" + runtime.GOARCH,
		acquireRuntimeFn: acquireLlamaServer,
		downloadFn:       defaultDownload,
		startBrainFn:     StartBrain,
		waitHealthyFn:    defaultWaitHealthy,
		healthDeadline:   60 * time.Second,
	}
}

// repoAnchor picks the directory the brain folder hangs off of. Prefers
// the binary's own directory (so a packaged CHEW puts brain/ next to
// itself); falls back to cwd when running under `go run`.
func repoAnchor() (string, error) {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if !isTempDir(dir) {
			return dir, nil
		}
	}
	return os.Getwd()
}

// isTempDir flags directories the OS puts `go run` temp binaries into,
// so we don't anchor brain storage at a path that vanishes mid-session.
func isTempDir(p string) bool {
	if tmp := os.TempDir(); tmp != "" && strings.HasPrefix(p, tmp) {
		return true
	}
	for _, pre := range []string{"/var/folders/", "/private/var/folders/", "/tmp/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

// Begin shows the plan; user's next input drives the state machine.
func (w *InstallBrain) Begin(reply func(string)) {
	reply(planText)
}

// Step processes one user input. After 'yes' it runs all three install
// steps inline and returns done=true.
func (w *InstallBrain) Step(input string, reply func(string)) (bool, error) {
	in := strings.ToLower(strings.TrimSpace(input))

	switch w.step {
	case stepShowPlan:
		switch in {
		case "yes", "y", "go":
			return w.runAll(reply)
		case "tell me more", "details":
			w.step = stepShowDetails
			reply(detailsText)
			return false, nil
		case "cancel", "no", "n":
			reply(cancelText)
			w.step = stepAborted
			return true, nil
		default:
			reply("Please answer 'yes', 'tell me more', or 'cancel'.")
			return false, nil
		}

	case stepShowDetails:
		// Any input returns to plan.
		reply(planText)
		w.step = stepShowPlan
		return false, nil

	case stepDone, stepAborted:
		return true, nil
	}
	return true, nil
}

// RunningBrain returns the brain spawned by a successful install, or nil
// if the wizard hasn't completed yet (or aborted).
func (w *InstallBrain) RunningBrain() *Brain {
	return w.brain
}

// runAll executes the three install steps in sequence. Failures along
// the way always halt the wizard (done=true) with a friendly user-facing
// message — never leak a path, URL, or stack trace to the user.
func (w *InstallBrain) runAll(reply func(string)) (bool, error) {
	if err := os.MkdirAll(w.modelDir, 0o755); err != nil {
		reply(fmt.Sprintf(downloadFailedText, "couldn't create my storage folder"))
		w.step = stepAborted
		return true, err
	}

	// [1/3] runtime — find an existing llama-server or auto-fetch one.
	reply(runtimeSetupText)
	progressCb := makeProgressEmitter(reply)
	binary, err := w.acquireRuntimeFn(w.modelDir, progressCb)
	if err != nil {
		reply(fmt.Sprintf(runtimeFailedText, summariseDownloadErr(err)))
		w.step = stepAborted
		return true, err
	}
	reply(runtimeReadyText)

	// [2/3] download Bonsai
	reply(downloadStartText)
	if err := w.downloadFn(installBrainModelURL, w.modelPath, progressCb); err != nil {
		reply(fmt.Sprintf(downloadFailedText, summariseDownloadErr(err)))
		w.step = stepAborted
		return true, err
	}
	reply(downloadDoneText)

	// [3/3] wake brain (also writes config so subsequent CHEW runs
	// know where to look).
	configPath := filepath.Join(w.chewHome, "config.json")
	if err := w.writeConfig(configPath); err != nil {
		reply(fmt.Sprintf(brainStartFailedText, "couldn't save settings"))
		w.step = stepAborted
		return true, err
	}
	reply(brainWakingText)
	brain, err := w.startBrainFn(BrainConfig{
		BinaryPath: binary,
		ModelPath:  w.modelPath,
		Alias:      "ChewBrain",
		Port:       8080,
		LogPath:    filepath.Join(w.chewHome, "brain.log"),
	})
	if err != nil {
		reply(fmt.Sprintf(brainStartFailedText, summariseSpawnErr(err)))
		w.step = stepAborted
		return true, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.healthDeadline)
	defer cancel()
	if err := w.waitHealthyFn(ctx, brain); err != nil {
		// Brain process is still alive but didn't respond. Stop it before
		// surrendering — leaking a half-alive llama-server is hostile.
		_ = brain.Stop()
		reply(brainHealthFailedText)
		w.step = stepAborted
		return true, err
	}
	w.brain = brain
	reply(brainAwakeText)

	w.step = stepDone
	return true, nil
}

// writeConfig writes ~/.chew/config.json with the model path baked in.
// Read by the REPL on startup (in a future change) to know where the
// brain lives without re-running the wizard.
func (w *InstallBrain) writeConfig(configPath string) error {
	body := strings.Join([]string{
		"{",
		`  "model_endpoint": "http://127.0.0.1:8080/v1/chat/completions",`,
		`  "model_alias": "ChewBrain",`,
		fmt.Sprintf(`  "model_path": %q`, w.modelPath),
		"}",
	}, "\n") + "\n"
	return os.WriteFile(configPath, []byte(body), 0o644)
}

// ----- defaults (override in tests) -----

// defaultDownload streams the URL to dest using stdlib net/http. Calls
// progress(done, total) every ~1 second so the wizard can emit a
// frog-flavoured progress line. No external deps, no hidden subprocess.
func defaultDownload(url, dest string, progress func(done, total int64)) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	// Don't set a request timeout — Bonsai is ~1.16 GB and a slow
	// connection can take 10+ minutes. The user can Ctrl-C if it stalls.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	total := resp.ContentLength
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	pw := &progressWriter{
		dest:        out,
		total:       total,
		progressCb:  progress,
		lastEmitted: time.Now(),
	}
	if _, err := io.Copy(pw, resp.Body); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("finalize: %w", err)
	}
	if progress != nil {
		// Final 100% tick.
		progress(total, total)
	}
	return nil
}

// progressWriter wraps an io.Writer (the destination file) and emits
// progress callbacks at most once per second.
type progressWriter struct {
	dest        io.Writer
	total       int64
	written     int64
	lastEmitted time.Time
	progressCb  func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.dest.Write(b)
	p.written += int64(n)
	if p.progressCb != nil && time.Since(p.lastEmitted) >= time.Second {
		p.progressCb(p.written, p.total)
		p.lastEmitted = time.Now()
	}
	return n, err
}

// defaultWaitHealthy is the production health-poller; tests override.
func defaultWaitHealthy(ctx context.Context, b *Brain) error {
	return b.WaitHealthy(ctx)
}

// makeProgressEmitter wraps a reply func into a progress callback that
// formats bytes as MB and emits one line per ~1-second tick.
func makeProgressEmitter(reply func(string)) func(done, total int64) {
	return func(done, total int64) {
		mbDone := float64(done) / (1024 * 1024)
		mbTotal := float64(total) / (1024 * 1024)
		var pct int
		if total > 0 {
			pct = int(100 * done / total)
		}
		reply(fmt.Sprintf(downloadProgressText, mbDone, mbTotal, pct))
	}
}

// summariseDownloadErr converts a download error into a short, friendly,
// non-technical phrase. The original error is still propagated to the
// caller for logging; this is purely for the chat output.
func summariseDownloadErr(err error) string {
	if err == nil {
		return "unknown"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "no such host"), strings.Contains(s, "lookup"):
		return "couldn't reach the internet"
	case strings.Contains(s, "timeout"), strings.Contains(s, "deadline"):
		return "the download timed out"
	case strings.Contains(s, "http status"):
		return "the source rejected the download"
	default:
		return "something went wrong"
	}
}

// summariseSpawnErr converts a spawn/runtime error into a friendly phrase.
func summariseSpawnErr(err error) string {
	if err == nil {
		return "unknown"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "permission"):
		return "I'm not allowed to start the runtime"
	case strings.Contains(s, "no such file"):
		return "the runtime is missing"
	default:
		return "something went wrong starting up"
	}
}

