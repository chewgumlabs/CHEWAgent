// dock.go — fixed-position mascot dock for interactive terminals.
//
// In TTY mode, CHEW occupies a fixed top dock. Chat output scrolls below
// via an ANSI scroll region, and mascot state changes redraw the sprite
// in place instead of adding another avatar block to the transcript.
//
// In non-TTY mode (piped stdout), the dock is inactive and mascot
// rendering is suppressed so scripts and smoke tests stay clean.

package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	chewsprite "github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/sprite"
)

const (
	spriteRows = 16
	dockRows   = spriteRows + 1 // sprite plus separator
)

// mascotDock manages a fixed-position mascot at the top of the terminal.
type mascotDock struct {
	mu     sync.Mutex
	active bool
}

func newMascotDock() *mascotDock {
	d := &mascotDock{}
	if !stdoutIsTTY() {
		return d
	}
	d.active = true

	// Clear the terminal, reserve the dock, then put the cursor in the
	// scrollable chat area. DECSTBM with an omitted bottom row means
	// "through the terminal bottom" on ANSI-compatible terminals.
	fmt.Printf("\033[2J\033[H\033[%d;r\033[%d;1H", dockRows+1, dockRows+1)
	d.paintSeparator()
	fmt.Printf("\033[%d;1H", dockRows+1)
	return d
}

func stdoutIsTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// render redraws the current mascot frame in the dock. It is a no-op when
// stdout is not a TTY.
func (d *mascotDock) render(s *mascotState) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.active {
		return
	}
	frame := s.nextFrameIdx()
	s.advance()

	bg := [3]uint8{30, 30, 30}
	sprite := chewsprite.RenderFullCellByIndex(frame, chewsprite.RenderOptions{TransparentBg: &bg})

	// Save cursor, draw at the top-left dock origin, redraw separator, and
	// restore cursor to the chat area.
	fmt.Print("\0337")
	fmt.Print("\033[1;1H")
	fmt.Print(sprite)
	d.paintSeparator()
	fmt.Print("\0338")
}

// teardown restores the full terminal scroll region. Safe to call more
// than once, including from both signal cleanup and deferred cleanup.
func (d *mascotDock) teardown() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.active {
		return
	}
	d.active = false
	fmt.Print("\033[r\033[9999;1H\n")
}

func (d *mascotDock) paintSeparator() {
	fmt.Printf("\033[%d;1H\033[2m%s\033[0m", dockRows, strings.Repeat("-", 32))
}
