package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "hello.txt")
	if err := os.WriteFile(p, []byte("hello, world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ReadFile{}
	res, err := tool.Execute(map[string]any{"path": p})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "hello, world") {
		t.Errorf("expected file contents in output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, p) {
		t.Errorf("expected path header, got: %s", res.Output)
	}
}

func TestReadFile_RejectsDirectories(t *testing.T) {
	tmp := t.TempDir()
	tool := &ReadFile{}
	_, err := tool.Execute(map[string]any{"path": tmp})
	if err == nil {
		t.Errorf("reading a directory should error")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error should mention 'directory', got: %v", err)
	}
}

func TestReadFile_TruncatesLargeFiles(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "big.txt")
	body := strings.Repeat("A", 10_000)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ReadFile{MaxBytes: 100}
	res, err := tool.Execute(map[string]any{"path": p})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "truncated at 100") {
		t.Errorf("expected truncation note, got tail: %s", res.Output[len(res.Output)-200:])
	}
}

func TestReadFile_NoPath(t *testing.T) {
	tool := &ReadFile{}
	_, err := tool.Execute(map[string]any{})
	if err == nil {
		t.Errorf("missing path should error")
	}
}

func TestListDir_ListsEntries(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "subdir"} {
		full := filepath.Join(tmp, name)
		if name == "subdir" {
			_ = os.Mkdir(full, 0o755)
		} else {
			_ = os.WriteFile(full, []byte("x"), 0o644)
		}
	}
	tool := &ListDir{}
	res, err := tool.Execute(map[string]any{"path": tmp})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"a.txt", "b.txt", "subdir"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("expected %q in output, got: %s", want, res.Output)
		}
	}
}

func TestListDir_TruncatesLongLists(t *testing.T) {
	tmp := t.TempDir()
	for i := 0; i < 50; i++ {
		_ = os.WriteFile(filepath.Join(tmp, "f"+string(rune('a'+i%26))+string(rune('0'+i%10))), []byte("x"), 0o644)
	}
	tool := &ListDir{MaxEntries: 5}
	res, _ := tool.Execute(map[string]any{"path": tmp})
	if !strings.Contains(res.Output, "more items not shown") {
		t.Errorf("expected truncation note, got: %s", res.Output)
	}
}

func TestSearch_FindsMatches(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello world\nfind me\nthird line\n"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("nothing here\n"), 0o644)
	tool := &Search{}
	res, err := tool.Execute(map[string]any{"pattern": "find me", "path": tmp})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "a.txt:2: find me") {
		t.Errorf("expected match in a.txt at line 2, got: %s", res.Output)
	}
}

func TestSearch_NoMatchesReturnsGhostMascot(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hi\n"), 0o644)
	tool := &Search{}
	res, _ := tool.Execute(map[string]any{"pattern": "nothing-like-this", "path": tmp})
	if !strings.Contains(res.Output, "no matches") {
		t.Errorf("expected 'no matches' message, got: %s", res.Output)
	}
	if res.Mascot != "ghost" {
		t.Errorf("no-matches should set mascot=ghost, got %s", res.Mascot)
	}
}

func TestSearch_SkipsBinaryFiles(t *testing.T) {
	tmp := t.TempDir()
	// File with a NUL byte in the head — should be skipped by the binary detector.
	_ = os.WriteFile(filepath.Join(tmp, "binary.bin"), []byte("hello\x00world findme"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, "text.txt"), []byte("findme here\n"), 0o644)
	tool := &Search{}
	res, _ := tool.Execute(map[string]any{"pattern": "findme", "path": tmp})
	if strings.Contains(res.Output, "binary.bin") {
		t.Errorf("binary file should be skipped, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "text.txt") {
		t.Errorf("text file should be matched, got: %s", res.Output)
	}
}

func TestSearch_SkipsHiddenAndVendorDirs(t *testing.T) {
	tmp := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmp, ".git"), 0o755)
	_ = os.MkdirAll(filepath.Join(tmp, "node_modules"), 0o755)
	_ = os.MkdirAll(filepath.Join(tmp, "vendor"), 0o755)
	_ = os.MkdirAll(filepath.Join(tmp, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".git", "HEAD"), []byte("findme\n"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, "node_modules", "junk.txt"), []byte("findme\n"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, "vendor", "v.txt"), []byte("findme\n"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, "src", "main.txt"), []byte("findme\n"), 0o644)
	tool := &Search{}
	res, _ := tool.Execute(map[string]any{"pattern": "findme", "path": tmp})
	if strings.Contains(res.Output, ".git") {
		t.Errorf(".git should be skipped, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "node_modules") {
		t.Errorf("node_modules should be skipped, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "vendor") {
		t.Errorf("vendor should be skipped, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "src") {
		t.Errorf("src should be searched, got: %s", res.Output)
	}
}

func TestWriteFile_Creates(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "new.txt")
	tool := &WriteFile{}
	res, err := tool.Execute(map[string]any{"path": p, "content": "hello"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "Wrote 5 bytes") {
		t.Errorf("expected byte count, got: %s", res.Output)
	}
	body, err := os.ReadFile(p)
	if err != nil || string(body) != "hello" {
		t.Errorf("file contents wrong: %q (err=%v)", body, err)
	}
}

func TestWriteFile_RefusesOverwrite(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "exists.txt")
	_ = os.WriteFile(p, []byte("preexisting"), 0o644)
	tool := &WriteFile{}
	_, err := tool.Execute(map[string]any{"path": p, "content": "stomp"})
	if err == nil {
		t.Errorf("write_file should refuse to overwrite")
	}
	body, _ := os.ReadFile(p)
	if string(body) != "preexisting" {
		t.Errorf("file should be unchanged, got: %q", body)
	}
}

func TestRunCommand_Echo(t *testing.T) {
	tool := &RunCommand{}
	res, err := tool.Execute(map[string]any{"command": "echo hello"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("expected echo output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "$ echo hello") {
		t.Errorf("expected command prefix, got: %s", res.Output)
	}
}

func TestRunCommand_NonZeroExit(t *testing.T) {
	tool := &RunCommand{}
	res, _ := tool.Execute(map[string]any{"command": "exit 3"})
	if !strings.Contains(res.Output, "[exit:") {
		t.Errorf("expected exit note, got: %s", res.Output)
	}
	if res.Mascot != "ghost" {
		t.Errorf("non-zero exit should set mascot=ghost, got: %s", res.Mascot)
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	tool := &RunCommand{Timeout: 100_000_000} // 100ms
	res, _ := tool.Execute(map[string]any{"command": "sleep 5"})
	if !strings.Contains(res.Output, "timed out") {
		t.Errorf("expected timeout note, got: %s", res.Output)
	}
}

func TestRunCommand_TruncatesLargeOutput(t *testing.T) {
	tool := &RunCommand{MaxOutputBytes: 50}
	res, _ := tool.Execute(map[string]any{"command": "yes hi | head -200"})
	if !strings.Contains(res.Output, "truncated at 50") {
		t.Errorf("expected truncation note, got tail: %q", res.Output[len(res.Output)-100:])
	}
}

// ----- project-scoped tool tests -----
//
// These tests verify that after setting a project root, relative paths
// resolve against the root, pwd reports the root, and absolute paths
// still bypass the root.

func TestResolveToolPath_RelativeFromRoot(t *testing.T) {
	root := "/tmp/project"
	got, err := resolveToolPath(&root, "src/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/project/src/main.go" {
		t.Errorf("expected /tmp/project/src/main.go, got %q", got)
	}
}

func TestResolveToolPath_DotResolvesToRoot(t *testing.T) {
	root := "/tmp/project"
	got, err := resolveToolPath(&root, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %q", got)
	}
}

func TestResolveToolPath_AbsolutePassesThrough(t *testing.T) {
	root := "/tmp/project"
	got, err := resolveToolPath(&root, "/absolute/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/absolute/path" {
		t.Errorf("absolute path should pass through, got %q", got)
	}
}

func TestResolveToolPath_NilRootPassesThrough(t *testing.T) {
	got, err := resolveToolPath(nil, "relative/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "relative/path" {
		t.Errorf("nil root should return path unchanged, got %q", got)
	}
}

func TestResolveToolPath_EmptyRootPassesThrough(t *testing.T) {
	root := ""
	got, err := resolveToolPath(&root, "relative/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "relative/path" {
		t.Errorf("empty root should return path unchanged, got %q", got)
	}
}

func TestResolveToolPath_RejectsEscape(t *testing.T) {
	root := "/tmp/project"
	_, err := resolveToolPath(&root, "../../etc/passwd")
	if err == nil {
		t.Errorf("path traversal outside root should be rejected")
	}
}

func TestReadFile_RelativeWithRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "data.txt"), []byte("project data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := tmp
	tool := &ReadFile{Root: &root}
	res, err := tool.Execute(map[string]any{"path": "data.txt"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "project data") {
		t.Errorf("expected file contents from project root, got: %s", res.Output)
	}
}

func TestReadFile_AbsoluteBypassesRoot(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "abs.txt")
	if err := os.WriteFile(abs, []byte("absolute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := "/some/other/root"
	tool := &ReadFile{Root: &root}
	res, err := tool.Execute(map[string]any{"path": abs})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "absolute") {
		t.Errorf("absolute path should bypass root, got: %s", res.Output)
	}
}

func TestListDir_RelativeWithRoot(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("x"), 0o644)
	_ = os.Mkdir(filepath.Join(tmp, "subdir"), 0o755)
	root := tmp
	tool := &ListDir{Root: &root}
	res, err := tool.Execute(map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "file.txt") || !strings.Contains(res.Output, "subdir") {
		t.Errorf("expected project root contents, got: %s", res.Output)
	}
}

func TestWriteFile_RelativeWithRoot(t *testing.T) {
	tmp := t.TempDir()
	root := tmp
	tool := &WriteFile{Root: &root}
	_, err := tool.Execute(map[string]any{"path": "new.txt", "content": "hello root"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(tmp, "new.txt"))
	if err != nil {
		t.Fatalf("file should exist in project root: %v", err)
	}
	if string(body) != "hello root" {
		t.Errorf("expected 'hello root', got %q", body)
	}
}

func TestSearch_RelativeWithRoot(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "code.go"), []byte("func main() {}\n"), 0o644)
	root := tmp
	tool := &Search{Root: &root}
	res, err := tool.Execute(map[string]any{"pattern": "func main", "path": "."})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "code.go") {
		t.Errorf("expected match in project root, got: %s", res.Output)
	}
}

func TestRunCommand_PwdReportsRoot(t *testing.T) {
	tmp := t.TempDir()
	root := tmp
	tool := &RunCommand{Root: &root}
	// pwd -P avoids symlink differences (macOS /var -> /private/var).
	res, _ := tool.Execute(map[string]any{"command": "pwd -P"})
	realTmp, _ := filepath.EvalSymlinks(tmp)
	if !strings.Contains(res.Output, realTmp) {
		t.Errorf("pwd should report project root %q, got: %s", realTmp, res.Output)
	}
}

func TestRunCommand_CmdDirFromRoot(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "marker.txt"), []byte("found it"), 0o644)
	root := tmp
	tool := &RunCommand{Root: &root}
	res, _ := tool.Execute(map[string]any{"command": "cat marker.txt"})
	if !strings.Contains(res.Output, "found it") {
		t.Errorf("command should see files in project root, got: %s", res.Output)
	}
}

func TestRegistry_SetRoot_Integration(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "test.txt"), []byte("from root\n"), 0o644)

	r := NewDefault()
	r.SetRoot(tmp)

	// Read via relative path should find the file in the root.
	res, err := r.Dispatch("read_file", map[string]any{"path": "test.txt"})
	if err != nil {
		t.Fatalf("dispatch read_file: %v", err)
	}
	if !strings.Contains(res.Output, "from root") {
		t.Errorf("expected file contents from root, got: %s", res.Output)
	}

	// pwd should report the root.
	res, err = r.Dispatch("run_command", map[string]any{"command": "pwd -P"})
	if err != nil {
		t.Fatalf("dispatch pwd: %v", err)
	}
	realTmp, _ := filepath.EvalSymlinks(tmp)
	if !strings.Contains(res.Output, realTmp) {
		t.Errorf("pwd should report root %q, got: %s", realTmp, res.Output)
	}

	// Clear root — pwd should no longer report the tmp dir.
	r.SetRoot("")
	res, _ = r.Dispatch("run_command", map[string]any{"command": "pwd -P"})
	if strings.Contains(res.Output, realTmp) {
		t.Errorf("after clearing root, pwd should not report old root: %s", res.Output)
	}
}
