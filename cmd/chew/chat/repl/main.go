// chew chat REPL — public-facing chat shell.
//
// Stdlib-only. Reads stdin, plans via ScriptedPlanner (when no LLM
// configured) or LLMPlanner (when one is reachable — TODO), renders
// CHEW the mascot in the corner, dispatches verbs.
//
// State:
//   - Plan(input) returns Plan{Response, Verbs, Halt, Mascot}
//   - REPL prints Response, dispatches Verbs (if any), updates mascot tick
//   - Mascot animation goroutine ticks frame index based on current state
//
// This v0 shell does NOT yet wire in the verb registry — verbs in a Plan
// are listed but not executed. The next iteration adds verb dispatch.
//
// Run with:
//   go run ./cmd/chew/chat/repl
//
// Zero deps. Pure stdlib.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	chewsprite "github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/sprite"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/tool"
)

const (
	mascotIdleInterval  = 600 * time.Millisecond
	mascotWalkInterval  = 200 * time.Millisecond
	mascotGhostInterval = 400 * time.Millisecond
)

func main() {
	printIntro()

	p := planner.NewScriptedPlanner()
	tools := tool.NewDefault() // web_search + web_fetch wired today; more verbs as we add them
	state := newMascotState("idle")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		plan := p.Plan(input)

		// Update mascot state hint from the plan
		if plan.Mascot != "" {
			state.set(plan.Mascot)
		}

		// Render mascot once at the new state, before the response, so the
		// user sees CHEW reacting.
		renderMascot(state)

		if plan.Response != "" {
			fmt.Println()
			fmt.Println(plan.Response)
		}

		// Dispatch verbs through the tool registry. Tools that aren't
		// registered yet (read_file, run_command, etc.) fall through to
		// ErrUnknownTool and we list them as "planned but not wired."
		for _, v := range plan.Verbs {
			res, err := tools.Dispatch(v.Name, v.Params)
			switch {
			case errors.Is(err, tool.ErrUnknownTool):
				fmt.Printf("\n(verb planned but not wired: %s %v)\n", v.Name, v.Params)
			case err != nil:
				fmt.Printf("\n(error running %s: %v)\n", v.Name, err)
				state.set("ghost")
			default:
				if res.Output != "" {
					fmt.Println()
					fmt.Println(res.Output)
				}
				if res.Mascot != "" {
					state.set(res.Mascot)
				}
			}
		}

		if plan.Halt {
			return
		}

		// Settle back to idle a moment after a walk action
		if state.current == "walk" {
			go func() {
				time.Sleep(800 * time.Millisecond)
				state.set("idle")
			}()
		}
	}
}

// mascotState is shared between the REPL and the (future) mascot animator
// goroutine. For v0 we render once per turn synchronously; future passes
// will tick a goroutine.
type mascotState struct {
	current   string // "idle" | "walk" | "ghost"
	frameIdx  int
	lastTick  time.Time
}

func newMascotState(initial string) *mascotState {
	return &mascotState{current: initial, lastTick: time.Now()}
}

func (s *mascotState) set(state string) {
	if state == s.current {
		return
	}
	s.current = state
	s.frameIdx = 0
	s.lastTick = time.Now()
}

// nextFrameIdx returns the frame index to render given the current animation
// state and a tick. Synchronous version for v0.
func (s *mascotState) nextFrameIdx() int {
	switch s.current {
	case "walk":
		// frames 3, 4, 5
		return 3 + (s.frameIdx % 3)
	case "ghost":
		// frames 6, 7
		return 6 + (s.frameIdx % 2)
	default:
		// idle: frames 0, 1, 2
		return 0 + (s.frameIdx % 3)
	}
}

// advance ticks the frame index forward. Called once per render in v0.
func (s *mascotState) advance() {
	s.frameIdx++
	s.lastTick = time.Now()
}

// renderMascot prints the current frame inline. Future iterations will
// position him in a corner via ANSI cursor save/restore.
func renderMascot(s *mascotState) {
	fmt.Println()
	idx := s.nextFrameIdx()
	transparentBg := [3]uint8{30, 30, 30}
	out := chewsprite.RenderFullCellByIndex(idx, chewsprite.RenderOptions{TransparentBg: &transparentBg})
	fmt.Print(out)
	s.advance()
}

func printIntro() {
	fmt.Println("┌──────────────────────────────────────────────────────┐")
	fmt.Println("│ CHEW chat — brainless mode                           │")
	fmt.Println("│ I can do basics (read/edit/find/run/git read-only).  │")
	fmt.Println("│ Type 'help' for commands or 'install brain' to       │")
	fmt.Println("│ upgrade me.                                          │")
	fmt.Println("└──────────────────────────────────────────────────────┘")
}
