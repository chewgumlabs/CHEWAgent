package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
)

func TestWorkStatusAnswersFromGumFacts(t *testing.T) {
	tmp := t.TempDir()
	pj, err := project.Create(filepath.Join(tmp, "site"))
	if err != nil {
		t.Fatal(err)
	}
	status := newWorkStatus()

	status.Start("build a tiny canvas", "Brain call in progress.", pj, nil)
	status.Fact("Sent request to Bonsai.", pj)

	answer := status.Answer(pj, true)
	for _, want := range []string{
		"I'm working on: build a tiny canvas",
		"Checkpoint: Brain call in progress.",
		"Latest Gum fact: Sent request to Bonsai.",
		"didn't interrupt the brain",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("status answer missing %q:\n%s", want, answer)
		}
	}
	body, err := os.ReadFile(pj.StatusPath())
	if err != nil {
		t.Fatalf("status snapshot should persist: %v", err)
	}
	if !strings.Contains(string(body), `"current_task": "build a tiny canvas"`) {
		t.Fatalf("status snapshot missing task:\n%s", body)
	}
}

func TestWorkStatusQuestionDetection(t *testing.T) {
	for _, input := range []string{
		"What are you currently doing?",
		"where are we at?",
		"what's the status",
	} {
		if !isWorkStatusQuestion(input) {
			t.Fatalf("expected work status question: %q", input)
		}
	}
	if isWorkStatusQuestion("brain status") {
		t.Fatal("brain status should stay available as its own command")
	}
	if !isBareStatusQuestion("status!") {
		t.Fatal("busy bare status should tolerate punctuation")
	}
}
