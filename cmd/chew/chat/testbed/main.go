// chew sprite testbed — interactive REPL to view CHEW frames.
//
// Type a frame number (0..7), a label (idle_0, walk_2, ghost_1...), or
// "list" to enumerate, "quit" to exit. The frame renders inline with
// truecolor half-blocks. Animation loops are NOT here yet — they wait
// for the user's timing rules.
//
// Run with:
//   go run ./cmd/chew/chat/testbed
//
// Built deliberately as scaffolding for the eventual chat REPL: stdlib-only
// stdin reader, no external dependencies, ANSI cursor handling for clean
// re-renders.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	chewsprite "github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/sprite"
)

func main() {
	printBanner()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "quit" || input == "exit" || input == "q" {
			fmt.Println("bye 👻")
			return
		}
		if input == "list" || input == "ls" {
			printFrameList()
			continue
		}
		if input == "help" || input == "?" {
			printBanner()
			continue
		}
		if input == "all" {
			renderAllChew()
			continue
		}
		if input == "gum all" {
			renderAllGum()
			continue
		}
		// "gum <X>" routes to gum sprite
		if strings.HasPrefix(input, "gum ") {
			arg := strings.TrimSpace(strings.TrimPrefix(input, "gum"))
			frame := resolveGumFrame(arg)
			if frame < 0 {
				fmt.Printf("unknown gum frame: %q\n", arg)
				continue
			}
			renderGumFrame(frame)
			continue
		}

		frame := resolveChewFrame(input)
		if frame < 0 {
			fmt.Printf("unknown frame: %q (try 'list' or 'help')\n", input)
			continue
		}
		renderChewFrame(frame)
	}
}

func printBanner() {
	fmt.Println("CHEW + GUM sprite testbed")
	fmt.Println("-------------------------")
	fmt.Println("commands:")
	fmt.Println("  0..7         render CHEW frame by index")
	fmt.Println("  <label>      render CHEW by name (idle_0..walk_2..ghost_1)")
	fmt.Println("  gum 0..5     render GUM frame by index")
	fmt.Println("  gum <label>  render GUM by name (down_0..left_1..up_1)")
	fmt.Println("  list         show all frame indices + labels for both sprites")
	fmt.Println("  all          render all CHEW frames")
	fmt.Println("  gum all      render all GUM frames")
	fmt.Println("  help         this banner")
	fmt.Println("  quit         exit")
	fmt.Println()
}

func printFrameList() {
	fmt.Println("CHEW")
	fmt.Println("frame  label")
	fmt.Println("-----  ------")
	for i, label := range chewsprite.FrameLabels {
		fmt.Printf("  %d    %s\n", i, label)
	}
	fmt.Println()
	fmt.Println("GUM (prefix with 'gum ')")
	fmt.Println("frame  label")
	fmt.Println("-----  ------")
	for i, label := range chewsprite.GumFrameLabels {
		fmt.Printf("  %d    %s\n", i, label)
	}
	fmt.Println()
}

func resolveChewFrame(input string) int {
	if n, err := strconv.Atoi(input); err == nil {
		if n >= 0 && n < chewsprite.NumFrames {
			return n
		}
		return -1
	}
	return chewsprite.FrameByLabel(input)
}

func resolveGumFrame(input string) int {
	if n, err := strconv.Atoi(input); err == nil {
		if n >= 0 && n < chewsprite.GumNumFrames {
			return n
		}
		return -1
	}
	return chewsprite.GumFrameByLabel(input)
}

// transparentBg makes transparent pixels render as a dim gray so the sprite
// silhouette is visible against any terminal theme. Set to nil to use the
// terminal's default bg instead.
var transparentBg = [3]uint8{30, 30, 30}

func renderChewFrame(frame int) {
	fmt.Println()
	out := chewsprite.RenderFullCellByIndex(frame, chewsprite.RenderOptions{TransparentBg: &transparentBg})
	fmt.Print(out)
	fmt.Printf("CHEW frame %d  (%s)\n\n", frame, chewsprite.FrameLabels[frame])
}

func renderGumFrame(frame int) {
	fmt.Println()
	out := chewsprite.RenderGumFullCellByIndex(frame, chewsprite.RenderOptions{TransparentBg: &transparentBg})
	fmt.Print(out)
	fmt.Printf("GUM frame %d  (%s)\n\n", frame, chewsprite.GumFrameLabels[frame])
}

func renderAllChew() {
	fmt.Println()
	for i := 0; i < chewsprite.NumFrames; i++ {
		fmt.Printf("--- CHEW %d (%s) ---\n", i, chewsprite.FrameLabels[i])
		out := chewsprite.RenderFullCellByIndex(i, chewsprite.RenderOptions{TransparentBg: &transparentBg})
		fmt.Print(out)
	}
	fmt.Println()
}

func renderAllGum() {
	fmt.Println()
	for i := 0; i < chewsprite.GumNumFrames; i++ {
		fmt.Printf("--- GUM %d (%s) ---\n", i, chewsprite.GumFrameLabels[i])
		out := chewsprite.RenderGumFullCellByIndex(i, chewsprite.RenderOptions{TransparentBg: &transparentBg})
		fmt.Print(out)
	}
	fmt.Println()
}
