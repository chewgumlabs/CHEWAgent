package wizard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ProfileConfigSchema      = "chew-profile-config.v0"
	DefaultProfileName       = "bonsai"
	ProviderLlamaServer      = "llama-server"
	ProviderOpenAICompatible = "openai-compatible"
	defaultBrainBaseURL      = "http://127.0.0.1:8080"
)

// BrainProfile is the portable description of a model that can inhabit CHEW.
// Public CHEWAgent ships with a managed local llama-server profile; internal
// CHEW can add OpenAI-compatible endpoints without changing the chat shell.
type BrainProfile struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Provider    string `json:"provider"`
	Source      string `json:"source,omitempty"`
	Managed     bool   `json:"managed"`
	BaseURL     string `json:"base_url,omitempty"`
	ModelAlias  string `json:"model_alias"`
	ModelPath   string `json:"model_path,omitempty"`
	Port        int    `json:"port,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// ProfileConfig is the on-disk model/profile contract.
type ProfileConfig struct {
	SchemaVersion string         `json:"schema_version"`
	ActiveProfile string         `json:"active_profile"`
	Profiles      []BrainProfile `json:"profiles"`
}

type legacyProfileConfig struct {
	ModelEndpoint string `json:"model_endpoint"`
	ModelAlias    string `json:"model_alias"`
	ModelPath     string `json:"model_path"`
}

func ProfileConfigPath(brainDir string) string {
	return filepath.Join(brainDir, "config.json")
}

func DefaultProfileConfig(brainDir string) ProfileConfig {
	return ProfileConfig{
		SchemaVersion: ProfileConfigSchema,
		ActiveProfile: DefaultProfileName,
		Profiles:      []BrainProfile{DefaultBonsaiProfile(brainDir)},
	}
}

func DefaultBonsaiProfile(brainDir string) BrainProfile {
	return BrainProfile{
		Name:        DefaultProfileName,
		DisplayName: "Bonsai",
		Provider:    ProviderLlamaServer,
		Source:      "chew.agent",
		Managed:     true,
		BaseURL:     defaultBrainBaseURL,
		ModelAlias:  "ChewBrain",
		ModelPath:   filepath.Join(brainDir, installBrainModelFile),
		Port:        8080,
		Notes:       "Default local CHEWAgent profile installed by 'install brain'.",
	}
}

func LoadProfileConfig(brainDir string) (ProfileConfig, error) {
	path := ProfileConfigPath(brainDir)
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultProfileConfig(brainDir), nil
	}
	if err != nil {
		return ProfileConfig{}, err
	}
	var cfg ProfileConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return ProfileConfig{}, err
	}
	if len(cfg.Profiles) == 0 && cfg.SchemaVersion == "" {
		var legacy legacyProfileConfig
		if err := json.Unmarshal(body, &legacy); err != nil {
			return ProfileConfig{}, err
		}
		cfg = profileConfigFromLegacy(brainDir, legacy)
	}
	return normalizeProfileConfig(brainDir, cfg), nil
}

func SaveProfileConfig(brainDir string, cfg ProfileConfig) error {
	cfg = normalizeProfileConfig(brainDir, cfg)
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(ProfileConfigPath(brainDir), body, 0o644)
}

func SetActiveProfile(brainDir, name string) (ProfileConfig, BrainProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ProfileConfig{}, BrainProfile{}, errors.New("profile name is required")
	}
	cfg, err := LoadProfileConfig(brainDir)
	if err != nil {
		return ProfileConfig{}, BrainProfile{}, err
	}
	prof, ok := cfg.Profile(name)
	if !ok {
		return ProfileConfig{}, BrainProfile{}, fmt.Errorf("unknown profile %q", name)
	}
	cfg.ActiveProfile = prof.Name
	if err := SaveProfileConfig(brainDir, cfg); err != nil {
		return ProfileConfig{}, BrainProfile{}, err
	}
	return cfg, prof, nil
}

func (c ProfileConfig) Active() (BrainProfile, error) {
	name := strings.TrimSpace(c.ActiveProfile)
	if name == "" {
		name = DefaultProfileName
	}
	if prof, ok := c.Profile(name); ok {
		return prof, nil
	}
	return BrainProfile{}, fmt.Errorf("active profile %q not found", name)
}

func (c ProfileConfig) Profile(name string) (BrainProfile, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, prof := range c.Profiles {
		if strings.ToLower(strings.TrimSpace(prof.Name)) == want {
			return normalizeProfile(prof), true
		}
	}
	return BrainProfile{}, false
}

func FormatProfileList(cfg ProfileConfig) string {
	cfg = normalizeProfileConfig("", cfg)
	profiles := append([]BrainProfile(nil), cfg.Profiles...)
	sort.SliceStable(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	var b strings.Builder
	b.WriteString("Brain profiles:\n")
	for _, prof := range profiles {
		marker := " "
		if strings.EqualFold(prof.Name, cfg.ActiveProfile) {
			marker = "*"
		}
		fmt.Fprintf(&b, "%s %s  %s  %s\n", marker, prof.Name, prof.Provider, profileLocation(prof))
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatProfileStatus(cfg ProfileConfig) string {
	prof, err := cfg.Active()
	if err != nil {
		return "Brain profile: " + err.Error()
	}
	lines := []string{
		fmt.Sprintf("Active brain profile: %s", prof.Name),
		fmt.Sprintf("provider: %s", prof.Provider),
		fmt.Sprintf("model: %s", prof.ModelAlias),
	}
	if prof.Managed {
		lines = append(lines, "runtime: managed by CHEW")
	}
	if prof.BaseURL != "" {
		lines = append(lines, "endpoint: "+chatCompletionsURL(prof.BaseURL))
	}
	if prof.Source != "" {
		lines = append(lines, "source: "+prof.Source)
	}
	return strings.Join(lines, "\n")
}

func profileConfigFromLegacy(brainDir string, legacy legacyProfileConfig) ProfileConfig {
	prof := DefaultBonsaiProfile(brainDir)
	if strings.TrimSpace(legacy.ModelPath) != "" {
		prof.ModelPath = strings.TrimSpace(legacy.ModelPath)
	}
	if strings.TrimSpace(legacy.ModelAlias) != "" {
		prof.ModelAlias = strings.TrimSpace(legacy.ModelAlias)
	}
	if strings.TrimSpace(legacy.ModelEndpoint) != "" {
		prof.BaseURL = baseURLFromChatEndpoint(legacy.ModelEndpoint)
	}
	return ProfileConfig{
		SchemaVersion: ProfileConfigSchema,
		ActiveProfile: prof.Name,
		Profiles:      []BrainProfile{prof},
	}
}

func normalizeProfileConfig(brainDir string, cfg ProfileConfig) ProfileConfig {
	cfg.SchemaVersion = ProfileConfigSchema
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = []BrainProfile{DefaultBonsaiProfile(brainDir)}
	}
	for i := range cfg.Profiles {
		cfg.Profiles[i] = normalizeProfile(cfg.Profiles[i])
		if cfg.Profiles[i].Name == "" && i == 0 {
			cfg.Profiles[i].Name = DefaultProfileName
		}
		if cfg.Profiles[i].Name == DefaultProfileName && cfg.Profiles[i].ModelPath == "" && brainDir != "" {
			cfg.Profiles[i].ModelPath = filepath.Join(brainDir, installBrainModelFile)
		}
	}
	if strings.TrimSpace(cfg.ActiveProfile) == "" {
		cfg.ActiveProfile = cfg.Profiles[0].Name
	}
	if _, ok := cfg.Profile(cfg.ActiveProfile); !ok {
		cfg.ActiveProfile = cfg.Profiles[0].Name
	}
	return cfg
}

func normalizeProfile(prof BrainProfile) BrainProfile {
	prof.Name = strings.TrimSpace(prof.Name)
	prof.DisplayName = strings.TrimSpace(prof.DisplayName)
	prof.Provider = strings.TrimSpace(prof.Provider)
	if prof.Provider == "" {
		prof.Provider = ProviderLlamaServer
	}
	prof.Source = strings.TrimSpace(prof.Source)
	prof.BaseURL = baseURLFromChatEndpoint(prof.BaseURL)
	prof.ModelAlias = strings.TrimSpace(prof.ModelAlias)
	if prof.ModelAlias == "" {
		prof.ModelAlias = "ChewBrain"
	}
	prof.ModelPath = strings.TrimSpace(prof.ModelPath)
	prof.Notes = strings.TrimSpace(prof.Notes)
	if prof.Port == 0 && prof.Provider == ProviderLlamaServer {
		prof.Port = 8080
	}
	return prof
}

func profileLocation(prof BrainProfile) string {
	switch prof.Provider {
	case ProviderLlamaServer:
		if prof.Managed {
			return "managed local runtime"
		}
		return "local runtime"
	case ProviderOpenAICompatible:
		return chatCompletionsURL(prof.BaseURL)
	default:
		if prof.BaseURL != "" {
			return chatCompletionsURL(prof.BaseURL)
		}
		return "configured"
	}
}

func baseURLFromChatEndpoint(endpoint string) string {
	s := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	s = strings.TrimSuffix(s, "/chat/completions")
	s = strings.TrimSuffix(s, "/v1")
	if s == "" {
		return ""
	}
	return s
}

func chatCompletionsURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	return base + "/v1/chat/completions"
}
