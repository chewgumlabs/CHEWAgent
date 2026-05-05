// gum.go — read/write GUM.md, CHEW's per-project memory.
//
// GUM is the Truth Steward in the chew/gum architecture. GUM.md is the
// file CHEW reads on arrival to catch up on what's true about the
// project — intent, ground truth, decisions, open questions. It's the
// human-friendly answer to "AGENTS.md" / "CLAUDE.md" / ".cursorrules"
// but tied to our character.
//
// For v0 we don't parse sections — GUM.Raw is the whole file. Future
// passes can extract structured fields. The starter template is what
// ships into a fresh project; the user fills in the prose.

package project

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// GUM is the parsed (or in v0, just-read-as-string) content of GUM.md.
type GUM struct {
	// Raw is the unparsed file body. Empty if no GUM.md exists yet.
	Raw string
}

// IsEmpty reports whether GUM.md hasn't been opened or is blank.
func (g GUM) IsEmpty() bool {
	return strings.TrimSpace(g.Raw) == ""
}

// Summary returns a short preview of the GUM content suitable for the
// chat REPL's "what we're working on" greeting. Keeps the first non-blank
// section (after the H1) up to ~600 chars.
func (g GUM) Summary() string {
	if g.IsEmpty() {
		return ""
	}
	const max = 600
	body := strings.TrimSpace(g.Raw)
	// Skip the H1 if present.
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	skipH1 := strings.HasPrefix(body, "# ")
	for i, line := range lines {
		if skipH1 && i == 0 {
			continue
		}
		out = append(out, line)
	}
	body = strings.TrimSpace(strings.Join(out, "\n"))
	if len(body) > max {
		body = body[:max] + "..."
	}
	return body
}

// ReadGUM returns the GUM.md at path. A missing file is not an error —
// the caller gets a zero-value GUM and can decide what to do.
func ReadGUM(path string) (GUM, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return GUM{}, nil
		}
		return GUM{}, fmt.Errorf("read %s: %w", path, err)
	}
	return GUM{Raw: string(body)}, nil
}

// WriteGUM writes GUM to path, refusing to silently overwrite existing
// non-empty content. If you want to update an existing GUM, use
// AppendDecision or read+modify+write yourself.
func WriteGUM(path string, g GUM) error {
	if existing, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(existing))) > 0 {
		return fmt.Errorf("%s already has content; refusing to overwrite", path)
	}
	if err := os.WriteFile(path, []byte(g.Raw), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// AppendDecision adds a dated bullet under the "## Recent decisions"
// section. If the section doesn't exist, it's added at the bottom.
// Useful for CHEW to record his own observations across sessions.
func AppendDecision(path, bullet string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	dated := fmt.Sprintf("- %s — %s", time.Now().Format("2006-01-02"), strings.TrimSpace(bullet))
	content := string(body)
	const heading = "## Recent decisions"
	if strings.Contains(content, heading) {
		// Insert just under the heading.
		content = strings.Replace(content, heading+"\n", heading+"\n"+dated+"\n", 1)
	} else {
		content = strings.TrimRight(content, "\n") + "\n\n" + heading + "\n" + dated + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// NewStarterGUM returns a fresh GUM body for a brand-new project. The
// template prompts the user to fill in the why, ground truth, and open
// questions. CHEW writes into "Recent decisions" automatically over
// time.
func NewStarterGUM(projectName string) GUM {
	if projectName == "" {
		projectName = "this project"
	}
	body := fmt.Sprintf(`# GUM.md — %s

This is CHEW's project memory. He reads it when he comes back, and
edits it when something important changes. Edit it yourself to teach
him what you want him to remember.

## Intent
<one paragraph: what we're building, in plain English>

## Why
<who's it for, what problem it solves, why bother>

## Ground truth
<facts that stay true: tech choices, constraints, the shape we've
agreed on. CHEW trusts these unless you change them.>

## Open questions
<things still being worked out. CHEW may add to this list as we
explore.>

## Recent decisions
<dated entries. CHEW appends here when significant choices are made.>
`, projectName)
	return GUM{Raw: body}
}

// SaveRaw writes whatever you hand it to disk verbatim. Used when the
// caller already has the full body (e.g. an LLM-generated one) and
// wants to commit it.
func SaveRaw(path string, raw string) error {
	return os.WriteFile(path, []byte(raw), 0o644)
}

// readAll is a tiny wrapper used by tests to mirror os.ReadFile in a
// place that doesn't require touching the OS.
//
//nolint:unused // intentionally unused; useful for future test refactors
func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
