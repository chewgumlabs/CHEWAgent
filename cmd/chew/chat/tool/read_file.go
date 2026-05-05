// read_file.go — print a file's contents.
//
// Caps the output at 1 MB by default so a runaway log file can't blow up
// the chat. Truncation gets noted to the user; the full file is still on
// disk if they want to inspect it elsewhere.

package tool

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadFile prints the contents of a file.
type ReadFile struct {
	MaxBytes int64 // 0 = 1 MB
}

// Name implements Tool.
func (r *ReadFile) Name() string { return "read_file" }

// Execute implements Tool. Required param: "path" (string).
func (r *ReadFile) Execute(params map[string]any) (Result, error) {
	path := stringParam(params, "path")
	if path == "" {
		return Result{}, errors.New("read_file requires a 'path' param")
	}
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("stat: %w", err)
	}
	if info.IsDir() {
		return Result{}, fmt.Errorf("%s is a directory; try 'ls %s' instead", path, path)
	}

	cap := r.MaxBytes
	if cap == 0 {
		cap = 1 << 20 // 1 MB
	}

	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	body, err := io.ReadAll(io.LimitReader(f, cap+1))
	if err != nil {
		return Result{}, err
	}
	truncated := int64(len(body)) > cap
	if truncated {
		body = body[:cap]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s (%d bytes) ---\n", path, info.Size())
	b.Write(body)
	if truncated {
		fmt.Fprintf(&b, "\n\n[... truncated at %d bytes; full file is %d ...]", cap, info.Size())
	}
	return Result{Output: b.String(), Mascot: "idle"}, nil
}
