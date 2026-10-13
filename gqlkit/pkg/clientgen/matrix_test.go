package clientgen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/gqlkit/gqlkit/pkg/util"

	"github.com/vektah/gqlparser/v2/ast"
)

// This file holds the INPUT-SPACE matrix for the Go type mapper. Line coverage
// was already high when TS issue #5 shipped: the list-suffix line ran under a
// plain `User[]` test, but no test ever combined a union binding with a list.
// So instead of one example per branch, every named-type kind below is crossed
// with every list/nullability wrapper, in every position a type can appear
// (object field, input field, operation return, operation argument), and the
// whole cross product is generated into one SDK that must compile.

// matrixKind is one named-type dimension of the matrix.
type matrixKind struct {
	gql      string // GraphQL type name
	fieldGo  string // expected namedTypeToGo text (types/ and inputs/)
	opGo     string // expected resolveOpType text (queries/ and mutations/)
	isStruct bool   // generated struct: always a pointer as a struct field
	output   bool   // valid as an output type (object field / return)
	input    bool   // valid as an input type (argument / input field)
	zero     string // expected Execute zero value for a non-null, non-list return
}

// matrixKinds covers every Kind the mapper branches on — built-in scalars,
// unbound custom scalars (Pascal and Hasura-lowercase), scalars bound to a
// basic type, a struct, a non-struct named type, a slice, and to packages
// whose names end in "types"/"inputs" (the substring trap), enums, objects,
// interfaces, unions and input objects (incl. lowercase names).
var matrixKinds = []matrixKind{
	{gql: "String", fieldGo: "string", opGo: "string", output: true, input: true, zero: `""`},
	{gql: "Int", fieldGo: "int", opGo: "int", output: true, input: true, zero: "0"},
	{gql: "Float", fieldGo: "float64", opGo: "float64", output: true, input: true, zero: "0"},
	{gql: "Boolean", fieldGo: "bool", opGo: "bool", output: true, input: true, zero: "false"},
	{gql: "ID", fieldGo: "string", opGo: "string", output: true, input: true, zero: `""`},
	{gql: "JSON", fieldGo: "json.RawMessage", opGo: "json.RawMessage", output: true, input: true, zero: "*new(json.RawMessage)"},
	{gql: "DateTime", fieldGo: "scalars.DateTime", opGo: "scalars.DateTime", output: true, input: true, zero: "*new(scalars.DateTime)"},
	{gql: "timestamptz", fieldGo: "scalars.Timestamptz", opGo: "scalars.Timestamptz", output: true, input: true, zero: "*new(scalars.Timestamptz)"},
	{gql: "Stamp", fieldGo: "time.Time", opGo: "time.Time", output: true, input: true, zero: "*new(time.Time)"},
	{gql: "Small", fieldGo: "int16", opGo: "int16", output: true, input: true, zero: "0"},
	{gql: "Num", fieldGo: "json.Number", opGo: "json.Number", output: true, input: true, zero: "*new(json.Number)"},
	{gql: "Bytes", fieldGo: "[]byte", opGo: "[]byte", output: true, input: true, zero: "nil"},
	{gql: "Money", fieldGo: "dbtypes.Money", opGo: "dbtypes.Money", output: true, input: true, zero: "*new(dbtypes.Money)"},
	{gql: "BigP", fieldGo: "*big.Int", opGo: "*big.Int", output: true, input: true, zero: "nil"},
	{gql: "Code", fieldGo: "myinputs.Code", opGo: "myinputs.Code", output: true, input: true, zero: "*new(myinputs.Code)"},
	{gql: "Status", fieldGo: "enums.Status", opGo: "enums.Status", output: true, input: true, zero: "*new(enums.Status)"},
	{gql: "order_by", fieldGo: "enums.Order_by", opGo: "enums.Order_by", output: true, input: true, zero: "*new(enums.Order_by)"},
	{gql: "Obj", fieldGo: "Obj", opGo: "types.Obj", isStruct: true, output: true, zero: "types.Obj{}"},
	{gql: "widget", fieldGo: "Widget", opGo: "types.Widget", isStruct: true, output: true, zero: "types.Widget{}"},
	{gql: "Node", fieldGo: "NodeValue", opGo: "types.NodeValue", isStruct: true, output: true, zero: "types.NodeValue{}"},
	{gql: "Pet", fieldGo: "PetValue", opGo: "types.PetValue", isStruct: true, output: true, zero: "types.PetValue{}"},
	{gql: "In", fieldGo: "In", opGo: "inputs.In", isStruct: true, input: true},
	{gql: "lower_in", fieldGo: "Lower_in", opGo: "inputs.Lower_in", isStruct: true, input: true},
}

// matrixWrappers is every list depth (0, 1, 2) crossed with inner and outer
// nullability that real schemas use. %s is the named type.
var matrixWrappers = []string{
	"%s", "%s!",
	"[%s]", "[%s!]", "[%s]!", "[%s!]!",
	"[[%s]]", "[[%s!]!]!", "[[%s!]]", "[[%s]!]!",
}

// matrixConfig binds the custom scalars. Money/Code live in packages named
// dbtypes / myinputs inside the generated module (written by
// writeMatrixExtPackages) so the SDK builds offline.
const matrixConfig = `{
  // bindings exercising every Go kind a scalar can map to
  "bindings": {
    "Stamp": { "model": "time.Time" },
    "Small": { "model": "int16" },
    "Num":   { "model": "encoding/json.Number" },
    "Bytes": { "model": "[]byte" },
    "Money": { "model": "example.com/matrix/ext/dbtypes.Money" },
    "Code":  { "model": "example.com/matrix/ext/myinputs.Code" },
    "BigP":  { "model": "*math/big.Int" }
  }
}`

const matrixModule = "example.com/matrix"

// slug turns a GraphQL type name into a field-name fragment.
func slug(gql string) string { return strings.ToLower(strings.ReplaceAll(gql, "_", "")) }

// buildMatrixSchema renders the SDL for the full cross product.
func buildMatrixSchema() string {
	var sb strings.Builder
	sb.WriteString(`
scalar DateTime
scalar timestamptz
scalar Stamp
scalar Small
scalar Num
scalar Bytes
scalar Money
scalar Code
scalar BigP
scalar JSON
enum Status { ACTIVE DRAFT in_progress }
enum order_by { asc desc }
interface Node { id: ID! }
type widget { id: ID! }
type A { a: Int }
type B { b: Int }
union Pet = A | B
input lower_in { x: Int }
`)
	// Object with every output kind × wrapper, plus self references.
	sb.WriteString("type Obj implements Node {\n  id: ID!\n  self: Obj\n  selfNN: Obj!\n  selfList: [Obj!]!\n")
	for _, k := range matrixKinds {
		if !k.output {
			continue
		}
		for wi, w := range matrixWrappers {
			fmt.Fprintf(&sb, "  f_%s_%d: %s\n", slug(k.gql), wi, fmt.Sprintf(w, k.gql))
		}
	}
	sb.WriteString("}\n")

	// Input with every input kind × wrapper, plus self references.
	sb.WriteString("input In {\n  self: In\n  selfList: [In!]\n")
	for _, k := range matrixKinds {
		if !k.input {
			continue
		}
		for wi, w := range matrixWrappers {
			fmt.Fprintf(&sb, "  i_%s_%d: %s\n", slug(k.gql), wi, fmt.Sprintf(w, k.gql))
		}
	}
	sb.WriteString("}\n")

	// Query: one return per output kind × wrapper; one field per input kind
	// carrying every wrapper as an argument.
	sb.WriteString("type Query {\n")
	for _, k := range matrixKinds {
		if k.output {
			for wi, w := range matrixWrappers {
				fmt.Fprintf(&sb, "  r_%s_%d: %s\n", slug(k.gql), wi, fmt.Sprintf(w, k.gql))
			}
		}
		if k.input {
			fmt.Fprintf(&sb, "  a_%s(", slug(k.gql))
			for wi, w := range matrixWrappers {
				if wi > 0 {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "w%d: %s", wi, fmt.Sprintf(w, k.gql))
			}
			sb.WriteString("): Int\n")
		}
	}
	sb.WriteString("}\n")

	// Mutation: the same returns with an input-object argument, so the
	// mutation path sees every return shape too.
	sb.WriteString("type Mutation {\n")
	for _, k := range matrixKinds {
		if !k.output {
			continue
		}
		for wi, w := range matrixWrappers {
			fmt.Fprintf(&sb, "  m_%s_%d(in: In!, opt: lower_in): %s\n", slug(k.gql), wi, fmt.Sprintf(w, k.gql))
		}
	}
	sb.WriteString("}\n")
	return sb.String()
}

// wantFieldGo is the specification of graphQLToGoType: lists prepend "[]";
// a generated struct is always a pointer (cycle breaking); otherwise nullable
// becomes a pointer unless the Go type is already a slice.
func wantFieldGo(t *ast.Type, k matrixKind) string {
	if t.Elem != nil {
		return "[]" + wantFieldGo(t.Elem, k)
	}
	if isNilableGoType(k.fieldGo) {
		return k.fieldGo
	}
	if k.isStruct || !t.NonNull {
		return "*" + k.fieldGo
	}
	return k.fieldGo
}

// wantOpGo is the specification of graphQLOpToGoType: like wantFieldGo but
// structs are by value when non-null and list elements are never forced to
// pointers.
func wantOpGo(t *ast.Type, k matrixKind) string {
	if t.Elem != nil {
		return "[]" + wantOpGo(t.Elem, k)
	}
	if !t.NonNull && !isNilableGoType(k.opGo) {
		return "*" + k.opGo
	}
	return k.opGo
}

// matrixKindByName indexes matrixKinds.
func matrixKindByName(t *testing.T, name string) matrixKind {
	t.Helper()
	for _, k := range matrixKinds {
		if k.gql == name {
			return k
		}
	}
	t.Fatalf("no matrix kind %q", name)
	return matrixKind{}
}

// newMatrixGenerator writes the matrix schema + config to a temp dir and
// returns a Generator for it.
func newMatrixGenerator(t *testing.T, outDir string) *Generator {
	t.Helper()
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.graphql")
	cfgPath := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(schemaPath, []byte(buildMatrixSchema()), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(matrixConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if outDir == "" {
		outDir = filepath.Join(dir, "out")
	}
	g, err := New(&Config{SchemaPath: schemaPath, ConfigPath: cfgPath, OutputDir: outDir, PackageName: "matrix", ModulePath: matrixModule})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// TestMatrixStructFieldTypes checks graphQLToGoType for every kind × wrapper
// in both struct contexts: object fields (types/) and input fields (inputs/).
func TestMatrixStructFieldTypes(t *testing.T) {
	g := newMatrixGenerator(t, "")
	checked := 0
	for _, typeName := range []string{"Obj", "In"} {
		for _, f := range g.schema.Types[typeName].Fields {
			k := matrixKindByName(t, getBaseTypeName(f.Type))
			want := wantFieldGo(f.Type, k)
			if got := g.graphQLToGoType(f.Type); got != want {
				t.Errorf("%s.%s: %s → %q, want %q", typeName, f.Name, f.Type.String(), got, want)
			}
			checked++
		}
	}
	// Guard against the matrix silently shrinking (e.g. a filter typo).
	if min := len(matrixWrappers) * 20; checked < min {
		t.Fatalf("only %d struct-field combinations checked, want >= %d", checked, min)
	}
}

// TestMatrixOperationTypes checks, for every query/mutation field, the
// return type, its Execute zero value, and every argument's Go type and
// round-tripped GraphQL annotation.
func TestMatrixOperationTypes(t *testing.T) {
	g := newMatrixGenerator(t, "")
	for _, root := range []struct {
		def  *ast.Definition
		spec opKindSpec
	}{
		{g.schema.Query, opKindSpecs[opKindQuery]},
		{g.schema.Mutation, opKindSpecs[opKindMutation]},
	} {
		for _, f := range root.def.Fields {
			if skipGenField(f.Name) {
				continue
			}
			k := matrixKindByName(t, getBaseTypeName(f.Type))
			data := g.buildOperationBuilderData(root.spec, f, rootGoNames(root.def)[f.Name].Method)

			if want := wantOpGo(f.Type, k); data.ReturnType != want {
				t.Errorf("%s.%s return %s → %q, want %q", root.def.Name, f.Name, f.Type.String(), data.ReturnType, want)
			}
			wantZero := "nil"
			if f.Type.NonNull && f.Type.Elem == nil {
				wantZero = k.zero
			}
			if data.ZeroValue != wantZero {
				t.Errorf("%s.%s zero for %s = %q, want %q", root.def.Name, f.Name, data.ReturnType, data.ZeroValue, wantZero)
			}
			if wantSel := k.gql == "Obj" || k.gql == "widget" || k.gql == "Node" || k.gql == "Pet"; data.HasSelect != wantSel {
				t.Errorf("%s.%s HasSelect = %v, want %v", root.def.Name, f.Name, data.HasSelect, wantSel)
			}

			for i, a := range f.Arguments {
				ak := matrixKindByName(t, getBaseTypeName(a.Type))
				got := data.Arguments[i]
				if want := wantOpGo(a.Type, ak); got.GoType != want {
					t.Errorf("%s(%s: %s) Go type %q, want %q", f.Name, a.Name, a.Type.String(), got.GoType, want)
				}
				if got.GraphQLType != a.Type.String() {
					t.Errorf("%s(%s) GraphQL annotation %q, want %q", f.Name, a.Name, got.GraphQLType, a.Type.String())
				}
				if got.MethodName != util.ToPascalCase(a.Name) {
					t.Errorf("%s(%s) method %q", f.Name, a.Name, got.MethodName)
				}
			}
		}
	}
}

// TestMatrixOperationImports pins that an operation file imports the SDK's
// types/ or inputs/ package exactly when its rendered types reference them —
// decided by schema kind, not by substring-matching the type text (which
// mistook dbtypes.Money / myinputs.Code for the local packages and produced
// "imported and not used").
func TestMatrixOperationImports(t *testing.T) {
	g := newMatrixGenerator(t, "")
	typesPkg := matrixModule + "/" + pkgTypes
	inputsPkg := matrixModule + "/" + pkgInputs
	for _, f := range g.schema.Query.Fields {
		if skipGenField(f.Name) {
			continue
		}
		data := g.buildOperationBuilderData(opKindSpecs[opKindQuery], f, rootGoNames(g.schema.Query)[f.Name].Method)
		imports := map[string]bool{}
		for _, imp := range g.collectOperationImports(f, data) {
			imports[imp] = true
		}
		refsTypes := strings.HasPrefix(strings.TrimLeft(data.ReturnType, "[]*"), pkgTypes+".")
		refsInputs := false
		for _, a := range data.Arguments {
			if strings.HasPrefix(strings.TrimLeft(a.GoType, "[]*"), pkgInputs+".") {
				refsInputs = true
			}
		}
		if imports[typesPkg] != refsTypes {
			t.Errorf("query %s (returns %s): imports types/ = %v, references it = %v", f.Name, data.ReturnType, imports[typesPkg], refsTypes)
		}
		if imports[inputsPkg] != refsInputs {
			t.Errorf("query %s: imports inputs/ = %v, references it = %v", f.Name, imports[inputsPkg], refsInputs)
		}
	}
}

// writeMatrixExtPackages creates the binding target packages inside the
// generated module so `go build` resolves them offline.
func writeMatrixExtPackages(t *testing.T, outDir string) {
	t.Helper()
	pkgs := map[string]string{
		"ext/dbtypes/money.go": "package dbtypes\n\n// Money is a struct binding target.\ntype Money struct{ Cents int64 }\n",
		"ext/myinputs/code.go": "package myinputs\n\n// Code is a non-struct binding target.\ntype Code string\n",
	}
	for rel, src := range pkgs {
		p := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// goBuild runs `go build` for pkgs inside module dir, isolated from any
// enclosing go.work, and returns the combined output.
func goBuild(t *testing.T, dir string, pkgs ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"build"}, pkgs...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// requireGoToolchain skips compile checks under -short or without `go`.
func requireGoToolchain(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping compile of generated SDK under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
}

// matrixConsumer exercises the generated SDK the way an application does:
// selector callbacks into self-referencing and lowercase types, batch with
// both op kinds, and every compile-time contract the templates promise.
const matrixConsumer = `package main

import (
	"context"

	"example.com/matrix/batch"
	"example.com/matrix/fields"
	"example.com/matrix/graphqlclient"
	"example.com/matrix/inputs"
	"example.com/matrix/mutations"
	"example.com/matrix/queries"
)

func main() {
	c := graphqlclient.NewClient("http://127.0.0.1:0")
	q := queries.NewQueryRoot(c)
	m := mutations.NewMutationRoot(c)
	ctx := context.Background()

	q.RObj1().Select(func(f *fields.ObjFields) {
		f.ID().Self(func(s *fields.ObjFields) { s.ID() }).SelfList(nil)
	})
	_, _ = q.RObj1().Execute(ctx)
	_, _ = q.RStatus1().Execute(ctx)
	var dest struct{}
	_ = batch.RunQueries(ctx, &dest, batch.QueryItems{"a": q.RObj0(), "b": q.AStatus().W1("x")})
	_ = batch.RunMutations(ctx, &dest, batch.MutationItems{"a": m.MObj1().In(inputs.In{})})
}
`

// TestMatrixGeneratedSDKCompiles is the codegen gate for the whole matrix:
// generate every kind × wrapper × position, then `go build` the SDK and a
// consumer. Before the zero-value and import fixes this failed with
// "invalid composite literal type" (enums, any-aliased scalars, interfaces,
// int16, json.Number) and "imported and not used" (dbtypes binding).
func TestMatrixGeneratedSDKCompiles(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	outDir := filepath.Join(work, "out")
	g := newMatrixGenerator(t, outDir)
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireGoToolchain(t)

	writeMatrixExtPackages(t, outDir)
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte("module "+matrixModule+"\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "cmd", "consumer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "cmd", "consumer", "main.go"), []byte(matrixConsumer), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := goBuild(t, outDir, "./..."); err != nil {
		t.Fatalf("matrix SDK does not compile: %v\n%s", err, out)
	}

	// Break-verification of the op-kind gate, through the real generated
	// code: a mutation builder in RunQueries (and the dual) must be a
	// COMPILE error. If the marker embeds ever regress to a shared marker,
	// this build starts succeeding and the test fails.
	misuse := map[string]string{
		"mutation-in-queries": `batch.RunQueries(ctx, nil, batch.QueryItems{"a": m.MObj1()})`,
		"query-in-mutations":  `batch.RunMutations(ctx, nil, batch.MutationItems{"a": q.RObj1()})`,
	}
	for name, call := range misuse {
		dir := filepath.Join(outDir, "misuse", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		src := `package main

import (
	"context"

	"example.com/matrix/batch"
	"example.com/matrix/graphqlclient"
	"example.com/matrix/mutations"
	"example.com/matrix/queries"
)

func main() {
	c := graphqlclient.NewClient("http://127.0.0.1:0")
	q, m, ctx := queries.NewQueryRoot(c), mutations.NewMutationRoot(c), context.Background()
	_, _ = q, m
	_ = ` + call + `
}
`
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := goBuild(t, outDir, "./misuse/"+name)
		if err == nil {
			t.Errorf("%s: compiled, but op-kind gating must reject it", name)
			continue
		}
		if !strings.Contains(out, "IsQueryOp") && !strings.Contains(out, "IsMutationOp") {
			t.Errorf("%s: failed for the wrong reason (want missing marker method):\n%s", name, out)
		}
	}
}
