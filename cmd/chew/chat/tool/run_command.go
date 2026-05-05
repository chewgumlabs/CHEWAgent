// run_command.go — execute a shell command.
//
// Caps:
//   - 30 second timeout (configurable)
//   - 8 KB combined stdout+stderr (configurable)
//
// The planner's regex constraints already gate which commands flow here:
//   - `git status|diff|log|...` (read-only git family)
//   - `run|exec|sh <cmd>` (user explicitly typed it)
//   - `force: <cmd>` (user opted past the safety on a mutating git verb)
//   - `pwd` (no args)
//
// All paths require the user to have *typed* the command, so consent is
// implicit. If we ever wire the LLM to *call* run_command on its own,
// we'll need a confirm-before-execute pattern at the REPL layer.

package tool

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// RunCommand executes a shell command with caps on time and output.
type RunCommand struct {
	MaxOutputBytes int
	Timeout        time.Duration
	Root           *string // if non-nil, set as working directory for commands
}

// Name implements Tool.
func (r *RunCommand) Name() string { return "run_command" }

// Execute implements Tool. Required param: "command" (string).
func (r *RunCommand) Execute(params map[string]any) (Result, error) {
	cmdStr := stringParam(params, "command")
	if cmdStr == "" {
		return Result{}, errors.New("run_command requires a 'command' param")
	}

	cap := r.MaxOutputBytes
	if cap == 0 {
		cap = 8 * 1024
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	if r.Root != nil && *r.Root != "" {
		cmd.Dir = *r.Root
	}

	out, runErr := cmd.CombinedOutput()
	truncated := false
	if len(out) > cap {
		out = out[:cap]
		truncated = true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "$ %s\n", cmdStr)
	b.Write(out)
	if !strings.HasSuffix(string(out), "\n") && len(out) > 0 {
		b.WriteByte('\n')
	}
	if truncated {
		fmt.Fprintf(&b, "[... output truncated at %d bytes ...]\n", cap)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fmt.Fprintf(&b, "[command timed out after %v]\n", timeout)
		return Result{Output: b.String(), Mascot: "ghost"}, nil
	}
	if runErr != nil {
		fmt.Fprintf(&b, "[exit: %v]\n", runErr)
		return Result{Output: b.String(), Mascot: "ghost"}, nil
	}
	return Result{Output: b.String(), Mascot: "idle"}, nil
}
