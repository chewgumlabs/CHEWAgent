// Package tool is the executable side of the planner's verbs.
//
// The planner produces a Plan with named verbs and their parameters; the
// REPL hands those verbs to a Registry which looks up a Tool implementation
// and runs it. This package owns the Tool interface, the Registry, and the
// real implementations (web_search, web_fetch, etc.).
//
// Tool authors only think about Execute. The Registry handles dispatch,
// error wrapping, and missing-tool fallbacks.
//
// Adding a new tool:
//   1. Create a struct that implements Tool.
//   2. Register it in NewDefault() (or via Register on a custom Registry).
//   3. The planner already exposes the verb? Done. Otherwise add a regex
//      rule in cmd/chew/chat/planner/scripted.go.

package tool

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Tool is anything that can be invoked by name with a JSON-shaped param map.
type Tool interface {
	// Name is the verb name the planner uses (e.g. "web_search").
	Name() string
	// Execute runs the tool. The result's Output is shown to the user
	// verbatim; Mascot, if non-empty, overrides the chat shell's mascot
	// state for this turn.
	Execute(params map[string]any) (Result, error)
}

// Result is what a Tool returns to the chat shell.
type Result struct {
	Output string // text to print to the user
	Mascot string // optional mascot-state hint: "idle" | "walk" | "ghost"
}

// Registry holds a name->Tool map and dispatches verbs.
type Registry struct {
	tools map[string]Tool
	root  string // active project root; empty means process cwd
}

// NewRegistry creates an empty Registry. Use NewDefault() to get one
// with the standard tool set already registered.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// NewDefault returns a Registry with the standard tool set:
//
//   - web_search   DuckDuckGo HTML search
//   - web_fetch    HTTPS GET + text extraction
//   - read_file    print a file's contents (1 MB cap)
//   - list_dir     enumerate a directory (200-entry cap)
//   - search       walk a tree, regex-match lines (100-hit cap)
//   - write_file   create a new file; refuses to overwrite
//   - run_command  exec via sh -c (30s timeout, 8 KB output cap)
//   - preview      build/reuse a local static preview server
func NewDefault() *Registry {
	r := NewRegistry()
	r.Register(&WebSearch{})
	r.Register(&WebFetch{})
	r.Register(&ReadFile{Root: &r.root})
	r.Register(&ListDir{Root: &r.root})
	r.Register(&Search{Root: &r.root})
	r.Register(&WriteFile{Root: &r.root})
	r.Register(&RunCommand{Root: &r.root})
	r.Register(&Preview{Root: &r.root})
	return r
}

// Register adds a Tool to the registry, replacing any existing tool with
// the same name. Safe to call before Dispatch starts.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// SetRoot sets the active project root for path resolution and command
// execution. Pass "" to clear (revert to process cwd).
func (r *Registry) SetRoot(path string) {
	if path == "" {
		r.root = ""
		return
	}
	r.root = filepath.Clean(path)
}

// Has reports whether a tool by that name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// Names returns all registered tool names in lexical order.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for k := range r.tools {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ErrUnknownTool is returned by Dispatch when no Tool is registered under
// the given name. The REPL surfaces this as a "planned but not wired"
// notice rather than treating it as a hard error.
var ErrUnknownTool = errors.New("unknown tool")

// Dispatch looks up a tool by name and runs it. Returns ErrUnknownTool
// (wrapped) if nothing is registered under that name; any other error
// comes from the tool itself.
func (r *Registry) Dispatch(name string, params map[string]any) (Result, error) {
	t, ok := r.tools[name]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
	res, err := t.Execute(params)
	if err != nil {
		return res, fmt.Errorf("%s: %w", name, err)
	}
	return res, nil
}

// stringParam pulls a string value from a verb's param map. Returns the
// empty string if the key is missing or not a string. Tools should
// validate their own required params and surface friendly errors when
// values are missing.
func stringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	v, ok := params[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// resolveToolPath resolves a relative path against the given root.
// Returns the path unchanged if root is nil/empty or path is absolute.
// Returns an error if the resolved path would escape the root.
func resolveToolPath(root *string, path string) (string, error) {
	if root == nil || *root == "" || path == "" || filepath.IsAbs(path) {
		return path, nil
	}
	resolved := filepath.Clean(filepath.Join(*root, path))
	if resolved != *root && !strings.HasPrefix(resolved, *root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q resolves outside the project root", path)
	}
	return resolved, nil
}
