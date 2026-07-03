package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// strp is a small helper returning a pointer to a string literal, used
// throughout the tests to populate the many *string fields on the
// introspection types (descriptions, default values, deprecation reasons).
func strp(s string) *string { return &s }

// named builds a TypeInfo for a named (non-wrapper) type.
func named(kind, name string) TypeInfo {
	n := name
	return TypeInfo{Kind: kind, Name: &n}
}

// nonNull wraps an inner TypeInfo in a NON_NULL wrapper.
func nonNull(inner TypeInfo) TypeInfo {
	return TypeInfo{Kind: "NON_NULL", OfType: &inner}
}

// list wraps an inner TypeInfo in a LIST wrapper.
func list(inner TypeInfo) TypeInfo {
	return TypeInfo{Kind: "LIST", OfType: &inner}
}

func TestFormatType(t *testing.T) {
	tests := []struct {
		name string
		in   TypeInfo
		want string
	}{
		{"named scalar", named("SCALAR", "String"), "String"},
		{"named object", named("OBJECT", "User"), "User"},
		{"non-null scalar", nonNull(named("SCALAR", "String")), "String!"},
		{"list of scalar", list(named("SCALAR", "String")), "[String]"},
		{"non-null list of non-null scalar",
			nonNull(list(nonNull(named("SCALAR", "String")))), "[String!]!"},
		{"deeply nested", nonNull(list(named("OBJECT", "User"))), "[User]!"},
		{"non-null with nil OfType returns empty", TypeInfo{Kind: "NON_NULL"}, ""},
		{"list with nil OfType returns empty", TypeInfo{Kind: "LIST"}, ""},
		{"named with nil Name returns empty", TypeInfo{Kind: "SCALAR"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatType(tt.in); got != tt.want {
				t.Errorf("formatType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsBuiltInScalar(t *testing.T) {
	for _, name := range []string{"Int", "Float", "String", "Boolean", "ID"} {
		if !isBuiltInScalar(name) {
			t.Errorf("isBuiltInScalar(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"DateTime", "JSON", "URL", "Query", ""} {
		if isBuiltInScalar(name) {
			t.Errorf("isBuiltInScalar(%q) = true, want false", name)
		}
	}
}

func TestIsBuiltInDirective(t *testing.T) {
	for _, name := range []string{"skip", "include", "deprecated", "specifiedBy"} {
		if !isBuiltInDirective(name) {
			t.Errorf("isBuiltInDirective(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"auth", "cacheControl", ""} {
		if isBuiltInDirective(name) {
			t.Errorf("isBuiltInDirective(%q) = true, want false", name)
		}
	}
}

func TestEscapeString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{`with "quotes"`, `with \"quotes\"`},
		{`back\slash`, `back\\slash`},
		{`both \ and "`, `both \\ and \"`},
	}
	for _, tt := range tests {
		if got := escapeString(tt.in); got != tt.want {
			t.Errorf("escapeString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWriteDescription(t *testing.T) {
	tests := []struct {
		name   string
		desc   *string
		indent string
		want   string
	}{
		{"nil description writes nothing", nil, "", ""},
		{"empty description writes nothing", strp(""), "", ""},
		{"single line no indent", strp("A user"), "", "\"A user\"\n"},
		{"single line with indent", strp("A field"), "  ", "  \"A field\"\n"},
		{"single line escapes quotes", strp(`say "hi"`), "", "\"say \\\"hi\\\"\"\n"},
		{
			"multi line uses block string",
			strp("line one\nline two"),
			"",
			"\"\"\"\nline one\nline two\n\"\"\"\n",
		},
		{
			"multi line with indent",
			strp("a\nb"),
			"  ",
			"  \"\"\"\n  a\n  b\n  \"\"\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			writeDescription(&sb, tt.desc, tt.indent)
			if got := sb.String(); got != tt.want {
				t.Errorf("writeDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteScalarBuiltInSkipped(t *testing.T) {
	var sb strings.Builder
	writeScalar(&sb, FullType{Kind: "SCALAR", Name: "String"})
	if sb.String() != "" {
		t.Errorf("built-in scalar should be skipped, got %q", sb.String())
	}
}

func TestWriteScalarCustom(t *testing.T) {
	var sb strings.Builder
	writeScalar(&sb, FullType{Kind: "SCALAR", Name: "DateTime", Description: strp("An ISO timestamp")})
	got := sb.String()
	if !strings.Contains(got, "scalar DateTime") {
		t.Errorf("expected custom scalar declaration, got %q", got)
	}
	if !strings.Contains(got, "\"An ISO timestamp\"") {
		t.Errorf("expected description, got %q", got)
	}
}

func TestWriteObject(t *testing.T) {
	obj := FullType{
		Kind:        "OBJECT",
		Name:        "User",
		Description: strp("A user account"),
		Interfaces: []TypeInfo{
			named("INTERFACE", "Node"),
			named("INTERFACE", "Timestamped"),
		},
		Fields: []Field{
			{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
			{Name: "name", Type: named("SCALAR", "String")},
		},
	}
	var sb strings.Builder
	writeObject(&sb, obj)
	got := sb.String()

	wantFragments := []string{
		"\"A user account\"\n",
		"type User implements Node & Timestamped {",
		"  id: ID!\n",
		"  name: String\n",
		"}\n\n",
	}
	for _, frag := range wantFragments {
		if !strings.Contains(got, frag) {
			t.Errorf("writeObject output missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestWriteObjectNoInterfaces(t *testing.T) {
	obj := FullType{
		Kind:   "OBJECT",
		Name:   "Simple",
		Fields: []Field{{Name: "x", Type: named("SCALAR", "Int")}},
	}
	var sb strings.Builder
	writeObject(&sb, obj)
	got := sb.String()
	if strings.Contains(got, "implements") {
		t.Errorf("no interfaces expected, got %q", got)
	}
	if !strings.HasPrefix(got, "type Simple {") {
		t.Errorf("expected 'type Simple {' prefix, got %q", got)
	}
}

func TestWriteInterface(t *testing.T) {
	iface := FullType{
		Kind:        "INTERFACE",
		Name:        "Node",
		Description: strp("An entity with an ID"),
		Interfaces:  []TypeInfo{named("INTERFACE", "Base")},
		Fields: []Field{
			{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
		},
	}
	var sb strings.Builder
	writeInterface(&sb, iface)
	got := sb.String()
	for _, frag := range []string{
		"interface Node implements Base {",
		"  id: ID!\n",
		"\"An entity with an ID\"\n",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("writeInterface missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestWriteUnion(t *testing.T) {
	u := FullType{
		Kind:        "UNION",
		Name:        "SearchResult",
		Description: strp("Any searchable entity"),
		PossibleTypes: []TypeInfo{
			named("OBJECT", "User"),
			named("OBJECT", "Post"),
			{Kind: "OBJECT"}, // nil Name -> empty slot, exercises the guard
		},
	}
	var sb strings.Builder
	writeUnion(&sb, u)
	got := sb.String()
	if !strings.Contains(got, "union SearchResult = User | Post | ") {
		t.Errorf("unexpected union output: %q", got)
	}
	if !strings.Contains(got, "\"Any searchable entity\"\n") {
		t.Errorf("expected description in union output: %q", got)
	}
}

func TestWriteEnum(t *testing.T) {
	e := FullType{
		Kind:        "ENUM",
		Name:        "Status",
		Description: strp("Lifecycle states"),
		EnumValues: []EnumValue{
			{Name: "ACTIVE", Description: strp("Currently active")},
			{Name: "ARCHIVED", IsDeprecated: true, DeprecationReason: strp("use INACTIVE")},
			{Name: "GONE", IsDeprecated: true}, // deprecated without reason
		},
	}
	var sb strings.Builder
	writeEnum(&sb, e)
	got := sb.String()
	for _, frag := range []string{
		"enum Status {",
		"  \"Currently active\"\n",
		"  ACTIVE\n",
		"  ARCHIVED @deprecated(reason: \"use INACTIVE\")\n",
		"  GONE @deprecated\n",
		"}\n\n",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("writeEnum missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestWriteInputObject(t *testing.T) {
	in := FullType{
		Kind:        "INPUT_OBJECT",
		Name:        "CreateUserInput",
		Description: strp("Input for creating a user"),
		InputFields: []InputValue{
			{Name: "name", Type: nonNull(named("SCALAR", "String"))},
			{Name: "role", Type: named("ENUM", "Role"), DefaultValue: strp("USER"), Description: strp("The role")},
			{Name: "active", Type: named("SCALAR", "Boolean"), DefaultValue: strp("true")},
		},
	}
	var sb strings.Builder
	writeInputObject(&sb, in)
	got := sb.String()
	for _, frag := range []string{
		"input CreateUserInput {",
		"  name: String!\n",
		"  \"The role\"\n",
		"  role: Role = USER\n",
		"  active: Boolean = true\n",
		"}\n\n",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("writeInputObject missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestWriteFieldsWithDeprecation(t *testing.T) {
	fields := []Field{
		{Name: "current", Type: named("SCALAR", "String")},
		{Name: "old", Type: named("SCALAR", "String"), IsDeprecated: true, DeprecationReason: strp("gone")},
		{Name: "older", Type: named("SCALAR", "String"), IsDeprecated: true},
		{Name: "described", Description: strp("has doc"), Type: named("SCALAR", "Int")},
		{Name: "user", Type: named("OBJECT", "User"), Args: []InputValue{
			{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
		}},
	}
	var sb strings.Builder
	writeFields(&sb, fields)
	got := sb.String()
	for _, frag := range []string{
		"  current: String\n",
		"  old: String @deprecated(reason: \"gone\")\n",
		"  older: String @deprecated\n",
		"  \"has doc\"\n",
		"  described: Int\n",
		"  user(id: ID!): User\n",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("writeFields missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestWriteArgumentsSingleLine(t *testing.T) {
	// <= 2 args and no descriptions -> single-line format.
	args := []InputValue{
		{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
		{Name: "limit", Type: named("SCALAR", "Int"), DefaultValue: strp("10")},
	}
	var sb strings.Builder
	writeArguments(&sb, args)
	got := sb.String()
	want := "(id: ID!, limit: Int = 10)"
	if got != want {
		t.Errorf("writeArguments single-line = %q, want %q", got, want)
	}
}

func TestWriteArgumentsMultiLineByCount(t *testing.T) {
	// > 2 args triggers multi-line format even without descriptions.
	args := []InputValue{
		{Name: "a", Type: named("SCALAR", "Int")},
		{Name: "b", Type: named("SCALAR", "Int")},
		{Name: "c", Type: named("SCALAR", "Int"), DefaultValue: strp("3")},
	}
	var sb strings.Builder
	writeArguments(&sb, args)
	got := sb.String()
	if !strings.HasPrefix(got, "(\n") {
		t.Errorf("expected multi-line format, got %q", got)
	}
	for _, frag := range []string{
		"    a: Int\n",
		"    b: Int\n",
		"    c: Int = 3\n",
		"  )",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("writeArguments multi-line missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestWriteArgumentsMultiLineByDescription(t *testing.T) {
	// A description on any arg triggers multi-line even with <= 2 args.
	args := []InputValue{
		{Name: "id", Type: nonNull(named("SCALAR", "ID")), Description: strp("the id")},
		{Name: "x", Type: named("SCALAR", "Int")},
	}
	var sb strings.Builder
	writeArguments(&sb, args)
	got := sb.String()
	if !strings.HasPrefix(got, "(\n") {
		t.Errorf("expected multi-line format triggered by description, got %q", got)
	}
	if !strings.Contains(got, "    \"the id\"\n") {
		t.Errorf("expected arg description, got %q", got)
	}
}

func TestWriteArgumentsEmpty(t *testing.T) {
	var sb strings.Builder
	writeArguments(&sb, nil)
	if sb.String() != "" {
		t.Errorf("empty args should write nothing, got %q", sb.String())
	}
}

func TestWriteDirective(t *testing.T) {
	d := Directive{
		Name:        "auth",
		Description: strp("Requires authentication"),
		Locations:   []string{"FIELD_DEFINITION", "OBJECT"},
		Args: []InputValue{
			{Name: "requires", Type: named("ENUM", "Role"), DefaultValue: strp("USER")},
		},
	}
	var sb strings.Builder
	writeDirective(&sb, d)
	got := sb.String()
	for _, frag := range []string{
		"\"Requires authentication\"\n",
		"directive @auth",
		"(requires: Role = USER)",
		" on FIELD_DEFINITION | OBJECT",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("writeDirective missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestWriteDirectiveNoArgsNoLocations(t *testing.T) {
	var sb strings.Builder
	writeDirective(&sb, Directive{Name: "bare"})
	got := sb.String()
	if !strings.Contains(got, "directive @bare\n\n") {
		t.Errorf("unexpected bare directive output: %q", got)
	}
}

func TestConvertToSDLDefaultRootTypesNoSchemaBlock(t *testing.T) {
	schema := &IntrospectionSchema{
		QueryType:    &TypeRef{Name: "Query"},
		MutationType: &TypeRef{Name: "Mutation"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{
				{Name: "me", Type: named("OBJECT", "User")},
			}},
			{Kind: "OBJECT", Name: "User", Fields: []Field{
				{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
			}},
			{Kind: "SCALAR", Name: "String"}, // built-in, skipped
			{Kind: "OBJECT", Name: "__Type"}, // introspection meta-type, skipped
		},
	}
	got := ConvertToSDL(schema)
	if strings.Contains(got, "schema {") {
		t.Errorf("default root type names should not emit a schema block, got:\n%s", got)
	}
	if strings.Contains(got, "scalar String") {
		t.Errorf("built-in scalar String should be skipped, got:\n%s", got)
	}
	if strings.Contains(got, "__Type") {
		t.Errorf("introspection meta-type should be skipped, got:\n%s", got)
	}
	// Alphabetical ordering: Query type sorts before User type.
	if strings.Index(got, "type Query") > strings.Index(got, "type User") {
		t.Errorf("types should be sorted alphabetically, got:\n%s", got)
	}
}

func TestConvertToSDLCustomRootTypesEmitsSchemaBlock(t *testing.T) {
	schema := &IntrospectionSchema{
		QueryType:        &TypeRef{Name: "RootQuery"},
		MutationType:     &TypeRef{Name: "RootMutation"},
		SubscriptionType: &TypeRef{Name: "RootSubscription"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "RootQuery", Fields: []Field{{Name: "ping", Type: named("SCALAR", "String")}}},
		},
	}
	got := ConvertToSDL(schema)
	for _, frag := range []string{
		"schema {",
		"  query: RootQuery\n",
		"  mutation: RootMutation\n",
		"  subscription: RootSubscription\n",
		"}\n\n",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("custom root types should emit schema block; missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestConvertToSDLSkipsBuiltInDirectives(t *testing.T) {
	schema := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types:     []FullType{{Kind: "OBJECT", Name: "Query"}},
		Directives: []Directive{
			{Name: "include", Locations: []string{"FIELD"}},
			{Name: "auth", Locations: []string{"FIELD_DEFINITION"}},
		},
	}
	got := ConvertToSDL(schema)
	if strings.Contains(got, "directive @include") {
		t.Errorf("built-in directive @include should be skipped, got:\n%s", got)
	}
	if !strings.Contains(got, "directive @auth") {
		t.Errorf("custom directive @auth should be emitted, got:\n%s", got)
	}
}

func TestConvertToSDLAllKinds(t *testing.T) {
	// End-to-end: one type of each kind flows through writeType dispatch.
	schema := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{{Name: "ok", Type: named("SCALAR", "Boolean")}}},
			{Kind: "SCALAR", Name: "DateTime"},
			{Kind: "INTERFACE", Name: "Node", Fields: []Field{{Name: "id", Type: nonNull(named("SCALAR", "ID"))}}},
			{Kind: "UNION", Name: "Media", PossibleTypes: []TypeInfo{named("OBJECT", "Photo")}},
			{Kind: "ENUM", Name: "Color", EnumValues: []EnumValue{{Name: "RED"}}},
			{Kind: "INPUT_OBJECT", Name: "Filter", InputFields: []InputValue{{Name: "q", Type: named("SCALAR", "String")}}},
		},
	}
	got := ConvertToSDL(schema)
	for _, frag := range []string{
		"scalar DateTime",
		"interface Node {",
		"union Media = Photo",
		"enum Color {",
		"input Filter {",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("ConvertToSDL missing %q\nfull:\n%s", frag, got)
		}
	}
}

func TestSaveToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.graphql")
	sdl := "type Query {\n  ok: Boolean\n}\n"
	if err := SaveToFile(sdl, path); err != nil {
		t.Fatalf("SaveToFile error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back file: %v", err)
	}
	if string(data) != sdl {
		t.Errorf("file contents = %q, want %q", string(data), sdl)
	}
}

func TestSaveAsJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	schema := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types:     []FullType{{Kind: "OBJECT", Name: "Query"}},
	}
	if err := SaveAsJSON(schema, path); err != nil {
		t.Fatalf("SaveAsJSON error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back file: %v", err)
	}
	content := string(data)
	// Round-trips through the wrapper: data.__schema envelope must be present.
	for _, frag := range []string{"\"__schema\"", "\"queryType\"", "\"Query\""} {
		if !strings.Contains(content, frag) {
			t.Errorf("SaveAsJSON output missing %q\nfull:\n%s", frag, content)
		}
	}
	// Pretty-printed with two-space indentation.
	if !strings.Contains(content, "\n  ") {
		t.Errorf("expected indented JSON, got:\n%s", content)
	}
}
