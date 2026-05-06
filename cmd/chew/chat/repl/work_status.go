package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
)

const statusSource = "chewagent-tui"

type workStatus struct {
	state      string
	task       string
	checkpoint string
	lastFact   string
	startedAt  time.Time
	finishedAt time.Time
	updatedAt  time.Time
	brainAlias string
}

func newWorkStatus() *workStatus {
	return &workStatus{
		state:      "idle",
		checkpoint: "waiting",
		lastFact:   "No active work yet.",
		updatedAt:  time.Now(),
	}
}

func (s *workStatus) Start(task, checkpoint string, pj *project.Project, brain *wizard.Brain) {
	now := time.Now()
	s.state = "working"
	s.task = cleanStatusText(task)
	s.checkpoint = cleanStatusText(checkpoint)
	if s.checkpoint == "" {
		s.checkpoint = "working"
	}
	s.lastFact = "Started."
	s.startedAt = now
	s.finishedAt = time.Time{}
	s.updatedAt = now
	s.brainAlias = ""
	if brain != nil {
		s.brainAlias = brain.Alias()
	}
	s.persist(pj, "task_started")
}

func (s *workStatus) Fact(fact string, pj *project.Project) {
	fact = cleanStatusText(fact)
	if fact == "" {
		return
	}
	s.lastFact = fact
	s.updatedAt = time.Now()
	s.persist(pj, "task_fact")
}

func (s *workStatus) Finish(fact string, pj *project.Project) {
	fact = cleanStatusText(fact)
	if fact == "" {
		fact = "Finished."
	}
	now := time.Now()
	s.state = "done"
	s.lastFact = fact
	s.checkpoint = "finished"
	s.finishedAt = now
	s.updatedAt = now
	s.persist(pj, "task_finished")
}

func (s *workStatus) Fail(fact string, pj *project.Project) {
	fact = cleanStatusText(fact)
	if fact == "" {
		fact = "Stopped with an error."
	}
	now := time.Now()
	s.state = "error"
	s.lastFact = fact
	s.checkpoint = "error"
	s.finishedAt = now
	s.updatedAt = now
	s.persist(pj, "task_failed")
}

func (s *workStatus) Answer(pj *project.Project, brainAwake bool) string {
	projectName := "no active project"
	if pj != nil {
		projectName = pj.Name
	}
	switch s.state {
	case "working":
		return strings.Join([]string{
			"I'm working on: " + defaultStatusText(s.task, "the current request"),
			"Checkpoint: " + defaultStatusText(s.checkpoint, "working"),
			"Project: " + projectName,
			"Latest Gum fact: " + defaultStatusText(s.lastFact, "started"),
			"Elapsed: " + statusElapsed(time.Since(s.startedAt)),
			"That answer came from Gum status, so I didn't interrupt the brain.",
		}, "\n")
	case "done", "error":
		return strings.Join([]string{
			"No active long task right now.",
			"Last checkpoint: " + defaultStatusText(s.checkpoint, s.state),
			"Last task: " + defaultStatusText(s.task, "none yet"),
			"Latest Gum fact: " + defaultStatusText(s.lastFact, "nothing recorded yet"),
			"Project: " + projectName,
		}, "\n")
	default:
		brain := "napping"
		if brainAwake {
			brain = "awake"
		}
		return strings.Join([]string{
			"No active long task right now.",
			"Brain: " + brain,
			"Project: " + projectName,
			"Latest Gum fact: " + defaultStatusText(s.lastFact, "nothing recorded yet"),
		}, "\n")
	}
}

func (s *workStatus) SideLines() []string {
	if s.state == "working" {
		return []string{
			"work: thinking",
			"now: " + defaultStatusText(s.task, "current request"),
			"fact: " + defaultStatusText(s.lastFact, "started"),
		}
	}
	if s.state == "done" || s.state == "error" {
		return []string{
			"work: " + s.state,
			"last: " + defaultStatusText(s.task, "none"),
		}
	}
	return []string{"work: idle"}
}

func (s *workStatus) persist(pj *project.Project, kind string) {
	if pj == nil {
		return
	}
	now := s.updatedAt
	if now.IsZero() {
		now = time.Now()
	}
	started := ""
	if !s.startedAt.IsZero() {
		started = s.startedAt.UTC().Format(time.RFC3339)
	}
	finished := ""
	if !s.finishedAt.IsZero() {
		finished = s.finishedAt.UTC().Format(time.RFC3339)
	}
	_ = project.SaveStatusSnapshot(pj, project.StatusSnapshot{
		UpdatedAt:   now.UTC().Format(time.RFC3339),
		Source:      statusSource,
		State:       s.state,
		CurrentTask: s.task,
		Checkpoint:  s.checkpoint,
		LastFact:    s.lastFact,
		StartedAt:   started,
		FinishedAt:  finished,
		BrainAlias:  s.brainAlias,
	})
	_ = project.AppendStatusEvent(pj, project.StatusEvent{
		At:         now.UTC().Format(time.RFC3339),
		Source:     statusSource,
		Kind:       kind,
		Task:       s.task,
		Checkpoint: s.checkpoint,
		Fact:       s.lastFact,
		BrainAlias: s.brainAlias,
	})
}

func isWorkStatusQuestion(input string) bool {
	n := normalizeStatusQuestion(input)
	switch n {
	case "what are we working on",
		"what are you working on",
		"what are you currently working on",
		"what are you doing",
		"what are you currently doing",
		"what is the status",
		"whats the status",
		"where are we",
		"where are we at",
		"where are we now",
		"where are things":
		return true
	default:
		return false
	}
}

func isBareStatusQuestion(input string) bool {
	n := normalizeStatusQuestion(input)
	return n == "status" || n == "current status"
}

func normalizeStatusQuestion(input string) string {
	n := strings.ToLower(strings.TrimSpace(input))
	n = strings.TrimRight(n, "?! .")
	n = strings.ReplaceAll(n, "’", "'")
	n = strings.ReplaceAll(n, "what's", "whats")
	n = strings.Join(strings.Fields(n), " ")
	return n
}

func cleanStatusText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

func defaultStatusText(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func statusElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
