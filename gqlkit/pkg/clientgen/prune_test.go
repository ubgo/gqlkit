package clientgen

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestGeneratePruneAcrossSchemaChange runs the real generator twice: first
// with a schema that has a Bar type and a bar query, then with both removed.
// With Prune, the second run deletes exactly the generated files for Bar and
// leaves a hand-written file in the SDK dir alone; dry run only lists.
func TestGeneratePruneAcrossSchemaChange(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	outDir := filepath.Join(work, "out")
	gen := func(sdl string, prune, dry bool) *Generator {
		t.Helper()
		if err := os.WriteFile(schemaPath, []byte(sdl), 0o644); err != nil {
			t.Fatal(err)
		}
		g, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir, PackageName: "p", ModulePath: "example.com/p", Prune: prune, PruneDryRun: dry})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := g.Generate(); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return g
	}

	gen("type Foo { id: ID! } type Bar { id: ID! } type Query { foo: Foo bar: Bar } type Mutation { touch: Boolean }", false, false)
	hand := filepath.Join(outDir, "queries", "custom.go")
	if err := os.WriteFile(hand, []byte("package queries\n\n// hand-written helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	after := "type Foo { id: ID! } type Query { foo: Foo }"
	// The Mutation root disappears too, so mutations/ is emptied and goes.
	want := []string{"fields/field_bar.go", "mutations/", "mutations/mutation_touch.go", "mutations/root.go", "queries/query_bar.go"}

	if got := gen(after, true, true).PrunedFiles(); !slices.Equal(got, want) {
		t.Fatalf("dry run listed %q, want %q", got, want)
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("dry run removed %s", rel)
		}
	}

	if got := gen(after, true, false).PrunedFiles(); !slices.Equal(got, want) {
		t.Fatalf("pruned %q, want %q", got, want)
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(outDir, rel)); !os.IsNotExist(err) {
			t.Errorf("%s still exists", rel)
		}
	}
	if _, err := os.Stat(hand); err != nil {
		t.Errorf("hand-written file removed: %v", err)
	}
	if got := gen(after, true, false).PrunedFiles(); len(got) != 0 {
		t.Errorf("second prune removed %q; want idempotent", got)
	}
}

// TestDryRunRequiresPrune pins that --dry-run alone is refused.
func TestDryRunRequiresPrune(t *testing.T) {
	cfg := Config{SchemaPath: "x.graphql", ModulePath: "example.com/x", PruneDryRun: true}
	if err := cfg.Validate(); !errors.Is(err, ErrDryRunWithoutPrune) {
		t.Fatalf("err = %v, want ErrDryRunWithoutPrune", err)
	}
}
