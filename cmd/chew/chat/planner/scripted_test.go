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

func TestScripted_WebFetch_OnURL(t *testing.T) {
	p := NewScriptedPlanner()
	cases := []string{
		"fetch https://example.com",
		"open https://example.com/foo",
		"visit https://docs.github.com/pages",
		"read https://example.com",
	}
	for _, in := range cases {
		got := p.Plan(in)
		if len(got.Verbs) != 1 || got.Verbs[0].Name != "web_fetch" {
			t.Errorf("input %q: expected one web_fetch verb, got %+v", in, got.Verbs)
			continue
		}
		url, _ := got.Verbs[0].Params["url"].(string)
		if url == "" || !strings.HasPrefix(url, "http") {
			t.Errorf("input %q: expected URL param, got %v", in, got.Verbs[0].Params)
		}
		if got.Mascot != "walk" {
			t.Errorf("input %q: web_fetch should set mascot=walk, got %s", in, got.Mascot)
		}
	}
}

func TestScripted_ReadFileStillRoutesToReadFile(t *testing.T) {
	// Make sure adding web_fetch in front of `read` didn't steal local file
	// reads. URL-less paths must still route to read_file.
	p := NewScriptedPlanner()
	got := p.Plan("read README.md")
	if len(got.Verbs) != 1 || got.Verbs[0].Name != "read_file" {
		t.Fatalf("expected read_file for plain path, got %+v", got.Verbs)
	}
	if got.Verbs[0].Params["path"] != "README.md" {
		t.Errorf("expected path=README.md, got %v", got.Verbs[0].Params["path"])
	}
}

func TestScripted_WebSearch_TriggerVariants(t *testing.T) {
	p := NewScriptedPlanner()
	cases := []struct {
		in    string
		query string
	}{
		{"web search github pages", "github pages"},
		{"websearch how to use git", "how to use git"},
		{"google golang context cancel", "golang context cancel"},
		{"search the web for free hosting", "free hosting"},
		{"search web for deno vs node", "deno vs node"},
	}
	for _, c := range cases {
		got := p.Plan(c.in)
		if len(got.Verbs) != 1 || got.Verbs[0].Name != "web_search" {
			t.Errorf("input %q: expected web_search, got %+v", c.in, got.Verbs)
			continue
		}
		q, _ := got.Verbs[0].Params["query"].(string)
		if q != c.query {
			t.Errorf("input %q: expected query=%q, got %q", c.in, c.query, q)
		}
		if got.Mascot != "walk" {
			t.Errorf("input %q: web_search should set mascot=walk, got %s", c.in, got.Mascot)
		}
	}
}

func TestScripted_LocalSearchStillLocal(t *testing.T) {
	// `find`, `grep`, and bare `search` (without "web") must still hit the
	// local file-search verb.
	p := NewScriptedPlanner()
	for _, in := range []string{"grep TODO", "find FIXME in src/", "search foo in ."} {
		got := p.Plan(in)
		if len(got.Verbs) != 1 || got.Verbs[0].Name != "search" {
			t.Errorf("input %q: expected local search verb, got %+v", in, got.Verbs)
		}
	}
}
