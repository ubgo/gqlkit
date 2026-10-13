package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/gqlkit/gqlkit/pkg/writer"
)

// TestGenerateCommandsRejectPositionalArgs drives the real root command the
// way a shell does after expanding an unquoted glob, for both generators.
func TestGenerateCommandsRejectPositionalArgs(t *testing.T) {
	dir, schema := writeSchema(t)
	for _, sub := range []string{"generate", "generate-ts"} {
		rootCmd.SetArgs([]string{sub, "-s", schema, filepath.Join(dir, "b.graphql"), filepath.Join(dir, "c.graphql")})
		err := rootCmd.Execute()
		if err == nil {
			t.Errorf("%s: stray positional args accepted", sub)
			continue
		}
		if !strings.Contains(err.Error(), "quote it") {
			t.Errorf("%s: error lacks the quoting hint: %v", sub, err)
		}
	}
}

// TestGenerateCommandsRejectMissingConfig pins that an explicit -c that does
// not exist is an error, not "no bindings".
func TestGenerateCommandsRejectMissingConfig(t *testing.T) {
	dir, schema := writeSchema(t)
	missing := filepath.Join(dir, "nope.jsonc")

	schemaPath, outputDir, packageName, modulePath, configPath = schema, filepath.Join(dir, "sdk"), "sdk", "ex.com/sdk", missing
	if err := generateCmd.RunE(generateCmd, nil); err == nil || !strings.Contains(err.Error(), "nope.jsonc") {
		t.Errorf("generate: err = %v, want missing-config error", err)
	}
	configPath = ""

	tsSchemaPath, tsOutputDir, tsConfigPath = schema, filepath.Join(dir, "ts"), missing
	if err := generateTSCmd.RunE(generateTSCmd, nil); err == nil || !strings.Contains(err.Error(), "nope.jsonc") {
		t.Errorf("generate-ts: err = %v, want missing-config error", err)
	}
	tsConfigPath = ""
}

// TestNoPositionalArgsAcceptsNone covers the happy path of the validator.
func TestNoPositionalArgsAcceptsNone(t *testing.T) {
	if err := noPositionalArgs(nil, nil); err != nil {
		t.Fatal(err)
	}
}

// TestGenerateCommandsPruneFlags drives --prune / --dry-run through the real
// root command for both generators: dry run keeps a stale generated file,
// --prune removes it, a hand-written file always survives, and --dry-run
// without --prune is refused.
func TestGenerateCommandsPruneFlags(t *testing.T) {
	dir, schema := writeSchema(t)
	t.Cleanup(func() { prune, dryRun, tsPrune, tsDryRun = false, false, false, false })
	for _, c := range []struct {
		sub, ext string
		extra    []string
	}{
		{"generate", ".go", []string{"-m", "ex.com/sdk"}},
		{"generate-ts", ".ts", nil},
	} {
		out := filepath.Join(dir, c.sub)
		run := func(flags ...string) error {
			prune, dryRun, tsPrune, tsDryRun = false, false, false, false
			args := append([]string{c.sub, "-s", schema, "-o", out}, c.extra...)
			rootCmd.SetArgs(append(args, flags...))
			return rootCmd.Execute()
		}
		if err := run(); err != nil {
			t.Fatalf("%s: initial run: %v", c.sub, err)
		}
		stale := filepath.Join(out, "stale"+c.ext)
		hand := filepath.Join(out, "hand"+c.ext)
		if err := os.WriteFile(stale, []byte(writer.BannerLine+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(hand, []byte("// mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := run("--dry-run"); err == nil || !strings.Contains(err.Error(), "--dry-run requires --prune") {
			t.Errorf("%s: --dry-run alone: err = %v", c.sub, err)
		}
		if err := run("--prune", "--dry-run"); err != nil {
			t.Fatalf("%s: dry run: %v", c.sub, err)
		}
		if _, err := os.Stat(stale); err != nil {
			t.Errorf("%s: dry run deleted the stale file", c.sub)
		}
		if err := run("--prune"); err != nil {
			t.Fatalf("%s: prune: %v", c.sub, err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Errorf("%s: stale generated file survived --prune", c.sub)
		}
		if _, err := os.Stat(hand); err != nil {
			t.Errorf("%s: hand-written file removed", c.sub)
		}
	}
}
