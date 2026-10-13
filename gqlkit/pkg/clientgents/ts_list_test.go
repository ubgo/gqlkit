package clientgents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTSListOf pins the precedence rule behind issue #5: "[]" binds tighter
// than "|", "&", "=>", "?:" and the type operators, so those elements must be
// parenthesised, while simple references keep the bare suffix.
func TestTSListOf(t *testing.T) {
	cases := []struct {
		name, elem, want string
	}{
		{"simple name", "string", "string[]"},
		{"qualified name", "Scalars.ID", "Scalars.ID[]"},
		{"already a list", "Todo[]", "Todo[][]"},
		{"union", "string | number", "(string | number)[]"},
		{"union of literals", `"a" | "b"`, `("a" | "b")[]`},
		{"intersection", "A & B", "(A & B)[]"},
		{"function type", "(x: string) => void", "((x: string) => void)[]"},
		{"constructor type", "new () => Foo", "(new () => Foo)[]"},
		{"conditional type", "T extends string ? A : B", "(T extends string ? A : B)[]"},
		{"keyof", "keyof Foo", "(keyof Foo)[]"},
		{"typeof", "typeof foo", "(typeof foo)[]"},
		{"readonly", "readonly string[]", "(readonly string[])[]"},
		{"generic with union arg", "Array<string | number>", "Array<string | number>[]"},
		{"record generic", "Record<string, unknown>", "Record<string, unknown>[]"},
		{"generic with arrow arg", "Array<() => void>", "Array<() => void>[]"},
		{"already parenthesised", "(string | number)", "(string | number)[]"},
		{"two parenthesised groups", "(A) | (B)", "((A) | (B))[]"},
		{"object literal", "{ a: string }", "{ a: string }[]"},
		{"object literal with union member", "{ a: 'x' | 'y' }", "{ a: 'x' | 'y' }[]"},
		{"tuple", "[string, number]", "[string, number][]"},
		{"quoted pipe", `"a|b"`, `"a|b"[]`},
		{"surrounding whitespace", "  number ", "number[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tsListOf(c.elem); got != c.want {
				t.Errorf("tsListOf(%q) = %q, want %q", c.elem, got, c.want)
			}
		})
	}
}

// TestTSOrNull pins where "| null" needs parentheses: only after a
// top-level arrow or conditional, which would otherwise absorb it.
func TestTSOrNull(t *testing.T) {
	cases := []struct{ elem, want string }{
		{"string", "string | null"},
		{"string | number", "string | number | null"},
		{"A & B", "A & B | null"},
		{"keyof Foo", "keyof Foo | null"},
		{"Todo[]", "Todo[] | null"},
		{"() => string", "(() => string) | null"},
		{"T extends string ? A : B", "(T extends string ? A : B) | null"},
		{"(() => string)", "(() => string) | null"},
		{"Array<() => void>", "Array<() => void> | null"},
	}
	for _, c := range cases {
		if got := tsOrNull(c.elem); got != c.want {
			t.Errorf("tsOrNull(%q) = %q, want %q", c.elem, got, c.want)
		}
	}
}

// TestGenerateUnionBindingInLists runs the real Generate path with a union
// scalar binding and checks every surface that renders a GraphQL list:
// object fields, input fields, field selectors, op arguments and op returns,
// across non-null, nullable and nested lists.
func TestGenerateUnionBindingInLists(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(`
input ItemFilter { idIn: [ID!] maybeIds: [ID] matrix: [[ID!]!] names: [String!] }
type Group { memberIds: [ID!]! maybe: [ID] matrix: [[ID!]!] tags: [String!]! }
type Query {
  itemIds(ids: [ID!]!, filter: ItemFilter): [ID!]!
  grid(rows: [[ID!]!]!): [[ID!]!]!
  names(ns: [String!]!): [String!]!
  group: Group
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(work, "config.jsonc")
	if err := os.WriteFile(cfgPath, []byte(`{ "bindings": { "ID": "string | number" } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(work, "out")
	g, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir, ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := read(t, outDir, "types/index.ts")
	mustContain(t, types, "object fields",
		"memberIds: (string | number)[];",
		"maybe?: (string | number | null)[] | null;",
		"matrix?: (string | number)[][] | null;",
		"tags: string[];", // plain bindings keep the bare suffix
	)
	inputs := read(t, outDir, "inputs/index.ts")
	mustContain(t, inputs, "input fields",
		"idIn?: (string | number)[] | null;",
		"maybeIds?: (string | number | null)[] | null;",
		"matrix?: (string | number)[][] | null;",
		"names?: string[] | null;",
	)
	fields := read(t, outDir, "fields/group.ts")
	mustContain(t, fields, "field selector",
		"memberIds: (string | number)[] }",
		"matrix: (string | number)[][] | null }",
	)
	itemIDs := read(t, outDir, "queries/item-ids.ts")
	mustContain(t, itemIDs, "op argument and return",
		"ids(v: (string | number)[]): this",
		"Promise<(string | number)[]>",
	)
	grid := read(t, outDir, "queries/grid.ts")
	mustContain(t, grid, "nested list op", "rows(v: (string | number)[][]): this", "Promise<(string | number)[][]>")
	names := read(t, outDir, "queries/names.ts")
	mustContain(t, names, "plain list op", "ns(v: string[]): this", "Promise<string[]>")

	for rel, s := range map[string]string{"types": types, "inputs": inputs, "fields": fields, "item-ids": itemIDs, "grid": grid} {
		if strings.Contains(s, "string | number[]") {
			t.Errorf("%s: union element rendered without parentheses\n---\n%s", rel, s)
		}
	}
}
