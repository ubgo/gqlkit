package clientgen

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// lowerFirst turns a Go method name into the camelCase GraphQL name that
// PascalCases back to it.
func lowerFirst(s string) string { return strings.ToLower(s[:1]) + s[1:] }

// collisionSchema crosses every scope where a schema name becomes a Go
// identifier with every way it can clash.
func collisionSchema() string {
	var args []string
	for _, n := range builderReservedNames {
		args = append(args, lowerFirst(n)+": Int!")
	}
	return `
enum Status { ACTIVE active Active IN_PROGRESS inProgress _ }
enum StatusActive { X }
interface Node { id: ID! }
interface Thing { id: ID! onUser: String }
type User implements Node & Thing { id: ID! onUser: String isNode: Boolean isThing: Boolean user_name: String userName: String }
type Dog { bark: String }
union Pet = User | Dog
input In { marshalJSON: String tags: [String!] a_b: Int aB: Int }
type Query {
  q(` + strings.Join(args, ", ") + `, first_name: Int!, firstName: Int!, in: In!): User
  thing: Thing
  user_name: Int
  userName: Int
}
type Mutation { m(` + strings.Join(args, ", ") + `): Boolean }
`
}

// collisionProgram uses every disambiguated name; if any clash survived, the
// SDK (or this program) does not compile.
const collisionProgram = `package main

import (
	"fmt"

	"example.com/col/enums"
	"example.com/col/fields"
	"example.com/col/graphqlclient"
	"example.com/col/inputs"
	"example.com/col/mutations"
	"example.com/col/queries"
	"example.com/col/types"
)

func main() {
	c := graphqlclient.NewClient("http://127.0.0.1:0")
	q := queries.NewQueryRoot(c)
	b := q.Q().Select_(1).Execute_(2).BuildQuery_(3).GetClient_(4).BaseBuilder_(5).
		FirstName(6).FirstName_(7).In(inputs.In{MarshalJSON_: nil, AB: nil, AB_: nil}).
		Select(func(f *fields.UserFields) { f.UserName().UserName_().OnUser().IsNode().IsThing() })
	fmt.Println(b.BuildQuery())
	_ = q.UserName()
	_ = q.UserName_()
	_ = q.Thing().Select(func(f *fields.ThingFields) {
		f.OnUser_().OnUser(func(u *fields.UserFields) { u.ID() })
	})
	_ = mutations.NewMutationRoot(c).M().IsMutationOp_(1).Select_(2)
	// The type StatusActive keeps its name, so all three case variants of
	// ACTIVE are suffixed; "_" maps to "" and so to the taken "Status".
	fmt.Println(enums.StatusActive_, enums.StatusActive__, enums.StatusActive___, enums.StatusInProgress, enums.StatusInprogress, enums.Status_)
	var _ enums.StatusActive = enums.StatusActiveX
	var u types.User
	var _ types.Node = u
	var _ types.Thing = u
	_ = u.IsNode_
}
`

// TestNameCollisionMatrix generates, compiles and runs collisionProgram,
// then checks the wire still uses the original GraphQL names.
func TestNameCollisionMatrix(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(collisionSchema()), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(work, "out")
	g, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir, PackageName: "col", ModulePath: "example.com/col"})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"queries/query_user_name.go", "queries/query_user_name_.go"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Errorf("colliding root fields must get distinct files: %v", err)
		}
	}

	requireGoToolchain(t)
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte("module example.com/col\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "cmd", "col"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "cmd", "col", "main.go"), []byte(collisionProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/col")
	cmd.Dir = outDir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("collision program: %v\n%s", err, out)
	}
	for _, want := range []string{"$select: Int!", "select: $select", "$execute: Int!", "$buildQuery: Int!", "$first_name: Int!", "$firstName: Int!", "userName", "user_name", "isNode"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("query lost GraphQL name %q:\n%s", want, out)
		}
	}
	if !strings.Contains(string(out), "ACTIVE active Active IN_PROGRESS inProgress _") {
		t.Errorf("enum wire values changed:\n%s", out)
	}
}

// TestBuilderReservedNamesComplete keeps builderReservedNames in sync with
// what a generated builder really has: BaseBuilder's methods, the marker
// types and methods (pkg/builder), and the methods operation_builder.tmpl
// declares itself. A new BaseBuilder method that is not reserved would let
// an argument silently shadow it.
func TestBuilderReservedNamesComplete(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "../builder/builder.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"BaseBuilder": true}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *goast.FuncDecl:
			if d.Recv == nil || !d.Name.IsExported() {
				continue
			}
			recv := d.Recv.List[0].Type
			if star, ok := recv.(*goast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*goast.Ident); ok && (id.Name == "BaseBuilder" || strings.HasSuffix(id.Name, "Marker")) {
				want[d.Name.Name] = true
			}
		case *goast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*goast.TypeSpec); ok && strings.HasSuffix(ts.Name.Name, "Marker") {
					want[ts.Name.Name] = true
				}
			}
		}
	}
	tmpl, err := os.ReadFile("../templater/template/operation_builder.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`func \(b \*\{\{ \.BuilderName \}\}\) ([A-Z]\w*)\(`).FindAllStringSubmatch(string(tmpl), -1) {
		want[m[1]] = true
	}
	var missing []string
	for name := range want {
		if !slices.Contains(builderReservedNames, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("builderReservedNames is missing %v", missing)
	}
}

// TestNameClaimer pins the suffix rule.
func TestNameClaimer(t *testing.T) {
	c := newNameClaimer("Select")
	got := []string{c.claim("Select"), c.claim("Select"), c.claim("Other"), c.claim("Other")}
	if want := []string{"Select_", "Select__", "Other", "Other_"}; !slices.Equal(got, want) {
		t.Errorf("claims = %q, want %q", got, want)
	}
}
