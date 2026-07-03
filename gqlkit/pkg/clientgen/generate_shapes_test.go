package clientgen

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// recursiveSchema reproduces the three failure modes reported on ubgo/gqlkit#4
// after the --module fix, distilled from the Shopify Admin schema:
//
//  1. A non-conventional query root name ("QueryRoot", not "Query") — the
//     generator used to skip root types by hardcoded name, so QueryRoot leaked
//     into types.go carrying the injected __schema / __type meta-fields, which
//     reference the ungenerated builtin __Schema / __Type types.
//  2. Object type cycles through NonNull fields (ProductVariant -> QuantityRule
//     -> ProductVariant) — emitted by value, these are an illegal Go recursive
//     type. They must be pointers.
//  3. Another type referencing the query root by value (Job.query: QueryRoot) —
//     so the root type struct + its field selector must actually be generated,
//     not omitted.
const recursiveSchema = `
schema { query: QueryRoot }

type QueryRoot {
  shop: Shop
  job: Job
}

type Job {
  id: ID!
  query: QueryRoot!
}

type Shop {
  id: ID!
  variant: ProductVariant
}

type ProductVariant {
  id: ID!
  rule: QuantityRule!
  variants: [ProductVariant!]!
}

type QuantityRule {
  min: Int
  variant: ProductVariant!
}
`

// TestGenerateRecursiveAndCustomRootCompiles is the regression guard for the two
// follow-up bugs on ubgo/gqlkit#4. It generates the schema above and asserts the
// output compiles, then checks the specific shapes that were wrong.
func TestGenerateRecursiveAndCustomRootCompiles(t *testing.T) {
	const modulePath = "example.com/shopgql"

	work := t.TempDir()
	t.Chdir(work)

	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(recursiveSchema), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	outDir := filepath.Join(work, "out")
	gen, err := New(&Config{
		SchemaPath:  schemaPath,
		OutputDir:   outDir,
		PackageName: "shopgql",
		ModulePath:  modulePath,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	typesSrc := readFile(t, filepath.Join(outDir, "types", "types.go"))

	// Bug 2a: no introspection meta-fields or meta-types leak into types.go.
	for _, bad := range []string{"__Schema", "__Type", "__schema", "__type", "__typename"} {
		if strings.Contains(typesSrc, bad) {
			t.Errorf("types.go still references introspection meta symbol %q", bad)
		}
	}

	// Bug 2b: the custom-named root type IS generated (Job.query references it),
	// with its real query fields — not skipped.
	if !strings.Contains(typesSrc, "type QueryRoot struct") {
		t.Error("types.go is missing the generated QueryRoot struct (referenced by Job.query)")
	}

	// Bug 1: object-typed fields are pointers, even when NonNull, so the
	// ProductVariant <-> QuantityRule cycle is legal Go. Assert via AST that no
	// struct field's type is a bare (non-pointer, non-slice) generated struct.
	assertObjectFieldsArePointers(t, filepath.Join(outDir, "types", "types.go"))

	// The real test: the whole SDK compiles (self-contained, stdlib only).
	if testing.Short() {
		t.Skip("skipping compile under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH; skipping compile check")
	}
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"),
		[]byte("module "+modulePath+"\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = outDir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated SDK does not compile: %v\n%s", err, out)
	}
}

// lowercaseNamesSchema reproduces the naming bugs found by fuzzing real public
// schemas (Hasura's SpaceX/PokeAPI, Apollo Federation, graphqlzero):
//
//   - Lowercase scalar / enum / object names (Hasura: timestamptz, uuid,
//     order_by) generated UNEXPORTED Go aliases, undefined cross-package.
//   - Apollo Federation types (_Service) are underscore-led — first-letter
//     uppercasing alone leaves them unexported.
//   - A placeholder field named "_" (federation empty-type marker) PascalCases
//     to "", emitting `func (q *QueryRoot) () *Builder` — a syntax error.
const lowercaseNamesSchema = `
schema { query: Query }

scalar timestamptz
enum order_by { asc desc }

type _Service { sdl: String }

type widget {
  id: ID!
  at: timestamptz
  ordering: order_by
  _: Int
}

type Query {
  widget: widget
  _service: _Service
  _: Int
}
`

// TestGenerateLowercaseAndFederationNamesCompile is the regression guard for the
// exported-name and placeholder-field bugs.
func TestGenerateLowercaseAndFederationNamesCompile(t *testing.T) {
	const modulePath = "example.com/hasura"

	work := t.TempDir()
	t.Chdir(work)

	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(lowercaseNamesSchema), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	outDir := filepath.Join(work, "out")
	gen, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir, PackageName: "hasura", ModulePath: modulePath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Generated Go identifiers must be exported (capitalized), even though the
	// GraphQL names are lowercase / underscore-led.
	scalarsSrc := readFile(t, filepath.Join(outDir, "scalars", "scalars.go"))
	if !strings.Contains(scalarsSrc, "Timestamptz") {
		t.Error("scalars.go should export Timestamptz (lowercase scalar not lifted)")
	}
	enumsSrc := readFile(t, filepath.Join(outDir, "enums", "enums.go"))
	if !strings.Contains(enumsSrc, "type Order_by") && !strings.Contains(enumsSrc, "type OrderBy") {
		t.Errorf("enums.go should export the order_by enum type; got:\n%s", enumsSrc)
	}
	// The "_" placeholder field must be dropped everywhere it would produce an
	// invalid identifier: an unnamed struct field in types.go and, critically,
	// an empty method name in queries/root.go (`func (q *QueryRoot) () ...`).
	// go/parser rejects both, so a clean parse proves the placeholder was
	// skipped.
	fset := token.NewFileSet()
	for _, rel := range []string{"types/types.go", "queries/root.go"} {
		p := filepath.Join(outDir, rel)
		if _, err := parser.ParseFile(fset, p, nil, 0); err != nil {
			t.Errorf("%s does not parse (placeholder field leaked?): %v", rel, err)
		}
	}

	// The whole SDK compiles — the real assertion. Every unexported-alias /
	// empty-identifier bug shows up here.
	if testing.Short() {
		t.Skip("skipping compile under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH; skipping compile check")
	}
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"),
		[]byte("module "+modulePath+"\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = outDir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated SDK does not compile: %v\n%s", err, out)
	}
}

// mutationSchema exercises the mutation code path: a mutation root, an input
// object argument, an object return with a selectable body, a lowercase input
// field, and a scalar return — enough to cover generateMutationRootFile and the
// mutation builder generation end to end.
const mutationSchema = `
scalar timestamptz

input CreateUserInput {
  name: String!
  role: String
  created_at: timestamptz
}

type User {
  id: ID!
  name: String
  friend: User
}

type Query { me: User }

type Mutation {
  createUser(input: CreateUserInput!): User!
  deleteUser(id: ID!): Boolean!
  touch(id: ID!): timestamptz
}
`

// TestGenerateMutationsCompile guards the mutation generation path (builders +
// mutations/root.go) which the query-only schemas don't reach.
func TestGenerateMutationsCompile(t *testing.T) {
	const modulePath = "example.com/mut"

	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(mutationSchema), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	outDir := filepath.Join(work, "out")
	gen, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir, PackageName: "mut", ModulePath: modulePath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	root := readFile(t, filepath.Join(outDir, "mutations", "root.go"))
	for _, want := range []string{"type MutationRoot struct", "func (m *MutationRoot) CreateUser()", "func (m *MutationRoot) DeleteUser()", "func (m *MutationRoot) Touch()"} {
		if !strings.Contains(root, want) {
			t.Errorf("mutations/root.go missing %q", want)
		}
	}

	if testing.Short() {
		t.Skip("skipping compile under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"),
		[]byte("module "+modulePath+"\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = outDir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated SDK with mutations does not compile: %v\n%s", err, out)
	}
}

// assertObjectFieldsArePointers parses types.go and fails if any struct field is
// a bare reference to another generated struct in the same package (i.e. an
// identifier with no leading * and not inside a slice). Such value fields are
// what produce Go's "invalid recursive type" error on cyclic object graphs.
func assertObjectFieldsArePointers(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse types.go: %v", err)
	}

	// Collect the set of struct type names declared in this file (the generated
	// object types). Interfaces and scalars are fine as values.
	structNames := map[string]bool{}
	forEachTypeSpec(f, func(ts *goast.TypeSpec) {
		if _, isStruct := ts.Type.(*goast.StructType); isStruct {
			structNames[ts.Name.Name] = true
		}
	})

	forEachTypeSpec(f, func(ts *goast.TypeSpec) {
		st, ok := ts.Type.(*goast.StructType)
		if !ok {
			return
		}
		for _, field := range st.Fields.List {
			// A bare *ast.Ident field type that names another generated struct
			// is a by-value object field — the bug. Pointers (*T) are
			// *goast.StarExpr and slices ([]T) are *goast.ArrayType, so both
			// pass.
			if ident, ok := field.Type.(*goast.Ident); ok && structNames[ident.Name] {
				name := "<anon>"
				if len(field.Names) > 0 {
					name = field.Names[0].Name
				}
				t.Errorf("%s.%s is a by-value struct field %q (must be a pointer to allow recursive types)", ts.Name.Name, name, ident.Name)
			}
		}
	})
}

// forEachTypeSpec calls fn for every top-level type declaration in the file.
func forEachTypeSpec(f *goast.File, fn func(*goast.TypeSpec)) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			if ts, ok := spec.(*goast.TypeSpec); ok {
				fn(ts)
			}
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
