package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const previewStateSchema = "chew-preview-state.v0"

type Preview struct {
	Root *string
}

type previewState struct {
	SchemaVersion string `json:"schema_version"`
	Root          string `json:"root"`
	ServeDir      string `json:"serve_dir"`
	URL           string `json:"url"`
	Port          int    `json:"port"`
	PID           int    `json:"pid"`
	Strategy      string `json:"strategy"`
	LogPath       string `json:"log_path"`
	StartedAt     string `json:"started_at"`
}

type previewPlan struct {
	Root     string
	ServeDir string
	Build    []string
	Port     int
	URL      string
	Strategy string
}

func (p *Preview) Name() string { return "preview" }

func (p *Preview) Execute(params map[string]any) (Result, error) {
	action := strings.ToLower(stringParam(params, "action"))
	if action == "" {
		action = "start"
	}
	root, err := p.projectRoot()
	if err != nil {
		return Result{}, err
	}
	switch action {
	case "start":
		return p.start(root, false)
	case "open":
		return p.start(root, true)
	case "status":
		return p.status(root)
	case "stop":
		return p.stop(root)
	default:
		return Result{}, fmt.Errorf("unknown preview action %q", action)
	}
}

func (p *Preview) projectRoot() (string, error) {
	if p.Root != nil && strings.TrimSpace(*p.Root) != "" {
		return filepath.Abs(*p.Root)
	}
	return os.Getwd()
}

func (p *Preview) start(root string, open bool) (Result, error) {
	if st, ok := livePreview(root); ok {
		if open {
			_ = openBrowser(st.URL)
		}
		return Result{
			Output: fmt.Sprintf("Preview already running: %s\nroot: %s\nstop: preview stop", st.URL, st.Root),
			Mascot: "idle",
		}, nil
	}
	plan, err := buildPreviewPlan(root)
	if err != nil {
		return Result{}, err
	}
	if len(plan.Build) > 0 {
		if out, err := runPreviewBuild(plan.Root, plan.Build); err != nil {
			return Result{Output: out, Mascot: "ghost"}, err
		}
		// Build may have created site/ or dist/. Re-resolve after it runs.
		plan, err = buildPreviewPlan(root)
		if err != nil {
			return Result{}, err
		}
		if plan.ServeDir == "" {
			return Result{}, fmt.Errorf("preview build ran, but no static site appeared in %s (expected site/, dist/, public/, or index.html)", root)
		}
	}
	logPath, err := previewLogPath(root)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir preview runtime dir: %w", err)
	}
	if err := ensurePreviewGitignore(root); err != nil {
		return Result{}, fmt.Errorf("prepare preview runtime ignore: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("open preview log: %w", err)
	}
	defer logFile.Close()

	python, err := findPython()
	if err != nil {
		return Result{}, err
	}
	cmd := exec.Command(python, "-m", "http.server", fmt.Sprint(plan.Port), "--bind", "127.0.0.1")
	cmd.Dir = plan.ServeDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start preview server: %w", err)
	}
	killStartedPreview := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	st := previewState{
		SchemaVersion: previewStateSchema,
		Root:          root,
		ServeDir:      plan.ServeDir,
		URL:           plan.URL,
		Port:          plan.Port,
		PID:           cmd.Process.Pid,
		Strategy:      plan.Strategy,
		LogPath:       logPath,
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := writePreviewState(root, st); err != nil {
		killStartedPreview()
		return Result{}, err
	}
	if err := waitForPreview(plan.URL, 5*time.Second); err != nil {
		killStartedPreview()
		return Result{Output: fmt.Sprintf("Preview failed to answer at %s.\nlog: %s", plan.URL, logPath), Mascot: "ghost"}, err
	}
	if open {
		_ = openBrowser(plan.URL)
	}
	_ = cmd.Process.Release()
	return Result{
		Output: fmt.Sprintf("Preview running: %s\nserving: %s\nlog: %s\nstop: preview stop", plan.URL, plan.ServeDir, logPath),
		Mascot: "idle",
	}, nil
}

func (p *Preview) status(root string) (Result, error) {
	st, err := readPreviewState(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{Output: "No preview running for this project.", Mascot: "idle"}, nil
		}
		return Result{}, err
	}
	if previewAlive(st) {
		return Result{Output: fmt.Sprintf("Preview running: %s\npid: %d\nserving: %s", st.URL, st.PID, st.ServeDir), Mascot: "idle"}, nil
	}
	_ = os.Remove(previewStatePath(root))
	return Result{Output: "Preview is not running. Cleared stale preview state.", Mascot: "ghost"}, nil
}

func (p *Preview) stop(root string) (Result, error) {
	st, err := readPreviewState(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{Output: "No preview to stop.", Mascot: "idle"}, nil
		}
		return Result{}, err
	}
	if previewAlive(st) {
		if proc, err := os.FindProcess(st.PID); err == nil {
			if runtime.GOOS == "windows" {
				_ = proc.Kill()
			} else {
				_ = proc.Signal(os.Interrupt)
				time.Sleep(150 * time.Millisecond)
				if processAlive(st.PID) {
					_ = proc.Kill()
				}
			}
		}
	}
	_ = os.Remove(previewStatePath(root))
	return Result{Output: fmt.Sprintf("Preview stopped: %s", st.URL), Mascot: "idle"}, nil
}

func buildPreviewPlan(root string) (previewPlan, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return previewPlan{}, err
	}
	port, err := choosePreviewPort(8765, 80)
	if err != nil {
		return previewPlan{}, err
	}
	plan := previewPlan{
		Root: root,
		Port: port,
		URL:  fmt.Sprintf("http://localhost:%d/", port),
	}
	if hasMakeTarget(filepath.Join(root, "Makefile"), "build") {
		plan.Build = []string{"make", "build"}
	}
	if dir, ok := detectStaticDir(root); ok {
		plan.ServeDir = dir
		plan.Strategy = "static"
		if filepath.Base(dir) != filepath.Base(root) {
			plan.Strategy = "static:" + relOrBase(root, dir)
		}
		return plan, nil
	}
	if len(plan.Build) > 0 {
		// Caller will run the build once, then call buildPreviewPlan again.
		plan.Strategy = "make build"
		return plan, nil
	}
	return previewPlan{}, fmt.Errorf("no previewable static site found in %s (expected site/, dist/, public/, or index.html)", root)
}

func detectStaticDir(root string) (string, bool) {
	for _, rel := range []string{"site", "dist", "public", "."} {
		dir := filepath.Clean(filepath.Join(root, rel))
		if pathExists(filepath.Join(dir, "index.html")) {
			return dir, true
		}
	}
	return "", false
}

func hasMakeTarget(path, target string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	prefix := target + ":"
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func runPreviewBuild(root string, argv []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > 12000 {
		text = text[:12000] + "\n[... build output truncated ...]\n"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return text, fmt.Errorf("preview build timed out")
	}
	if err != nil {
		return text, fmt.Errorf("preview build failed: %w", err)
	}
	return text, nil
}

func choosePreviewPort(start, attempts int) (int, error) {
	for port := start; port < start+attempts; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no free preview port found near %d", start)
}

func livePreview(root string) (previewState, bool) {
	st, err := readPreviewState(root)
	if err != nil {
		return previewState{}, false
	}
	if previewAlive(st) {
		return st, true
	}
	_ = os.Remove(previewStatePath(root))
	return previewState{}, false
}

func previewAlive(st previewState) bool {
	if st.PID <= 0 || st.URL == "" {
		return false
	}
	if !processAlive(st.PID) {
		return false
	}
	return waitForPreview(st.URL, 500*time.Millisecond) == nil
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func waitForPreview(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := http.Client{Timeout: 400 * time.Millisecond}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
			lastErr = fmt.Errorf("preview returned %s", resp.Status)
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("preview did not answer")
	}
	return lastErr
}

func findPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python3 not found; cannot start static preview server")
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func previewStatePath(root string) string {
	return filepath.Join(root, ".chew", "runtime", "preview.pid.json")
}

func previewLogPath(root string) (string, error) {
	return filepath.Abs(filepath.Join(root, ".chew", "runtime", "preview.log"))
}

func readPreviewState(root string) (previewState, error) {
	body, err := os.ReadFile(previewStatePath(root))
	if err != nil {
		return previewState{}, err
	}
	var st previewState
	if err := json.Unmarshal(body, &st); err != nil {
		return previewState{}, err
	}
	return st, nil
}

func writePreviewState(root string, st previewState) error {
	path := previewStatePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func ensurePreviewGitignore(root string) error {
	dir := filepath.Join(root, ".chew")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, ".gitignore")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(path, []byte("runtime/\n"), 0o644)
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "runtime/" {
			return nil
		}
	}
	text := string(body)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "runtime/\n"
	return os.WriteFile(path, []byte(text), 0o644)
}

func relOrBase(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return filepath.Base(path)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
