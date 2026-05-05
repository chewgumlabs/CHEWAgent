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

// Brain is a running llama-server subprocess.
type Brain struct {
	cmd      *exec.Cmd
	endpoint string // e.g. "http://127.0.0.1:8080"
	logPath  string
	pidPath  string // <brainDir>/brain.pid; written on Start, removed on Stop
	stopOnce bool
}

// StartBrain spawns llama-server with the given config. Returns immediately
// after the process is launched; call WaitHealthy to confirm readiness.
//
// We deliberately do NOT put llama-server in its own process group: keeping
// it in CHEW's foreground group means SIGHUP from a closed terminal kills
// it for free, no signal handler needed. The pidfile written here is the
// belt to that suspenders — next CHEW launch can clean up an orphan from
// a hard-killed previous run.
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

	pidPath := ""
	if cfg.LogPath != "" {
		// brain.pid lives next to brain.log so the brain dir owns both.
		pidPath = filepath.Join(filepath.Dir(cfg.LogPath), "brain.pid")
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644)
	}

	return &Brain{
		cmd:      cmd,
		endpoint: fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
		logPath:  cfg.LogPath,
		pidPath:  pidPath,
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
		if b.pidPath != "" {
			_ = os.Remove(b.pidPath)
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

// KillStaleBrain looks for a pidfile from a previous CHEW run and, if the
// PID is still alive AND looks like our llama-server, kills it. Called by
// the REPL on startup so a hard-crashed previous run doesn't leave the
// brain hogging RAM.
//
// Safe to call when no pidfile exists — returns nil.
func KillStaleBrain(brainDir string) error {
	pidPath := filepath.Join(brainDir, "brain.pid")
	body, err := os.ReadFile(pidPath)
	if err != nil {
		return nil // no pidfile, nothing to do
	}
	defer os.Remove(pidPath)

	var pid int
	if _, err := fmt.Sscanf(string(body), "%d", &pid); err != nil || pid <= 1 {
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	// On unix, FindProcess always succeeds; verify with a 0-signal test.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return nil // not running
	}
	// Double-check the command line matches llama-server before killing.
	if !looksLikeLlamaServer(pid) {
		return nil // some other process snagged this PID
	}

	_ = proc.Signal(syscall.SIGTERM)
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return nil
		}
	}
	_ = proc.Kill()
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
