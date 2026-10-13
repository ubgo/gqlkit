package clientgents

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/ubgo/gqlkit/gqlkit/pkg/writer"
)

// tsBanner is the first line of every generated TS file — the same banner
// the Go generator writes (writer.BannerLine), so one rule identifies gqlkit
// output in both SDKs. Prune deletes only files that start with it.
const tsBanner = writer.BannerLine

// maxBannerProbe bounds how much of a file Prune reads to find its first
// line; the banner is far shorter, so a longer first line cannot match.
const maxBannerProbe = 256

// Permissions for generated output: directories are traversable by all,
// files readable by all, both writable only by the owner — the same as a
// checked-out source tree, since the SDK is committed alongside app code.
const (
	outputDirPerm  os.FileMode = 0o755
	outputFilePerm os.FileMode = 0o644
)

// TSWriter handles writing generated TypeScript files to disk. Unlike the Go
// writer, it does not apply any formatting since TypeScript formatting is
// typically handled by external tools (prettier, etc.).
type TSWriter struct {
	outputDir string          // root directory for all generated TS output
	written   map[string]bool // slash-separated paths written this run, for Prune
}

// NewTSWriter returns a writer rooted at outputDir; nothing touches the
// disk until EnsureDir or WriteFile is called.
func NewTSWriter(outputDir string) *TSWriter {
	return &TSWriter{outputDir: outputDir, written: map[string]bool{}}
}

// WriteFile writes content to a file in the output directory, automatically
// creating intermediate directories. No formatting is applied.
func (w *TSWriter) WriteFile(filename, content string) error {
	fullPath := filepath.Join(w.outputDir, filename)

	parentDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(parentDir, outputDirPerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), outputFilePerm); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filename, err)
	}
	w.written[filepath.ToSlash(filepath.Clean(filename))] = true

	return nil
}

// EnsureDir creates the output directory (and parents) if missing. Existing
// files are left alone: generation overwrites, it never cleans.
func (w *TSWriter) EnsureDir() error {
	return os.MkdirAll(w.outputDir, outputDirPerm)
}

// Prune removes stale generated files under the output directory and returns
// their slash-separated relative paths, sorted. Same rule as the Go writer:
// a file is removed only when it is a regular file (symlinks are never
// touched or followed), its first line is exactly tsBanner, and this writer
// did not write it in the current run — so hand-written files always
// survive. Afterwards, any of the SDK's own subdirectories (generatedDirs)
// left empty is removed too; no other directory is ever removed. With dryRun
// the same list is returned and nothing is deleted. A missing output
// directory prunes nothing.
//
// Call it AFTER the run's writes; running it twice is idempotent.
func (w *TSWriter) Prune(dryRun bool) ([]string, error) {
	var stale []string
	err := filepath.WalkDir(w.outputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == w.outputDir && os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(w.outputDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if w.written[rel] {
			return nil
		}
		owned, err := hasTSBanner(path)
		if err != nil || !owned {
			return err
		}
		stale = append(stale, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prune %s: %w", w.outputDir, err)
	}
	sort.Strings(stale)
	if dryRun {
		return stale, nil
	}
	for _, rel := range stale {
		if err := os.Remove(filepath.Join(w.outputDir, filepath.FromSlash(rel))); err != nil {
			return nil, fmt.Errorf("prune %s: %w", rel, err)
		}
	}
	for _, dir := range generatedDirs {
		p := filepath.Join(w.outputDir, dir)
		if entries, err := os.ReadDir(p); err == nil && len(entries) == 0 {
			if err := os.Remove(p); err != nil {
				return nil, fmt.Errorf("prune %s: %w", dir, err)
			}
		}
	}
	return stale, nil
}

// hasTSBanner reports whether the file's first line is exactly tsBanner (a
// trailing "\r" from CRLF files is tolerated).
func hasTSBanner(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	// Read-only handle; a Close error cannot change the probe result.
	defer func() { _ = f.Close() }()
	line, err := bufio.NewReaderSize(f, maxBannerProbe).ReadSlice('\n')
	if err != nil && len(line) == 0 {
		return false, nil // empty file, or first line longer than the probe
	}
	return string(bytes.TrimRight(line, "\r\n")) == tsBanner, nil
}
