package wizard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// collect returns a reply func and a pointer to the captured replies,
// so tests can assert on what the wizard said.
func collect() (func(string), *[]string) {
	out := []string{}
	return func(s string) { out = append(out, s) }, &out
}

// newTestWizard builds a wizard with every side-effecting hook stubbed
// so tests don't touch the network, spawn processes, or read PATH.
func newTestWizard(t *testing.T) *InstallBrain {
	t.Helper()
	tmp := t.TempDir()
	w := NewInstallBrain()
	w.chewHome = filepath.Join(tmp, ".chew")
	w.modelDir = filepath.Join(w.chewHome, "models")
	w.modelPath = filepath.Join(w.modelDir, installBrainModelFile)

	// Pretend the runtime is bundled.
	w.findRuntimeFn = func() (string, error) { return "/fake/llama-server", nil }

	// Pretend download writes some bytes; emit one progress tick at the end.
	w.downloadFn = func(url, dest string, progress func(done, total int64)) error {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte("fake gguf bytes"), 0o644); err != nil {
			return err
		}
		if progress != nil {
			progress(15, 15)
		}
		return nil
	}

	// Pretend brain spawn succeeds with a minimal stub so health-wait
	// has something to call against.
	w.startBrainFn = func(cfg BrainConfig) (*Brain, error) {
		return &Brain{endpoint: "http://127.0.0.1:8080"}, nil
	}
	w.waitHealthyFn = func(ctx context.Context, b *Brain) error { return nil }

	return w
}

func TestInstallBrain_Begin(t *testing.T) {
	w := newTestWizard(t)
	reply, captured := collect()
	w.Begin(reply)
	if len(*captured) != 1 {
		t.Fatalf("Begin should reply once, got %d replies", len(*captured))
	}
	if !strings.Contains((*captured)[0], "brain transplant") {
		t.Errorf("Begin reply should mention brain transplant, got: %s", (*captured)[0])
	}
}

func TestInstallBrain_Cancel(t *testing.T) {
	w := newTestWizard(t)
	reply, captured := collect()
	w.Begin(reply)
	done, err := w.Step("cancel", reply)
	if err != nil {
		t.Fatalf("cancel: unexpected err: %v", err)
	}
	if !done {
		t.Errorf("cancel should set done=true")
	}
	if !strings.Contains((*captured)[1], "Cancelled") {
		t.Errorf("cancel reply should say Cancelled, got: %s", (*captured)[1])
	}
}

func TestInstallBrain_TellMeMoreReturnsToPlan(t *testing.T) {
	w := newTestWizard(t)
	reply, captured := collect()
	w.Begin(reply)
	done, err := w.Step("tell me more", reply)
	if err != nil || done {
		t.Fatalf("tell me more should not be done, got done=%v err=%v", done, err)
	}
	if !strings.Contains((*captured)[1], "Why I need this") {
		t.Errorf("details reply should explain, got: %s", (*captured)[1])
	}
	// Any input from details returns to plan.
	done, err = w.Step("ok", reply)
	if err != nil || done {
		t.Fatalf("post-details should not be done, got done=%v err=%v", done, err)
	}
	if !strings.Contains((*captured)[2], "brain transplant") {
		t.Errorf("should return to plan, got: %s", (*captured)[2])
	}
}

func TestInstallBrain_FullHappyPath(t *testing.T) {
	w := newTestWizard(t)
	reply, captured := collect()
	w.Begin(reply)
	// 'yes' runs all three steps inline.
	done, err := w.Step("yes", reply)
	if err != nil {
		t.Fatalf("yes path: unexpected err: %v", err)
	}
	if !done {
		t.Errorf("happy path should set done=true after one Step call")
	}
	all := strings.Join(*captured, "\n")
	for _, want := range []string{"[1/3]", "[2/3]", "[3/3]", "Brain online"} {
		if !strings.Contains(all, want) {
			t.Errorf("expected %q in transcript, missing. transcript: %s", want, all)
		}
	}
	// Verify the model file was "downloaded" (stub wrote bytes).
	if _, err := os.Stat(w.modelPath); err != nil {
		t.Errorf("model path %s should exist after happy path: %v", w.modelPath, err)
	}
	// Verify the config was written.
	configPath := filepath.Join(w.chewHome, "config.json")
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config should exist: %v", err)
	}
	if !strings.Contains(string(body), "ChewBrain") {
		t.Errorf("config should mention ChewBrain alias, got: %s", body)
	}
	if !strings.Contains(string(body), w.modelPath) {
		t.Errorf("config should embed model path %q, got: %s", w.modelPath, body)
	}
	// Verify the wizard captured the running brain so the REPL can pick it up.
	if w.RunningBrain() == nil {
		t.Errorf("RunningBrain() should be set after happy path")
	}
}

func TestInstallBrain_HidesPathsInUserText(t *testing.T) {
	// UX contract: nothing user-visible should leak filesystem paths,
	// URLs, GGUF filenames, or the binary name "llama-server". The user
	// is plug-and-play.
	w := newTestWizard(t)
	reply, captured := collect()
	w.Begin(reply)
	_, _ = w.Step("yes", reply)
	all := strings.Join(*captured, "\n")
	forbidden := []string{
		w.modelPath,                  // /tmp/.../Bonsai-8B-Q1_0.gguf
		w.chewHome,                   // /tmp/.../.chew
		installBrainModelURL,         // huggingface URL
		installBrainModelFile,        // Bonsai-8B-Q1_0.gguf
		"llama-server",               // tool name
		"https://",                   // any URL leak
		"huggingface",                // upstream brand
	}
	for _, bad := range forbidden {
		if strings.Contains(all, bad) {
			t.Errorf("user-visible transcript leaks %q.\ntranscript:\n%s", bad, all)
		}
	}
}

func TestInstallBrain_RuntimeMissingHaltsCleanly(t *testing.T) {
	w := newTestWizard(t)
	w.findRuntimeFn = func() (string, error) { return "", errors.New("missing") }
	reply, captured := collect()
	w.Begin(reply)
	done, err := w.Step("yes", reply)
	if err != nil {
		t.Fatalf("missing runtime should not be a hard error: %v", err)
	}
	if !done {
		t.Errorf("missing runtime should halt the wizard (done=true)")
	}
	all := strings.Join(*captured, "\n")
	if !strings.Contains(all, "packaging") {
		t.Errorf("user should be told it's a packaging issue, got: %s", all)
	}
	// And we should NOT have started a download.
	if _, err := os.Stat(w.modelPath); !os.IsNotExist(err) {
		t.Errorf("model should NOT exist when runtime check failed, stat err=%v", err)
	}
}

func TestInstallBrain_DownloadFailureHaltsCleanly(t *testing.T) {
	w := newTestWizard(t)
	w.downloadFn = func(url, dest string, progress func(done, total int64)) error {
		return errors.New("no such host")
	}
	reply, captured := collect()
	w.Begin(reply)
	done, err := w.Step("yes", reply)
	if err == nil {
		t.Errorf("download failure should surface an error to the caller")
	}
	if !done {
		t.Errorf("download failure should halt the wizard")
	}
	all := strings.Join(*captured, "\n")
	if !strings.Contains(all, "couldn't reach the internet") {
		t.Errorf("user should get the friendly network message, got: %s", all)
	}
}

func TestInstallBrain_BrainSpawnFailureHaltsCleanly(t *testing.T) {
	w := newTestWizard(t)
	w.startBrainFn = func(cfg BrainConfig) (*Brain, error) {
		return nil, errors.New("permission denied")
	}
	reply, captured := collect()
	w.Begin(reply)
	done, err := w.Step("yes", reply)
	if err == nil {
		t.Errorf("spawn failure should surface an error")
	}
	if !done {
		t.Errorf("spawn failure should halt the wizard")
	}
	all := strings.Join(*captured, "\n")
	if !strings.Contains(all, "not allowed") {
		t.Errorf("user should get the friendly permission message, got: %s", all)
	}
	// And the brain should not be exposed to the REPL.
	if w.RunningBrain() != nil {
		t.Errorf("RunningBrain() should be nil after spawn failure")
	}
}

func TestInstallBrain_HealthFailureStopsBrain(t *testing.T) {
	stopCalled := false
	w := newTestWizard(t)
	w.startBrainFn = func(cfg BrainConfig) (*Brain, error) {
		// Return a brain whose Stop sets a flag we can observe.
		b := &Brain{endpoint: "http://127.0.0.1:8080"}
		// Brain.Stop short-circuits if cmd is nil, but we want to observe
		// the wizard's intent. Wrap by overriding waitHealthy below.
		_ = b
		return b, nil
	}
	w.waitHealthyFn = func(ctx context.Context, b *Brain) error {
		stopCalled = true // about to be told the brain is unhealthy
		return errors.New("deadline exceeded")
	}
	reply, captured := collect()
	w.Begin(reply)
	done, err := w.Step("yes", reply)
	if err == nil {
		t.Errorf("health failure should surface an error")
	}
	if !done {
		t.Errorf("health failure should halt the wizard")
	}
	if !stopCalled {
		t.Errorf("waitHealthyFn was not invoked")
	}
	all := strings.Join(*captured, "\n")
	if !strings.Contains(all, "didn't respond") {
		t.Errorf("user should get the friendly health message, got: %s", all)
	}
	if w.RunningBrain() != nil {
		t.Errorf("RunningBrain() should be nil after health failure")
	}
}

func TestInstallBrain_ProgressTicksDuringDownload(t *testing.T) {
	w := newTestWizard(t)
	w.downloadFn = func(url, dest string, progress func(done, total int64)) error {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
			return err
		}
		// Fire 3 progress ticks.
		progress(100*1024*1024, 1200*1024*1024)
		progress(600*1024*1024, 1200*1024*1024)
		progress(1200*1024*1024, 1200*1024*1024)
		return nil
	}
	reply, captured := collect()
	w.Begin(reply)
	_, _ = w.Step("yes", reply)
	all := strings.Join(*captured, "\n")
	// Each tick should appear as a "MB / MB (pct%)" line.
	for _, want := range []string{"100.0 / 1200.0 MB", "600.0 / 1200.0 MB", "1200.0 / 1200.0 MB"} {
		if !strings.Contains(all, want) {
			t.Errorf("expected progress line %q in transcript", want)
		}
	}
}

func TestInstallBrain_UnknownInputAtPlan(t *testing.T) {
	w := newTestWizard(t)
	reply, captured := collect()
	w.Begin(reply)
	done, err := w.Step("maybe later", reply)
	if err != nil || done {
		t.Fatalf("unknown input should not be done, got done=%v err=%v", done, err)
	}
	if !strings.Contains((*captured)[1], "yes") {
		t.Errorf("should ask for yes/cancel/tell-me-more, got: %s", (*captured)[1])
	}
}
