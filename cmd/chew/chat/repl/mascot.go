// mascot.go — mascot state + terminal rendering for the REPL.
//
// CHEW has three states tied to actual system activity:
//   idle  — waiting for input (CHEW idle frames)
//   walk  — verb running / brain thinking (CHEW walk frames)
//   ghost — error / brainless / unreachable (CHEW ghost frames)
//
// The TUI can briefly render Gum over the same state machine when the
// records/planning layer is active. Gum maps the same state names onto her
// six-frame sheet:
//   idle  — down frames
//   walk  — left frames
//   ghost — up frames
//
// The REPL tracks mascot state separately from the output surface. The
// TUI renderer consumes it for the fixed header; plain text mode keeps
// output script-friendly and does not draw the sprite.

package main

import "time"

// mascotState tracks current animation state + frame offset.
type mascotState struct {
	current  string // "idle" | "walk" | "ghost"
	frameIdx int
	lastTick time.Time
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

// nextChewFrameIdx returns the CHEW frame index to render for the current state.
func (s *mascotState) nextChewFrameIdx() int {
	switch s.current {
	case "walk":
		// walk_0, walk_1, walk_2
		return 3 + (s.frameIdx % 3)
	case "ghost":
		// ghost_0, ghost_1
		return 6 + (s.frameIdx % 2)
	default:
		// idle_0, idle_1, idle_2
		return s.frameIdx % 3
	}
}

// nextGumFrameIdx returns the Gum frame index to render for the current state.
func (s *mascotState) nextGumFrameIdx() int {
	switch s.current {
	case "walk":
		// left_0, left_1
		return 2 + (s.frameIdx % 2)
	case "ghost":
		// up_0, up_1
		return 4 + (s.frameIdx % 2)
	default:
		// down_0, down_1
		return s.frameIdx % 2
	}
}

// advance ticks the frame index forward.
func (s *mascotState) advance() {
	s.frameIdx++
	s.lastTick = time.Now()
}
