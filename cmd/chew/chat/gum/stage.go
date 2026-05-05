// Package gum is the Truth Steward — the deterministic, brainless layer
// that watches the project's shape and tells the brain what to do at
// each stage.
//
// Why "GUM"? In the chew/gum architecture, CHEW is the agent (the one
// who talks, executes verbs, takes actions). GUM is the keeper of what's
// true and the manager of where we are. The brain is good at language
// but bad at tracking state across turns. GUM observes — what files
// exist, what's in GUM.md, what's been committed — and hands the brain
// a stage-appropriate playbook on every turn.
//
// The package is deliberately stdlib-only and synchronous. Detection is
// fast (a few stat calls and one or two small file reads), so we re-run
// it whenever the brain's system prompt is rebuilt. No background
// watchers, no event loop.

package gum

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
)

// Stage is the state of a project as GUM sees it.
type Stage int

const (
	// StageNoProject — user hasn't set up a folder yet. CHEW's job is
	// to push for one before any building happens.
	StageNoProject Stage = iota

	// StageEmptyProject — folder is set up but Intent isn't filled in
	// and there are no source files yet. CHEW's job is to ask what
	// they're building and keep project memory invisible.
	StageEmptyProject

	// StageIntentKnown — GUM.md's Intent section has real content
	// (placeholders replaced) but there are no source files. CHEW's
	// job is to suggest one concrete first file.
	StageIntentKnown

	// StageStarted — source files exist. CHEW walks one step at a time.
	StageStarted

	// StageMature — multiple commits beyond the first, multiple files.
	// CHEW focuses on the user's specific ask + suggests save points.
	StageMature
)

// String returns a human-readable name (mostly for logs/tests).
func (s Stage) String() string {
	switch s {
	case StageNoProject:
		return "no_project"
	case StageEmptyProject:
		return "empty_project"
	case StageIntentKnown:
		return "intent_known"
	case StageStarted:
		return "started"
	case StageMature:
		return "mature"
	}
	return "unknown"
}

// Detect looks at the project's folder and returns the current stage.
// pj may be nil — that just means StageNoProject.
//
// All file IO failures are swallowed — they get treated as "this signal
// isn't available" and we move on. The function is best-effort and
// always returns a stage; never an error.
func Detect(pj *project.Project) Stage {
	if pj == nil {
		return StageNoProject
	}

	intentFilled := hasFilledIntent(pj.GUM.Raw)
	srcCount := countSourceFiles(pj.Path)
	commits := countCommits(pj.Path)

	switch {
	case srcCount >= 3 && commits >= 2:
		return StageMature
	case srcCount > 0:
		return StageStarted
	case intentFilled:
		return StageIntentKnown
	default:
		return StageEmptyProject
	}
}

// hasFilledIntent reports whether the Intent section of a GUM.md body
// has non-placeholder content. The starter template uses
// "<one paragraph: ...>" placeholders; if the section still contains
// only that, it's NOT filled.
func hasFilledIntent(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	section := extractSection(raw, "## Intent")
	if section == "" {
		return false
	}
	// Strip placeholder lines (anything entirely inside <...>) and see
	// what's left.
	var real []string
	for _, line := range strings.Split(section, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "<") && strings.HasSuffix(t, ">") {
			continue
		}
		real = append(real, t)
	}
	return len(real) > 0
}

// extractSection returns the body under a markdown heading (everything
// after the heading line, up to the next ## heading or EOF). Empty if
// the heading isn't found.
func extractSection(raw, heading string) string {
	lines := strings.Split(raw, "\n")
	var out []string
	in := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == heading {
			in = true
			continue
		}
		if in && strings.HasPrefix(t, "## ") {
			break
		}
		if in {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// countSourceFiles walks the project root (one level deep + a couple of
// well-known sub-paths like src/ and lib/) counting non-meta files.
// Skips: GUM.md, README*, LICENSE*, hidden files/dirs, common vendor dirs.
//
// "Source file" is loose on purpose — we just want to know "did the
// user start filling this folder with stuff."
func countSourceFiles(dir string) int {
	if dir == "" {
		return 0
	}
	count := 0
	depth := 0
	const maxDepth = 4
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if p == dir {
			return nil
		}
		// Compute depth relative to dir.
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		depth = strings.Count(rel, string(filepath.Separator))
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "target" || base == "build" || base == "dist" {
				return filepath.SkipDir
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if isMetaFile(d.Name()) {
			return nil
		}
		count++
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return count
	}
	return count
}

// isMetaFile returns true for files we don't count as "real source"
// (GUM.md, README, LICENSE, .gitignore, etc.).
func isMetaFile(name string) bool {
	switch name {
	case "GUM.md", ".gitignore", ".DS_Store":
		return true
	}
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "README") {
		return true
	}
	if strings.HasPrefix(upper, "LICENSE") {
		return true
	}
	return false
}

// countCommits returns the number of commits in the project's git log.
// 0 if the folder isn't a git repo or git isn't available. Best-effort.
//
// We don't shell out for this — we just count files in .git/refs/heads/
// (each branch has at least one commit) and check if HEAD exists. For
// our purposes (mature vs not) a precise count isn't needed; "is there
// more than the one initial save point" is enough.
func countCommits(dir string) int {
	gitDir := filepath.Join(dir, ".git")
	headPath := filepath.Join(gitDir, "HEAD")
	if _, err := os.Stat(headPath); err != nil {
		return 0
	}
	// Count entries in logs/HEAD (one line per commit landed on this
	// HEAD's history). Cheap proxy for "did anything happen besides
	// the initial commit."
	logPath := filepath.Join(gitDir, "logs", "HEAD")
	body, err := os.ReadFile(logPath)
	if err != nil {
		return 1 // HEAD exists but no log — at least one commit assumed
	}
	n := 0
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
