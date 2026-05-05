package main

import (
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
)

func seedProfileConfig(t *testing.T, brainDir string) {
	t.Helper()
	cfg := wizard.DefaultProfileConfig(brainDir)
	cfg.Profiles = append(cfg.Profiles, wizard.BrainProfile{
		Name:       "qwen",
		Provider:   wizard.ProviderOpenAICompatible,
		Source:     "chew.internal",
		BaseURL:    "http://127.0.0.1:9911",
		ModelAlias: "qwen3.5",
	})
	if err := wizard.SaveProfileConfig(brainDir, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestProfileActionsStatusAndList(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), "brain")
	seedProfileConfig(t, brainDir)
	p := planner.NewScriptedPlanner()

	var replies []string
	reply := func(s string) { replies = append(replies, s) }

	handleProfileStatusWithReply(p, brainDir, reply)
	handleProfileListWithReply(p, brainDir, reply)

	if len(replies) != 2 {
		t.Fatalf("expected two replies, got %d", len(replies))
	}
	if !strings.Contains(replies[0], "Active brain profile: bonsai") {
		t.Fatalf("status should show active bonsai, got:\n%s", replies[0])
	}
	if !strings.Contains(replies[1], "* bonsai") || !strings.Contains(replies[1], "qwen") {
		t.Fatalf("list should include active bonsai and qwen, got:\n%s", replies[1])
	}
}

func TestProfileUseSwitchesWhenBrainSleeps(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), "brain")
	seedProfileConfig(t, brainDir)
	p := planner.NewScriptedPlanner()
	var brain atomic.Pointer[wizard.Brain]
	var replies []string

	handleProfileUseWithReply(p, &brain, brainDir, "qwen", func(s string) {
		replies = append(replies, s)
	})

	cfg, err := wizard.LoadProfileConfig(brainDir)
	if err != nil {
		t.Fatal(err)
	}
	active, err := cfg.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.Name != "qwen" {
		t.Fatalf("active profile = %q, want qwen", active.Name)
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "qwen") {
		t.Fatalf("reply should mention qwen, got %v", replies)
	}
}

func TestProfileUseRejectsWhileBrainAwake(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), "brain")
	seedProfileConfig(t, brainDir)
	p := planner.NewScriptedPlanner()
	var brain atomic.Pointer[wizard.Brain]
	brain.Store(wizard.AttachBrain("http://127.0.0.1:9911", "qwen3.5"))
	var replies []string

	handleProfileUseWithReply(p, &brain, brainDir, "qwen", func(s string) {
		replies = append(replies, s)
	})

	cfg, err := wizard.LoadProfileConfig(brainDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != wizard.DefaultProfileName {
		t.Fatalf("active profile changed while awake: %q", cfg.ActiveProfile)
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "nap") {
		t.Fatalf("reply should ask user to nap first, got %v", replies)
	}
}

func TestBrainStatusReflectsActualState(t *testing.T) {
	p := planner.NewScriptedPlanner()
	brainDir := filepath.Join(t.TempDir(), "brain")
	seedProfileConfig(t, brainDir)
	if _, _, err := wizard.SetActiveProfile(brainDir, "qwen"); err != nil {
		t.Fatal(err)
	}
	var brain atomic.Pointer[wizard.Brain]
	var replies []string

	handleBrainStatusWithReply(p, &brain, brainDir, func(s string) {
		replies = append(replies, s)
	})
	if len(replies) != 1 || !strings.Contains(replies[0], "napping") {
		t.Fatalf("napping status should say napping, got %v", replies)
	}

	brain.Store(wizard.AttachBrain("http://127.0.0.1:9911", "qwen3.5"))
	handleBrainStatusWithReply(p, &brain, brainDir, func(s string) {
		replies = append(replies, s)
	})
	if !strings.Contains(replies[len(replies)-1], "awake") {
		t.Fatalf("awake status should say awake, got %v", replies)
	}
}

func TestNappingFallbackDoesNotClaimBrainMissing(t *testing.T) {
	p := planner.NewScriptedPlanner()
	brainDir := filepath.Join(t.TempDir(), "brain")
	seedProfileConfig(t, brainDir)
	if _, _, err := wizard.SetActiveProfile(brainDir, "qwen"); err != nil {
		t.Fatal(err)
	}

	setFallbackForBrainState(p, brainDir)
	got := p.Plan("hello!")
	if got.Mascot == "ghost" {
		t.Fatalf("napping fallback should not use ghost mascot: %+v", got)
	}
	lower := strings.ToLower(got.Response)
	if strings.Contains(lower, "install brain") || strings.Contains(lower, "brainless") {
		t.Fatalf("napping fallback should suggest wake up, not install brain: %q", got.Response)
	}
	if !strings.Contains(lower, "wake up") {
		t.Fatalf("napping fallback should mention wake up: %q", got.Response)
	}
}

func TestBrainChatUsesProfileAlias(t *testing.T) {
	brain := wizard.AttachBrain("http://127.0.0.1:9911/v1", "qwen3.5")

	session := newChatSession(brain, nil)
	if session.endpoint != "http://127.0.0.1:9911/v1/chat/completions" {
		t.Fatalf("endpoint = %q", session.endpoint)
	}
	if session.alias != "qwen3.5" {
		t.Fatalf("alias = %q", session.alias)
	}
}

func TestSummariseSpawnErrHidesProfileInternals(t *testing.T) {
	for _, err := range []error{
		errors.New("active brain profile is missing an endpoint"),
		errors.New(`unsupported brain provider "private"`),
	} {
		got := summariseSpawnErr(err)
		if got != "brain settings need fixing" {
			t.Fatalf("summary = %q, want generic settings message", got)
		}
		if strings.Contains(strings.ToLower(got), "profile") {
			t.Fatalf("summary should not expose hidden profile vocabulary: %q", got)
		}
	}
}
