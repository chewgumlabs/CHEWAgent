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

func TestScripted_StatusTriggersBrainStatus(t *testing.T) {
	p := NewScriptedPlanner()
	for _, in := range []string{"status", "brain?", "are you ok?"} {
		got := p.Plan(in)
		if got.LaunchWizard != "brain_status" {
			t.Errorf("input %q: expected LaunchWizard=brain_status, got %q", in, got.LaunchWizard)
		}
	}
}

func TestScripted_InstallBrainSignalsWizardHandoff(t *testing.T) {
	p := NewScriptedPlanner()
	for _, in := range []string{"install brain", "install brain!"} {
		got := p.Plan(in)
		if got.LaunchWizard != "install_brain" {
			t.Errorf("input %q: expected LaunchWizard=install_brain, got %q", in, got.LaunchWizard)
		}
		// The wizard owns all the user-facing text now, so the planner shouldn't
		// pre-render anything that'd duplicate the wizard's plan screen.
		if got.Response != "" {
			t.Errorf("planner should not return its own text for install brain, got: %s", got.Response)
		}
	}
}

func TestScripted_MakeProjectAndFolderAreDistinct(t *testing.T) {
	p := NewScriptedPlanner()

	projectCases := []string{
		"make project comic tracker",
		"new project named comic tracker",
		"make me a project called comic tracker",
	}
	for _, in := range projectCases {
		got := p.Plan(in)
		if got.LaunchWizard != "create_project" {
			t.Fatalf("input %q: expected create_project, got %q", in, got.LaunchWizard)
		}
		if got.LaunchArgs["name"] != "comic tracker" {
			t.Fatalf("input %q: expected name comic tracker, got %q", in, got.LaunchArgs["name"])
		}
	}

	folderCases := []string{
		"make folder notes",
		"new folder named notes",
		"make me a folder called notes",
	}
	for _, in := range folderCases {
		got := p.Plan(in)
		if got.LaunchWizard != "create_folder" {
			t.Fatalf("input %q: expected create_folder, got %q", in, got.LaunchWizard)
		}
		if got.LaunchArgs["name"] != "notes" {
			t.Fatalf("input %q: expected name notes, got %q", in, got.LaunchArgs["name"])
		}
	}
}

func TestScripted_NaturalProjectStartCreatesProject(t *testing.T) {
	p := NewScriptedPlanner()
	cases := []struct {
		in   string
		name string
	}{
		{"let's make a website", "website"},
		{"I want to build a portfolio site", "portfolio site"},
		{"can we create a dashboard?", "dashboard"},
		{"build a game!", "game"},
	}
	for _, c := range cases {
		got := p.Plan(c.in)
		if got.LaunchWizard != "create_project" {
			t.Fatalf("input %q: expected create_project, got %q / %q", c.in, got.LaunchWizard, got.Response)
		}
		if got.LaunchArgs["name"] != c.name {
			t.Fatalf("input %q: expected name %q, got %q", c.in, c.name, got.LaunchArgs["name"])
		}
	}
}

func TestScripted_ProjectNamePromptDoesNotTeachCommandSyntax(t *testing.T) {
	p := NewScriptedPlanner()
	for _, in := range []string{"make project", "let's build something"} {
		got := p.Plan(in)
		if strings.Contains(strings.ToLower(got.Response), "make project") {
			t.Fatalf("input %q should ask conversationally, got: %s", in, got.Response)
		}
		if !strings.Contains(strings.ToLower(got.Response), "what should i call") {
			t.Fatalf("input %q should ask for a name, got: %s", in, got.Response)
		}
	}
}

func TestScripted_SetFallbackSwapsAndRestores(t *testing.T) {
	p := NewScriptedPlanner()
	called := false
	p.SetFallback(func(input string) Plan {
		called = true
		return Plan{Response: "from custom fallback: " + input}
	})
	got := p.Plan("explain monads")
	if !called {
		t.Errorf("custom fallback was not invoked")
	}
	if !strings.Contains(got.Response, "from custom fallback") {
		t.Errorf("expected custom fallback response, got: %s", got.Response)
	}
	// nil restores the default brainless fallback.
	p.SetFallback(nil)
	got = p.Plan("explain monads")
	if !strings.Contains(got.Response, "install brain") {
		t.Errorf("default fallback should mention install brain, got: %s", got.Response)
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

func TestScripted_PreviewCommands(t *testing.T) {
	p := NewScriptedPlanner()
	cases := []struct {
		in     string
		action string
	}{
		{"preview", "start"},
		{"start preview", "start"},
		{"serve", "start"},
		{"preview open", "open"},
		{"open preview", "open"},
		{"show site", "open"},
		{"preview status", "status"},
		{"status preview", "status"},
		{"preview stop", "stop"},
		{"stop preview", "stop"},
	}
	for _, c := range cases {
		got := p.Plan(c.in)
		if len(got.Verbs) != 1 || got.Verbs[0].Name != "preview" {
			t.Errorf("input %q: expected one preview verb, got %+v", c.in, got.Verbs)
			continue
		}
		if got.Verbs[0].Params["action"] != c.action {
			t.Errorf("input %q: expected action=%q, got %v", c.in, c.action, got.Verbs[0].Params["action"])
		}
		if got.Mascot != "walk" {
			t.Errorf("input %q: preview should set mascot=walk, got %s", c.in, got.Mascot)
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

func TestScripted_WakeBrainTriggers(t *testing.T) {
	p := NewScriptedPlanner()
	for _, in := range []string{"wake up", "wake", "wake brain", "start brain", "wake up!", "can you wake up?", "please wake up"} {
		got := p.Plan(in)
		if got.LaunchWizard != "wake_brain" {
			t.Errorf("input %q: expected LaunchWizard=wake_brain, got %q", in, got.LaunchWizard)
		}
	}
}

func TestScripted_NapBrainTriggers(t *testing.T) {
	p := NewScriptedPlanner()
	for _, in := range []string{"nap", "sleep", "sleep brain", "stop brain", "nap!"} {
		got := p.Plan(in)
		if got.LaunchWizard != "nap_brain" {
			t.Errorf("input %q: expected LaunchWizard=nap_brain, got %q", in, got.LaunchWizard)
		}
	}
}

func TestScripted_ProfileCommandsAreHiddenButWired(t *testing.T) {
	p := NewScriptedPlanner()

	status := p.Plan("profile status")
	if status.LaunchWizard != "profile_status" {
		t.Fatalf("profile status should launch profile_status, got %q", status.LaunchWizard)
	}
	list := p.Plan("profile list")
	if list.LaunchWizard != "profile_list" {
		t.Fatalf("profile list should launch profile_list, got %q", list.LaunchWizard)
	}
	use := p.Plan("profile use qwen")
	if use.LaunchWizard != "profile_use" || use.LaunchArgs["name"] != "qwen" {
		t.Fatalf("profile use should launch profile_use with qwen, got %+v", use)
	}

	help := p.Plan("help")
	if strings.Contains(strings.ToLower(help.Response), "profile") {
		t.Fatalf("profile switching should not be advertised in public help, got:\n%s", help.Response)
	}
}

func TestScripted_RememberNoteTriggersWizard(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("remember use Go for the prototype")
	if got.LaunchWizard != "remember_note" {
		t.Errorf("expected LaunchWizard=remember_note, got %q", got.LaunchWizard)
	}
	if got.LaunchArgs["note"] != "use Go for the prototype" {
		t.Errorf("expected note='use Go for the prototype', got %q", got.LaunchArgs["note"])
	}
	if got.Mascot != "walk" {
		t.Errorf("expected mascot=walk, got %s", got.Mascot)
	}
}

func TestScripted_RememberBareShowsError(t *testing.T) {
	p := NewScriptedPlanner()
	got := p.Plan("remember")
	if got.LaunchWizard != "" {
		t.Errorf("bare remember should not launch a wizard, got %q", got.LaunchWizard)
	}
	if got.Response == "" {
		t.Errorf("bare remember should produce an error response")
	}
}

func TestScripted_RememberCaseInsensitive(t *testing.T) {
	p := NewScriptedPlanner()
	for _, in := range []string{"Remember picked SQLite", "REMEMBER picked SQLite"} {
		got := p.Plan(in)
		if got.LaunchWizard != "remember_note" {
			t.Errorf("input %q: expected LaunchWizard=remember_note, got %q", in, got.LaunchWizard)
		}
	}
}

func TestScripted_PickVoice(t *testing.T) {
	// PickVoice should hand out frog-flavored variants from the named pool
	// in round-robin order. Empty for unknown keys.
	p := NewScriptedPlanner()
	first := p.PickVoice("brain_waking")
	second := p.PickVoice("brain_waking")
	if first == "" || second == "" {
		t.Errorf("brain_waking pool should have variants; got %q / %q", first, second)
	}
	if first == second {
		t.Errorf("expected round-robin to advance, got %q twice", first)
	}
	if got := p.PickVoice("nonexistent_pool"); got != "" {
		t.Errorf("unknown pool should return empty, got %q", got)
	}
}
