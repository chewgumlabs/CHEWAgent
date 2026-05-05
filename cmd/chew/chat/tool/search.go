// search.go — local file pattern search (grep-equivalent).
//
// Walks a directory tree, matches each line of every reasonably-sized
// text file against the pattern, returns up to N hits. Skips hidden
// dirs (.git, .vscode), big nested trees (node_modules, vendor), and
// files that look binary.

package tool

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Search is a local-file content search.
type Search struct {
	MaxMatches  int // 0 = 100
	MaxFiles    int // 0 = 1000
	MaxFileSize int64
}

// Name implements Tool.
func (s *Search) Name() string { return "search" }

// Execute implements Tool. Required params: "pattern" (string),
// optional "path" (string, default ".").
func (s *Search) Execute(params map[string]any) (Result, error) {
	pattern := stringParam(params, "pattern")
	if pattern == "" {
		return Result{}, errors.New("search requires a 'pattern' param")
	}
	target := stringParam(params, "path")
	if target == "" {
		target = "."
	}

	// Try regex; if it doesn't compile, fall back to literal substring
	// match (so users typing `find foo+` get reasonable behaviour).
	re, err := regexp.Compile(pattern)
	if err != nil {
		re = regexp.MustCompile(regexp.QuoteMeta(pattern))
	}

	maxMatches := s.MaxMatches
	if maxMatches == 0 {
		maxMatches = 100
	}
	maxFiles := s.MaxFiles
	if maxFiles == 0 {
		maxFiles = 1000
	}
	maxFileSize := s.MaxFileSize
	if maxFileSize == 0 {
		maxFileSize = 1 << 20 // 1 MB
	}

	var b strings.Builder
	matches := 0
	filesScanned := 0

	walkErr := filepath.WalkDir(target, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable subdirs
		}
		if d.IsDir() {
			base := filepath.Base(p)
			if base != "." && (strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if matches >= maxMatches || filesScanned >= maxFiles {
			return io.EOF // sentinel — stop the walk
		}
		info, _ := d.Info()
		if info == nil || info.Size() > maxFileSize {
			return nil
		}

		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()

		// Cheap binary detector: read first 512 bytes, look for NUL.
		head := make([]byte, 512)
		n, _ := f.Read(head)
		if bytes.IndexByte(head[:n], 0) >= 0 {
			return nil
		}
		// Rewind so the line scanner starts from the top.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil
		}

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if re.MatchString(line) {
				if len(line) > 200 {
					line = line[:200] + "..."
				}
				fmt.Fprintf(&b, "%s:%d: %s\n", p, lineNo, line)
				matches++
				if matches >= maxMatches {
					break
				}
			}
		}
		filesScanned++
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, io.EOF) {
		return Result{}, walkErr
	}

	header := fmt.Sprintf("Search %q in %s — %d matches in %d files scanned\n\n", pattern, target, matches, filesScanned)
	if matches == 0 {
		return Result{Output: header + "(no matches)\n", Mascot: "ghost"}, nil
	}
	if matches >= maxMatches {
		fmt.Fprintf(&b, "\n[... stopped at %d matches; refine the pattern for fewer hits ...]\n", maxMatches)
	}
	return Result{Output: header + b.String(), Mascot: "idle"}, nil
}
