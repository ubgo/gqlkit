package builder

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// selectionShapes enumerates the selection-tree shapes a generated selector
// can produce: none, flat, nested, deep, an empty child (dropped), and a
// repeated field (GraphQL merges identical selections).
var selectionShapes = map[string]func(*FieldSelection){
	"none": func(*FieldSelection) {},
	"flat": func(s *FieldSelection) { s.AddField("id"); s.AddField("name") },
	"nested": func(s *FieldSelection) {
		s.AddField("id")
		c := NewFieldSelection()
		c.AddField("title")
		s.AddChild("posts", c)
	},
	"deep": func(s *FieldSelection) {
		leaf := NewFieldSelection()
		leaf.AddField("id")
		mid := NewFieldSelection()
		mid.AddChild("parent", leaf)
		s.AddChild("parent", mid)
	},
	"empty child only": func(s *FieldSelection) { s.AddChild("posts", NewFieldSelection()) },
	"fragments": func(s *FieldSelection) {
		s.AddField("__typename")
		u := NewFieldSelection()
		u.AddField("name")
		s.AddFragment("User", u)
		d := NewFieldSelection()
		d.AddField("bark")
		s.AddFragment("Dog", d)
		s.AddFragment("Empty", NewFieldSelection()) // dropped
	},
	"fragment only": func(s *FieldSelection) {
		u := NewFieldSelection()
		u.AddField("id")
		s.AddFragment("User", u)
	},
	"repeated field":   func(s *FieldSelection) { s.AddField("id"); s.AddField("id") },
}

// argSets enumerates argument layouts: none, one, several with list /
// non-null / nested-list type annotations and underscore names.
var argSets = map[string][][2]string{
	"none":  nil,
	"one":   {{"id", "ID!"}},
	"many":  {{"first", "Int"}, {"after", "String"}, {"filter", "UserFilter!"}},
	"lists": {{"ids", "[ID!]!"}, {"grid", "[[Int!]]"}, {"order_by", "[order_by!]"}},
}

// TestBuildQueryAndFragmentMatrix crosses op kind × argument layout ×
// selection shape and checks, for every combination, that BuildQuery and
// GetOpFragment (spliced into a document the way batch does) both parse as
// GraphQL, declare each variable exactly once, pass each argument exactly
// once, and agree with each other on the variable set.
func TestBuildQueryAndFragmentMatrix(t *testing.T) {
	for _, op := range []string{"query", "mutation"} {
		for argName, args := range argSets {
			for selName, shape := range selectionShapes {
				t.Run(fmt.Sprintf("%s/%s/%s", op, argName, selName), func(t *testing.T) {
					b := NewBaseBuilder(nil, op, "Op", "users")
					for i, a := range args {
						b.SetArg(a[0], i, a[1])
					}
					shape(b.GetSelection())

					q := b.BuildQuery()
					if _, err := parser.ParseQuery(&ast.Source{Input: q}); err != nil {
						t.Fatalf("BuildQuery does not parse: %v\n%s", err, q)
					}
					if !strings.HasPrefix(q, op+" Op") {
						t.Errorf("BuildQuery does not start with %q:\n%s", op+" Op", q)
					}

					frag := b.GetOpFragment("al")
					doc := frag.OpType + " Batch"
					if len(frag.VarDecls) > 0 {
						doc += "(" + strings.Join(frag.VarDecls, ", ") + ")"
					}
					doc += " {\n  " + frag.AliasedField + "\n}"
					if _, err := parser.ParseQuery(&ast.Source{Input: doc}); err != nil {
						t.Fatalf("fragment document does not parse: %v\n%s", err, doc)
					}
					if frag.OpType != op {
						t.Errorf("fragment OpType = %q, want %q", frag.OpType, op)
					}

					if len(frag.VarValues) != len(args) || len(b.GetVariables()) != len(args) {
						t.Errorf("variable counts: fragment %d, builder %d, want %d", len(frag.VarValues), len(b.GetVariables()), len(args))
					}
					for i, a := range args {
						if n := strings.Count(q, "$"+a[0]+": "+a[1]); n != 1 {
							t.Errorf("BuildQuery declares $%s %d times", a[0], n)
						}
						if n := strings.Count(q, a[0]+": $"+a[0]); n != 1 {
							t.Errorf("BuildQuery passes %s %d times", a[0], n)
						}
						pref := "al_" + a[0]
						if v, ok := frag.VarValues[pref]; !ok || v != i {
							t.Errorf("fragment value %s = %v (present %v), want %d", pref, v, ok, i)
						}
						if n := strings.Count(doc, "$"+pref+": "+a[1]); n != 1 {
							t.Errorf("fragment declares $%s %d times", pref, n)
						}
					}
				})
			}
		}
	}
}

// TestBuildIgnoresEmptyChildren pins that a child selection with nothing
// selected is dropped instead of rendering an invalid empty `{ }` block, at
// any depth.
func TestBuildIgnoresEmptyChildren(t *testing.T) {
	s := NewFieldSelection()
	s.AddField("id")
	mid := NewFieldSelection()
	mid.AddChild("leaf", NewFieldSelection())
	s.AddChild("mid", mid)
	if got := s.Build(0); got != "id\n" {
		t.Errorf("Build = %q, want only the id line", got)
	}
}

// TestFragmentRendering pins inline-fragment output: after fields and
// children, sorted by type, nested indentation, empty fragments dropped, and a
// zero-value FieldSelection accepting AddFragment.
func TestFragmentRendering(t *testing.T) {
	s := NewFieldSelection()
	s.AddField("__typename")
	c := NewFieldSelection()
	c.AddField("id")
	s.AddChild("owner", c)
	z := &FieldSelection{}
	z.AddField("bark")
	s.AddFragment("Dog", z)
	cat := NewFieldSelection()
	cat.AddField("meow")
	s.AddFragment("Cat", cat)
	s.AddFragment("Empty", NewFieldSelection())
	want := "__typename\nowner {\n  id\n}\n... on Cat {\n  meow\n}\n... on Dog {\n  bark\n}\n"
	if got := s.Build(0); got != want {
		t.Errorf("Build =\n%s\nwant\n%s", got, want)
	}
}

// TestBuilderTemplateMirrorsFieldSelection enforces the two-faces rule for
// the selection runtime: builder.tmpl must carry the same FieldSelection code
// as this package.
func TestBuilderTemplateMirrorsFieldSelection(t *testing.T) {
	pkg, err := os.ReadFile("builder.go")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := os.ReadFile("../templater/template/builder.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	section := func(name, s string) string {
		i, j := strings.Index(s, "// FieldSelection tracks"), strings.Index(s, "// BaseBuilder")
		if i < 0 || j < i {
			t.Fatalf("%s: FieldSelection section not found", name)
		}
		return s[i:j]
	}
	if section("go", string(pkg)) != section("tmpl", string(tmpl)) {
		t.Error("builder.tmpl FieldSelection has drifted from pkg/builder/builder.go")
	}
}
