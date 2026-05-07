package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/gum"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
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

func TestBuildSystemPromptIncludesDefaultPublicGumKey(t *testing.T) {
	t.Setenv("CHEW_GUM_KEY", "")

	prompt := buildSystemPrompt(nil)
	for _, want := range []string{"Public Gum", "Checkpoint:", "Never ask the user to open"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing public Gum key text %q:\n%s", want, prompt)
		}
	}
}

func TestChatSessionSendsBearerTokenAndExtraBody(t *testing.T) {
	var gotAuth string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "choices": [
		    {"message": {"role": "assistant", "content": "Fine. I am extremely overpowered now."}}
		  ]
		}`))
	}))
	defer server.Close()

	brain := wizard.AttachBrainWithOptions(server.URL+"/v1", "deepseek-v4-pro", wizard.BrainAttachOptions{
		APIKey:   "secret-test-key",
		ChatPath: "/chat/completions",
		ExtraBody: map[string]any{
			"reasoning_effort": "high",
			"thinking":         map[string]any{"type": "enabled"},
			"model":            "do-not-override",
		},
	})
	session := newChatSession(brain, nil)
	reply, err := session.ask("wake up and think harder")
	if err != nil {
		t.Fatal(err)
	}

	if reply != "Fine. I am extremely overpowered now." {
		t.Fatalf("reply = %q", reply)
	}
	if gotAuth != "Bearer secret-test-key" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotPayload["model"] != "deepseek-v4-pro" {
		t.Fatalf("model should not be overridden: %#v", gotPayload["model"])
	}
	if gotPayload["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort missing: %#v", gotPayload)
	}
	thinking, ok := gotPayload["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking extra body missing: %#v", gotPayload["thinking"])
	}
}
