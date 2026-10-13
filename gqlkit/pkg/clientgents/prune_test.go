package clientgents

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// pruneSchemaV1 has a mutation root and a type that V2 drops, so a V2 run
// leaves stale files in mutations/ and fields/.
const pruneSchemaV1 = `
type Old { id: ID! }
type Keep { id: ID! }
type Query { keep: Keep old: Old }
type Mutation { touch: Boolean }
`

const pruneSchemaV2 = `
type Keep { id: ID! }
type Query { keep: Keep }
`

// pruneRun generates sdl into out with the given prune settings and returns
// the generator, so a test can read PrunedFiles.
func pruneRun(t *testing.T, sdl, out string, prune, dryRun bool) *Generator {
	t.Helper()
	schema := filepath.Join(t.TempDir(), "schema.graphql")
	if err := os.WriteFile(schema, []byte(sdl), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(&Config{SchemaPath: schema, OutputDir: out, Prune: prune, PruneDryRun: dryRun})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return g
}

// writeFile writes content at out/rel, creating parents.
func writeFile(t *testing.T, out, rel, content string) {
	t.Helper()
	p := filepath.Join(out, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// exists reports whether out/rel exists (without following a final symlink).
func exists(out, rel string) bool {
	_, err := os.Lstat(filepath.Join(out, rel))
	return err == nil
}

// TestGeneratedFilesCarryBanner pins that every generated .ts file starts
// with the banner: Prune relies on it to recognise gqlkit output, so a file
// without it would never be cleaned up.
func TestGeneratedFilesCarryBanner(t *testing.T) {
	sdk := generateSDK(t, kitchenSink+`
scalar Instant
input Window { from: Instant }
type Clock { now: Instant }
extend type Query { clock(w: Window): Clock }
`, `{ "bindings": { "Instant": { "type": "Date", "decode": "d", "encode": "e", "codecImport": "../c.js" } } }`)
	count := 0
	err := filepath.WalkDir(sdk, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		count++
		owned, err := hasTSBanner(path)
		if err != nil {
			return err
		}
		if !owned {
			rel, _ := filepath.Rel(sdk, path)
			t.Errorf("%s does not start with %q", rel, tsBanner)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count < 20 {
		t.Fatalf("walked only %d files; expected the full SDK", count)
	}
}

// TestGeneratePrune pins the prune rule end to end: stale generated files
// go, hand-written files and symlinks stay, generated directories left empty
// go unless a hand-written file keeps them, dry-run deletes nothing, and a
// second run prunes nothing.
func TestGeneratePrune(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sdk")
	pruneRun(t, pruneSchemaV1, out, false, false)

	// Hand-written files: one beside generated ones, one in a directory that
	// will lose all its generated files, one with a near-miss banner.
	writeFile(t, out, "queries/custom.ts", "export const x = 1;\n")
	writeFile(t, out, "fields/notes.md", "# notes\n")
	writeFile(t, out, "types/almost.ts", " "+tsBanner+"\n")
	// A symlink to a banner file is never followed or removed.
	if err := os.Symlink(filepath.Join(out, "fields", "keep.ts"), filepath.Join(out, "queries", "link.ts")); err != nil {
		t.Fatal(err)
	}

	wantStale := []string{"fields/old.ts", "mutations/index.ts", "mutations/root.ts", "mutations/touch.ts", "queries/old.ts"}

	dry := pruneRun(t, pruneSchemaV2, out, true, true)
	if !reflect.DeepEqual(dry.PrunedFiles(), wantStale) {
		t.Fatalf("dry-run listed %v, want %v", dry.PrunedFiles(), wantStale)
	}
	for _, rel := range wantStale {
		if !exists(out, rel) {
			t.Errorf("dry-run deleted %s", rel)
		}
	}

	real := pruneRun(t, pruneSchemaV2, out, true, false)
	if !reflect.DeepEqual(real.PrunedFiles(), wantStale) {
		t.Fatalf("prune removed %v, want %v", real.PrunedFiles(), wantStale)
	}
	for _, rel := range wantStale {
		if exists(out, rel) {
			t.Errorf("stale %s survived", rel)
		}
	}
	if exists(out, "mutations") {
		t.Error("emptied generated dir mutations/ was kept")
	}
	for _, rel := range []string{"queries/custom.ts", "fields/notes.md", "types/almost.ts", "queries/link.ts", "fields/keep.ts", "queries/keep.ts"} {
		if !exists(out, rel) {
			t.Errorf("%s was removed", rel)
		}
	}

	again := pruneRun(t, pruneSchemaV2, out, true, false)
	if len(again.PrunedFiles()) != 0 {
		t.Errorf("second prune removed %v, want nothing", again.PrunedFiles())
	}
}

// TestGeneratePruneKeepsDirWithHandWrittenFile pins that a generated dir is
// removed only when empty: a hand-written file in mutations/ keeps it.
func TestGeneratePruneKeepsDirWithHandWrittenFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sdk")
	pruneRun(t, pruneSchemaV1, out, false, false)
	writeFile(t, out, "mutations/extra.ts", "export {};\n")
	pruneRun(t, pruneSchemaV2, out, true, false)
	if !exists(out, "mutations/extra.ts") {
		t.Fatal("hand-written mutations/extra.ts was removed")
	}
	entries, err := os.ReadDir(filepath.Join(out, "mutations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("mutations/ holds %d entries, want only extra.ts", len(entries))
	}
}

// TestGeneratePruneOffByDefault pins that without Prune nothing is deleted.
func TestGeneratePruneOffByDefault(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sdk")
	pruneRun(t, pruneSchemaV1, out, false, false)
	g := pruneRun(t, pruneSchemaV2, out, false, false)
	if len(g.PrunedFiles()) != 0 || !exists(out, "mutations/touch.ts") {
		t.Error("files were pruned without Prune")
	}
}

// TestConfigDryRunRequiresPrune pins that a dry run is never silently a no-op.
func TestConfigDryRunRequiresPrune(t *testing.T) {
	err := (&Config{SchemaPath: "s.graphql", PruneDryRun: true}).Validate()
	if !errors.Is(err, ErrDryRunWithoutPrune) || !strings.Contains(err.Error(), "--prune") {
		t.Errorf("Validate() = %v, want ErrDryRunWithoutPrune", err)
	}
}
