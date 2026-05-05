// runtime_install.go — auto-fetch the llama-server binary if it isn't
// already present.
//
// The lookup chain (acquireLlamaServer):
//
//   1. <brainDir>/runtime/llama-server (or .exe on Windows) — installed by a
//      previous run of this wizard
//   2. <exe-dir>/bin/llama-server-<GOOS>-<GOARCH>                    — bundled
//      directly into a shipped CHEW package
//   3. exec.LookPath("llama-server")                                 — system
//      install (e.g. `brew install llama.cpp`)
//   4. download from llama.cpp's GitHub releases — pinned to llamaTag below
//
// (1)–(3) are all "already installed somewhere" — no work, just a path.
// (4) is the thin auto-install: download a ~9 MB tarball, extract, return
// the path. The user sees one progress line; total time is a few seconds
// on a typical connection.

package wizard

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// llamaTag pins the llama.cpp release we ship with. Update by hand; we
// don't do "fetch latest" so a release shifting under us can't break a
// user's install mid-flight.
//
// b9033 published 2026-05-05.
const llamaTag = "b9033"

// acquireLlamaServer returns a usable path to llama-server, downloading +
// extracting it under brainDir if necessary. Progress callbacks fire at
// most once per second during the download; total may be -1 if the
// server didn't send Content-Length.
func acquireLlamaServer(brainDir string, progress func(done, total int64)) (string, error) {
	// 1. Bundled with the repo at <repo>/bin/<GOOS>-<GOARCH>/llama-server.
	//    This is the happy path on first clone — zero network, zero risk.
	repoRoot := filepath.Dir(brainDir)
	bundled := filepath.Join(repoRoot, "bin",
		runtime.GOOS+"-"+runtime.GOARCH, "llama-server")
	if runtime.GOOS == "windows" {
		bundled += ".exe"
	}
	if info, err := os.Stat(bundled); err == nil && !info.IsDir() {
		if runtime.GOOS != "windows" {
			_ = os.Chmod(bundled, 0o755)
		}
		return bundled, nil
	}

	// 2. Previously auto-fetched under <brainDir>/runtime/ (older fallback).
	if p, err := findExecutable(filepath.Join(brainDir, "runtime"), "llama-server"); err == nil {
		return p, nil
	}

	// 3. On PATH (developer machines, system installs).
	if onPath, err := exec.LookPath("llama-server"); err == nil {
		return onPath, nil
	}

	// 4. Download from GitHub releases.
	asset := platformAssetName()
	if asset == "" {
		return "", fmt.Errorf("no llama-server release available for %s/%s — please install llama.cpp manually",
			runtime.GOOS, runtime.GOARCH)
	}
	runtimeDir := filepath.Join(brainDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", fmt.Errorf("create runtime dir: %w", err)
	}
	url := fmt.Sprintf("https://github.com/ggml-org/llama.cpp/releases/download/%s/%s", llamaTag, asset)
	archivePath := filepath.Join(runtimeDir, asset)
	if err := downloadWithProgress(url, archivePath, progress); err != nil {
		return "", fmt.Errorf("download runtime: %w", err)
	}
	defer os.Remove(archivePath)

	// Extract by suffix.
	switch {
	case strings.HasSuffix(asset, ".tar.gz"):
		if err := extractTarGz(archivePath, runtimeDir); err != nil {
			return "", fmt.Errorf("extract runtime: %w", err)
		}
	case strings.HasSuffix(asset, ".zip"):
		if err := extractZip(archivePath, runtimeDir); err != nil {
			return "", fmt.Errorf("extract runtime: %w", err)
		}
	default:
		return "", fmt.Errorf("unknown archive format: %s", asset)
	}

	// Locate the extracted llama-server binary (path varies between
	// platforms; release zips put it in different subdirs).
	p, err := findExecutable(runtimeDir, "llama-server")
	if err != nil {
		return "", fmt.Errorf("extracted archive but couldn't find llama-server inside: %w", err)
	}
	return p, nil
}

// platformAssetName maps the current GOOS/GOARCH to llama.cpp's release asset.
// Returns "" if we don't have a mapping for this platform.
func platformAssetName() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return fmt.Sprintf("llama-%s-bin-macos-arm64.tar.gz", llamaTag)
	case "darwin/amd64":
		return fmt.Sprintf("llama-%s-bin-macos-x64.tar.gz", llamaTag)
	case "linux/amd64":
		return fmt.Sprintf("llama-%s-bin-ubuntu-x64.tar.gz", llamaTag)
	case "linux/arm64":
		return fmt.Sprintf("llama-%s-bin-ubuntu-arm64.tar.gz", llamaTag)
	case "windows/amd64":
		return fmt.Sprintf("llama-%s-bin-win-cpu-x64.zip", llamaTag)
	default:
		return ""
	}
}

// findExecutable walks `dir` and returns the first regular file whose
// basename matches `name` (or `name + ".exe"` on Windows). Returns an
// error if nothing matches.
func findExecutable(dir, name string) (string, error) {
	target := name
	if runtime.GOOS == "windows" {
		target = name + ".exe"
	}
	var found string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable subdirs rather than abort
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == target {
			found = p
			return io.EOF // sentinel to stop the walk
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%s not found under %s", target, dir)
	}
	// Make sure it's executable on POSIX (release tars usually preserve
	// the mode, but belt-and-suspenders).
	if runtime.GOOS != "windows" {
		_ = os.Chmod(found, 0o755)
	}
	return found, nil
}

// downloadWithProgress streams url to dest, calling progress at most
// once per second while the body is read.
func downloadWithProgress(url, dest string, progress func(done, total int64)) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer out.Close()
	pw := &progressTickerWriter{
		dest:        out,
		total:       resp.ContentLength,
		progressCb:  progress,
		lastEmitted: time.Now(),
	}
	if _, err := io.Copy(pw, resp.Body); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if progress != nil && resp.ContentLength > 0 {
		progress(resp.ContentLength, resp.ContentLength)
	}
	return nil
}

type progressTickerWriter struct {
	dest        io.Writer
	total       int64
	written     int64
	lastEmitted time.Time
	progressCb  func(done, total int64)
}

func (p *progressTickerWriter) Write(b []byte) (int, error) {
	n, err := p.dest.Write(b)
	p.written += int64(n)
	if p.progressCb != nil && time.Since(p.lastEmitted) >= time.Second {
		p.progressCb(p.written, p.total)
		p.lastEmitted = time.Now()
	}
	return n, err
}

// extractTarGz unpacks a .tar.gz archive into dest. Symlinks and special
// files are skipped (we only need regular files for this use case).
func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		// Refuse paths that try to escape dest (zip-slip / tar-slip).
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || strings.HasPrefix(clean, "/") {
			continue
		}
		target := filepath.Join(dest, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// skip symlinks, devices, etc.
		}
	}
}

// extractZip unpacks a .zip archive into dest. Mirrors extractTarGz's
// path-escape protection.
func extractZip(archive, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		clean := filepath.Clean(f.Name)
		if strings.HasPrefix(clean, "..") || strings.HasPrefix(clean, "/") {
			continue
		}
		target := filepath.Join(dest, clean)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()&0o777)
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = rc.Close()
			_ = out.Close()
			return err
		}
		_ = rc.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}
