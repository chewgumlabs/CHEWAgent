package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileConfig_DefaultsToBonsai(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), "brain")

	cfg, err := LoadProfileConfig(brainDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaVersion != ProfileConfigSchema {
		t.Fatalf("schema = %q, want %q", cfg.SchemaVersion, ProfileConfigSchema)
	}
	prof, err := cfg.Active()
	if err != nil {
		t.Fatal(err)
	}
	if prof.Name != DefaultProfileName {
		t.Fatalf("active profile = %q, want %q", prof.Name, DefaultProfileName)
	}
	if prof.Provider != ProviderLlamaServer {
		t.Fatalf("provider = %q, want %q", prof.Provider, ProviderLlamaServer)
	}
	if prof.ModelPath != filepath.Join(brainDir, installBrainModelFile) {
		t.Fatalf("model path = %q", prof.ModelPath)
	}
}

func TestProfileConfig_SaveLoadAndSwitch(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), "brain")
	cfg := DefaultProfileConfig(brainDir)
	cfg.Profiles = append(cfg.Profiles, BrainProfile{
		Name:       "qwen",
		Provider:   ProviderOpenAICompatible,
		Source:     "chew.internal",
		BaseURL:    "http://127.0.0.1:9911/v1",
		ModelAlias: "qwen3.5",
	})

	if err := SaveProfileConfig(brainDir, cfg); err != nil {
		t.Fatal(err)
	}
	_, prof, err := SetActiveProfile(brainDir, "QWEN")
	if err != nil {
		t.Fatal(err)
	}
	if prof.Name != "qwen" {
		t.Fatalf("switched profile = %q, want qwen", prof.Name)
	}

	reloaded, err := LoadProfileConfig(brainDir)
	if err != nil {
		t.Fatal(err)
	}
	active, err := reloaded.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.Name != "qwen" || active.BaseURL != "http://127.0.0.1:9911" {
		t.Fatalf("active after reload = %+v", active)
	}
	list := FormatProfileList(reloaded)
	if !strings.Contains(list, "* qwen") || !strings.Contains(list, "bonsai") {
		t.Fatalf("profile list should mark qwen active and include bonsai, got:\n%s", list)
	}
	status := FormatProfileStatus(reloaded)
	if !strings.Contains(status, "qwen") || !strings.Contains(status, "/v1/chat/completions") {
		t.Fatalf("profile status should show active endpoint, got:\n%s", status)
	}
}

func TestProfileConfig_LoadsLegacyConfig(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyModel := filepath.Join(brainDir, "old.gguf")
	body := []byte(`{
  "model_endpoint": "http://127.0.0.1:8080/v1/chat/completions",
  "model_alias": "ChewBrain",
  "model_path": "` + legacyModel + `"
}`)
	if err := os.WriteFile(ProfileConfigPath(brainDir), body, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProfileConfig(brainDir)
	if err != nil {
		t.Fatal(err)
	}
	prof, err := cfg.Active()
	if err != nil {
		t.Fatal(err)
	}
	if prof.Provider != ProviderLlamaServer {
		t.Fatalf("legacy provider = %q", prof.Provider)
	}
	if prof.ModelPath != legacyModel {
		t.Fatalf("legacy model path = %q, want %q", prof.ModelPath, legacyModel)
	}
	if prof.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("legacy base URL = %q", prof.BaseURL)
	}
}

func TestCheckAndWakeOpenAICompatibleProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHEW_HOME", root)
	brainDir := filepath.Join(root, "brain")
	cfg := DefaultProfileConfig(brainDir)
	cfg.ActiveProfile = "qwen"
	cfg.Profiles = append(cfg.Profiles, BrainProfile{
		Name:       "qwen",
		Provider:   ProviderOpenAICompatible,
		BaseURL:    "http://127.0.0.1:9911/v1",
		ModelAlias: "qwen3.5",
	})
	if err := SaveProfileConfig(brainDir, cfg); err != nil {
		t.Fatal(err)
	}

	state, gotDir := CheckBrain()
	if state != BrainNapping {
		t.Fatalf("state = %v, want BrainNapping", state)
	}
	if gotDir != brainDir {
		t.Fatalf("brainDir = %q, want %q", gotDir, brainDir)
	}

	brain, err := WakeBrain()
	if err != nil {
		t.Fatal(err)
	}
	if brain.Endpoint() != "http://127.0.0.1:9911" {
		t.Fatalf("endpoint = %q", brain.Endpoint())
	}
	if brain.Alias() != "qwen3.5" {
		t.Fatalf("alias = %q", brain.Alias())
	}
}

func TestBrainDirHonorsStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CHEW_STATE_HOME", stateHome)
	got, err := BrainDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(stateHome, "brain") {
		t.Fatalf("BrainDir = %q, want state-local brain dir", got)
	}
}
