package clientgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

// mappingSchema exercises every type kind the Go type-mapper must handle:
// object, input object, enum, interface, custom scalar (Pascal + Hasura
// lowercase), Federation underscore-led type, lists, and NonNull vs nullable.
const mappingSchema = `
scalar DateTime
scalar timestamptz
enum Role { ADMIN USER }
interface Node { id: ID! }
input Filter { q: String nested: Filter req: Filter! }
type widget { id: ID! }
type _Service { sdl: String }
type Post { id: ID! }
type User {
  id: ID!
  role: Role!
  at: DateTime!
  bestFriend: User!
  manager: User
  friends: [User!]
  posts: [Post!]!
  tags: [String!]!
  ts: timestamptz
  meta: Node
}
type Query { me: User }
`

// newTestGenerator parses an SDL string and returns a Generator ready for
// type-mapping assertions. It writes the schema to a temp file because New()
// loads schemas by path.
func newTestGenerator(t *testing.T, sdl string) *Generator {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "schema.graphql")
	if err := os.WriteFile(p, []byte(sdl), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	g, err := New(&Config{SchemaPath: p, OutputDir: dir, PackageName: "sdk", ModulePath: "ex.com/sdk"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// fieldType returns the AST type of typeName.fieldName, failing if absent.
func fieldType(t *testing.T, g *Generator, typeName, fieldName string) *ast.Type {
	t.Helper()
	def := g.schema.Types[typeName]
	if def == nil {
		t.Fatalf("type %q not in schema", typeName)
	}
	for _, f := range def.Fields {
		if f.Name == fieldName {
			return f.Type
		}
	}
	t.Fatalf("field %q not on type %q", fieldName, typeName)
	return nil
}

// TestGraphQLToGoTypePointers is the core guard for bug #1: object / input
// fields must be pointers regardless of nullability, so recursive object graphs
// are legal Go. These assertions are binding-independent (objects/inputs never
// have scalar bindings).
func TestGraphQLToGoTypePointers(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		typ, field, want string
	}{
		{"User", "bestFriend", "*User"},        // NonNull object -> pointer
		{"User", "manager", "*User"},           // nullable object -> pointer
		{"User", "friends", "[]*User"},         // [User!] -> slice of pointers
		{"User", "posts", "[]*Post"},           // [Post!]! -> slice of pointers
		{"Filter", "nested", "*Filter"},        // recursive input, nullable
		{"Filter", "req", "*Filter"},           // recursive input, NonNull -> still pointer
	}
	for _, c := range cases {
		got := g.graphQLToGoType(fieldType(t, g, c.typ, c.field))
		if got != c.want {
			t.Errorf("graphQLToGoType(%s.%s) = %q, want %q", c.typ, c.field, got, c.want)
		}
	}

	// Scalar list stays a value slice (a []T cycle is already legal).
	if got := g.graphQLToGoType(fieldType(t, g, "User", "tags")); got != "[]string" && got != "[]*string" {
		// tags: [String!]! — element non-null scalar. Element is not a struct,
		// so it stays by value: "[]string".
		if got != "[]string" {
			t.Errorf("graphQLToGoType(User.tags) = %q, want a string slice", got)
		}
	}
}

// TestIsStructKind pins the object/input discriminator used by the pointer rule.
func TestIsStructKind(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		name string
		want bool
	}{
		{"User", true},         // object
		{"Filter", true},       // input object
		{"Role", false},        // enum
		{"Node", false},        // interface -> not a generated struct field type
		{"DateTime", false},    // scalar (bound)
		{"timestamptz", false}, // scalar (unbound)
		{"Missing", false},     // unknown
		{"", false},            // empty
	}
	for _, c := range cases {
		if got := g.isStructKind(c.name); got != c.want {
			t.Errorf("isStructKind(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestNamedTypeToGoExportsNames is the reference-site guard for bug #3: the
// types-package resolver must emit EXPORTED identifiers for lowercase /
// underscore-led names, matching what the definition sites generate.
func TestNamedTypeToGoExportsNames(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		name, want string
	}{
		{"timestamptz", "scalars.Timestamptz"},
		{"Role", "enums.Role"},
		{"widget", "Widget"},    // lowercase object -> exported
		{"_Service", "Service"}, // Federation -> underscore stripped
		{"Filter", "Filter"},    // input object
	}
	for _, c := range cases {
		if got := g.namedTypeToGo(c.name); got != c.want {
			t.Errorf("namedTypeToGo(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestCustomScalarBinding covers the config-loading path (loadClientConfig) and
// the bindings branch of namedTypeToGo / isStructKind: a scalar bound to a plain
// Go type must render as that type, not a generated alias, and must not be
// treated as a struct (so it is never force-pointered as an object).
func TestCustomScalarBinding(t *testing.T) {
	dir := t.TempDir()
	schema := filepath.Join(dir, "schema.graphql")
	if err := os.WriteFile(schema, []byte(`
scalar UUID
type Thing { id: UUID! code: UUID }
type Query { thing: Thing }
`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.jsonc")
	// JSONC (comments allowed) binding UUID -> string. The binding uses "model"
	// (the fully-qualified Go type); typegql.Build derives GoType from it.
	if err := os.WriteFile(cfgPath, []byte(`{
  // bind the UUID scalar to a plain Go string
  "bindings": { "UUID": { "model": "string" } }
}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	g, err := New(&Config{SchemaPath: schema, OutputDir: dir, PackageName: "sdk", ModulePath: "ex.com/sdk", ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if g.isStructKind("UUID") {
		t.Error("bound scalar UUID should not be a struct kind")
	}
	if got := g.namedTypeToGo("UUID"); got != "string" {
		t.Errorf("namedTypeToGo(UUID) = %q, want string (from binding)", got)
	}
	// NonNull bound scalar -> value; nullable -> pointer to the bound type.
	if got := g.graphQLToGoType(fieldType(t, g, "Thing", "id")); got != "string" {
		t.Errorf("Thing.id = %q, want string", got)
	}
	if got := g.graphQLToGoType(fieldType(t, g, "Thing", "code")); got != "*string" {
		t.Errorf("Thing.code = %q, want *string", got)
	}
}

// TestResolveOpTypeExportsNames is the operation-builder reference-site guard
// for bug #3: return/arg types use "types."/"inputs." prefixes and must also be
// exported so they match the generated type identifiers.
func TestResolveOpTypeExportsNames(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		typ  *ast.Type
		want string
	}{
		{namedType("widget", true), "types.Widget"},
		{namedType("_Service", false), "*types.Service"},
		{namedType("Filter", true), "inputs.Filter"},
		{namedType("timestamptz", false), "*scalars.Timestamptz"},
		// Operation return types don't pointerize object list ELEMENTS — a
		// return value has no struct-cycle problem, so [widget!]! stays a value
		// slice. (Contrast graphQLToGoType, which pointerizes struct fields.)
		{listType(namedType("widget", true), true), "[]types.Widget"},
	}
	for _, c := range cases {
		if got := g.graphQLOpToGoType(c.typ); got != c.want {
			t.Errorf("graphQLOpToGoType(%s) = %q, want %q", formatGraphQLType(c.typ), got, c.want)
		}
	}
}
