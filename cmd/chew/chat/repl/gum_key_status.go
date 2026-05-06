package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/gum"
)

func runGumKeyStatusCommand(key gum.Key) (string, error) {
	if len(key.StatusCommand) == 0 {
		return "", fmt.Errorf("Gum key %s has no status command", key.Label())
	}
	argv := make([]string, 0, len(key.StatusCommand))
	for _, part := range key.StatusCommand {
		part = strings.TrimSpace(os.ExpandEnv(part))
		if part != "" {
			argv = append(argv, part)
		}
	}
	if len(argv) == 0 {
		return "", fmt.Errorf("Gum key %s has an empty status command", key.Label())
	}
	timeout := time.Duration(key.StatusTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if key.WorkingDirectory != "" {
		cmd.Dir = os.ExpandEnv(key.WorkingDirectory)
	}
	cmd.Env = append(os.Environ(),
		"CHEW_GUM_KEY_NAME="+key.Name,
		"CHEW_GUM_KEY_DISPLAY="+key.Label(),
	)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	const max = 12000
	if len(text) > max {
		text = text[:max] + "\n..."
	}
	if ctx.Err() != nil {
		return text, fmt.Errorf("status command timed out")
	}
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%v: %s", err, text)
		}
		return "", err
	}
	return text, nil
}
