package clientgen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tidwall/jsonc"
)

// Config holds all settings needed to run the Go SDK generator, typically
// populated from CLI flags. SchemaPath and ConfigPath are required.
type Config struct {
	// SchemaPath is the path to the GraphQL SDL file. Required.
	SchemaPath string
	// OutputDir is the directory the generated SDK is written to. Defaults
	// to "./sdk".
	OutputDir string
	// PackageName is the SDK's root package name. Defaults to "sdk". If it
	// contains "/" and Package is empty, it is treated as an import path:
	// Package takes the whole value and PackageName its last element.
	PackageName string
	// ModulePath is the import path of the generated SDK root (the CLI's
	// --module). When set and Package is empty, it becomes Package.
	ModulePath string

	// ConfigPath is the optional config.jsonc holding scalar bindings. An
	// empty path or a missing file means "no bindings".
	ConfigPath string

	// Package is the import path every generated cross-package import is
	// rooted at ("<Package>/types", "<Package>/builder", ...). Resolution
	// order in Validate: explicit value, then ModulePath, then a "/"-bearing
	// PackageName, then the nearest go.mod at or above the working directory
	// + OutputDir. It is never empty after a successful Validate.
	Package string

	// Prune, after a successful generation, deletes files under OutputDir
	// that carry the gqlkit banner (writer.BannerLine) as their first line
	// but were not written by this run — leftovers of removed types or
	// operations. Files without the banner are never touched. Off by default.
	Prune bool
	// PruneDryRun makes Prune only report what it would delete. Validate
	// rejects it without Prune, so a dry run is never silently a no-op.
	PruneDryRun bool
}

// Validate checks that required fields are set and applies defaults for
// optional fields (OutputDir defaults to "./sdk", PackageName defaults to "sdk").
// When Package is empty, it is derived from ModulePath, a "/"-bearing
// PackageName, or the nearest go.mod; if none applies it returns
// ErrPackageUndetermined (defaults are still applied).
func (c *Config) Validate() error {
	if c.SchemaPath == "" {
		return ErrSchemaPathRequired
	}
	if c.PruneDryRun && !c.Prune {
		return ErrDryRunWithoutPrune
	}
	if c.OutputDir == "" {
		c.OutputDir = "./sdk"
	}
	if c.PackageName == "" {
		c.PackageName = "sdk"
	}

	// The --module (-m) flag is the import path of the generated SDK root.
	// Every local package (types, enums, scalars, inputs, fields, builder,
	// graphqlclient, batch) is emitted as "<module>/<pkg>", so Package must
	// carry it. Without this wiring the -m value was dropped and cross-package
	// imports came out as "" / "/types", so the generated SDK didn't compile.
	// The dedicated flag wins over the -p-with-slash convenience and go.mod
	// auto-detection below.
	if c.Package == "" && c.ModulePath != "" {
		c.Package = c.ModulePath
	}

	// If --package was given as an import path (contains "/"),
	// use it as the Package import path and extract the package name.
	if c.Package == "" && strings.Contains(c.PackageName, "/") {
		c.Package = c.PackageName
		if idx := strings.LastIndex(c.Package, "/"); idx >= 0 {
			c.PackageName = c.Package[idx+1:]
		}
	}

	// Auto-detect from the enclosing Go module if still empty.
	if c.Package == "" {
		pkg, err := detectPackagePath(c.OutputDir)
		if err != nil {
			return err
		}
		c.Package = pkg
	}
	return nil
}

// detectPackagePath derives the SDK import path from the nearest go.mod at or
// above the working directory: its module path plus OutputDir relative to the
// module root (e.g. "github.com/user/proj" + "sdk" → "github.com/user/proj/sdk",
// or "…/proj/internal/sdk" when run from a sub-directory).
//
// Returns ErrPackageUndetermined when there is no go.mod up the tree, or when
// OutputDir lies outside that module — guessing would emit imports such as
// "/types" that cannot compile, which used to happen silently with exit 0.
func detectPackagePath(outputDir string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPackageUndetermined, err)
	}
	root, mod := findModule(cwd)
	if mod == "" {
		return "", fmt.Errorf("%w: no go.mod found in %s or any parent directory", ErrPackageUndetermined, cwd)
	}
	out, err := filepath.Abs(outputDir)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPackageUndetermined, err)
	}
	rel, err := filepath.Rel(root, out)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: output %s is outside module %s (%s)", ErrPackageUndetermined, out, mod, root)
	}
	if rel == "." {
		return mod, nil
	}
	return mod + "/" + filepath.ToSlash(rel), nil
}

// findModule walks from dir up to the filesystem root and returns the
// directory and module path of the first go.mod that declares a module, or
// ("", "") when none does.
func findModule(dir string) (string, string) {
	for {
		if mod := readModulePath(filepath.Join(dir, "go.mod")); mod != "" {
			return dir, mod
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// readModulePath reads the "module" line from a go.mod file.
func readModulePath(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	// Read-only handle: a Close error cannot affect the parsed result and
	// there is nothing a caller could do about it.
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// loadClientConfig reads and parses a JSONC config file (supports comments)
// into a ClientConfig struct containing scalar-to-Go type bindings.
// If path is empty or the file does not exist, returns an empty config.
func loadClientConfig(path string) (*ClientConfig, error) {
	if path == "" {
		return &ClientConfig{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ClientConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var config ClientConfig
	err = json.Unmarshal(jsonc.ToJSON(content), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config, nil
}
