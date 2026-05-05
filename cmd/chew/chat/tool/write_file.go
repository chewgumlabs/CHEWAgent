// write_file.go — create a new file with optional content.
//
// Refuses to overwrite an existing file. The user has to delete the file
// first, or pass write through a different verb (which doesn't exist
// yet — by design, rare-action verbs like "force overwrite" stay opt-in).

package tool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile creates a file. Refuses to overwrite.
type WriteFile struct{}

// Name implements Tool.
func (w *WriteFile) Name() string { return "write_file" }

// Execute implements Tool. Required param: "path" (string).
// Optional param: "content" (string).
func (w *WriteFile) Execute(params map[string]any) (Result, error) {
	path := stringParam(params, "path")
	if path == "" {
		return Result{}, errors.New("write_file requires a 'path' param")
	}
	content := stringParam(params, "content")

	if _, err := os.Stat(path); err == nil {
		return Result{}, fmt.Errorf("%s already exists; refusing to overwrite", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, fmt.Errorf("create parent dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Result{}, err
	}
	out := fmt.Sprintf("Wrote %d bytes to %s.", len(content), path)
	return Result{Output: out, Mascot: "idle"}, nil
}
