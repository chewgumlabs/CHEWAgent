// chew chat REPL — public-facing chat shell.
//
// Stdlib-only at the application layer. Reads stdin, plans via
// ScriptedPlanner (deterministic regex vocabulary), dispatches verbs
// through the tool registry, and renders CHEW the mascot inline.
//
// Brain lifecycle:
//   - On startup we CHECK whether Bonsai is installed (cheap stat) but
//     do NOT load it. The user types `wake up` to spawn llama-server
//     when they want thinking-mode; `nap` to stop it and free RAM.
//   - The brain dies with the REPL: SIGINT/SIGTERM/SIGHUP handlers stop
//     it cleanly, and llama-server inherits the REPL's process group so
//     a closed terminal kills it for free. A pidfile lets the next CHEW
//     run clean up an orphan from a hard-killed previous session.
//
// Run with:
//   go run ./cmd/chew/chat/repl

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/tool"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
)

func main() {
	p := planner.NewScriptedPlanner()
	tools := tool.NewDefault()
	state := newMascotState("idle")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024) // big buffer for pasted input

	// brain is held in an atomic so the signal handler can read it
	// without racing the main goroutine that mutates it on wake/nap.
	var brain atomic.Pointer[wizard.Brain]
	stopBrain := func() {
		if b := brain.Swap(nil); b != nil {
			_ = b.Stop()
		}
	}
	defer stopBrain()

	// Cleanup signal handler: SIGINT (Ctrl+C), SIGTERM, SIGHUP (terminal
	// closed). All three skip Go's deferred funcs by default, so we wire
	// our own handler that stops the brain before exiting.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigCh
		stopBrain()
		fmt.Println("\n*splash*")
		os.Exit(0)
	}()

	// Startup: clean up any orphan from a previous run, then check what's
	// on disk. We do NOT auto-load — the user controls when Bonsai gets
	// loaded into RAM by typing `wake up`.
	brainState, brainDir := wizard.CheckBrain()
	if brainDir != "" {
		_ = wizard.KillStaleBrain(brainDir)
	}
	switch brainState {
	case wizard.BrainNapping:
		fmt.Println("Brain installed but napping. Type 'wake up' to load it.")
	case wizard.BrainNotInstalled:
		fmt.Println("No brain installed yet. Type 'install brain' to set one up.")
	}
	printBrainlessIntro()

	// Project state — the active folder CHEW is working out of. Resumed
	// from <brainDir>/last-project.txt if there is one.
	var proj atomic.Pointer[project.Project]
	if last := project.LoadLast(brainDir); last != "" {
		if pj, err := project.Open(last); err == nil {
			proj.Store(pj)
			fmt.Println()
			fmt.Println(fmt.Sprintf(p.PickVoice("project_resumed"), pj.Name))
			if !pj.GUM.IsEmpty() {
				fmt.Println()
				fmt.Println(fmt.Sprintf(p.PickVoice("project_summary"), pj.GUM.Summary()))
			}
		}
	}

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
		renderMascot(state)

		if plan.Response != "" {
			fmt.Println()
			fmt.Println(plan.Response)
		}

		// Dispatch verbs through the tool registry.
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

		// Wizard / system action handoff.
		if plan.LaunchWizard != "" {
			switch plan.LaunchWizard {
			case "install_brain":
				newBrain := runInstallBrainWizard(scanner)
				if newBrain != nil {
					stopBrain() // drop any prior brain (shouldn't happen but safe)
					brain.Store(newBrain)
					p.SetFallback(brainFallback(newBrain))
				}
			case "wake_brain":
				handleWakeBrain(p, &brain)
			case "nap_brain":
				handleNapBrain(p, &brain)
			case "open_project":
				handleOpenProject(p, &proj, brainDir, plan.LaunchArgs["path"])
			case "create_project":
				handleCreateProject(p, plan.LaunchArgs["name"])
			case "forget_project":
				handleForgetProject(p, &proj, brainDir)
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

// handleWakeBrain spawns the brain on demand. Idempotent: a no-op if the
// brain is already loaded.
func handleWakeBrain(p *planner.ScriptedPlanner, brain *atomic.Pointer[wizard.Brain]) {
	if brain.Load() != nil {
		fmt.Println()
		fmt.Println(p.PickVoice("brain_already_awake"))
		return
	}
	state, _ := wizard.CheckBrain()
	if state != wizard.BrainNapping {
		fmt.Println()
		fmt.Println(p.PickVoice("brain_not_installed"))
		return
	}
	fmt.Println()
	fmt.Println(p.PickVoice("brain_waking"))
	newBrain, err := wizard.WakeBrain()
	if err != nil {
		fmt.Println()
		fmt.Printf(p.PickVoice("brain_wake_failed")+"\n", summariseSpawnErr(err))
		return
	}
	brain.Store(newBrain)
	p.SetFallback(brainFallback(newBrain))
	fmt.Println()
	fmt.Println(p.PickVoice("brain_awake"))
}

// handleNapBrain stops the running brain and restores the brainless
// fallback. Idempotent.
func handleNapBrain(p *planner.ScriptedPlanner, brain *atomic.Pointer[wizard.Brain]) {
	b := brain.Swap(nil)
	if b == nil {
		fmt.Println()
		fmt.Println(p.PickVoice("brain_already_napping"))
		return
	}
	_ = b.Stop()
	p.SetFallback(nil) // restore default brainless fallback
	fmt.Println()
	fmt.Println(p.PickVoice("brain_napping"))
}

// handleOpenProject sets up shop in the given folder. The folder must
// exist; if there's no GUM.md, we write a starter so CHEW has something
// to read next time.
func handleOpenProject(p *planner.ScriptedPlanner, proj *atomic.Pointer[project.Project], brainDir, path string) {
	if path == "" {
		fmt.Println()
		fmt.Println(fmt.Sprintf(p.PickVoice("project_failed"), "no path given"))
		return
	}
	pj, err := project.Open(path)
	if err != nil {
		// If the path is a file (not a folder), give a specific hint.
		info, statErr := os.Stat(strings.Trim(path, `'"`))
		if statErr == nil && !info.IsDir() {
			fmt.Println()
			fmt.Println(fmt.Sprintf(p.PickVoice("project_path_is_file"), filepath.Base(path)))
			return
		}
		fmt.Println()
		fmt.Println(fmt.Sprintf(p.PickVoice("project_failed"), err.Error()))
		return
	}
	gumStatus := ""
	if !pj.HasGUM() {
		// Seed a starter GUM.md so the next session has something to read.
		starter := project.NewStarterGUM(pj.Name)
		if err := project.WriteGUM(pj.GUMPath(), starter); err == nil {
			pj.GUM = starter
		}
		gumStatus = p.PickVoice("project_no_gum")
	} else {
		gumStatus = "Found GUM.md — I'll catch up on what's true."
	}
	proj.Store(pj)
	_ = project.SaveLast(brainDir, pj.Path)

	fmt.Println()
	fmt.Println(fmt.Sprintf(p.PickVoice("project_opened"), pj.Name, gumStatus))
	if !pj.GUM.IsEmpty() && pj.HasGUM() {
		// If the user opened a folder with existing notes, show the summary.
		summary := pj.GUM.Summary()
		if summary != "" && len(summary) < 1500 {
			fmt.Println()
			fmt.Println(fmt.Sprintf(p.PickVoice("project_summary"), summary))
		}
	}
}

// handleCreateProject creates a fresh folder under ~/Documents/ with a
// starter GUM.md inside.
func handleCreateProject(p *planner.ScriptedPlanner, name string) {
	if name == "" {
		fmt.Println()
		fmt.Println(fmt.Sprintf(p.PickVoice("project_failed"), "give the folder a name: 'make folder <name>'"))
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Println()
		fmt.Println(fmt.Sprintf(p.PickVoice("project_failed"), "couldn't find your home directory"))
		return
	}
	target := filepath.Join(home, "Documents", strings.TrimSpace(name))
	pj, err := project.Create(target)
	if err != nil {
		fmt.Println()
		fmt.Println(fmt.Sprintf(p.PickVoice("project_failed"), err.Error()))
		return
	}
	fmt.Println()
	fmt.Println(fmt.Sprintf(p.PickVoice("project_created"), pj.Path, pj.Path))
}

// handleForgetProject clears the active project and the saved last-project.
func handleForgetProject(p *planner.ScriptedPlanner, proj *atomic.Pointer[project.Project], brainDir string) {
	proj.Store(nil)
	_ = project.ClearLast(brainDir)
	fmt.Println()
	fmt.Println(p.PickVoice("project_forgotten"))
}

// summariseSpawnErr converts a wake error into a short friendly phrase.
func summariseSpawnErr(err error) string {
	if err == nil {
		return "unknown"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "no brain installed"):
		return "no brain installed yet"
	case strings.Contains(s, "address already in use"):
		return "port 8080 is busy"
	case strings.Contains(s, "permission denied"):
		return "permission denied"
	case strings.Contains(s, "deadline"), strings.Contains(s, "timeout"):
		return "didn't respond in time"
	default:
		return "something went wrong"
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
		if done {
			break
		}
	}
	return w.RunningBrain()
}

func printBrainlessIntro() {
	fmt.Println("┌──────────────────────────────────────────────────────┐")
	fmt.Println("│ CHEW chat                                            │")
	fmt.Println("│ Commands: read, ls, write, find, run, git, web,      │")
	fmt.Println("│ fetch, install brain, wake up, nap, help, quit.      │")
	fmt.Println("└──────────────────────────────────────────────────────┘")
}
