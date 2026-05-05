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
// findLlamaServer locates the binary. Search order:
//  1. <exe-dir>/bin/llama-server-<GOOS>-<GOARCH>  — the bundled binary, used in
//     packaged distributions
//  2. exec.LookPath("llama-server")               — PATH fallback for dev
//
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
	"runtime"
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
	stopOnce bool
}

// StartBrain spawns llama-server with the given config. Returns immediately
// after the process is launched; call WaitHealthy to confirm readiness.
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

	// New process group so we can signal the whole tree on Stop().
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting llama-server: %w", err)
	}

	return &Brain{
		cmd:      cmd,
		endpoint: fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
		logPath:  cfg.LogPath,
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
// Safe to call multiple times.
func (b *Brain) Stop() error {
	if b == nil || b.cmd == nil || b.cmd.Process == nil {
		return nil
	}
	if b.stopOnce {
		return nil
	}
	b.stopOnce = true

	// Signal the whole process group so any child threads die with us.
	pgid := -b.cmd.Process.Pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- b.cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
		_ = syscall.Kill(pgid, syscall.SIGKILL)
		<-done
		return errors.New("brain did not exit on SIGTERM; killed with SIGKILL")
	}
}

// findLlamaServer locates the llama-server binary. Bundled-first, PATH fallback.
//
// The bundled path follows: <directory of the running CHEW binary>/bin/llama-server-<GOOS>-<GOARCH>
// e.g. /Applications/CHEW.app/Contents/MacOS/bin/llama-server-darwin-arm64
//
// Returns the absolute path if found, or an error explaining what's missing.
func findLlamaServer() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		bundled := filepath.Join(exeDir, "bin", fmt.Sprintf("llama-server-%s-%s", runtime.GOOS, runtime.GOARCH))
		if info, statErr := os.Stat(bundled); statErr == nil && !info.IsDir() {
			return bundled, nil
		}
	}
	// PATH fallback for dev environments.
	if onPath, lookErr := exec.LookPath("llama-server"); lookErr == nil {
		return onPath, nil
	}
	return "", errors.New("llama-server not found (no bundled binary, not on PATH)")
}
