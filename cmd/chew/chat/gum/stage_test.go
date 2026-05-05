package gum

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
)

func TestDetect_NoProject(t *testing.T) {
	if got := Detect(nil); got != StageNoProject {
		t.Errorf("nil project → expected StageNoProject, got %s", got)
	}
}

func TestDetect_EmptyProject(t *testing.T) {
	tmp := t.TempDir()
	pj, err := project.Create(filepath.Join(tmp, "fresh"))
	if err != nil {
		t.Fatal(err)
	}
	got := Detect(pj)
	if got != StageEmptyProject {
		t.Errorf("brand-new project (placeholder GUM, no source) → expected StageEmptyProject, got %s", got)
	}
}

func TestDetect_IntentKnown(t *testing.T) {
	tmp := t.TempDir()
	pj, err := project.Create(filepath.Join(tmp, "named"))
	if err != nil {
		t.Fatal(err)
	}
	// Replace placeholder Intent with real content.
	gumPath := filepath.Join(pj.Path, "GUM.md")
	body, _ := os.ReadFile(gumPath)
	updated := strings.Replace(string(body),
		"<one paragraph: what we're building, in plain English>",
		"A site that tracks comic book prices over time.",
		1)
	if err := os.WriteFile(gumPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	// Re-read GUM into the project struct.
	pj2, err := project.Open(pj.Path)
	if err != nil {
		t.Fatal(err)
	}
	got := Detect(pj2)
	if got != StageIntentKnown {
		t.Errorf("filled-Intent + no source files → expected StageIntentKnown, got %s", got)
	}
}

func TestDetect_Started(t *testing.T) {
	tmp := t.TempDir()
	pj, err := project.Create(filepath.Join(tmp, "started"))
	if err != nil {
		t.Fatal(err)
	}
	// Add a source file.
	if err := os.WriteFile(filepath.Join(pj.Path, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Detect(pj)
	if got != StageStarted {
		t.Errorf("source file present → expected StageStarted, got %s", got)
	}
}

func TestDetect_Mature(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available; skipping")
	}
	tmp := t.TempDir()
	pj, err := project.Create(filepath.Join(tmp, "mature"))
	if err != nil {
		t.Fatal(err)
	}
	// Add several source files + extra commits.
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(pj.Path, name), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Stage + commit them so the log gains depth.
	for i := 0; i < 2; i++ {
		cmds := [][]string{
			{"add", "-A"},
			{"commit", "-q", "--no-gpg-sign", "--allow-empty", "-m", "more stuff"},
		}
		for _, args := range cmds {
			c := exec.Command("git", args...)
			c.Dir = pj.Path
			c.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
				"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
			)
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
	got := Detect(pj)
	if got != StageMature {
		t.Errorf("multi-file + multi-commit project → expected StageMature, got %s", got)
	}
}

func TestHasFilledIntent(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"empty", "", false},
		{"missing section", "# project\n## Why\nbecause\n", false},
		{"placeholder only", "## Intent\n<one paragraph: what we're building>\n", false},
		{"filled in", "## Intent\nA real description of what this is.\n", true},
		{"placeholder plus filler around", "## Intent\n<placeholder>\n\nactual content here\n", true},
	}
	for _, c := range cases {
		got := hasFilledIntent(c.body)
		if got != c.want {
			t.Errorf("%s: hasFilledIntent = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExtractSection(t *testing.T) {
	body := `# project
## Intent
the intent text
spans multiple lines

## Why
why text
`
	got := extractSection(body, "## Intent")
	if !strings.Contains(got, "the intent text") {
		t.Errorf("expected intent body, got: %q", got)
	}
	if strings.Contains(got, "why text") {
		t.Errorf("section should stop at next heading, got: %q", got)
	}
}

func TestInstructions_AllStagesHaveText(t *testing.T) {
	for _, s := range []Stage{StageNoProject, StageEmptyProject, StageIntentKnown, StageStarted, StageMature} {
		got := Instructions(s)
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s: empty instruction block", s)
		}
		if !strings.Contains(got, "GUM stage:") {
			t.Errorf("%s: instruction block missing 'GUM stage:' header — got: %s", s, got[:min(80, len(got))])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
