// Package planner converts user input into a verb plan + response text.
// Two implementations:
//   - ScriptedPlanner: regex/pattern table, no LLM, brainless mode
//   - LLMPlanner: queries the model (deferred — implemented when model present)
//
// Both return the same Plan shape. The chat shell never knows which planner
// is active; it just calls Plan(input) and gets back a response.
//
// All user-visible strings live in voice.go. This file is the engine —
// the regex table and the round-robin counter — and shouldn't need to
// change when you rewrite CHEW's lines.

package planner

import (
	"fmt"
	"regexp"
	"strings"
)

// Plan is the result of planning. The chat shell renders Response and
// (optionally) executes Verbs.
type Plan struct {
	Response     string            // text to print to the user
	Verbs        []Verb            // verbs to dispatch (may be empty for pure-talk responses)
	Halt         bool              // signal the chat shell to exit (e.g., on "quit")
	Mascot       string            // mascot state hint: "idle" | "walk" | "ghost"
	LaunchWizard string            // if non-empty, the chat shell hands off to a system action
	//                                by this name (e.g. "install_brain", "open_project").
	//                                Response and Verbs are still rendered first; the action
	//                                runs after.
	LaunchArgs map[string]string // arg map for the system action — e.g.
	//                                {"path": "/Users/.../comic-tracker"} for open_project.
}

// Verb is a generic verb invocation: name + JSON-shaped params.
type Verb struct {
	Name   string
	Params map[string]any
}

// Planner is the interface both implementations satisfy.
type Planner interface {
	Plan(input string) Plan
}

// ----- ScriptedPlanner -----

// ScriptedPlanner walks a vocabulary table of (regex, handler) entries.
// First match wins. Each rule pulls its response from a named pool in
// voice.go and cycles through the variants round-robin so the chat reads
// like a text adventure rather than an install-helper transcript.
type ScriptedPlanner struct {
	rules    []scriptRule
	fallback func(input string) Plan
	counters map[string]int // round-robin index per pool key
}

type scriptRule struct {
	pattern *regexp.Regexp
	handler func(matches []string) Plan
}

// NewScriptedPlanner builds the default vocabulary. The chat shell uses
// this when no LLM is configured/reachable.
func NewScriptedPlanner() *ScriptedPlanner {
	p := &ScriptedPlanner{counters: map[string]int{}}
	registerCoreVocabulary(p)
	p.fallback = p.makeFallback()
	return p
}

// Add registers a (pattern, handler) entry. First-added rules take
// precedence (we walk in registration order).
func (p *ScriptedPlanner) Add(pattern string, handler func(matches []string) Plan) {
	p.rules = append(p.rules, scriptRule{
		pattern: regexp.MustCompile(pattern),
		handler: handler,
	})
}

// Plan walks the rule table and returns the first match's response, or
// the fallback if nothing matches.
func (p *ScriptedPlanner) Plan(input string) Plan {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Plan{Response: ""} // silent no-op on empty input
	}
	for _, r := range p.rules {
		if matches := r.pattern.FindStringSubmatch(trimmed); matches != nil {
			return r.handler(matches)
		}
	}
	return p.fallback(trimmed)
}

// pick returns the next variant from the named pool and advances the
// round-robin counter. Pools are defined in voice.go.
func (p *ScriptedPlanner) pick(key string) string {
	pool := responsePools[key]
	if len(pool) == 0 {
		return ""
	}
	i := p.counters[key]
	p.counters[key] = i + 1
	return pool[i%len(pool)]
}

// PickVoice exposes the round-robin pool picker so the REPL (and other
// callers outside this package) can pull frog-flavored variants for
// system-action messages. Returns "" if the pool key is unknown.
func (p *ScriptedPlanner) PickVoice(key string) string {
	return p.pick(key)
}

// makeFallback returns the closure used when nothing matches. It cycles
// through fallback variants so the "I don't know that one" moment doesn't
// read identically every time.
func (p *ScriptedPlanner) makeFallback() func(string) Plan {
	return func(input string) Plan {
		return Plan{
			Response: fmt.Sprintf(p.pick("fallback"), truncate(input, 60)),
			Mascot:   "ghost", // brainless = ghost-mode visual cue
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// ----- core vocabulary -----

// registerCoreVocabulary wires up the v0 command set: ~15 patterns covering
// core dev workflow (read/edit/find/run/git basics, plus help/list/install).
// First match wins; order from most-specific to most-general.
//
// The strings live in voice.go. Each handler picks a variant from the
// matching pool and fills any %s/%q slots from the regex captures.
func registerCoreVocabulary(p *ScriptedPlanner) {
	// system commands
	p.Add(`(?i)^(quit|exit|q|bye)$`, func(_ []string) Plan {
		return Plan{Response: p.pick("quit"), Halt: true}
	})
	p.Add(`(?i)^(help|\?)$`, func(_ []string) Plan {
		return Plan{Response: p.helpText(), Mascot: "idle"}
	})
	p.Add(`(?i)^(list|commands)$`, func(_ []string) Plan {
		return Plan{Response: p.helpText(), Mascot: "idle"}
	})

	// install brain — signal the chat shell to hand off to the wizard. The
	// wizard owns all the user-facing text from here on (download progress,
	// done message, etc). The planner returns no Response so the user goes
	// straight into the wizard's flow.
	p.Add(`(?i)^install brain$`, func(_ []string) Plan {
		return Plan{LaunchWizard: "install_brain", Mascot: "idle"}
	})

	// wake up — signal the REPL to spawn the installed brain on demand.
	// Loading Bonsai takes ~3-4s and ~1.5 GB RAM; the user controls when.
	p.Add(`(?i)^(wake( up)?|wake brain|start brain)$`, func(_ []string) Plan {
		return Plan{LaunchWizard: "wake_brain", Mascot: "walk"}
	})

	// nap / sleep — signal the REPL to stop the running brain and free
	// memory. Subsequent free-form questions go back to the brainless
	// fallback until the user types 'wake up' again.
	p.Add(`(?i)^(nap|sleep|sleep brain|stop brain)$`, func(_ []string) Plan {
		return Plan{LaunchWizard: "nap_brain", Mascot: "idle"}
	})

	// here <path> — explicitly set the project folder.
	p.Add(`(?i)^here\s+(.+)$`, func(m []string) Plan {
		return Plan{
			LaunchWizard: "open_project",
			LaunchArgs:   map[string]string{"path": strings.TrimSpace(m[1])},
			Mascot:       "walk",
		}
	})

	// make folder <name> — create a fresh project folder under ~/Documents/.
	p.Add(`(?i)^(make|new)\s+(folder|project)\s+(.+)$`, func(m []string) Plan {
		return Plan{
			LaunchWizard: "create_project",
			LaunchArgs:   map[string]string{"name": strings.TrimSpace(m[3])},
			Mascot:       "walk",
		}
	})

	// forget project — clear the active project and last-project memory.
	p.Add(`(?i)^(forget|leave)\s+project$`, func(_ []string) Plan {
		return Plan{LaunchWizard: "forget_project", Mascot: "idle"}
	})

	// "I'd like to build / let's make / help me make X" — project intent.
	// Doesn't dispatch anything; just nudges the user to set up a folder.
	p.Add(`(?i)^(i('?d| would)? like to|i want to|let'?s|let me|help me|i'?m (going to|gonna|trying to))\s+(build|make|create|design|develop|setup|set up|start)\s+(.+)$`,
		func(m []string) Plan {
			thing := strings.TrimSpace(m[5])
			return Plan{
				Response: fmt.Sprintf(p.pick("project_pitch"), thing),
				Mascot:   "idle",
			}
		})

	// Bare path (drag-into-terminal often pastes a path). Treat as
	// "open as project folder." Must come BEFORE the read/cat/show rule
	// in case anyone types just `/Users/.../foo` on its own.
	p.Add(`^['"]?(/|~/|~$|[A-Za-z]:[\\/])[^\n]*['"]?$`, func(m []string) Plan {
		// m[0] is the whole match; trim quotes
		raw := strings.TrimSpace(m[0])
		raw = strings.Trim(raw, `'"`)
		return Plan{
			LaunchWizard: "open_project",
			LaunchArgs:   map[string]string{"path": raw},
			Mascot:       "walk",
		}
	})

	// web — fetch a URL. MUST come before the generic `read <file>` rule
	// because URLs would otherwise route to read_file with a confusing path.
	p.Add(`(?i)^(fetch|open|visit|read)\s+(https?://\S+)$`, func(m []string) Plan {
		url := m[2]
		return Plan{
			Verbs:    []Verb{{Name: "web_fetch", Params: map[string]any{"url": url}}},
			Response: fmt.Sprintf(p.pick("web_fetch"), url),
			Mascot:   "walk",
		}
	})

	// web — search the open web. Triggers always include "web" or "google"
	// so this never collides with the local `find|grep|search` rule below.
	p.Add(`(?i)^(?:web\s+search|websearch|google|search\s+(?:the\s+)?web(?:\s+for)?)\s+(.+)$`, func(m []string) Plan {
		query := strings.TrimSpace(m[1])
		return Plan{
			Verbs:    []Verb{{Name: "web_search", Params: map[string]any{"query": query}}},
			Response: fmt.Sprintf(p.pick("web_search"), query),
			Mascot:   "walk",
		}
	})

	// file ops — read
	p.Add(`(?i)^(read|cat|show)\s+(.+)$`, func(m []string) Plan {
		return Plan{
			Verbs:    []Verb{{Name: "read_file", Params: map[string]any{"path": m[2]}}},
			Response: fmt.Sprintf(p.pick("read"), m[2]),
			Mascot:   "walk",
		}
	})

	// file ops — list dir
	p.Add(`(?i)^(ls|dir|list dir|list folder)\s*(.*)$`, func(m []string) Plan {
		dir := strings.TrimSpace(m[2])
		if dir == "" {
			dir = "."
		}
		return Plan{
			Verbs:    []Verb{{Name: "list_dir", Params: map[string]any{"path": dir}}},
			Response: fmt.Sprintf(p.pick("ls"), dir),
			Mascot:   "walk",
		}
	})

	// file ops — write/create file (template)
	p.Add(`(?i)^(write|create|new)\s+(?:file\s+)?(\S+)(?:\s+with\s+(.*))?$`, func(m []string) Plan {
		path, body := m[2], m[3]
		params := map[string]any{"path": path}
		if body != "" {
			params["content"] = body
		}
		return Plan{
			Verbs:    []Verb{{Name: "write_file", Params: params}},
			Response: fmt.Sprintf(p.pick("write"), path),
			Mascot:   "walk",
		}
	})

	// search/find
	p.Add(`(?i)^(find|grep|search)\s+(?:for\s+)?["']?([^"']+?)["']?(?:\s+in\s+(\S+))?$`, func(m []string) Plan {
		pattern := m[2]
		target := strings.TrimSpace(m[3])
		if target == "" {
			target = "."
		}
		return Plan{
			Verbs:    []Verb{{Name: "search", Params: map[string]any{"pattern": pattern, "path": target}}},
			Response: fmt.Sprintf(p.pick("find"), pattern, target),
			Mascot:   "walk",
		}
	})

	// run shell command
	p.Add(`(?i)^(run|exec|sh)\s+(.+)$`, func(m []string) Plan {
		cmd := m[2]
		return Plan{
			Verbs:    []Verb{{Name: "run_command", Params: map[string]any{"command": cmd}}},
			Response: fmt.Sprintf(p.pick("run"), cmd),
			Mascot:   "walk",
		}
	})

	// git family — read-only by default
	p.Add(`(?i)^git\s+(status|diff|log|branch|show|blame)(.*)$`, func(m []string) Plan {
		sub := strings.TrimSpace(m[1] + " " + m[2])
		return Plan{
			Verbs:    []Verb{{Name: "run_command", Params: map[string]any{"command": "git " + sub}}},
			Response: fmt.Sprintf(p.pick("git_read"), sub),
			Mascot:   "walk",
		}
	})

	// git mutating — explicit confirmation needed. Variants take TWO %s
	// slots, both the sub-command name (one for narration, one for the
	// example).
	p.Add(`(?i)^git\s+(commit|push|merge|reset|rebase|checkout)`, func(m []string) Plan {
		sub := m[1]
		return Plan{
			Response: fmt.Sprintf(p.pick("git_mutating"), sub, sub),
			Mascot:   "idle",
		}
	})
	p.Add(`(?i)^force:\s+(.+)$`, func(m []string) Plan {
		cmd := m[1]
		return Plan{
			Verbs:    []Verb{{Name: "run_command", Params: map[string]any{"command": cmd}}},
			Response: fmt.Sprintf(p.pick("force_unlock"), cmd),
			Mascot:   "walk",
		}
	})

	// "where am I" / pwd
	p.Add(`(?i)^(pwd|where am i|cwd)$`, func(_ []string) Plan {
		return Plan{
			Verbs:  []Verb{{Name: "run_command", Params: map[string]any{"command": "pwd"}}},
			Mascot: "walk",
		}
	})

	// brain status check
	p.Add(`(?i)^(status|brain|are you smart|are you ok)$`, func(_ []string) Plan {
		return Plan{Response: p.pick("status"), Mascot: "ghost"}
	})

	// "who is shane / who made you / who created chew" — point at the
	// canonical bio URL. Brainless answer matches the brain's prompted
	// answer so behaviour is consistent whether or not Bonsai is awake.
	p.Add(`(?i)^(who('?s| is)?\s+shane|who('?s| made| created| built| wrote)\s+(you|chew|chewagent|this)|who'?s behind (you|chew|this)|whose project is this)\??$`,
		func(_ []string) Plan {
			return Plan{Response: p.pick("about_shane"), Mascot: "idle"}
		})
}

// helpText composes the cycled intro line with the static body from voice.go.
func (p *ScriptedPlanner) helpText() string {
	return p.pick("help_intro") + "\n\n" + helpBody
}

// SetFallback swaps the function used when no rule matches. Pass nil to
// restore the default (brainless-mode) fallback. The chat shell uses this
// to point the fallback at a brain-backed LLM after `install brain`
// completes — so free-form questions stop hitting the "I don't know that
// one" message and start hitting the actual model.
func (p *ScriptedPlanner) SetFallback(fn func(input string) Plan) {
	if fn == nil {
		p.fallback = p.makeFallback()
		return
	}
	p.fallback = fn
}
