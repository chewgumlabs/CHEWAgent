package wizard

import (
	"os"
	"path/filepath"
	"testing"
)

// withProcessHooks overrides the process-inspection hooks for a single test
// and restores the originals via t.Cleanup. Pass nil to keep the default.
func withProcessHooks(t *testing.T, alive func(int) bool, llama func(int) bool, kill func(int)) {
	t.Helper()
	origAlive, origLlama, origKill := checkProcessAlive, checkLlamaServer, killBrainProcess
	t.Cleanup(func() {
		checkProcessAlive = origAlive
		checkLlamaServer = origLlama
		killBrainProcess = origKill
	})
	if alive != nil {
		checkProcessAlive = alive
	}
	if llama != nil {
		checkLlamaServer = llama
	}
	if kill != nil {
		killBrainProcess = kill
	}
}

// --- readBrainMeta / writeBrainMeta round-trip ---

func TestReadWriteBrainMeta_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := BrainMeta{
		BrainPID:  1234,
		OwnerPID:  5678,
		StartedAt: "2026-05-05T00:00:00Z",
		Command:   "/usr/bin/llama-server",
	}
	path, err := writeBrainMeta(dir, original)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "brain.pid.json" {
		t.Errorf("unexpected filename: %s", filepath.Base(path))
	}

	meta, metaPath, legacy := readBrainMeta(dir)
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if legacy {
		t.Error("should not be legacy")
	}
	if metaPath != path {
		t.Errorf("path mismatch: got %s, want %s", metaPath, path)
	}
	if meta.BrainPID != 1234 || meta.OwnerPID != 5678 {
		t.Errorf("meta mismatch: %+v", meta)
	}
	if meta.StartedAt != "2026-05-05T00:00:00Z" {
		t.Errorf("started_at mismatch: %s", meta.StartedAt)
	}
	if meta.Command != "/usr/bin/llama-server" {
		t.Errorf("command mismatch: %s", meta.Command)
	}
}

func TestReadBrainMeta_LegacyPidfile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "brain.pid"), []byte("4242\n"), 0o644)

	meta, metaPath, legacy := readBrainMeta(dir)
	if meta == nil {
		t.Fatal("expected non-nil meta from legacy pidfile")
	}
	if !legacy {
		t.Error("should be legacy")
	}
	if meta.BrainPID != 4242 {
		t.Errorf("expected PID 4242, got %d", meta.BrainPID)
	}
	if meta.OwnerPID != 0 {
		t.Errorf("legacy should have no owner, got %d", meta.OwnerPID)
	}
	if metaPath != filepath.Join(dir, "brain.pid") {
		t.Errorf("unexpected metaPath: %s", metaPath)
	}
}

func TestReadBrainMeta_JSONTakesPrecedenceOverLegacy(t *testing.T) {
	dir := t.TempDir()
	// Both files present — JSON wins.
	_ = os.WriteFile(filepath.Join(dir, "brain.pid"), []byte("1111\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "brain.pid.json"), []byte(`{"brain_pid":2222,"owner_pid":3333}`), 0o644)

	meta, _, legacy := readBrainMeta(dir)
	if meta == nil {
		t.Fatal("expected meta")
	}
	if legacy {
		t.Error("JSON should win over legacy")
	}
	if meta.BrainPID != 2222 {
		t.Errorf("expected JSON brain PID 2222, got %d", meta.BrainPID)
	}
}

func TestWriteBrainMeta_RemovesLegacyPidfile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "brain.pid"), []byte("1111\n"), 0o644)

	if _, err := writeBrainMeta(dir, BrainMeta{BrainPID: 2222, OwnerPID: 3333}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "brain.pid")); !os.IsNotExist(err) {
		t.Errorf("legacy brain.pid should be removed after writing brain.pid.json, stat err=%v", err)
	}
}

func TestReadBrainMeta_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "brain.pid.json"), []byte("not json{{{"), 0o644)

	meta, metaPath, _ := readBrainMeta(dir)
	if meta != nil {
		t.Error("expected nil meta for malformed JSON")
	}
	if metaPath == "" {
		t.Error("expected non-empty metaPath for cleanup")
	}
}

func TestReadBrainMeta_MalformedLegacy(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "brain.pid"), []byte("not-a-number\n"), 0o644)

	meta, metaPath, legacy := readBrainMeta(dir)
	if meta != nil {
		t.Error("expected nil meta for malformed legacy")
	}
	if !legacy {
		t.Error("should be legacy path")
	}
	if metaPath == "" {
		t.Error("expected non-empty metaPath for cleanup")
	}
}

func TestReadBrainMeta_NoPidfile(t *testing.T) {
	dir := t.TempDir()
	meta, metaPath, _ := readBrainMeta(dir)
	if meta != nil || metaPath != "" {
		t.Errorf("expected nil/empty for missing pidfile, got meta=%v path=%s", meta, metaPath)
	}
}

// --- KillStaleBrain lifecycle cases ---

func TestKillStaleBrain_NoPidfile(t *testing.T) {
	dir := t.TempDir()
	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
}

func TestKillStaleBrain_ActiveOwner(t *testing.T) {
	dir := t.TempDir()
	writeBrainMeta(dir, BrainMeta{
		BrainPID: 1000, OwnerPID: 2000,
		StartedAt: "2026-05-05T00:00:00Z",
		Command:   "/fake/llama-server",
	})

	killed := false
	withProcessHooks(t,
		func(pid int) bool { return true }, // both alive
		func(pid int) bool { return true },
		func(pid int) { killed = true },
	)

	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
	if killed {
		t.Error("should NOT kill when owner is alive")
	}
	// Metadata must survive — the active session owns it.
	if _, err := os.Stat(filepath.Join(dir, "brain.pid.json")); err != nil {
		t.Error("brain.pid.json should NOT be removed when owner is alive")
	}
}

func TestKillStaleBrain_OrphanedBrain(t *testing.T) {
	dir := t.TempDir()
	writeBrainMeta(dir, BrainMeta{
		BrainPID: 1000, OwnerPID: 2000,
		StartedAt: "2026-05-05T00:00:00Z",
		Command:   "/fake/llama-server",
	})

	killed := false
	withProcessHooks(t,
		func(pid int) bool { return pid == 1000 }, // brain alive, owner dead
		func(pid int) bool { return true },
		func(pid int) { killed = true },
	)

	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
	if !killed {
		t.Error("should kill orphaned brain")
	}
	if _, err := os.Stat(filepath.Join(dir, "brain.pid.json")); !os.IsNotExist(err) {
		t.Error("brain.pid.json should be removed after killing orphan")
	}
}

func TestKillStaleBrain_ReusedPID(t *testing.T) {
	dir := t.TempDir()
	writeBrainMeta(dir, BrainMeta{
		BrainPID: 1000, OwnerPID: 2000,
		StartedAt: "2026-05-05T00:00:00Z",
		Command:   "/fake/llama-server",
	})

	killed := false
	withProcessHooks(t,
		func(pid int) bool { return pid == 1000 }, // PID alive, owner dead
		func(pid int) bool { return false },       // NOT llama-server
		func(pid int) { killed = true },
	)

	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
	if killed {
		t.Error("should NOT kill when PID is reused by non-llama process")
	}
	// Stale metadata still cleaned up.
	if _, err := os.Stat(filepath.Join(dir, "brain.pid.json")); !os.IsNotExist(err) {
		t.Error("brain.pid.json should be removed for reused PID")
	}
}

func TestKillStaleBrain_BothDead(t *testing.T) {
	dir := t.TempDir()
	writeBrainMeta(dir, BrainMeta{
		BrainPID: 1000, OwnerPID: 2000,
		StartedAt: "2026-05-05T00:00:00Z",
		Command:   "/fake/llama-server",
	})

	killed := false
	withProcessHooks(t,
		func(pid int) bool { return false }, // both dead
		func(pid int) bool { return true },
		func(pid int) { killed = true },
	)

	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
	if killed {
		t.Error("should NOT attempt kill when brain is already dead")
	}
	if _, err := os.Stat(filepath.Join(dir, "brain.pid.json")); !os.IsNotExist(err) {
		t.Error("brain.pid.json should be removed when both are dead")
	}
}

func TestKillStaleBrain_MalformedMeta(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "brain.pid.json"), []byte("garbage"), 0o644)

	killed := false
	withProcessHooks(t,
		func(pid int) bool { return true },
		func(pid int) bool { return true },
		func(pid int) { killed = true },
	)

	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
	if killed {
		t.Error("should NOT kill when metadata is malformed")
	}
	if _, err := os.Stat(filepath.Join(dir, "brain.pid.json")); !os.IsNotExist(err) {
		t.Error("malformed brain.pid.json should be removed")
	}
}

func TestKillStaleBrain_LegacyOrphan(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "brain.pid"), []byte("1000\n"), 0o644)

	killed := false
	withProcessHooks(t,
		func(pid int) bool { return pid == 1000 }, // PID alive (no owner to check)
		func(pid int) bool { return true },        // is llama-server
		func(pid int) { killed = true },
	)

	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
	if !killed {
		t.Error("should kill orphaned brain from legacy pidfile")
	}
	if _, err := os.Stat(filepath.Join(dir, "brain.pid")); !os.IsNotExist(err) {
		t.Error("legacy brain.pid should be removed after kill")
	}
}

func TestKillStaleBrain_LegacyStalePID(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "brain.pid"), []byte("9999\n"), 0o644)

	killed := false
	withProcessHooks(t,
		func(pid int) bool { return false }, // PID dead
		func(pid int) bool { return true },
		func(pid int) { killed = true },
	)

	if err := KillStaleBrain(dir); err != nil {
		t.Fatal(err)
	}
	if killed {
		t.Error("should NOT kill when legacy PID is dead")
	}
	if _, err := os.Stat(filepath.Join(dir, "brain.pid")); !os.IsNotExist(err) {
		t.Error("stale legacy brain.pid should be removed")
	}
}

func TestKillStaleBrain_NewFormatCleansLegacyToo(t *testing.T) {
	dir := t.TempDir()
	// Simulate upgrade: both files present. brain.pid.json is authoritative.
	writeBrainMeta(dir, BrainMeta{BrainPID: 1000, OwnerPID: 2000})
	_ = os.WriteFile(filepath.Join(dir, "brain.pid"), []byte("9999\n"), 0o644)

	withProcessHooks(t,
		func(pid int) bool { return false }, // both dead
		nil, nil,
	)

	_ = KillStaleBrain(dir)

	if _, err := os.Stat(filepath.Join(dir, "brain.pid.json")); !os.IsNotExist(err) {
		t.Error("brain.pid.json should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "brain.pid")); !os.IsNotExist(err) {
		t.Error("legacy brain.pid should also be removed")
	}
}
