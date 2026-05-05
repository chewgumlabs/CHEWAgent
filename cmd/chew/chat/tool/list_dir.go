// list_dir.go — list directory contents with type + size.

package tool

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ListDir lists a directory.
type ListDir struct {
	MaxEntries int // 0 = 200
}

// Name implements Tool.
func (l *ListDir) Name() string { return "list_dir" }

// Execute implements Tool. Required param: "path" (string).
func (l *ListDir) Execute(params map[string]any) (Result, error) {
	path := stringParam(params, "path")
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("stat: %w", err)
	}
	if !info.IsDir() {
		return Result{}, errors.New("not a directory; use 'read' for files")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return Result{}, err
	}

	cap := l.MaxEntries
	if cap == 0 {
		cap = 200
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Contents of %s (%d items):\n\n", path, len(entries))

	truncated := false
	for i, e := range entries {
		if i >= cap {
			truncated = true
			break
		}
		marker := "f"
		size := ""
		einfo, _ := e.Info()
		switch {
		case e.IsDir():
			marker = "d"
			size = "—"
		case einfo != nil:
			size = humanSize(einfo.Size())
		}
		fmt.Fprintf(&b, "  %s  %8s  %s\n", marker, size, e.Name())
	}
	if truncated {
		fmt.Fprintf(&b, "\n[... %d more items not shown ...]\n", len(entries)-cap)
	}
	return Result{Output: b.String(), Mascot: "idle"}, nil
}

// humanSize formats a byte count as a short human-readable string.
func humanSize(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n < KB:
		return fmt.Sprintf("%dB", n)
	case n < MB:
		return fmt.Sprintf("%.1fK", float64(n)/KB)
	case n < GB:
		return fmt.Sprintf("%.1fM", float64(n)/MB)
	default:
		return fmt.Sprintf("%.1fG", float64(n)/GB)
	}
}
