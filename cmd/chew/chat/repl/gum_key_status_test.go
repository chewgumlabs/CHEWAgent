package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/gum"
)

func TestRunGumKeyStatusCommand(t *testing.T) {
	t.Setenv("CHEW_GUM_KEY_HELPER", "1")
	key := gum.NormalizeKey(gum.Key{
		SchemaVersion: gum.KeySchema,
		Name:          "test",
		StatusCommand: []string{os.Args[0], "-test.run=TestGumKeyStatusHelper"},
	})

	out, err := runGumKeyStatusCommand(key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "gum says ok") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestGumKeyStatusHelper(t *testing.T) {
	if os.Getenv("CHEW_GUM_KEY_HELPER") != "1" {
		return
	}
	fmt.Print("gum says ok")
	os.Exit(0)
}
