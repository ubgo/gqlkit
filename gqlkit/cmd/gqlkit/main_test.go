package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const testSDL = `
type Query { me: User }
type User { id: ID! name: String friend: User }
`

// writeSchema writes a minimal SDL to a temp file and returns its path. It also
// chdirs into the temp dir because Generate() drops a schema.json in the cwd.
func writeSchema(t *testing.T) (dir, schemaPath string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	schemaPath = filepath.Join(dir, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(testSDL), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	return dir, schemaPath
}

// TestGenerateCommand runs the Go generate command's RunE end to end.
func TestGenerateCommand(t *testing.T) {
	dir, schema := writeSchema(t)
	out := filepath.Join(dir, "sdk")

	schemaPath, outputDir, packageName, modulePath, configPath = schema, out, "sdk", "ex.com/sdk", ""
	if err := generateCmd.RunE(generateCmd, nil); err != nil {
		t.Fatalf("generate RunE: %v", err)
	}
	for _, f := range []string{"types/types.go", "queries/root.go", "builder/builder.go"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("expected generated file %s: %v", f, err)
		}
	}
}

// TestGenerateCommandRequiresSchema hits the explicit empty-schema guard.
func TestGenerateCommandRequiresSchema(t *testing.T) {
	schemaPath = ""
	if err := generateCmd.RunE(generateCmd, nil); err == nil {
		t.Fatal("generate should error when --schema is empty")
	}
}

// TestGenerateCommandBadSchema covers the New() failure path.
func TestGenerateCommandBadSchema(t *testing.T) {
	t.Chdir(t.TempDir())
	schemaPath, outputDir, packageName, modulePath, configPath = "does-not-exist.graphql", "sdk", "sdk", "ex.com/sdk", ""
	if err := generateCmd.RunE(generateCmd, nil); err == nil {
		t.Fatal("generate should error on a missing schema file")
	}
}

// TestGenerateTSCommand runs the TypeScript generate command end to end.
func TestGenerateTSCommand(t *testing.T) {
	dir, schema := writeSchema(t)
	out := filepath.Join(dir, "tssdk")

	tsSchemaPath, tsOutputDir, tsConfigPath = schema, out, ""
	if err := generateTSCmd.RunE(generateTSCmd, nil); err != nil {
		t.Fatalf("generate-ts RunE: %v", err)
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) == 0 {
		t.Errorf("expected TypeScript SDK files in %s (err=%v)", out, err)
	}
}

// TestGenerateTSCommandBadSchema covers the TS New() failure path.
func TestGenerateTSCommandBadSchema(t *testing.T) {
	t.Chdir(t.TempDir())
	tsSchemaPath, tsOutputDir, tsConfigPath = "nope.graphql", "sdk", ""
	if err := generateTSCmd.RunE(generateTSCmd, nil); err == nil {
		t.Fatal("generate-ts should error on a missing schema file")
	}
}

// TestVersionCommand executes the wired-up root command's version subcommand.
func TestVersionCommand(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
}

// TestRootHasSubcommands checks the command wiring in init().
func TestRootHasSubcommands(t *testing.T) {
	want := map[string]bool{"generate": false, "generate-ts": false, "version": false}
	for _, c := range rootCmd.Commands() {
		want[c.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("root command missing subcommand %q", name)
		}
	}
}
