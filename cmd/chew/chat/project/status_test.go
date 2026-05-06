package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusSnapshotAndEventsPersistUnderGumDir(t *testing.T) {
	tmp := t.TempDir()
	pj, err := Create(filepath.Join(tmp, "site"))
	if err != nil {
		t.Fatal(err)
	}

	if err := SaveStatusSnapshot(pj, StatusSnapshot{
		Source:      "test",
		State:       "working",
		CurrentTask: "make a website",
		Checkpoint:  "brain call",
		LastFact:    "sent request",
	}); err != nil {
		t.Fatalf("SaveStatusSnapshot: %v", err)
	}
	if err := AppendStatusEvent(pj, StatusEvent{
		Source:     "test",
		Kind:       "task_started",
		Task:       "make a website",
		Checkpoint: "brain call",
		Fact:       "sent request",
	}); err != nil {
		t.Fatalf("AppendStatusEvent: %v", err)
	}

	status, err := os.ReadFile(pj.StatusPath())
	if err != nil {
		t.Fatalf("read status snapshot: %v", err)
	}
	for _, want := range []string{StatusSnapshotSchema, `"current_task": "make a website"`, `"project_name": "site"`} {
		if !strings.Contains(string(status), want) {
			t.Fatalf("status snapshot missing %q:\n%s", want, status)
		}
	}
	events, err := os.ReadFile(pj.StatusEventsPath())
	if err != nil {
		t.Fatalf("read status events: %v", err)
	}
	if !strings.Contains(string(events), StatusEventSchema) || !strings.Contains(string(events), "task_started") {
		t.Fatalf("status events missing expected event:\n%s", events)
	}
}
