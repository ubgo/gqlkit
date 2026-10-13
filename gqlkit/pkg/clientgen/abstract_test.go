package clientgen

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// abstractSchema covers an interface and a union used as a root result
// (nullable, non-null, list), as object fields (nullable, list) and an
// interface with no implementer.
const abstractSchema = `
interface Node { id: ID! }
interface Lonely { x: Int }
type Todo implements Node { id: ID! text: String! owner: Node pets: [Pet!] }
type User implements Node { id: ID! name: String! }
type Dog { bark: String }
union Pet = Dog | User
type Query {
  node(id: ID!): Node
  nodeNN: Node!
  nodes: [Node!]!
  search: [Pet!]!
  lonely: Lonely
}
`

// abstractProgram decodes every edge case through the generated types and
// prints one line per case; it also prints a selector-built query.
const abstractProgram = `package main

import (
	"encoding/json"
	"fmt"

	"example.com/abs/fields"
	"example.com/abs/graphqlclient"
	"example.com/abs/queries"
	"example.com/abs/types"
)

func show(v types.NodeValue) string {
	switch x := v.Value.(type) {
	case *types.Todo:
		return "Todo:" + x.Text
	case *types.User:
		return "User:" + x.Name
	case nil:
		return fmt.Sprintf("nil typename=%q raw=%s", v.Typename, string(v.Raw))
	}
	return "?"
}

func main() {
	for _, in := range []string{
		` + "`" + `{"__typename":"Todo","id":"1","text":"t"}` + "`" + `,
		` + "`" + `{"__typename":"User","id":"2","name":"u"}` + "`" + `,
		` + "`" + `{"__typename":"Robot","id":"3"}` + "`" + `,
		` + "`" + `{"id":"4"}` + "`" + `,
		` + "`" + `null` + "`" + `,
	} {
		var v types.NodeValue
		if err := json.Unmarshal([]byte(in), &v); err != nil {
			fmt.Println("error", err)
			continue
		}
		fmt.Println(show(v))
	}

	var todo types.Todo
	_ = json.Unmarshal([]byte(` + "`" + `{"id":"1","text":"t","owner":{"__typename":"User","id":"9","name":"o"},"pets":[{"__typename":"Dog","bark":"woof"},{"__typename":"User","id":"5","name":"p"}]}` + "`" + `), &todo)
	fmt.Println(show(*todo.Owner), len(todo.Pets))
	if d, ok := todo.Pets[0].Value.(*types.Dog); ok {
		fmt.Println("dog", *d.Bark)
	}
	var _ types.Pet = &types.User{}
	var _ types.Node = types.User{}

	var round types.NodeValue
	_ = json.Unmarshal([]byte(` + "`" + `{"__typename":"Robot","id":"3"}` + "`" + `), &round)
	b, _ := json.Marshal(round)
	fmt.Println(string(b))

	q := queries.NewQueryRoot(graphqlclient.NewClient("http://127.0.0.1:0"))
	fmt.Print(q.Search().Select(func(f *fields.PetFields) {
		f.OnDog(func(d *fields.DogFields) { d.Bark() }).OnUser(func(u *fields.UserFields) { u.Name() })
	}).BuildQuery())
	fmt.Println()
}
`

// TestAbstractTypesDecodeAndSelect is the build + runtime gate for
// interface/union support: generate, compile, decode known / unknown /
// missing __typename and null, nested and list fields, round-trip an unknown
// member losslessly, and render inline fragments with __typename.
func TestAbstractTypesDecodeAndSelect(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(abstractSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(work, "out")
	g, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir, PackageName: "abs", ModulePath: "example.com/abs"})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}

	// Result types use the wrappers.
	for file, want := range map[string]string{
		"queries/query_node.go":    "(*types.NodeValue, error)",
		"queries/query_node_nn.go": "(types.NodeValue, error)",
		"queries/query_nodes.go":   "([]types.NodeValue, error)",
		"queries/query_search.go":  "([]types.PetValue, error)",
	} {
		if src := readFile(t, filepath.Join(outDir, file)); !strings.Contains(src, want) {
			t.Errorf("%s: Execute does not return %s", file, want)
		}
	}

	requireGoToolchain(t)
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte("module example.com/abs\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "cmd", "abs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "cmd", "abs", "main.go"), []byte(abstractProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/abs")
	cmd.Dir = outDir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("abstract program: %v\n%s", err, out)
	}
	want := strings.Join([]string{
		"Todo:t",
		"User:u",
		`nil typename="Robot" raw={"__typename":"Robot","id":"3"}`,
		`nil typename="" raw={"id":"4"}`,
		`nil typename="" raw=`,
		"User:o 2",
		"dog woof",
		`{"__typename":"Robot","id":"3"}`,
		"query Search {",
		"  search {",
		"    __typename",
		"    ... on Dog {",
		"      bark",
		"    }",
		"    ... on User {",
		"      name",
		"    }",
		"  }",
		"}",
	}, "\n") + "\n"
	if string(out) != want {
		t.Errorf("output mismatch\n got:\n%s\nwant:\n%s", out, want)
	}
}

// TestAbstractWrapperNameCollision pins the up-front error for a schema
// type that already uses a wrapper's Go name.
func TestAbstractWrapperNameCollision(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	sdl := "interface Node { id: ID! } type NodeValue { x: Int } type T implements Node { id: ID! } type Query { n: Node v: NodeValue }"
	if err := os.WriteFile(schemaPath, []byte(sdl), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(&Config{SchemaPath: schemaPath, OutputDir: filepath.Join(work, "out"), PackageName: "c", ModulePath: "example.com/c"})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); !errors.Is(err, ErrGoNameCollision) {
		t.Fatalf("Generate err = %v, want ErrGoNameCollision", err)
	}
}
