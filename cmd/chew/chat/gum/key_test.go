package gum

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadKeyFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "internal.gum-key.json")
	body := `{
  "schema_version": "chew-gum-key.v0",
  "name": "internal",
  "display_name": "Internal Gum",
  "instructions": "Use the private workflow cards.",
  "status_command": ["echo", "ok"]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(KeyEnv, path)

	key, ok, err := LoadKeyFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected key to load")
	}
	if key.Name != "internal" || key.Label() != "Internal Gum" {
		t.Fatalf("unexpected key: %+v", key)
	}
	if key.StatusTimeoutMS == 0 {
		t.Fatal("expected default timeout")
	}
}

func TestLoadKeyRejectsWrongSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"x","name":"bad"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadKey(path)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("expected schema error, got %v", err)
	}
}
