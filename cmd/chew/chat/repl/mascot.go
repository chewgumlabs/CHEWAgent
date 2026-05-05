// mascot.go — mascot state + terminal rendering for the REPL.
//
// CHEW has three states tied to actual system activity:
//   idle  — waiting for input (frames 0–2)
//   walk  — verb running / brain thinking (frames 3–5)
//   ghost — error / brainless / unreachable (frames 6–7)
//
// The fixed top dock renders these states in place for interactive
// terminals; non-TTY runs suppress mascot output entirely.

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

// nextFrameIdx returns the frame index to render given the current state.
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
