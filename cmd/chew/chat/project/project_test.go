package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpen_ExistingFolderWithoutGUM(t *testing.T) {
	tmp := t.TempDir()
	p, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if p.Path != tmp {
		t.Errorf("expected Path=%q, got %q", tmp, p.Path)
	}
	if p.Name != filepath.Base(tmp) {
		t.Errorf("expected Name=%q, got %q", filepath.Base(tmp), p.Name)
	}
	if p.HasGUM() {
		t.Errorf("HasGUM should be false on a fresh folder")
	}
	if !p.GUM.IsEmpty() {
		t.Errorf("GUM should be empty on a fresh folder")
	}
}

func TestOpen_FolderWithGUM(t *testing.T) {
	tmp := t.TempDir()
	gumBody := "# GUM.md — test\n\n## Intent\nbuilding a thing\n"
	if err := os.WriteFile(filepath.Join(tmp, "GUM.md"), []byte(gumBody), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !p.HasGUM() {
		t.Errorf("HasGUM should be true when GUM.md exists")
	}
	if !strings.Contains(p.GUM.Raw, "building a thing") {
		t.Errorf("expected GUM.Raw to include intent text, got: %s", p.GUM.Raw)
	}
}

func TestOpen_RejectsFiles(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "not-a-folder.txt")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	_, err := Open(f)
	if err == nil {
		t.Errorf("Open on a file should error")
	}
}

func TestOpen_RejectsMissingPath(t *testing.T) {
	_, err := Open("/nonexistent/path/that/should/not/exist")
	if err == nil {
		t.Errorf("Open on missing path should error")
	}
}

func TestCreate_WritesStarterGUM(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "comic-tracker")
	p, err := Create(target)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Name != "comic-tracker" {
		t.Errorf("expected Name=comic-tracker, got %q", p.Name)
	}
	if !p.HasGUM() {
		t.Errorf("Create should write a starter GUM.md")
	}
	body, _ := os.ReadFile(p.GUMPath())
	for _, want := range []string{"# GUM.md — comic-tracker", "## Intent", "## Why", "## Ground truth", "## Open questions", "## Recent decisions"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("starter GUM.md missing %q.\ngot: %s", want, body)
		}
	}
}

func TestCreate_RefusesExistingFolder(t *testing.T) {
	tmp := t.TempDir()
	_, err := Create(tmp)
	if err == nil {
		t.Errorf("Create should refuse to overwrite an existing folder")
	}
}

func TestResolvePath_TrimsQuotes(t *testing.T) {
	tmp := t.TempDir()
	quoted := `'` + tmp + `'`
	got, err := resolvePath(quoted)
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	if got != tmp {
		t.Errorf("expected %q, got %q", tmp, got)
	}
}

func TestResolvePath_ExpandsTilde(t *testing.T) {
	got, err := resolvePath("~/")
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	home, _ := os.UserHomeDir()
	if got != home {
		t.Errorf("expected ~/ to expand to %q, got %q", home, got)
	}
}

func TestLooksLikePath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/Users/foo/Documents/bar", true},
		{"~/Documents/bar", true},
		{"~", true},
		{"'/Users/foo/Comic Books/bar'", true},
		{"build a website", false},
		{"hello world", false},
		{"read README.md", false},
		{"", false},
		{"C:\\Users\\foo", true},
	}
	for _, c := range cases {
		got := LooksLikePath(c.in)
		if got != c.want {
			t.Errorf("LooksLikePath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAppendDecision_WithExistingHeading(t *testing.T) {
	tmp := t.TempDir()
	gumPath := filepath.Join(tmp, "GUM.md")
	if err := WriteGUM(gumPath, NewStarterGUM("test")); err != nil {
		t.Fatal(err)
	}
	if err := AppendDecision(gumPath, "picked SQLite for storage"); err != nil {
		t.Fatalf("AppendDecision: %v", err)
	}
	body, _ := os.ReadFile(gumPath)
	if !strings.Contains(string(body), "picked SQLite for storage") {
		t.Errorf("decision wasn't appended.\ngot: %s", body)
	}
}

func TestAppendDecision_AddsHeadingIfMissing(t *testing.T) {
	tmp := t.TempDir()
	gumPath := filepath.Join(tmp, "GUM.md")
	if err := os.WriteFile(gumPath, []byte("# project\n\nsome notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendDecision(gumPath, "first decision"); err != nil {
		t.Fatalf("AppendDecision: %v", err)
	}
	body, _ := os.ReadFile(gumPath)
	if !strings.Contains(string(body), "## Recent decisions") {
		t.Errorf("expected heading to be added.\ngot: %s", body)
	}
	if !strings.Contains(string(body), "first decision") {
		t.Errorf("expected decision body.\ngot: %s", body)
	}
}

func TestSaveAndLoadLast(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	target := filepath.Join(tmp, "project")
	_ = os.MkdirAll(target, 0o755)

	if got := LoadLast(brainDir); got != "" {
		t.Errorf("LoadLast on fresh state should be empty, got %q", got)
	}
	if err := SaveLast(brainDir, target); err != nil {
		t.Fatalf("SaveLast: %v", err)
	}
	if got := LoadLast(brainDir); got != target {
		t.Errorf("LoadLast = %q, want %q", got, target)
	}
	// Pointing at a vanished folder returns "".
	_ = os.RemoveAll(target)
	if got := LoadLast(brainDir); got != "" {
		t.Errorf("LoadLast for missing folder should return empty, got %q", got)
	}
}

func TestGUMSummary(t *testing.T) {
	g := GUM{Raw: "# GUM.md — comic-tracker\n\n## Intent\nTracking comic book prices over time.\n\n## Why\nBecause I'm collecting them.\n"}
	s := g.Summary()
	if !strings.Contains(s, "Tracking comic book prices") {
		t.Errorf("Summary should include intent, got: %s", s)
	}
	if strings.HasPrefix(s, "# GUM.md") {
		t.Errorf("Summary should skip the H1, got: %s", s)
	}
}
