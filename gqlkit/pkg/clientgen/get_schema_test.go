package clientgen

import (
	"os"
	"path/filepath"
	"testing"
)

// Pins clientgen.Generator.GetSchema (clientgen.go:81).
//
// It is the generator's only read-only door onto the parsed schema, and
// the reason it exists is external: callers that embed gqlkit (and the
// TypeScript generator, which reuses the same parse) inspect types through
// it instead of re-parsing the SDL. A GetSchema that returned nil, or a
// fresh empty schema, would leave this module's own tests entirely green
// while every embedder silently saw a schema with no types in it.
//
// So this asserts the two things an embedder depends on: the value is
// non-nil, and it is the SAME parsed schema the generator uses — carrying
// the types actually declared in the SDL.
func TestGetSchemaReturnsTheParsedSchema(t *testing.T) {
	work := t.TempDir()
	schemaPath := filepath.Join(work, "schema.graphql")
	const sdl = `
type Query {
  user(id: ID!): User
}

type User {
  id: ID!
  email: String!
}
`
	if err := os.WriteFile(schemaPath, []byte(sdl), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	gen, err := New(&Config{
		SchemaPath:  schemaPath,
		OutputDir:   filepath.Join(work, "out"),
		PackageName: "sdk",
		ModulePath:  "example.com/demo",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	schema := gen.GetSchema()
	if schema == nil {
		t.Fatal("GetSchema returned nil — embedders would have no schema to inspect")
	}
	if schema.Types["User"] == nil {
		t.Fatal("GetSchema returned a schema without the declared User type")
	}
	if got := schema.Query.Name; got != "Query" {
		t.Fatalf("GetSchema query root = %q, want %q", got, "Query")
	}

	// Identity, not just equivalence: a copy would drift from whatever the
	// generator mutates during Generate.
	if gen.GetSchema() != schema {
		t.Fatal("GetSchema returned a different pointer on the second call — it must expose the generator's own schema")
	}
}
