package clientgents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateHasuraFederationShapes runs the TS generator against the same
// class of "non-conventional" schema the Go generator was hardened for: a custom
// query root name (QueryRoot), Hasura lowercase / underscore-led type and scalar
// names, and a Federation _Service type. TypeScript has no unexported-identifier
// problem (all TS names are usable) and no value-cycle problem, so — unlike Go —
// the only failure mode that actually applies is the "_" placeholder (covered by
// TestGeneratePlaceholderUnderscoreField). This test asserts the output is clean:
// lowercase/underscore names round-trip, and a NON-conventionally-named root is
// generated as a normal type (intended parity with Go — other types may
// reference it, e.g. Job.query: QueryRoot — so it must exist and be selectable).
func TestGenerateHasuraFederationShapes(t *testing.T) {
	dir := runGenerate(t, `
scalar timestamptz
scalar uuid
type widget { id: uuid! createdAt: timestamptz }
type _Service { sdl: String }
type users { id: uuid! name: String widgets: [widget!]! }
schema { query: QueryRoot }
type QueryRoot {
  users: [users!]!
  widget(id: uuid!): widget
  _service: _Service
}
`)

	types := read(t, dir, "types/index.ts")

	// Lowercase + underscore-led type names are emitted verbatim (valid TS
	// identifiers, if unconventional). Assert they at least appear and reference
	// only types the generator also produced.
	mustContain(t, types, "types (hasura/federation)",
		"export interface widget {",
		"export interface users {",
		"export interface _Service {",
	)

	// A non-conventionally-named root IS generated as a normal type + selector —
	// intended parity with the Go generator (which only skips the literal names
	// Query/Mutation/Subscription so that a type referencing QueryRoot resolves).
	if !strings.Contains(types, "interface QueryRoot") {
		t.Error("QueryRoot (non-conventional root) should be generated as a type for referenceability")
	}
	if _, err := os.Stat(filepath.Join(dir, "fields/query-root.ts")); err != nil {
		t.Errorf("QueryRoot field selector should exist: %v", err)
	}
	// Introspection meta-fields must NOT leak into the root interface (no __*).
	if strings.Contains(types, "__schema") || strings.Contains(types, "__type") {
		t.Error("introspection meta-fields leaked into types/index.ts")
	}

	// Operation builders should still be generated off schema.Query (= QueryRoot).
	if _, err := os.Stat(filepath.Join(dir, "queries/users.ts")); err != nil {
		t.Errorf("expected queries/users.ts for QueryRoot.users: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "queries/root.ts")); err != nil {
		t.Errorf("expected queries/root.ts: %v", err)
	}

	// Custom lowercase scalars emitted.
	scalars := read(t, dir, "scalars/index.ts")
	mustContain(t, scalars, "scalars (hasura)",
		"export type timestamptz =", "export type uuid =")

	// Every non-empty type import a generated file references must resolve to a
	// file that exists. Walk the fields/ dir and confirm no dangling ./import.
	assertNoDanglingFieldImports(t, dir)
}

// TestGeneratePlaceholderUnderscoreField is the sharp edge: a Query field
// literally named "_" (a common placeholder in Federation / stub schemas).
// ToPascalCase("_") == "", so before the fix the TS generator emitted
// queries/_.ts with a generic `export class Builder` (colliding across multiple
// placeholders) and an empty GraphQL operation name. skipGenField now drops it,
// mirroring the Go generator.
func TestGeneratePlaceholderUnderscoreField(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	// `_: Boolean` is the canonical "empty type" placeholder GraphQL SDL trick.
	if err := os.WriteFile(schemaPath, []byte(`
type Query {
  _: Boolean
  real: String!
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(work, "out")
	g, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// The "_" placeholder must be skipped entirely — no operation-builder file.
	if _, err := os.Stat(filepath.Join(outDir, "queries", "_.ts")); !os.IsNotExist(err) {
		t.Errorf("queries/_.ts should not be generated for the \"_\" placeholder field (err=%v)", err)
	}

	// The root factory must not reference an empty-named Builder for "_".
	root := read(t, outDir, "queries/root.ts")
	if strings.Contains(root, "export class Builder {") || strings.Contains(root, `from "./_"`) || strings.Contains(root, "_(): Builder") {
		t.Errorf("queries/root.ts still references the placeholder builder:\n%s", root)
	}

	// The genuinely-named field must still generate correctly.
	if _, err := os.Stat(filepath.Join(outDir, "queries", "real.ts")); err != nil {
		t.Errorf("queries/real.ts (the valid field) missing: %v", err)
	}
}

// assertNoDanglingFieldImports parses `import { X } from "./y"` lines in every
// fields/*.ts file and asserts the referenced sibling file exists. A dangling
// import is exactly the "references a type it never generated" failure shape.
func assertNoDanglingFieldImports(t *testing.T, dir string) {
	t.Helper()
	fieldsDir := filepath.Join(dir, "fields")
	entries, err := os.ReadDir(fieldsDir)
	if err != nil {
		t.Fatalf("read fields dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ts") || e.Name() == "index.ts" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(fieldsDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "import ") {
				continue
			}
			idx := strings.Index(line, `"./`)
			if idx == -1 {
				continue
			}
			rest := line[idx+1:]
			end := strings.Index(rest, `"`)
			if end == -1 {
				continue
			}
			rel := rest[:end] // e.g. "./post"
			target := filepath.Join(fieldsDir, strings.TrimPrefix(rel, "./")+".ts")
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s imports %q but %s does not exist (dangling import)", e.Name(), rel, target)
			}
		}
	}
}
