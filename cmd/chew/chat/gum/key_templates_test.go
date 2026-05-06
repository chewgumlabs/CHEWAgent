package gum

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPublicGumKeyTemplateLoads(t *testing.T) {
	root := repoRoot(t)
	key, err := LoadKey(filepath.Join(root, "gum-keys", "public.gum-key.json"))
	if err != nil {
		t.Fatal(err)
	}
	if key.Name != "chew-public" {
		t.Fatalf("unexpected public key name: %q", key.Name)
	}
	if key.Label() != "Public Gum" {
		t.Fatalf("unexpected public key label: %q", key.Label())
	}
	if len(key.StatusCommand) != 0 {
		t.Fatalf("public key should not depend on a private status command: %#v", key.StatusCommand)
	}
	for _, want := range []string{"Checkpoint:", "Next:", "Blocked:", "Never ask the user to open"} {
		if !strings.Contains(key.Instructions, want) {
			t.Fatalf("public key instructions missing %q:\n%s", want, key.Instructions)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("could not find repo root")
		}
		dir = next
	}
}
