// Package project gives CHEW a place to call home — a "project," which
// is just a folder on disk that CHEW treats as his workspace. Folders
// instead of "repos" so non-tech users don't trip on git terminology.
//
// Each project carries a GUM.md (CHEW's project memory; see gum.go).
// When CHEW comes back to a folder he's been to before, he reads GUM.md
// to catch up on what's true.
//
// Persistence between REPL sessions: the active project's path is stored
// in <chew-repo>/brain/last-project.txt. On launch, CHEW reads that file
// and reopens the last project (if it still exists).

package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Project is a folder CHEW is working out of.
type Project struct {
	// Path is the absolute path to the folder.
	Path string
	// Name is the folder's basename (used in human-friendly messages).
	Name string
	// GUM is the parsed GUM.md content for this project (may be empty
	// raw if GUM.md doesn't exist or hasn't been opened yet).
	GUM GUM
}

// Open returns a Project for an existing folder. Reads GUM.md if it's
// there. If the folder doesn't have a GUM.md, the Project is still
// returned but Project.GUM will be the zero value — the caller can
// decide whether to write a starter template or let it stay empty.
func Open(path string) (*Project, error) {
	abs, err := resolvePath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("can't open folder: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s isn't a folder", abs)
	}
	p := &Project{Path: abs, Name: filepath.Base(abs)}
	gum, _ := ReadGUM(p.GUMPath()) // empty gum is fine; explicit init is the caller's choice
	p.GUM = gum
	return p, nil
}

// Create makes a new folder at the given path and seeds it with a
// starter GUM.md. Refuses to overwrite an existing folder.
func Create(path string) (*Project, error) {
	abs, err := resolvePath(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err == nil {
		return nil, fmt.Errorf("folder %s already exists; try 'here %s' to use it", abs, abs)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	p := &Project{Path: abs, Name: filepath.Base(abs)}
	gum := NewStarterGUM(p.Name)
	if err := WriteGUM(p.GUMPath(), gum); err != nil {
		// Folder was made; the GUM write failed. Surface but don't
		// roll back — the user can write GUM.md themselves.
		return p, fmt.Errorf("created folder but couldn't write GUM.md: %w", err)
	}
	p.GUM = gum
	return p, nil
}

// GUMPath returns the absolute path to the project's GUM.md.
func (p *Project) GUMPath() string {
	return filepath.Join(p.Path, "GUM.md")
}

// HasGUM reports whether a GUM.md exists in this project.
func (p *Project) HasGUM() bool {
	if p == nil {
		return false
	}
	info, err := os.Stat(p.GUMPath())
	return err == nil && !info.IsDir()
}

// resolvePath normalizes a user-supplied path into an absolute one,
// expanding ~ and trimming surrounding quotes (drag-into-terminal often
// pastes quoted paths on macOS).
func resolvePath(path string) (string, error) {
	p := strings.TrimSpace(path)
	p = strings.Trim(p, `'"`)
	if p == "" {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		p = abs
	}
	return filepath.Clean(p), nil
}

// LooksLikePath returns true if the input is plausibly a filesystem
// path (absolute, ~-rooted, or beginning with quoted /). Used by the
// chat shell to detect a "drag a folder" paste vs a normal command.
func LooksLikePath(input string) bool {
	s := strings.TrimSpace(input)
	s = strings.Trim(s, `'"`)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || s == "~" {
		// Reject if it has spaces unless it was originally quoted
		// (drag-paste includes the quotes around paths that have spaces).
		return true
	}
	// Windows drive-letter path
	if len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return true
	}
	return false
}

// ----- last-project memory -----

// LastProjectPath returns the path to the file we use to remember the
// most recently opened project across REPL sessions. brainDir is the
// canonical CHEW state folder (`<repo>/brain/`).
func LastProjectPath(brainDir string) string {
	return filepath.Join(brainDir, "last-project.txt")
}

// SaveLast writes the project's path to last-project.txt so the next
// REPL launch can reopen it.
func SaveLast(brainDir, projectPath string) error {
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(LastProjectPath(brainDir), []byte(projectPath+"\n"), 0o644)
}

// LoadLast returns the last project's path, or "" if there isn't one
// (or if the file is unreadable, or if the recorded folder no longer
// exists). Never returns an error — a missing file is fine.
func LoadLast(brainDir string) string {
	body, err := os.ReadFile(LastProjectPath(brainDir))
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(body))
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return ""
	}
	return path
}

// ClearLast removes the saved last-project record (e.g. on `forget project`).
func ClearLast(brainDir string) error {
	err := os.Remove(LastProjectPath(brainDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
