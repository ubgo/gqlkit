package clientgents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		{"User", "id", "string"},             // ID -> string
		{"User", "count", "number"},          // Int -> number
		{"User", "ratio", "number | null"},   // nullable Float -> number | null
		{"User", "active", "boolean"},        // Boolean -> boolean
		{"User", "role", "Role"},             // enum -> name
		{"User", "at", "DateTime"},           // custom scalar -> alias name
		{"User", "ts", "timestamptz | null"}, // Hasura lowercase custom scalar -> name
		{"User", "bestFriend", "User"},       // object -> name
		{"User", "manager", "User | null"},   // nullable object -> name | null
		{"User", "friends", "User[] | null"}, // [User!] -> User[] | null
		{"User", "posts", "Post[]"},          // [Post!]! -> Post[]
		{"User", "tags", "string[]"},         // [String!]! -> string[]
		{"User", "meta", "Node | null"},      // nullable interface -> name | null
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
	// The redeclared scalar keeps the built-in "string" binding, which is
	// simple TS text, so it is inlined; only its scalars/ alias is GqlString.
	if got := g.fieldTSType(fieldType(t, g, "Thing", "s")); got != "string" {
		t.Errorf("fieldTSType(Thing.s) = %q, want string", got)
	}
	if got := scalarAliasName("String"); got != "GqlString" {
		t.Errorf("scalarAliasName(String) = %q, want GqlString", got)
	}
}

// TestFieldTSTypeOperationPositions pins the operation-argument/return
// mapping, which shares fieldTSType with interface fields. An unbound custom
// scalar renders as its scalars/ alias here too: a separate resolver once
// rendered it as `any` while the import collector still imported the alias,
// leaving an unused import in the operation file.
func TestFieldTSTypeOperationPositions(t *testing.T) {
	g := newTestGenerator(t, mappingSchema)
	cases := []struct {
		typ  *ast.Type
		want string
	}{
		{namedType("Int", true), "number"},
		{namedType("String", false), "string | null"},
		{namedType("Role", true), "Role"},
		{namedType("DateTime", false), "DateTime | null"}, // unbound custom scalar -> alias
		{namedType("User", true), "User"},
		{namedType("Filter", false), "Filter | null"}, // input object
		{listType(namedType("Role", true), true), "Role[]"},
		{listType(namedType("Int", true), false), "number[] | null"},
		{listType(namedType("Int", false), true), "(number | null)[]"},
		{nil, "any"},
	}
	for _, c := range cases {
		if got := g.fieldTSType(c.typ); got != c.want {
			t.Errorf("fieldTSType(%s) = %q, want %q", formatGraphQLType(c.typ), got, c.want)
		}
	}
}

// TestIsCompositeType pins the composite-type discriminator (object, interface, union).
func TestIsCompositeType(t *testing.T) {
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
		if got := isCompositeType(g.schema, c.t); got != c.want {
			t.Errorf("case %d: isCompositeType = %v, want %v", i, got, c.want)
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

// TestBindingCodecValidate pins the codec-field rules: a codec needs its
// module, a module needs a codec, and names must be plain identifiers.
func TestBindingCodecValidate(t *testing.T) {
	cases := []struct {
		name    string
		b       TSBinding
		wantErr error
	}{
		{"no codec", TSBinding{Type: "string"}, nil},
		{"empty type", TSBinding{}, ErrBindingTypeRequired},
		{"blank type", TSBinding{Type: "  "}, ErrBindingTypeRequired},
		{"decode only", TSBinding{Type: "Date", Decode: "toDate", CodecImport: "../codecs"}, nil},
		{"both", TSBinding{Type: "DateTime", Import: "luxon", Decode: "fromISO", Encode: "toISO", CodecImport: "../codecs"}, nil},
		{"decode without module", TSBinding{Type: "Date", Decode: "toDate"}, ErrCodecImportRequired},
		{"module without codec", TSBinding{Type: "Date", CodecImport: "../codecs"}, ErrCodecWithoutFunctions},
		{"dotted name", TSBinding{Type: "Date", Decode: "DateTime.fromISO", CodecImport: "luxon"}, ErrCodecNameInvalid},
		{"expression", TSBinding{Type: "Date", Encode: "x => x", CodecImport: "../c"}, ErrCodecNameInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.b.Validate("S"); !errors.Is(err, c.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, c.wantErr)
			}
		})
	}
}

// TestLoadClientConfigRejectsGoFormBinding pins the empty-type bug: a
// Go-generator binding in the TS config used to generate `export type X = ;`
// and exit 0. It must fail on load, naming the scalar and the TS form.
func TestLoadClientConfigRejectsGoFormBinding(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.jsonc")
	if err := os.WriteFile(p, []byte(`{ "bindings": { "DateTime": { "model": "time.Time" } } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadClientConfig(p)
	if !errors.Is(err, ErrBindingTypeRequired) {
		t.Fatalf("loadClientConfig = %v, want ErrBindingTypeRequired", err)
	}
	for _, want := range []string{`"DateTime"`, `"type": "DateTime", "import": "luxon"`, `{"model": ...}`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// TestLoadClientConfigRejectsBadCodec checks the rule is enforced on load.
func TestLoadClientConfigRejectsBadCodec(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.jsonc")
	if err := os.WriteFile(p, []byte(`{ "bindings": { "DateTime": { "type": "Date", "decode": "toDate" } } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClientConfig(p); !errors.Is(err, ErrCodecImportRequired) {
		t.Errorf("loadClientConfig = %v, want ErrCodecImportRequired", err)
	}
}
