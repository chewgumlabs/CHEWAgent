package planner

import (
	"strings"
	"testing"
)

func TestScripted_QuitVariants(t *testing.T) {
	p := NewScriptedPlanner()
	for _, in := range []string{"quit", "exit", "q", "bye", "QUIT"} {
		got := p.Plan(in)
		if !got.Halt {
			t.Errorf("input %q: expected Halt=true, got %+v", in, got)
		}
	}
}

func TestScripted_Help(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("help")
	if !strings.Contains(got.Response, "brainless") {
		t.Errorf("help should mention brainless mode, got: %s", got.Response)
	}
	if got.Halt {
		t.Errorf("help should not halt")
	}
}

func TestScripted_ReadDispatchesVerb(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("read main.go")
	if len(got.Verbs) != 1 || got.Verbs[0].Name != "read_file" {
		t.Fatalf("expected one read_file verb, got %+v", got.Verbs)
	}
	if got.Verbs[0].Params["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", got.Verbs[0].Params["path"])
	}
	if got.Mascot != "walk" {
		t.Errorf("read should set mascot=walk, got %s", got.Mascot)
	}
}

func TestScripted_FindWithIn(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan(`find "TODO" in src/`)
	if len(got.Verbs) != 1 || got.Verbs[0].Name != "search" {
		t.Fatalf("expected one search verb, got %+v", got.Verbs)
	}
	if got.Verbs[0].Params["pattern"] != "TODO" {
		t.Errorf("pattern: want TODO, got %v", got.Verbs[0].Params["pattern"])
	}
	if got.Verbs[0].Params["path"] != "src/" {
		t.Errorf("path: want src/, got %v", got.Verbs[0].Params["path"])
	}
}

func TestScripted_FindWithoutIn(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("grep TODO")
	if len(got.Verbs) != 1 {
		t.Fatalf("expected one verb, got %+v", got.Verbs)
	}
	if got.Verbs[0].Params["pattern"] != "TODO" {
		t.Errorf("pattern: want TODO, got %v", got.Verbs[0].Params["pattern"])
	}
	if got.Verbs[0].Params["path"] != "." {
		t.Errorf("path: want default '.', got %v", got.Verbs[0].Params["path"])
	}
}

func TestScripted_GitReadOnly(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("git status")
	if len(got.Verbs) != 1 || got.Verbs[0].Name != "run_command" {
		t.Fatalf("expected run_command, got %+v", got.Verbs)
	}
	if got.Verbs[0].Params["command"] != "git status" {
		t.Errorf("command: want 'git status', got %v", got.Verbs[0].Params["command"])
	}
}

func TestScripted_GitMutatingNeedsForce(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("git push origin main")
	if len(got.Verbs) != 0 {
		t.Errorf("git push should not dispatch a verb without 'force:' prefix, got %+v", got.Verbs)
	}
	if !strings.Contains(got.Response, "force:") {
		t.Errorf("response should mention force prefix, got: %s", got.Response)
	}
}

func TestScripted_ForcePrefixUnlocks(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("force: git push origin main")
	if len(got.Verbs) != 1 || got.Verbs[0].Name != "run_command" {
		t.Fatalf("force should dispatch, got %+v", got.Verbs)
	}
	if got.Verbs[0].Params["command"] != "git push origin main" {
		t.Errorf("command: want 'git push origin main', got %v", got.Verbs[0].Params["command"])
	}
}

func TestScripted_UnknownInputFallback(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("explain monads in haskell")
	if len(got.Verbs) != 0 {
		t.Errorf("free-form question should not dispatch a verb, got %+v", got.Verbs)
	}
	if !strings.Contains(got.Response, "install brain") {
		t.Errorf("fallback should suggest install brain, got: %s", got.Response)
	}
	if got.Mascot != "ghost" {
		t.Errorf("fallback should set mascot=ghost, got %s", got.Mascot)
	}
}

func TestScripted_EmptyInputSilent(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("")
	if got.Response != "" || len(got.Verbs) != 0 {
		t.Errorf("empty input should be silent, got %+v", got)
	}
}

func TestScripted_StatusReportsBrainless(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("status")
	if !strings.Contains(got.Response, "brainless") {
		t.Errorf("status should mention brainless, got: %s", got.Response)
	}
	if got.Mascot != "ghost" {
		t.Errorf("status should set mascot=ghost, got %s", got.Mascot)
	}
}

func TestScripted_InstallBrainStartsWizard(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("install brain")
	if !strings.Contains(got.Response, "llama.cpp") {
		t.Errorf("install brain should mention llama.cpp, got: %s", got.Response)
	}
}
