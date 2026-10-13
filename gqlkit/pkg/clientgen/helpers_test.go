package clientgen

import (
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

// TestExportName pins the identifier-export transform behind bug #3 (Hasura /
// Federation lowercase + underscore-led type names generated unexported Go
// identifiers). Every case here is a shape seen in a real public schema.
func TestExportName(t *testing.T) {
	cases := []struct{ in, want string }{
		// Already-exported names are untouched — existing SDKs keep identifiers.
		{"JSON", "JSON"},
		{"DateTime", "DateTime"},
		{"ID", "ID"},
		{"URL", "URL"},
		{"User", "User"},
		// Hasura lowercase scalars / enums are lifted.
		{"timestamptz", "Timestamptz"},
		{"uuid", "Uuid"},
		{"order_by", "Order_by"},
		{"jsonb", "Jsonb"},
		// Apollo Federation underscore-led types: leading underscores stripped.
		{"_Service", "Service"},
		{"_Entity", "Entity"},
		{"_Any", "Any"},
		{"__Foo", "Foo"},
		// Digit-led (can't start a Go identifier): deterministic prefix.
		{"3d", "X3d"},
		// Degenerate inputs are returned as-is (caller skips them elsewhere).
		{"", ""},
		{"_", "_"},
		{"__", "__"},
	}
	for _, c := range cases {
		if got := exportName(c.in); got != c.want {
			t.Errorf("exportName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestExportNameAlwaysExportedOrDegenerate is a property check: for any input
// that contains at least one letter, the result must be a usable exported Go
// identifier (first rune upper-case or the deterministic 'X' digit-prefix).
func TestExportNameAlwaysExportedOrDegenerate(t *testing.T) {
	inputs := []string{"a", "z9", "snake_case_name", "_x", "___y", "ALLCAPS", "mixedCase", "_9lives"}
	for _, in := range inputs {
		got := exportName(in)
		if got == "" {
			t.Errorf("exportName(%q) returned empty", in)
			continue
		}
		first := []rune(got)[0]
		exported := (first >= 'A' && first <= 'Z')
		degenerate := got == in && !hasLetter(in) // all-underscore etc.
		if !exported && !degenerate {
			t.Errorf("exportName(%q) = %q is neither exported nor degenerate", in, got)
		}
	}
}

func hasLetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// TestSkipGenField pins the field-skip predicate behind bug #4 (placeholder `_`
// fields → empty identifier) and the introspection meta-field filter (bug #2).
func TestSkipGenField(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"__schema", true},   // introspection meta-field
		{"__type", true},     // introspection meta-field
		{"__typename", true}, // introspection meta-field
		{"_", true},          // placeholder → ToPascalCase("") empty
		{"name", false},      // normal field
		{"id", false},        // normal field
		{"_service", false},  // Federation field → ToPascalCase = "Service", kept
		{"order_by", false},  // snake_case field → PascalCases fine
		{"createdAt", false},
	}
	for _, c := range cases {
		if got := skipGenField(c.in); got != c.want {
			t.Errorf("skipGenField(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestGetZeroValue pins the Execute-method zero-value expressions. The
// interesting rows are the non-struct named types: `T{}` is only a valid
// expression for struct (and slice/map) types, so enums (`type X string`),
// `any` scalar aliases, interface types and non-struct bindings must use
// `*new(T)`. The DateTime row used to pin `scalars.DateTime{}`, which does not
// compile for the `= any` alias an unbound scalar generates.
func TestGetZeroValue(t *testing.T) {
	cases := []struct {
		in       string
		isStruct bool
		want     string
	}{
		{"*types.User", false, "nil"},
		{"*interface{}", false, "nil"},
		{"[]*types.Post", false, "nil"},
		{"[][]string", false, "nil"},
		{"map[string]int", false, "nil"},
		{"interface{}", false, "nil"},
		{"any", false, "nil"},
		{"string", false, `""`},
		{"bool", false, "false"},
		{"int", false, "0"},
		{"int8", false, "0"},
		{"int16", false, "0"},
		{"uint8", false, "0"},
		{"byte", false, "0"},
		{"rune", false, "0"},
		{"uintptr", false, "0"},
		{"float32", false, "0"},
		{"float64", false, "0"},
		{"complex128", false, "0"},
		{"types.User", true, "types.User{}"},
		{"inputs.Filter", true, "inputs.Filter{}"},
		{"types.Node", false, "*new(types.Node)"},
		{"enums.Status", false, "*new(enums.Status)"},
		{"scalars.DateTime", false, "*new(scalars.DateTime)"},
		{"time.Time", false, "*new(time.Time)"},
		{"json.Number", false, "*new(json.Number)"},
		{"json.RawMessage", false, "*new(json.RawMessage)"},
		{"error", false, "*new(error)"},
	}
	for _, c := range cases {
		if got := getZeroValue(c.in, c.isStruct); got != c.want {
			t.Errorf("getZeroValue(%q, %v) = %q, want %q", c.in, c.isStruct, got, c.want)
		}
	}
}

// namedType builds a NonNull/nullable named ast.Type for tests.
func namedType(name string, nonNull bool) *ast.Type {
	return &ast.Type{NamedType: name, NonNull: nonNull}
}

// listType wraps an element type in a (nullable/NonNull) list.
func listType(elem *ast.Type, nonNull bool) *ast.Type {
	return &ast.Type{Elem: elem, NonNull: nonNull}
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

// TestFormatGraphQLType reconstructs the SDL type notation (used for SetArg
// variable declarations). Round-trips list + non-null combinations.
func TestFormatGraphQLType(t *testing.T) {
	cases := []struct {
		in   *ast.Type
		want string
	}{
		{namedType("Int", false), "Int"},
		{namedType("ID", true), "ID!"},
		{listType(namedType("String", true), false), "[String!]"},
		{listType(namedType("AiModelOrder", false), true), "[AiModelOrder]!"},
		{listType(listType(namedType("Int", true), true), true), "[[Int!]!]!"},
		{nil, ""},
	}
	for i, c := range cases {
		if got := formatGraphQLType(c.in); got != c.want {
			t.Errorf("case %d: formatGraphQLType = %q, want %q", i, got, c.want)
		}
	}
}
