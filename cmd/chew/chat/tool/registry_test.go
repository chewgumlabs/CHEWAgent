package tool

import (
	"errors"
	"strings"
	"testing"
)

type fakeTool struct {
	name string
	res  Result
	err  error
}

func (f *fakeTool) Name() string                                 { return f.name }
func (f *fakeTool) Execute(_ map[string]any) (Result, error)     { return f.res, f.err }

func TestRegistry_DispatchUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Dispatch("nope", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !errors.Is(err, ErrUnknownTool) {
		t.Errorf("expected ErrUnknownTool, got: %v", err)
	}
}

func TestRegistry_RegisterAndDispatch(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeTool{name: "echo", res: Result{Output: "hi"}})
	res, err := r.Dispatch("echo", nil)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.Output != "hi" {
		t.Errorf("expected hi, got %q", res.Output)
	}
}

func TestRegistry_Names_Sorted(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeTool{name: "zeta"})
	r.Register(&fakeTool{name: "alpha"})
	r.Register(&fakeTool{name: "mu"})
	got := r.Names()
	want := []string{"alpha", "mu", "zeta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestRegistry_DispatchPropagatesToolError(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeTool{name: "broken", err: errors.New("kaboom")})
	_, err := r.Dispatch("broken", nil)
	if err == nil {
		t.Fatal("expected error from broken tool")
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("expected kaboom in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("expected tool name in error, got: %v", err)
	}
}

func TestRegistry_NewDefaultHasAllStandardTools(t *testing.T) {
	r := NewDefault()
	want := []string{"web_search", "web_fetch", "read_file", "list_dir", "search", "write_file", "run_command"}
	for _, name := range want {
		if !r.Has(name) {
			t.Errorf("NewDefault should register %q", name)
		}
	}
}

func TestStringParam(t *testing.T) {
	cases := []struct {
		params map[string]any
		key    string
		want   string
	}{
		{map[string]any{"q": "  hello  "}, "q", "hello"},
		{map[string]any{"q": "hi"}, "missing", ""},
		{map[string]any{"q": 42}, "q", ""}, // wrong type → empty
		{nil, "q", ""},
	}
	for _, c := range cases {
		got := stringParam(c.params, c.key)
		if got != c.want {
			t.Errorf("stringParam(%v, %q) = %q, want %q", c.params, c.key, got, c.want)
		}
	}
}
