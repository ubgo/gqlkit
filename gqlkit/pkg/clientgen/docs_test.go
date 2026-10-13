package clientgen

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docSchema gives every declaration kind a description, single- and
// multi-line, plus one query without a description (fallback text).
const docSchema = `
"""Lifecycle of a record."""
enum Status { ACTIVE }
"""Anything with an id."""
interface Node { id: ID! }
"""
A user account.
Second line.
"""
type User implements Node { id: ID! status: Status }
"""Filter for users."""
input UserFilter { status: Status }
type Query {
  """Fetch one user by id."""
  user(id: ID!): User
  plain: Int
}
type Mutation {
  """Archive a user."""
  archive(f: UserFilter): Boolean
}
`

// typeDocs parses a generated file and returns each type declaration's
// ATTACHED doc text (what go doc and editors show). A comment separated from
// the declaration by a blank line is not attached and so is absent here.
func typeDocs(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	docs := map[string]string{}
	for _, d := range f.Decls {
		gd, ok := d.(*goast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*goast.TypeSpec)
			docs[ts.Name.Name] = gd.Doc.Text()
		}
	}
	return docs
}

// TestDescriptionsAttachToDeclarations is the golden check for descriptions:
// type, interface, input and enum descriptions must be ATTACHED doc comments
// (they used to be followed by a blank line, so go doc dropped them), and a
// root field's description must become its builder's doc comment, with the
// generated sentence kept as the fallback.
func TestDescriptionsAttachToDeclarations(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(docSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(work, "out")
	g, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir, PackageName: "docs", ModulePath: "example.com/docs"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	cases := []struct {
		file, typeName, want string
	}{
		{"types/types.go", "User", "User A user account.\nSecond line."},
		{"types/types.go", "Node", "Node Anything with an id."},
		{"inputs/inputs.go", "UserFilter", "UserFilter Filter for users."},
		{"enums/enums.go", "Status", "Status Lifecycle of a record."},
		{"queries/query_user.go", "UserBuilder", "UserBuilder Fetch one user by id."},
		{"queries/query_plain.go", "PlainBuilder", "PlainBuilder builds a query for plain"},
		{"mutations/mutation_archive.go", "ArchiveMutationBuilder", "ArchiveMutationBuilder Archive a user."},
	}
	for _, c := range cases {
		doc, ok := typeDocs(t, filepath.Join(outDir, c.file))[c.typeName]
		if !ok {
			t.Errorf("%s: type %s not found", c.file, c.typeName)
			continue
		}
		if !strings.HasPrefix(doc, c.want) {
			t.Errorf("%s: %s doc = %q, want prefix %q", c.file, c.typeName, doc, c.want)
		}
	}
}
