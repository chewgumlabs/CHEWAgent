package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/gum"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
)

func TestIntentFromChatCapturesBuildIntent(t *testing.T) {
	got := intentFromChat("Sure! Let's make a website.")
	if got != "website" {
		t.Fatalf("intentFromChat = %q, want website", got)
	}
}

func TestIntentFromChatRejectsQuestions(t *testing.T) {
	for _, input := range []string{
		"What should we build?",
		"How do I start?",
		"I don't know yet",
	} {
		if got := intentFromChat(input); got != "" {
			t.Fatalf("intentFromChat(%q) = %q, want empty", input, got)
		}
	}
}

func TestCaptureIntentFromChatUpdatesProjectMemory(t *testing.T) {
	tmp := t.TempDir()
	pj, err := project.Create(filepath.Join(tmp, "site"))
	if err != nil {
		t.Fatal(err)
	}
	if gum.Detect(pj) != gum.StageEmptyProject {
		t.Fatal("fresh project should start empty")
	}

	if !captureIntentFromChat(pj, "Sure! Let's make a website.") {
		t.Fatal("captureIntentFromChat should capture the website intent")
	}
	if gum.Detect(pj) != gum.StageIntentKnown {
		t.Fatalf("project should advance to intent-known, got %s", gum.Detect(pj))
	}
	if !strings.Contains(pj.GUM.Raw, "Website.") {
		t.Fatalf("in-memory GUM should refresh with captured intent:\n%s", pj.GUM.Raw)
	}
}

func TestBuildSystemPromptHidesStarterMemoryBoilerplate(t *testing.T) {
	tmp := t.TempDir()
	pj, err := project.Create(filepath.Join(tmp, "site"))
	if err != nil {
		t.Fatal(err)
	}

	prompt := buildSystemPrompt(pj)
	for _, forbidden := range []string{
		"Open GUM.md",
		"one paragraph",
		"Intent section",
		"# GUM.md",
		"Edit it yourself",
		"make project <name>",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("system prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSystemPromptIncludesGumKeyInstructions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(path, []byte(`{
  "schema_version": "chew-gum-key.v0",
  "name": "internal",
  "display_name": "Internal Gum",
  "summary": "Private workflow spine.",
  "instructions": "Use the workflow status provider before guessing."
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHEW_GUM_KEY", path)

	prompt := buildSystemPrompt(nil)
	for _, want := range []string{"active Gum key", "Internal Gum", "workflow status provider"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
	}
}
