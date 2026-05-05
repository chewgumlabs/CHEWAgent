// chew chat REPL — public-facing chat shell.
//
// Stdlib-only at the application layer. Reads stdin, plans via
// ScriptedPlanner (deterministic regex vocabulary), dispatches verbs
// through the tool registry, and renders CHEW the mascot inline.
//
// After `install brain` succeeds, the REPL takes ownership of the
// running brain (a llama-server subprocess) and swaps the planner's
// fallback function so free-form questions hit the brain. The brain
// dies with the REPL on exit.
//
// Run with:
//   go run ./cmd/chew/chat/repl

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/tool"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
)

func main() {
	printIntro()

	p := planner.NewScriptedPlanner()
	tools := tool.NewDefault() // web_search + web_fetch wired today; more verbs as we add them
	state := newMascotState("idle")
	scanner := bufio.NewScanner(os.Stdin)
	// Bigger buffer than the default 64 KB so users can paste long inputs.
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	// brain is held here so we can Stop it on exit. Set after a successful
	// install_brain wizard run.
	var brain *wizard.Brain
	defer func() {
		if brain != nil {
			_ = brain.Stop()
		}
	}()

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

		// Wizard handoff. The planner emits LaunchWizard when the user
		// asks for a flow that's a state machine rather than a single
		// turn (e.g., 'install brain').
		if plan.LaunchWizard != "" {
			switch plan.LaunchWizard {
			case "install_brain":
				newBrain := runInstallBrainWizard(scanner)
				if newBrain != nil {
					if brain != nil {
						_ = brain.Stop() // shouldn't happen but safe
					}
					brain = newBrain
					// Swap the planner's fallback so free-form questions
					// reach the brain instead of the "I don't know that
					// one" message.
					p.SetFallback(brainFallback(brain))
				}
			default:
				fmt.Printf("\n(unknown wizard requested: %s)\n", plan.LaunchWizard)
			}
		}

		// Settle back to idle a moment after a walk action.
		if state.current == "walk" {
			go func() {
				time.Sleep(800 * time.Millisecond)
				state.set("idle")
			}()
		}
	}
}

// runInstallBrainWizard runs the brain-install state machine inline, reading
// user input from the supplied scanner so it shares stdin with the REPL.
// Returns the running *Brain on success, nil on cancel/failure.
func runInstallBrainWizard(scanner *bufio.Scanner) *wizard.Brain {
	w := wizard.NewInstallBrain()

	reply := func(s string) {
		fmt.Println()
		fmt.Println(s)
	}

	w.Begin(reply)

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			return nil
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		done, _ := w.Step(input, reply)
		// w already emitted any user-facing error message via reply.
		if done {
			break
		}
	}
	return w.RunningBrain()
}

func printIntro() {
	fmt.Println("┌──────────────────────────────────────────────────────┐")
	fmt.Println("│ CHEW chat — brainless mode                           │")
	fmt.Println("│ I do file ops, search, web search, fetch URLs, git   │")
	fmt.Println("│ read-only. Type 'help' for the menu, 'install brain' │")
	fmt.Println("│ to put a brain in me.                                │")
	fmt.Println("└──────────────────────────────────────────────────────┘")
}
