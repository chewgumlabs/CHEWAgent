package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/tool"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
)

func TestHandleCreateProjectMovesIntoNewProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	brainDir := filepath.Join(home, "brain")
	target := filepath.Join(home, "Documents", "Comic Tracker")

	p := planner.NewScriptedPlanner()
	reg := tool.NewDefault()
	var proj atomic.Pointer[project.Project]
	var brain atomic.Pointer[wizard.Brain]
	var replies []string

	handleCreateProjectWithReply(p, reg, &proj, &brain, brainDir, "Comic Tracker", func(s string) {
		replies = append(replies, s)
	})

	pj := proj.Load()
	if pj == nil {
		t.Fatal("create project should set the active project")
	}
	if pj.Path != target {
		t.Fatalf("active project path = %q, want %q", pj.Path, target)
	}
	if got := project.LoadLast(brainDir); got != target {
		t.Fatalf("last project = %q, want %q", got, target)
	}
	res, err := reg.Dispatch("list_dir", map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("list_dir from active project: %v", err)
	}
	if !strings.Contains(res.Output, "GUM.md") {
		t.Fatalf("active project root should contain GUM.md, got:\n%s", res.Output)
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "moved in") {
		t.Fatalf("reply should say CHEW moved in, got %v", replies)
	}
}

func TestHandleCreateFolderDoesNotChangeActiveProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	brainDir := filepath.Join(home, "brain")
	target := filepath.Join(home, "Documents", "Scratch")

	p := planner.NewScriptedPlanner()
	var proj atomic.Pointer[project.Project]
	var replies []string

	handleCreateFolderWithReply(p, "Scratch", func(s string) {
		replies = append(replies, s)
	})

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("plain folder should exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("plain folder target is not a directory: %s", target)
	}
	if _, err := os.Stat(filepath.Join(target, "GUM.md")); !os.IsNotExist(err) {
		t.Fatalf("plain folder should not get GUM.md, stat err=%v", err)
	}
	if proj.Load() != nil {
		t.Fatal("make folder should not set an active project")
	}
	if got := project.LoadLast(brainDir); got != "" {
		t.Fatalf("make folder should not save last project, got %q", got)
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "Not a project") {
		t.Fatalf("reply should distinguish plain folders from projects, got %v", replies)
	}
}
