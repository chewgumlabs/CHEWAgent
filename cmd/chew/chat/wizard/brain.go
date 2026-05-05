// brain.go — manages the llama-server subprocess that hosts CHEW's brain.
//
// Lifecycle:
//
//	b, err := StartBrain(BrainConfig{...})    spawns llama-server, returns handle
//	err := b.WaitHealthy(ctx)                  blocks until /health returns 200, or ctx cancels
//	err := b.Stop()                            sends SIGTERM, falls back to SIGKILL after 10s
//
// The brain is owned by whoever called StartBrain. Typical usage: the REPL
// starts the brain at chat-shell launch time (after the wizard has written
// a config), holds the *Brain reference, and calls Stop() on exit.
//
// Binary lookup + auto-fetch lives in runtime_install.go (acquireLlamaServer).
// All knobs are explicit fields on BrainConfig so tests can dial them.

package wizard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// BrainConfig captures everything needed to launch llama-server.
type BrainConfig struct {
	BinaryPath string // absolute path to llama-server
	ModelPath  string // absolute path to GGUF
	Alias      string // model alias served on the OpenAI-compat endpoint
	Port       int    // localhost port to bind
	LogPath    string // where to redirect stdout/stderr
}

// BrainMeta is ownership metadata written alongside a running brain so
// concurrent CHEW sessions can distinguish an active brain from an orphan.
type BrainMeta struct {
	BrainPID  int    `json:"brain_pid"`
	OwnerPID  int    `json:"owner_pid"`
	StartedAt string `json:"started_at"`
	Command   string `json:"command"`
}

// Process-inspection hooks — tests override these to avoid real signals/exec.
var (
	checkProcessAlive = processAlive
	checkLlamaServer  = looksLikeLlamaServer
	killBrainProcess  = defaultKillBrainProcess
)

// processAlive returns true if a process with the given PID exists.
// Uses the Unix signal-0 trick; works on macOS and Linux.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// Brain is a running llama-server subprocess.
type Brain struct {
	cmd      *exec.Cmd
	endpoint string // e.g. "http://127.0.0.1:8080"
	logPath  string
	metaPath string // <brainDir>/brain.pid.json; written on Start, removed on Stop
	stopOnce bool
}

// StartBrain spawns llama-server with the given config. Returns immediately
// after the process is launched; call WaitHealthy to confirm readiness.
//
// We deliberately do NOT put llama-server in its own process group: keeping
// it in CHEW's foreground group means SIGHUP from a closed terminal kills
// it for free, no signal handler needed. The metadata file written here
// records both the brain PID and the owner CHEW PID, so a concurrent
// session's KillStaleBrain can skip an active brain and only reap orphans.
func StartBrain(cfg BrainConfig) (*Brain, error) {
	if cfg.BinaryPath == "" {
		return nil, errors.New("BrainConfig.BinaryPath is required")
	}
	if cfg.ModelPath == "" {
		return nil, errors.New("BrainConfig.ModelPath is required")
	}
	if cfg.Alias == "" {
		cfg.Alias = "ChewBrain"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	args := []string{
		"--model", cfg.ModelPath,
		"--alias", cfg.Alias,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", cfg.Port),
		// Bonsai is small; let llama.cpp pick sensible defaults for ctx and
		// threads. Add --reasoning-budget 0 if/when we ship a thinking model.
	}

	cmd := exec.Command(cfg.BinaryPath, args...)

	// Redirect output to a log file so we don't pollute the chat window.
	if cfg.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
			return nil, fmt.Errorf("brain log dir: %w", err)
		}
		f, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("brain log file: %w", err)
		}
		cmd.Stdout = f
		cmd.Stderr = f
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting llama-server: %w", err)
	}

	metaPath := ""
	if cfg.LogPath != "" {
		metaPath, _ = writeBrainMeta(filepath.Dir(cfg.LogPath), BrainMeta{
			BrainPID:  cmd.Process.Pid,
			OwnerPID:  os.Getpid(),
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			Command:   cfg.BinaryPath,
		})
	}

	return &Brain{
		cmd:      cmd,
		endpoint: fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
		logPath:  cfg.LogPath,
		metaPath: metaPath,
	}, nil
}

// Endpoint returns the http://host:port the brain is serving on.
func (b *Brain) Endpoint() string { return b.endpoint }

// LogPath returns the file the brain writes its logs to (may be empty).
func (b *Brain) LogPath() string { return b.logPath }

// PID returns the OS process ID of the running brain, or 0 if not started.
func (b *Brain) PID() int {
	if b == nil || b.cmd == nil || b.cmd.Process == nil {
		return 0
	}
	return b.cmd.Process.Pid
}

// WaitHealthy polls /health until it returns 200 or ctx is canceled.
// The default poll interval is 500ms; tests can override via
// WaitHealthyOpts.
func (b *Brain) WaitHealthy(ctx context.Context) error {
	return b.WaitHealthyOpts(ctx, WaitHealthyOpts{
		PollInterval: 500 * time.Millisecond,
		HTTPClient:   &http.Client{Timeout: 2 * time.Second},
	})
}

// WaitHealthyOpts is the testable variant.
type WaitHealthyOpts struct {
	PollInterval time.Duration
	HTTPClient   *http.Client
}

func (b *Brain) WaitHealthyOpts(ctx context.Context, opts WaitHealthyOpts) error {
	if opts.PollInterval == 0 {
		opts.PollInterval = 500 * time.Millisecond
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 2 * time.Second}
	}
	url := b.endpoint + "/health"
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("brain not healthy: %w", ctx.Err())
		default:
		}
		resp, err := opts.HTTPClient.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("brain not healthy: %w", ctx.Err())
		case <-time.After(opts.PollInterval):
		}
	}
}

// Stop sends SIGTERM, waits up to 10 seconds, then escalates to SIGKILL.
// Removes the pidfile on the way out. Safe to call multiple times.
func (b *Brain) Stop() error {
	if b == nil || b.cmd == nil || b.cmd.Process == nil {
		return nil
	}
	if b.stopOnce {
		return nil
	}
	b.stopOnce = true
	defer func() {
		if b.metaPath != "" {
			_ = os.Remove(b.metaPath)
			_ = os.Remove(filepath.Join(filepath.Dir(b.metaPath), "brain.pid"))
		}
	}()

	_ = b.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- b.cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
		_ = b.cmd.Process.Kill()
		<-done
		return errors.New("brain did not exit on SIGTERM; killed with SIGKILL")
	}
}

// writeBrainMeta writes brain ownership metadata to brainDir/brain.pid.json.
func writeBrainMeta(brainDir string, meta BrainMeta) (string, error) {
	path := filepath.Join(brainDir, "brain.pid.json")
	data, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	_ = os.Remove(filepath.Join(brainDir, "brain.pid"))
	return path, nil
}

// readBrainMeta reads brain ownership metadata from brainDir.
// Tries brain.pid.json first, then falls back to the legacy brain.pid.
//
// Returns:
//   - meta: parsed metadata; nil if no readable metadata or malformed content
//   - metaPath: file path found (set even for malformed files, for cleanup)
//   - legacy: true when the data came from a bare brain.pid (no owner info)
func readBrainMeta(brainDir string) (meta *BrainMeta, metaPath string, legacy bool) {
	jsonPath := filepath.Join(brainDir, "brain.pid.json")
	if data, err := os.ReadFile(jsonPath); err == nil {
		var m BrainMeta
		if err := json.Unmarshal(data, &m); err == nil && m.BrainPID > 0 {
			return &m, jsonPath, false
		}
		return nil, jsonPath, false // malformed
	}

	pidPath := filepath.Join(brainDir, "brain.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return nil, "", false // no metadata at all
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid <= 1 {
		return nil, pidPath, true // malformed legacy
	}
	return &BrainMeta{BrainPID: pid}, pidPath, true
}

// defaultKillBrainProcess sends SIGTERM, waits up to 5 s, then SIGKILL.
func defaultKillBrainProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !checkProcessAlive(pid) {
			return
		}
	}
	_ = proc.Kill()
}

// KillStaleBrain looks for brain metadata from a previous CHEW run. If the
// owning CHEW process is still alive, the brain belongs to another active
// session and is left alone. If the owner is gone and the brain PID is still
// a running llama-server, it is killed as an orphan.
//
// Backward-compatible: reads a legacy brain.pid (bare PID) when no
// brain.pid.json exists. Legacy files have no owner info so orphan detection
// falls through to the llama-server command-name check only.
//
// Safe to call when no metadata exists — returns nil.
func KillStaleBrain(brainDir string) error {
	meta, metaPath, _ := readBrainMeta(brainDir)
	if meta == nil {
		// No readable metadata, or malformed file — clean up if present.
		if metaPath != "" {
			_ = os.Remove(metaPath)
			legacyPath := filepath.Join(brainDir, "brain.pid")
			if metaPath != legacyPath {
				_ = os.Remove(legacyPath)
			}
		}
		return nil
	}

	removeMeta := func() {
		os.Remove(metaPath)
		// If we handled the new format, also clean up any stale legacy pidfile.
		legacyPath := filepath.Join(brainDir, "brain.pid")
		if metaPath != legacyPath {
			os.Remove(legacyPath)
		}
	}

	// Owner still alive → another CHEW session owns this brain.
	if meta.OwnerPID > 0 && checkProcessAlive(meta.OwnerPID) {
		return nil
	}

	// Brain process not running → just clean up stale metadata.
	if meta.BrainPID <= 0 || !checkProcessAlive(meta.BrainPID) {
		removeMeta()
		return nil
	}

	// PID alive but not llama-server → PID was reused; clean metadata only.
	if !checkLlamaServer(meta.BrainPID) {
		removeMeta()
		return nil
	}

	// Orphaned llama-server — kill it and clean up.
	killBrainProcess(meta.BrainPID)
	removeMeta()
	return nil
}

// looksLikeLlamaServer is best-effort PID-comm verification. On platforms
// where /proc isn't available, we fall back to "trust the pidfile."
func looksLikeLlamaServer(pid int) bool {
	// /proc on linux
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		return strings.HasPrefix(string(data), "llama-server")
	}
	// macOS: ps lookup
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=").Output()
	if err == nil {
		return strings.Contains(string(out), "llama-server")
	}
	// Unverifiable; assume yes (we wrote the pidfile, so it was ours).
	return true
}

// (findLlamaServer moved to runtime_install.go as part of acquireLlamaServer,
// which adds auto-download from llama.cpp releases.)
