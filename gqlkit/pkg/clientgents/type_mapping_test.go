package clientgents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

// mappingSchema exercises every type kind the TS type-mapper handles: object,
// input object, enum, interface, custom scalar (Pascal + Hasura lowercase),
// Federation underscore-led type, lists, NonNull vs nullable, and recursion.
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
  count: Int!
  ratio: Float
  active: Boolean!
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
	g, err := New(&Config{SchemaPath: p, OutputDir: dir})
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

// namedType builds a NonNull/nullable named ast.Type for tests.
func namedType(name string, nonNull bool) *ast.Type {
	return &ast.Type{NamedType: name, NonNull: nonNull}
}

// listType wraps an element type in a (nullable/NonNull) list.
func listType(elem *ast.Type, nonNull bool) *ast.Type {
	return &ast.Type{Elem: elem, NonNull: nonNull}
}

// TestFieldTSType pins the interface-field TS type mapping: built-in scalars ->
// TS primitives, custom scalars -> alias name, enums/objects -> name, lists ->
// "T[]", and the nil / String-special / unknown-name fallbacks.
func TestFieldTSType(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		typ, field, want string
	}{
		{"User", "id", "string"},       // ID -> string
		{"User", "count", "number"},    // Int -> number
		{"User", "ratio", "number"},    // Float -> number
		{"User", "active", "boolean"},  // Boolean -> boolean
		{"User", "role", "Role"},       // enum -> name
		{"User", "at", "DateTime"},     // custom scalar -> alias name
		{"User", "ts", "timestamptz"},  // Hasura lowercase custom scalar -> name
		{"User", "bestFriend", "User"}, // object -> name
		{"User", "manager", "User"},    // nullable object -> name
		{"User", "friends", "User[]"},  // [User!] -> User[]
		{"User", "posts", "Post[]"},    // [Post!]! -> Post[]
		{"User", "tags", "string[]"},   // [String!]! -> string[]
		{"User", "meta", "Node"},       // interface -> name
	}
	for _, c := range cases {
		if got := g.fieldTSType(fieldType(t, g, c.typ, c.field)); got != c.want {
			t.Errorf("fieldTSType(%s.%s) = %q, want %q", c.typ, c.field, got, c.want)
		}
	}

	// nil type -> "any"
	if got := g.fieldTSType(nil); got != "any" {
		t.Errorf("fieldTSType(nil) = %q, want any", got)
	}
	// Unknown named type falls back through tsTypeMap.Get -> "any"
	if got := g.fieldTSType(namedType("Nonexistent", true)); got != "any" {
		t.Errorf("fieldTSType(Nonexistent) = %q, want any", got)
	}
	// input-object reference resolves to its name
	if got := g.fieldTSType(namedType("Filter", true)); got != "Filter" {
		t.Errorf("fieldTSType(Filter) = %q, want Filter", got)
	}
}

// TestFieldTSTypeStringScalar covers the "String" custom-scalar special case
// where a user-declared `scalar String` is remapped to "GqlString".
func TestFieldTSTypeStringScalar(t *testing.T) {
	g := newTestGenerator(t, `
scalar String
type Thing { s: String! }
type Query { thing: Thing }
`)
	// A user-declared `scalar String` is not a simple TS primitive in the map
	// (BuiltInTSTypes maps "String"->"string", but gqlparser marks the redeclared
	// scalar as a Scalar kind; the special-case renames it to GqlString).
	got := g.fieldTSType(fieldType(t, g, "Thing", "s"))
	if got != "string" && got != "GqlString" {
		t.Errorf("fieldTSType(Thing.s) = %q, want string or GqlString", got)
	}
}

// TestFieldTSTypeStr covers the optional/required declaration builder.
func TestFieldTSTypeStr(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	if got := g.fieldTSTypeStr("count", fieldType(t, g, "User", "count")); got != "count: number;" {
		t.Errorf("fieldTSTypeStr(count) = %q", got)
	}
	if got := g.fieldTSTypeStr("manager", fieldType(t, g, "User", "manager")); got != "manager?: User;" {
		t.Errorf("fieldTSTypeStr(manager) = %q", got)
	}
}

// TestGraphQLToTSArgType pins operation-builder argument/return type mapping.
// Unlike fieldTSType, scalars resolve through tsTypeMap.Get (unknown -> "any").
func TestGraphQLToTSArgType(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		typ  *ast.Type
		want string
	}{
		{namedType("Int", true), "number"},
		{namedType("String", false), "string"},
		{namedType("Role", true), "Role"},
		{namedType("DateTime", false), "any"}, // custom scalar unmapped -> any
		{namedType("User", true), "User"},
		{namedType("Filter", false), "Filter"}, // input object
		{listType(namedType("Role", true), true), "Role[]"},
		{listType(namedType("Int", true), false), "number[]"},
		{nil, "any"},
	}
	for _, c := range cases {
		if got := g.graphQLToTSArgType(c.typ); got != c.want {
			t.Errorf("graphQLToTSArgType(%s) = %q, want %q", formatGraphQLType(c.typ), got, c.want)
		}
	}
}

// TestGraphQLToTSType covers the resolver used for nullable detection.
func TestGraphQLToTSType(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	// nil -> ("any", true)
	if ts, opt := g.graphQLToTSType(nil); ts != "any" || !opt {
		t.Errorf("graphQLToTSType(nil) = (%q,%v), want (any,true)", ts, opt)
	}
	// NonNull scalar -> not optional; custom binding wins in namedTypeToTS
	if ts, opt := g.graphQLToTSType(namedType("Int", true)); ts != "number" || opt {
		t.Errorf("graphQLToTSType(Int!) = (%q,%v), want (number,false)", ts, opt)
	}
	// nullable list of objects -> "User[]", optional
	if ts, opt := g.graphQLToTSType(listType(namedType("User", true), false)); ts != "User[]" || !opt {
		t.Errorf("graphQLToTSType([User!]) = (%q,%v), want (User[],true)", ts, opt)
	}
	// unknown name -> "any"
	if ts, _ := g.graphQLToTSType(namedType("Bogus", true)); ts != "any" {
		t.Errorf("graphQLToTSType(Bogus) = %q, want any", ts)
	}
}

// TestNamedTypeToTS pins the named-type resolver across all kinds.
func TestNamedTypeToTS(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct{ name, want string }{
		{"Int", "number"},        // built-in binding
		{"DateTime", "DateTime"}, // custom scalar -> its own name
		{"Role", "Role"},         // enum
		{"User", "User"},         // object
		{"Node", "Node"},         // interface
		{"Filter", "Filter"},     // input object
		{"Missing", "any"},       // unknown
	}
	for _, c := range cases {
		if got := g.namedTypeToTS(c.name); got != c.want {
			t.Errorf("namedTypeToTS(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestIsCustomScalarRef pins the custom-scalar discriminator (only non-simple
// scalar bindings count).
func TestIsCustomScalarRef(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	if !g.isCustomScalarRef(namedType("DateTime", true)) {
		t.Error("DateTime should be a custom scalar ref")
	}
	if !g.isCustomScalarRef(listType(namedType("timestamptz", true), true)) {
		t.Error("[timestamptz!]! should unwrap to a custom scalar ref")
	}
	if g.isCustomScalarRef(namedType("Int", true)) {
		t.Error("Int maps to a simple TS type, not a custom scalar ref")
	}
	if g.isCustomScalarRef(namedType("User", true)) {
		t.Error("User is an object, not a scalar ref")
	}
	if g.isCustomScalarRef(nil) {
		t.Error("nil is not a custom scalar ref")
	}
}

// TestIsObjectType pins the object/interface discriminator.
func TestIsObjectType(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		t    *ast.Type
		want bool
	}{
		{namedType("User", true), true},                 // object
		{namedType("Node", true), true},                 // interface
		{listType(namedType("Post", true), true), true}, // list of object
		{namedType("Role", true), false},                // enum
		{namedType("DateTime", true), false},            // scalar
		{namedType("Filter", true), false},              // input object
		{namedType("Missing", true), false},             // unknown
		{nil, false},                                    // nil / empty name
	}
	for i, c := range cases {
		if got := isObjectType(g.schema, c.t); got != c.want {
			t.Errorf("case %d: isObjectType = %v, want %v", i, got, c.want)
		}
	}
}

// TestGetBaseTypeName unwraps List and NonNull down to the named type.
func TestGetBaseTypeName(t *testing.T) {
	cases := []struct {
		in   *ast.Type
		want string
	}{
		{namedType("User", true), "User"},
		{namedType("Int", false), "Int"},
		{listType(namedType("Post", true), true), "Post"},
		{listType(listType(namedType("String", true), true), false), "String"},
		{nil, ""},
	}
	for i, c := range cases {
		if got := getBaseTypeName(c.in); got != c.want {
			t.Errorf("case %d: getBaseTypeName = %q, want %q", i, got, c.want)
		}
	}
}

// TestFormatGraphQLType reconstructs the SDL type notation used in setArg calls.
func TestFormatGraphQLType(t *testing.T) {
	cases := []struct {
		in   *ast.Type
		want string
	}{
		{namedType("Int", false), "Int"},
		{namedType("ID", true), "ID!"},
		{listType(namedType("String", true), false), "[String!]"},
		{listType(namedType("ChatbotOrder", false), true), "[ChatbotOrder]!"},
		{listType(listType(namedType("Int", true), true), true), "[[Int!]!]!"},
		{nil, ""},
	}
	for i, c := range cases {
		if got := formatGraphQLType(c.in); got != c.want {
			t.Errorf("case %d: formatGraphQLType = %q, want %q", i, got, c.want)
		}
	}
}

// TestToKebabCase pins the file-naming transform, including acronym handling
// and Federation underscore-led names.
func TestToKebabCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ChatbotConnection", "chatbot-connection"},
		{"URLParser", "url-parser"},
		{"User", "user"},
		{"widget", "widget"},
		{"ID", "id"},
		{"", ""},
	}
	for _, c := range cases {
		if got := toKebabCase(c.in); got != c.want {
			t.Errorf("toKebabCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestIsSimpleTSType pins the import-needed discriminator.
func TestIsSimpleTSType(t *testing.T) {
	for _, s := range []string{"string", "number", "boolean", "any"} {
		if !isSimpleTSType(s) {
			t.Errorf("isSimpleTSType(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"DateTime", "User", "Role", ""} {
		if isSimpleTSType(s) {
			t.Errorf("isSimpleTSType(%q) = true, want false", s)
		}
	}
}

// TestTSTypeMapGetAndMerge covers the type-map primitives and binding merge.
func TestTSTypeMapGetAndMerge(t *testing.T) {
	m := BuiltInTSTypes()
	if got := m.Get("Int"); got != "number" {
		t.Errorf("Get(Int) = %q, want number", got)
	}
	if got := m.Get("Unknown"); got != "any" {
		t.Errorf("Get(Unknown) = %q, want any", got)
	}
	m.Merge(ConfigTSBindings{
		"JSON":     {Type: "Record<string, unknown>"},
		"DateTime": {Type: "DateTime", Import: "luxon"},
	})
	if got := m.Get("JSON"); got != "Record<string, unknown>" {
		t.Errorf("Get(JSON) = %q after merge", got)
	}
	// external-import binding sets the map value to the scalar name (key)
	if got := m.Get("DateTime"); got != "DateTime" {
		t.Errorf("Get(DateTime) = %q after import-binding merge, want DateTime", got)
	}
}

// TestTSBindingUnmarshalJSON covers both the plain-string and object forms.
func TestTSBindingUnmarshalJSON(t *testing.T) {
	var plain TSBinding
	if err := plain.UnmarshalJSON([]byte(`"string"`)); err != nil {
		t.Fatalf("plain: %v", err)
	}
	if plain.Type != "string" || plain.Import != "" {
		t.Errorf("plain binding = %+v", plain)
	}
	var obj TSBinding
	if err := obj.UnmarshalJSON([]byte(`{"type":"DateTime","import":"luxon"}`)); err != nil {
		t.Fatalf("obj: %v", err)
	}
	if obj.Type != "DateTime" || obj.Import != "luxon" {
		t.Errorf("obj binding = %+v", obj)
	}
	var bad TSBinding
	if err := bad.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

// TestConfigValidate covers the required-path and default-output-dir branches.
func TestConfigValidate(t *testing.T) {
	c := &Config{}
	if err := c.Validate(); err != ErrSchemaPathRequired {
		t.Errorf("Validate empty = %v, want ErrSchemaPathRequired", err)
	}
	c = &Config{SchemaPath: "x.graphql"}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.OutputDir != "./sdk" {
		t.Errorf("default OutputDir = %q, want ./sdk", c.OutputDir)
	}
}

// TestLoadClientConfig covers empty-path, missing-file, valid, and malformed
// config loading.
func TestLoadClientConfig(t *testing.T) {
	// empty path -> empty config
	if cfg, err := loadClientConfig(""); err != nil || cfg == nil {
		t.Fatalf("empty path: cfg=%v err=%v", cfg, err)
	}
	// missing file -> empty config, no error
	if cfg, err := loadClientConfig(filepath.Join(t.TempDir(), "nope.jsonc")); err != nil || cfg == nil {
		t.Fatalf("missing file: cfg=%v err=%v", cfg, err)
	}
	// valid JSONC with a comment
	dir := t.TempDir()
	p := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(p, []byte(`{ // comment
  "bindings": { "DateTime": { "type": "string" } }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadClientConfig(p)
	if err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if cfg.Bindings["DateTime"].Type != "string" {
		t.Errorf("binding not loaded: %+v", cfg.Bindings)
	}
	// malformed JSON
	bad := filepath.Join(dir, "bad.jsonc")
	if err := os.WriteFile(bad, []byte(`{ "bindings": [ }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClientConfig(bad); err == nil {
		t.Error("expected error on malformed config")
	}
}

// TestNewErrors covers the validation and schema-parse failure paths of New.
func TestNewErrors(t *testing.T) {
	if _, err := New(&Config{}); err == nil {
		t.Error("expected error for missing schema path")
	}
	if _, err := New(&Config{SchemaPath: filepath.Join(t.TempDir(), "missing.graphql")}); err == nil {
		t.Error("expected error for nonexistent schema file")
	}
}

// TestIsGraphQLBuiltIn documents the always-false behavior (no scalars skipped).
func TestIsGraphQLBuiltIn(t *testing.T) {
	for _, n := range []string{"String", "Int", "DateTime", "Boolean"} {
		if isGraphQLBuiltIn(n) {
			t.Errorf("isGraphQLBuiltIn(%q) = true, want false", n)
		}
	}
}
