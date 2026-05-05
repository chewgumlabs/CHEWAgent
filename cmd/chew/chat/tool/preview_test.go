package tool

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPreviewPlanDetectsStaticSite(t *testing.T) {
	tmp := t.TempDir()
	site := filepath.Join(tmp, "site")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := buildPreviewPlan(tmp)
	if err != nil {
		t.Fatalf("buildPreviewPlan: %v", err)
	}
	if plan.ServeDir != site {
		t.Fatalf("ServeDir = %q, want %q", plan.ServeDir, site)
	}
	if plan.Strategy != "static:site" {
		t.Fatalf("Strategy = %q, want static:site", plan.Strategy)
	}
	if plan.URL == "" || !strings.HasPrefix(plan.URL, "http://localhost:") {
		t.Fatalf("URL = %q, want localhost preview URL", plan.URL)
	}
}

func TestBuildPreviewPlanIncludesMakeBuildBeforeStaticExists(t *testing.T) {
	tmp := t.TempDir()
	makefile := "build:\n\tmkdir -p site\n"
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), []byte(makefile), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := buildPreviewPlan(tmp)
	if err != nil {
		t.Fatalf("buildPreviewPlan: %v", err)
	}
	if strings.Join(plan.Build, " ") != "make build" {
		t.Fatalf("Build = %v, want make build", plan.Build)
	}
	if plan.ServeDir != "" {
		t.Fatalf("ServeDir = %q, want empty before build output exists", plan.ServeDir)
	}
	if plan.Strategy != "make build" {
		t.Fatalf("Strategy = %q, want make build", plan.Strategy)
	}
}

func TestBuildPreviewPlanErrorsWithoutStaticSite(t *testing.T) {
	tmp := t.TempDir()
	_, err := buildPreviewPlan(tmp)
	if err == nil {
		t.Fatal("expected error with no previewable site")
	}
	if !strings.Contains(err.Error(), "no previewable static site") {
		t.Fatalf("expected previewable-site error, got %v", err)
	}
}

func TestPreviewStateRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	want := previewState{
		SchemaVersion: previewStateSchema,
		Root:          tmp,
		ServeDir:      filepath.Join(tmp, "site"),
		URL:           "http://localhost:8765/",
		Port:          8765,
		PID:           12345,
		Strategy:      "static:site",
		LogPath:       filepath.Join(tmp, ".chew", "runtime", "preview.log"),
		StartedAt:     "2026-05-05T12:00:00Z",
	}
	if err := writePreviewState(tmp, want); err != nil {
		t.Fatalf("writePreviewState: %v", err)
	}
	got, err := readPreviewState(tmp)
	if err != nil {
		t.Fatalf("readPreviewState: %v", err)
	}
	if got != want {
		t.Fatalf("state round trip mismatch:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestEnsurePreviewGitignore(t *testing.T) {
	tmp := t.TempDir()
	if err := ensurePreviewGitignore(tmp); err != nil {
		t.Fatalf("ensurePreviewGitignore: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(tmp, ".chew", ".gitignore"))
	if err != nil {
		t.Fatalf("read .chew/.gitignore: %v", err)
	}
	if string(body) != "runtime/\n" {
		t.Fatalf(".chew/.gitignore = %q, want runtime/ ignore", string(body))
	}
}

func TestEnsurePreviewGitignoreAppendsToExistingFile(t *testing.T) {
	tmp := t.TempDir()
	chewDir := filepath.Join(tmp, ".chew")
	if err := os.MkdirAll(chewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(chewDir, ".gitignore")
	if err := os.WriteFile(path, []byte("cache/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensurePreviewGitignore(tmp); err != nil {
		t.Fatalf("ensurePreviewGitignore: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .chew/.gitignore: %v", err)
	}
	if string(body) != "cache/\nruntime/\n" {
		t.Fatalf(".chew/.gitignore = %q, want existing content plus runtime/", string(body))
	}
}

func TestPreviewExecuteStartStatusStop(t *testing.T) {
	if _, err := findPython(); err != nil {
		t.Skip(err)
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<h1>preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := tmp
	preview := &Preview{Root: &root}
	start, err := preview.Execute(map[string]any{"action": "start"})
	if err != nil {
		t.Fatalf("start preview: %v\n%s", err, start.Output)
	}
	defer func() {
		_, _ = preview.Execute(map[string]any{"action": "stop"})
	}()
	if !strings.Contains(start.Output, "Preview running: http://localhost:") {
		t.Fatalf("unexpected start output: %s", start.Output)
	}

	status, err := preview.Execute(map[string]any{"action": "status"})
	if err != nil {
		t.Fatalf("preview status: %v", err)
	}
	if !strings.Contains(status.Output, "Preview running:") {
		t.Fatalf("unexpected status output: %s", status.Output)
	}

	stop, err := preview.Execute(map[string]any{"action": "stop"})
	if err != nil {
		t.Fatalf("preview stop: %v", err)
	}
	if !strings.Contains(stop.Output, "Preview stopped:") {
		t.Fatalf("unexpected stop output: %s", stop.Output)
	}
	if _, err := os.Stat(previewStatePath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview state should be removed after stop, stat err=%v", err)
	}
}
