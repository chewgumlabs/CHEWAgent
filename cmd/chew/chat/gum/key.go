package gum

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	KeySchema = "chew-gum-key.v0"
	KeyEnv    = "CHEW_GUM_KEY"
)

type Key struct {
	SchemaVersion    string   `json:"schema_version"`
	Name             string   `json:"name"`
	DisplayName      string   `json:"display_name,omitempty"`
	Source           string   `json:"source,omitempty"`
	Summary          string   `json:"summary,omitempty"`
	Instructions     string   `json:"instructions,omitempty"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	StatusCommand    []string `json:"status_command,omitempty"`
	StatusTimeoutMS  int      `json:"status_timeout_ms,omitempty"`
}

func LoadKeyFromEnv() (Key, bool, error) {
	path := strings.TrimSpace(os.Getenv(KeyEnv))
	if path == "" {
		return Key{}, false, nil
	}
	key, err := LoadKey(path)
	if err != nil {
		return Key{}, false, err
	}
	return key, true, nil
}

func LoadKey(path string) (Key, error) {
	path = strings.TrimSpace(os.ExpandEnv(path))
	if path == "" {
		return Key{}, fmt.Errorf("Gum key path is empty")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return Key{}, err
		}
		path = abs
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Key{}, fmt.Errorf("read Gum key %s: %w", path, err)
	}
	var key Key
	if err := json.Unmarshal(body, &key); err != nil {
		return Key{}, fmt.Errorf("parse Gum key %s: %w", path, err)
	}
	key = NormalizeKey(key)
	if key.SchemaVersion != KeySchema {
		return Key{}, fmt.Errorf("Gum key schema %q is not %q", key.SchemaVersion, KeySchema)
	}
	if key.Name == "" {
		return Key{}, fmt.Errorf("Gum key name is required")
	}
	return key, nil
}

func NormalizeKey(key Key) Key {
	key.SchemaVersion = strings.TrimSpace(key.SchemaVersion)
	key.Name = strings.TrimSpace(key.Name)
	key.DisplayName = strings.TrimSpace(key.DisplayName)
	key.Source = strings.TrimSpace(key.Source)
	key.Summary = strings.TrimSpace(key.Summary)
	key.Instructions = strings.TrimSpace(key.Instructions)
	key.WorkingDirectory = strings.TrimSpace(key.WorkingDirectory)
	if key.StatusTimeoutMS <= 0 {
		key.StatusTimeoutMS = 45000
	}
	clean := make([]string, 0, len(key.StatusCommand))
	for _, part := range key.StatusCommand {
		part = strings.TrimSpace(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	key.StatusCommand = clean
	return key
}

func (k Key) Label() string {
	if strings.TrimSpace(k.DisplayName) != "" {
		return strings.TrimSpace(k.DisplayName)
	}
	if strings.TrimSpace(k.Name) != "" {
		return strings.TrimSpace(k.Name)
	}
	return "public"
}
