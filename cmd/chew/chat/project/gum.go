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
// ships into a fresh project; CHEW fills it through conversation.

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

// Summary returns a short preview of meaningful project memory. Starter
// template instructions and placeholder fields are intentionally ignored
// so a fresh project does not dump boilerplate into the chat.
func (g GUM) Summary() string {
	if g.IsEmpty() {
		return ""
	}
	const max = 420
	lines := strings.Split(strings.TrimSpace(g.Raw), "\n")
	out := make([]string, 0, 4)
	section := ""
	inPlaceholder := false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if i == 0 && strings.HasPrefix(line, "# ") {
			continue
		}
		if inPlaceholder {
			if strings.Contains(line, ">") {
				inPlaceholder = false
			}
			continue
		}
		if strings.HasPrefix(line, "<") {
			if !strings.Contains(line, ">") {
				inPlaceholder = true
			}
			continue
		}
		if isStarterGUMBoilerplate(line) {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		if section != "" {
			out = append(out, section+": "+line)
			section = ""
		} else {
			out = append(out, line)
		}
	}
	body := strings.TrimSpace(strings.Join(out, "\n"))
	if len(body) > max {
		body = body[:max] + "..."
	}
	return body
}

func isStarterGUMBoilerplate(line string) bool {
	return strings.Contains(line, "CHEW's project memory") ||
		strings.Contains(line, "edits it when something important changes") ||
		strings.Contains(line, "him what you want him to remember")
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
	const placeholder = "<dated entries. CHEW appends here when significant choices are made.>"
	content = strings.Replace(content, placeholder+"\n", "", 1)
	content = strings.Replace(content, placeholder, "", 1)
	if strings.Contains(content, heading) {
		// Insert just under the heading.
		content = strings.Replace(content, heading+"\n", heading+"\n"+dated+"\n", 1)
	} else {
		content = strings.TrimRight(content, "\n") + "\n\n" + heading + "\n" + dated + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// NewStarterGUM returns a fresh GUM body for a brand-new project. It is
// a machine-owned memory file: users can edit it if they go looking, but
// the chat UX should fill it through conversation.
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

// SetIntent replaces the Intent section with a sentence captured from
// conversation. If the section is missing, it is added near the top.
func SetIntent(path, intent string) error {
	intent = normalizeIntentSentence(intent)
	if intent == "" {
		return errors.New("intent is empty")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := replaceMarkdownSection(string(body), "## Intent", intent+"\n")
	return os.WriteFile(path, []byte(updated), 0o644)
}

func normalizeIntentSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, `"'`)
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - ('a' - 'A')
	}
	s = string(r)
	if !strings.ContainsAny(s[len(s)-1:], ".!?") {
		s += "."
	}
	return s
}

func replaceMarkdownSection(raw, heading, sectionBody string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != heading {
			continue
		}
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "## ") {
			j++
		}
		replacement := append([]string{}, lines[:i+1]...)
		replacement = append(replacement, strings.TrimRight(sectionBody, "\n"))
		replacement = append(replacement, "")
		replacement = append(replacement, lines[j:]...)
		return strings.TrimRight(strings.Join(replacement, "\n"), "\n") + "\n"
	}

	insert := heading + "\n" + strings.TrimRight(sectionBody, "\n") + "\n"
	if strings.HasPrefix(strings.TrimSpace(raw), "# ") {
		lines := strings.Split(raw, "\n")
		if len(lines) > 1 {
			out := append([]string{lines[0], "", insert}, lines[1:]...)
			return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
		}
	}
	return strings.TrimRight(insert+"\n"+raw, "\n") + "\n"
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
