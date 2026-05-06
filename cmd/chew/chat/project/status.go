package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	StatusSnapshotSchema = "chew-gum-status.v0"
	StatusEventSchema    = "chew-gum-status-event.v0"
)

type StatusSnapshot struct {
	SchemaVersion string `json:"schema_version"`
	UpdatedAt     string `json:"updated_at"`
	Source        string `json:"source"`
	ProjectName   string `json:"project_name,omitempty"`
	ProjectPath   string `json:"project_path,omitempty"`
	State         string `json:"state"`
	CurrentTask   string `json:"current_task,omitempty"`
	Checkpoint    string `json:"checkpoint,omitempty"`
	LastFact      string `json:"last_fact,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
	BrainAlias    string `json:"brain_alias,omitempty"`
}

type StatusEvent struct {
	SchemaVersion string `json:"schema_version"`
	At            string `json:"at"`
	Source        string `json:"source"`
	ProjectName   string `json:"project_name,omitempty"`
	ProjectPath   string `json:"project_path,omitempty"`
	Kind          string `json:"kind"`
	Task          string `json:"task,omitempty"`
	Checkpoint    string `json:"checkpoint,omitempty"`
	Fact          string `json:"fact,omitempty"`
	BrainAlias    string `json:"brain_alias,omitempty"`
}

func (p *Project) GumDir() string {
	return filepath.Join(p.Path, ".gum")
}

func (p *Project) StatusPath() string {
	return filepath.Join(p.GumDir(), "status.json")
}

func (p *Project) StatusEventsPath() string {
	return filepath.Join(p.GumDir(), "status.jsonl")
}

func SaveStatusSnapshot(p *Project, s StatusSnapshot) error {
	if p == nil {
		return nil
	}
	s.SchemaVersion = StatusSnapshotSchema
	s.Source = strings.TrimSpace(s.Source)
	if s.Source == "" {
		s.Source = "chewagent"
	}
	s.ProjectName = p.Name
	s.ProjectPath = p.Path
	if strings.TrimSpace(s.UpdatedAt) == "" {
		s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(p.GumDir(), 0o755); err != nil {
		return fmt.Errorf("create .gum status dir: %w", err)
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode status snapshot: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(p.StatusPath(), body, 0o644); err != nil {
		return fmt.Errorf("write status snapshot: %w", err)
	}
	return nil
}

func AppendStatusEvent(p *Project, e StatusEvent) error {
	if p == nil {
		return nil
	}
	e.SchemaVersion = StatusEventSchema
	e.Source = strings.TrimSpace(e.Source)
	if e.Source == "" {
		e.Source = "chewagent"
	}
	e.ProjectName = p.Name
	e.ProjectPath = p.Path
	if strings.TrimSpace(e.At) == "" {
		e.At = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(p.GumDir(), 0o755); err != nil {
		return fmt.Errorf("create .gum status dir: %w", err)
	}
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode status event: %w", err)
	}
	body = append(body, '\n')
	f, err := os.OpenFile(p.StatusEventsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open status event log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(body); err != nil {
		return fmt.Errorf("write status event: %w", err)
	}
	return nil
}
