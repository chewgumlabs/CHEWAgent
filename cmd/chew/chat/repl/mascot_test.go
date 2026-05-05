package main

import (
	"testing"
	"time"
)

func TestMascotStateChewFrameMapping(t *testing.T) {
	tests := []struct {
		state string
		idx   int
		want  int
	}{
		{state: "idle", idx: 0, want: 0},
		{state: "idle", idx: 2, want: 2},
		{state: "idle", idx: 3, want: 0},
		{state: "walk", idx: 0, want: 3},
		{state: "walk", idx: 2, want: 5},
		{state: "walk", idx: 3, want: 3},
		{state: "ghost", idx: 0, want: 6},
		{state: "ghost", idx: 1, want: 7},
		{state: "ghost", idx: 2, want: 6},
	}

	for _, tt := range tests {
		s := newMascotState(tt.state)
		s.frameIdx = tt.idx
		if got := s.nextChewFrameIdx(); got != tt.want {
			t.Errorf("state=%s idx=%d: CHEW frame=%d, want %d", tt.state, tt.idx, got, tt.want)
		}
	}
}

func TestMascotStateGumFrameMapping(t *testing.T) {
	tests := []struct {
		state string
		idx   int
		want  int
	}{
		{state: "idle", idx: 0, want: 0},
		{state: "idle", idx: 1, want: 1},
		{state: "idle", idx: 2, want: 0},
		{state: "walk", idx: 0, want: 2},
		{state: "walk", idx: 1, want: 3},
		{state: "walk", idx: 2, want: 2},
		{state: "ghost", idx: 0, want: 4},
		{state: "ghost", idx: 1, want: 5},
		{state: "ghost", idx: 2, want: 4},
	}

	for _, tt := range tests {
		s := newMascotState(tt.state)
		s.frameIdx = tt.idx
		if got := s.nextGumFrameIdx(); got != tt.want {
			t.Errorf("state=%s idx=%d: Gum frame=%d, want %d", tt.state, tt.idx, got, tt.want)
		}
	}
}

func TestGumBlipExpires(t *testing.T) {
	app := newTUIApp()
	if app.gumBlipActive() {
		t.Fatal("new app should not start with Gum blip active")
	}

	app.blipGum()
	if !app.gumBlipActive() {
		t.Fatal("blipGum should activate the Gum overlay")
	}

	app.gumBlipUntil = time.Now().Add(-time.Millisecond)
	if !app.clearExpiredGumBlip() {
		t.Fatal("clearExpiredGumBlip should report an expired overlay")
	}
	if app.gumBlipActive() {
		t.Fatal("expired Gum overlay should no longer be active")
	}
}
