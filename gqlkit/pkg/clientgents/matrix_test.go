package clientgents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This file walks the generator's input space through the real Generate path:
// every GraphQL kind × binding shape × list depth and nullability × output
// position, plus odd names and minimal schemas. Each generated SDK is also
// compiled with tsc (strict, noUnusedLocals) when the repo's TypeScript
// toolchain is installed, because the build is the only test that sees a
// dangling or unused import.

// repoRoot is the monorepo root relative to this package directory.
const repoRoot = "../../.."

// tscBin and gqlkitTSDir are the repo-local TypeScript compiler and the
// gqlkit-ts runtime the generated SDK imports. Both come from the repo's own
// setup tasks (example-ts:install, gqlkit-ts:build). They are made absolute at
// package init, while the working directory is still this package: the
// generator tests t.Chdir into temp dirs before typecheckSDK runs.
var (
	tscBin      = absFromPackage(repoRoot, "example-ts", "node_modules", ".bin", "tsc")
	gqlkitTSDir = absFromPackage(repoRoot, "gqlkit-ts")
)

// absFromPackage joins parts relative to the package directory and makes the
// result absolute. On the (theoretical) Abs failure it keeps the relative
// path; typecheckSDK then reports it missing and skips.
func absFromPackage(parts ...string) string {
	p := filepath.Join(parts...)
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// generateSDK writes sdl (and config, when non-empty) to a temp dir, runs the
// real Generate, and returns the output dir.
func generateSDK(t *testing.T, sdl, config string) string {
	t.Helper()
	work := t.TempDir()
	t.Chdir(work)
	cfg := &Config{SchemaPath: filepath.Join(work, "schema.graphql"), OutputDir: filepath.Join(work, "sdk")}
	if err := os.WriteFile(cfg.SchemaPath, []byte(sdl), 0o644); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		cfg.ConfigPath = filepath.Join(work, "config.jsonc")
		if err := os.WriteFile(cfg.ConfigPath, []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return cfg.OutputDir
}

// typecheckSDK compiles sdkDir plus any extra files (path relative to the SDK
// parent → content) with tsc in strict mode with noUnusedLocals. It skips,
// saying why, when the TypeScript toolchain or the built gqlkit-ts runtime is
// absent, so `go test` stays runnable on a Go-only machine.
func typecheckSDK(t *testing.T, sdkDir string, extra map[string]string) {
	t.Helper()
	if out, err := runTSC(t, sdkDir, extra); err != nil {
		t.Fatalf("tsc failed on the generated SDK: %v\n%s", err, out)
	}
}

// typecheckSDKFails is typecheckSDK for code that must NOT compile: it
// asserts tsc fails and reports wantCode (e.g. "TS2684"), proving a misuse is
// rejected at the type level without a suppression directive in the fixture.
func typecheckSDKFails(t *testing.T, sdkDir string, extra map[string]string, wantCode string) {
	t.Helper()
	out, err := runTSC(t, sdkDir, extra)
	if err == nil {
		t.Fatalf("tsc accepted code that must be rejected with %s", wantCode)
	}
	if !strings.Contains(string(out), wantCode) {
		t.Fatalf("tsc failed, but not with %s:\n%s", wantCode, out)
	}
}

// tsModuleMode is one TypeScript module setup a consumer may compile the
// generated SDK under. packageType is written to package.json ("" = none).
type tsModuleMode struct {
	name        string
	options     string // module-related compilerOptions, as JSON members
	packageType string
}

// tsModeCommonJS is the classic setup every tsc test uses by default.
var tsModeCommonJS = tsModuleMode{"commonjs/node10", `"module": "commonjs", "moduleResolution": "node10"`, ""}

// tsModuleModes are the setups the module-resolution matrix covers: Node
// CommonJS, bundlers (Vite, webpack, esbuild) and Node native ESM, the
// strictest — it needs explicit ".js" extensions and no directory imports.
var tsModuleModes = []tsModuleMode{
	tsModeCommonJS,
	{"bundler", `"module": "esnext", "moduleResolution": "bundler", "verbatimModuleSyntax": true`, ""},
	{"nodenext-esm", `"module": "nodenext", "moduleResolution": "nodenext", "verbatimModuleSyntax": true`, "module"},
}

// runTSC compiles under tsModeCommonJS; see runTSCMode.
func runTSC(t *testing.T, sdkDir string, extra map[string]string) ([]byte, error) {
	t.Helper()
	return runTSCMode(t, sdkDir, extra, tsModeCommonJS)
}

// runTSCMode writes extra files, a strict tsconfig for mode and a
// node_modules/gqlkit-ts link to the repo runtime next to sdkDir, then runs
// tsc and returns its output. It skips (see typecheckSDK) when the toolchain
// is absent.
func runTSCMode(t *testing.T, sdkDir string, extra map[string]string, mode tsModuleMode) ([]byte, error) {
	t.Helper()
	if testing.Short() {
		t.Skip("tsc check skipped in -short mode")
	}
	if _, err := os.Stat(tscBin); err != nil {
		t.Skipf("tsc check skipped: %s missing (run `task example-ts:install`)", tscBin)
	}
	if _, err := os.Stat(filepath.Join(gqlkitTSDir, "dist", "index.d.ts")); err != nil {
		t.Skipf("tsc check skipped: gqlkit-ts not built (run `task gqlkit-ts:build`)")
	}

	root := filepath.Dir(sdkDir)
	for rel, content := range extra {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "node_modules", "gqlkit-ts")
	if _, err := os.Lstat(link); err != nil {
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(gqlkitTSDir, link); err != nil {
			t.Fatal(err)
		}
	}
	pkg := `{ "private": true }`
	if mode.packageType != "" {
		pkg = fmt.Sprintf(`{ "private": true, "type": %q }`, mode.packageType)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	tsconfig := fmt.Sprintf(`{
  "compilerOptions": {
    "target": "ES2022", "lib": ["ES2022", "DOM"], %s,
    "strict": true, "noUnusedLocals": true, "noEmit": true, "skipLibCheck": true, "types": []
  },
  "include": ["**/*.ts"],
  "exclude": ["node_modules"]
}`, mode.options)
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(tsconfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return exec.Command(tscBin, "-p", filepath.Join(root, "tsconfig.json")).CombinedOutput()
}

// matrixKind is one GraphQL element type the matrix places in every wrapper
// and position. elem is the TS text expected for the bare element, listElem
// the text expected before "[]" and orNull the text expected for a nullable
// element (each parenthesised only where TS precedence needs it).
type matrixKind struct {
	key      string // short field-name fragment
	gql      string // GraphQL named type
	elem     string
	listElem string
	orNull   string
	// variants is set for abstract kinds: the {E, N, L} forms of the
	// { __typename } union an operation is typed with before select().
	variants *[3]string
	input    bool // usable in input positions (args, input fields)
	output   bool // usable in output positions (fields, returns)
	object   bool // rendered as a selector callback, not a typed leaf
}

// matrixKinds covers every GraphQL kind and every binding shape. The bindings
// it relies on are declared in matrixConfig; the types in matrixSDL.
var matrixKinds = []matrixKind{
	{key: "int", gql: "Int", elem: "number", listElem: "number", orNull: "number | null", input: true, output: true},
	{key: "str", gql: "String", elem: "string", listElem: "string", orNull: "string | null", input: true, output: true},
	{key: "unbound", gql: "DateTime", elem: "DateTime", listElem: "DateTime", orNull: "DateTime | null", input: true, output: true},
	{key: "simple", gql: "Cursor", elem: "string", listElem: "string", orNull: "string | null", input: true, output: true},
	{key: "union", gql: "ID", elem: "string | number", listElem: "(string | number)", orNull: "string | number | null", input: true, output: true},
	{key: "inter", gql: "Money", elem: "number & { readonly brand: 'Money' }", listElem: "(number & { readonly brand: 'Money' })", orNull: "number & { readonly brand: 'Money' } | null", input: true, output: true},
	{key: "generic", gql: "JSON", elem: "Record<string, unknown>", listElem: "Record<string, unknown>", orNull: "Record<string, unknown> | null", input: true, output: true},
	{key: "objlit", gql: "Point", elem: "{ x: number; y: number }", listElem: "{ x: number; y: number }", orNull: "{ x: number; y: number } | null", input: true, output: true},
	{key: "fn", gql: "Thunk", elem: "() => string", listElem: "(() => string)", orNull: "(() => string) | null", input: true, output: true},
	{key: "imported", gql: "Branded", elem: "Branded", listElem: "Branded", orNull: "Branded | null", input: true, output: true},
	{key: "enum", gql: "Role", elem: "Role", listElem: "Role", orNull: "Role | null", input: true, output: true},
	{key: "input", gql: "Filter", elem: "Filter", listElem: "Filter", orNull: "Filter | null", input: true},
	{key: "obj", gql: "Leaf", elem: "Leaf", listElem: "Leaf", orNull: "Leaf | null", output: true, object: true},
	{key: "iface", gql: "Node", elem: "Node", listElem: "Node", orNull: "Node | null", output: true, object: true,
		variants: &[3]string{`{ __typename: "Leaf" }`, `{ __typename: "Leaf" } | null`, `{ __typename: "Leaf" }`}},
	{key: "gunion", gql: "Either", elem: "Either", listElem: "Either", orNull: "Either | null", output: true, object: true,
		variants: &[3]string{`{ __typename: "Leaf" } | { __typename: "Other" }`, `{ __typename: "Leaf" } | { __typename: "Other" } | null`, `({ __typename: "Leaf" } | { __typename: "Other" })`}},
}

// matrixWrapper is one list depth / nullability combination. gql wraps a
// named type; ts is the expected TS text, written over the kind's forms:
// {E} the element, {N} the nullable element, {L} the element before "[]".
// nullable marks the outermost type nullable (an optional interface property).
type matrixWrapper struct {
	key      string
	gql      string // fmt pattern with one %s for the named type
	ts       string
	nullable bool
}

var matrixWrappers = []matrixWrapper{
	{"n", "%s", "{N}", true},
	{"nn", "%s!", "{E}", false},
	{"l", "[%s]", "({N})[] | null", true},
	{"ln", "[%s!]", "{L}[] | null", true},
	{"nl", "[%s]!", "({N})[]", false},
	{"nln", "[%s!]!", "{L}[]", false},
	{"ll", "[[%s]]", "(({N})[] | null)[] | null", true},
	{"nlnlnn", "[[%s!]!]!", "{L}[][]", false},
	{"lnll", "[[%s]!]", "({N})[][] | null", true},
}

// matrixConfig binds the scalars of matrixKinds to one binding shape each.
const matrixConfig = `{
  "bindings": {
    "Cursor": "string",
    "ID": "string | number",
    "Money": "number & { readonly brand: 'Money' }",
    "JSON": "Record<string, unknown>",
    "Point": "{ x: number; y: number }",
    "Thunk": "() => string",
    "Branded": { "type": "Brand", "import": "./brand.js" }
  }
}`

// matrixBrandModule backs the imported Branded binding during tsc.
const matrixBrandModule = "export type Brand = string & { readonly __brand: 'Branded' };\n"

// matrixFieldName is the GraphQL field / arg name for one kind × wrapper.
func matrixFieldName(k matrixKind, w matrixWrapper) string {
	return k.key + "_" + w.key
}

// matrixTSText is the TS type text expected for an element with the given
// forms inside wrapper w.
func matrixTSText(w matrixWrapper, elem, orNull, listElem string) string {
	return strings.NewReplacer("{E}", elem, "{N}", orNull, "{L}", listElem).Replace(w.ts)
}

// matrixSDL renders the schema: an output type M and an input type MI with
// one field per kind × wrapper, and one query per kind × wrapper whose single
// argument and return use the same wrapped type.
func matrixSDL() string {
	var b strings.Builder
	b.WriteString(`scalar DateTime
scalar Cursor
scalar Money
scalar JSON
scalar Point
scalar Thunk
scalar Branded
enum Role { ADMIN USER }
input Filter { q: String }
interface Node { id: ID! }
type Leaf implements Node { id: ID! name: String }
type Other { v: Int }
union Either = Leaf | Other
`)
	b.WriteString("type M {\n")
	for _, k := range matrixKinds {
		if !k.output {
			continue
		}
		for _, w := range matrixWrappers {
			fmt.Fprintf(&b, "  %s: %s\n", matrixFieldName(k, w), fmt.Sprintf(w.gql, k.gql))
		}
	}
	b.WriteString("}\ninput MI {\n")
	for _, k := range matrixKinds {
		if !k.input {
			continue
		}
		for _, w := range matrixWrappers {
			fmt.Fprintf(&b, "  %s: %s\n", matrixFieldName(k, w), fmt.Sprintf(w.gql, k.gql))
		}
	}
	b.WriteString("}\ntype Query {\n  m: M\n")
	for _, k := range matrixKinds {
		for _, w := range matrixWrappers {
			gql := fmt.Sprintf(w.gql, k.gql)
			switch {
			case k.input && k.output:
				fmt.Fprintf(&b, "  q_%s(a: %s): %s\n", matrixFieldName(k, w), gql, gql)
			case k.input:
				fmt.Fprintf(&b, "  q_%s(a: %s): Boolean\n", matrixFieldName(k, w), gql)
			default:
				fmt.Fprintf(&b, "  q_%s: %s\n", matrixFieldName(k, w), gql)
			}
		}
	}
	b.WriteString("}\ntype Mutation { m_touch(input: MI!): M }\n")
	return b.String()
}

// TestGenerateTypeMatrix asserts the TS text of every kind × wrapper in every
// position, then compiles the whole SDK.
func TestGenerateTypeMatrix(t *testing.T) {
	sdk := generateSDK(t, matrixSDL(), matrixConfig)

	types := read(t, sdk, "types/index.ts")
	inputs := read(t, sdk, "inputs/index.ts")
	selector := read(t, sdk, "fields/m.ts")

	for _, k := range matrixKinds {
		for _, w := range matrixWrappers {
			name := matrixFieldName(k, w)
			text := matrixTSText(w, k.elem, k.orNull, k.listElem)
			opt := ""
			if w.nullable {
				opt = "?"
			}
			t.Run(name, func(t *testing.T) {
				if k.output {
					mustContain(t, types, "types/index.ts", fmt.Sprintf("  %s%s: %s;\n", name, opt, text))
					if k.object {
						u := matrixTSText(w, "U", "U | null", "U")
						mustContain(t, selector, "fields/m.ts", fmt.Sprintf("%s<U extends object>(selector: (f: %sFields) => %sFields<U>): MFields<T & { %s: %s }>", name, k.gql, k.gql, name, u))
					} else {
						// A selected field is always present: never optional.
						mustContain(t, selector, "fields/m.ts", fmt.Sprintf("%s(): MFields<T & { %s: %s }>", name, name, text))
					}
				}
				if k.input {
					mustContain(t, inputs, "inputs/index.ts", fmt.Sprintf("  %s%s: %s;\n", name, opt, text))
				}

				op := read(t, sdk, "queries/"+toKebabCase("q_"+name)+tsExt)
				if k.input {
					mustContain(t, op, "op arg", fmt.Sprintf("a(v: %s): this", text))
					mustContain(t, op, "op arg GraphQL type", fmt.Sprintf(`setArg("a", v, %q)`, fmt.Sprintf(w.gql, k.gql)))
				}
				switch {
				case k.object:
					// An object result has no type until select(); an abstract
					// one is its { __typename } variants (valid unselected).
					generic := "SelectionRequired"
					if k.variants != nil {
						generic = matrixTSText(w, k.variants[0], k.variants[1], k.variants[2])
					}
					mustContain(t, op, "op default generic", fmt.Sprintf("<TResult = %s>", generic))
					mustContain(t, op, "op select result", fmt.Sprintf("Builder<%s> {", matrixTSText(w, "T", "T | null", "T")))
				case k.output:
					mustContain(t, op, "op return", fmt.Sprintf("execute(request?: RequestOptions): Promise<%s>", text))
				}
			})
		}
	}

	typecheckSDK(t, sdk, map[string]string{"sdk/scalars/brand.ts": matrixBrandModule})
}

// TestGenerateOddNames covers names that are legal GraphQL but unusual in TS:
// reserved words as field, argument, operation and enum-value names, leading
// underscores, and self-referencing types (which must stay selector
// callbacks, never scalar leaves — the 871c081 regression).
func TestGenerateOddNames(t *testing.T) {
	sdk := generateSDK(t, `
enum Kw { delete class in default }
type _Hidden { _id: ID! }
type Tree {
  delete: Boolean
  class: String
  default: Int
  _private: String
  kind: Kw
  parent: Tree
  children: [Tree!]!
  siblings: [Tree]
  hidden: _Hidden
}
type Query {
  delete(in: ID!, new: Int): Boolean
  class(default: Kw): Tree
  _underscored: String
  tree: Tree!
}
`, "")

	tree := read(t, sdk, "fields/tree.ts")
	mustContain(t, tree, "reserved-word selectors",
		"delete(): TreeFields<T & { delete: boolean | null }>",
		"class(): TreeFields<T & { class: string | null }>",
		"default(): TreeFields<T & { default: number | null }>",
		"_private(): TreeFields<T & { _private: string | null }>",
	)
	mustContain(t, tree, "self-reference stays a selector",
		"parent<U extends object>(selector: (f: TreeFields) => TreeFields<U>): TreeFields<T & { parent: U | null }>",
		"children<U extends object>(selector: (f: TreeFields) => TreeFields<U>): TreeFields<T & { children: U[] }>",
		"siblings<U extends object>(selector: (f: TreeFields) => TreeFields<U>): TreeFields<T & { siblings: (U | null)[] | null }>",
	)
	// Leading-underscore types get an unusual but consistent file name
	// ("_Hidden" → "_-hidden.ts"); what matters is that the import resolves.
	assertNoDanglingFieldImports(t, sdk)
	if strings.Contains(tree, `from "./tree.js"`) || strings.Contains(tree, `addField("parent")`) {
		t.Errorf("self-reference rendered as a self-import or scalar leaf:\n%s", tree)
	}

	enums := read(t, sdk, "enums/index.ts")
	mustContain(t, enums, "reserved-word enum values", `delete = "delete",`, `class = "class",`, `in = "in",`, `default = "default",`)

	del := read(t, sdk, "queries/delete.ts")
	mustContain(t, del, "reserved-word op + args", "export class DeleteBuilder", "in(v: string): this", "new(v: number | null): this")
	mustContain(t, read(t, sdk, "queries/root.ts"), "root methods", "delete(): DeleteBuilder", "class(): ClassBuilder", "_underscored(): UnderscoredBuilder")

	typecheckSDK(t, sdk, nil)
}

// TestGenerateMinimalSchemas covers schema shapes with parts missing: no
// mutation root (no mutations/ dir), no enums / inputs / custom scalars
// (empty barrels that must still compile), and a subscription root, which
// has no builders and must not leak into types/ or fields/.
func TestGenerateMinimalSchemas(t *testing.T) {
	cases := []struct {
		name, sdl    string
		hasMutations bool
	}{
		{"query only, one scalar", `type Query { ping: String }`, false},
		{"query and mutation, no objects", `type Query { ping: String } type Mutation { touch(id: ID!): Boolean! }`, true},
		{"subscription root", `type Query { ping: String } type Subscription { ticks: Int! } type Tick { at: Int }`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sdk := generateSDK(t, c.sdl, "")
			for _, rel := range []string{"builder/index.ts", "scalars/index.ts", "enums/index.ts", "types/index.ts", "inputs/index.ts", "fields/index.ts", "queries/root.ts", "queries/index.ts"} {
				read(t, sdk, rel)
			}
			_, err := os.Stat(filepath.Join(sdk, dirMutations))
			if got := err == nil; got != c.hasMutations {
				t.Errorf("mutations/ exists = %v, want %v", got, c.hasMutations)
			}
			if _, err := os.Stat(filepath.Join(sdk, "subscriptions")); err == nil {
				t.Error("subscriptions/ must not be generated")
			}
			if types := read(t, sdk, "types/index.ts"); strings.Contains(types, "interface Subscription") || strings.Contains(types, "interface Query") {
				t.Errorf("operation root leaked into types/index.ts:\n%s", types)
			}
			typecheckSDK(t, sdk, nil)
		})
	}
}

// TestGenerateAbstractSelections covers unions and interfaces: union
// aliases in types/index.ts, on<Type>() inline fragments, __typename always
// requested (so an unselected abstract operation is still valid GraphQL), a
// discriminated result the caller can narrow, shared interface fields that
// apply to every variant, and an interface field that clashes with a
// generated on<Type>() method.
func TestGenerateAbstractSelections(t *testing.T) {
	sdk := generateSDK(t, `
interface Node { id: ID! onTodo: String }
type Todo implements Node { id: ID! onTodo: String text: String! }
type User implements Node { id: ID! onTodo: String name: String best: SearchResult }
union SearchResult = Todo | User
type Holder { one: SearchResult items: [SearchResult!]! }
type Query { search(term: String!): [SearchResult!]! node(id: ID!): Node holder: Holder }
`, "")

	mustContain(t, read(t, sdk, "types/index.ts"), "union alias and fields",
		"export type SearchResult = Todo | User;", "one?: SearchResult | null;", "items: SearchResult[];")

	union := read(t, sdk, "fields/search-result.ts")
	mustContain(t, union, "union selector",
		`export class SearchResultFields<T extends object = { __typename: "Todo" } | { __typename: "User" }> {`,
		`if (!typenameAdded) selection.addField("__typename");`,
		`onTodo<U extends object>(selector: (f: TodoFields) => TodoFields<U>): SearchResultFields<Exclude<T, { __typename: "Todo" }> | (Extract<T, { __typename: "Todo" }> & U)>`,
		`this.#selection.addChild("... on Todo", child);`,
		`import { UserFields } from "./user.js";`,
	)
	iface := read(t, sdk, "fields/node.ts")
	mustContain(t, iface, "interface selector",
		"onTodo<U extends object>(selector: (f: TodoFields) => TodoFields<U>)",
		"onUser<U extends object>(selector: (f: UserFields) => UserFields<U>)",
		// the schema field loses to the generated variant method
		"onTodo_(): NodeFields<T & { onTodo: string | null }>",
		"id(): NodeFields<T & { id: string }>",
	)

	search := read(t, sdk, "queries/search.ts")
	mustContain(t, search, "union op",
		`export class SearchBuilder<TResult = ({ __typename: "Todo" } | { __typename: "User" })[]> {`,
		`if (!builder) this.#builder.getSelection().addField("__typename");`,
		"selector(new SearchResultFields(this.#builder.getSelection(), true));",
	)
	if strings.Contains(search, "../types") {
		t.Errorf("abstract op must not import its type by name:\n%s", search)
	}
	mustContain(t, read(t, sdk, "fields/holder.ts"), "union field callback",
		"one<U extends object>(selector: (f: SearchResultFields) => SearchResultFields<U>): HolderFields<T & { one: U | null }>")

	typecheckSDK(t, sdk, map[string]string{"usage.ts": `
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";

export async function run(client: GraphQLClient): Promise<string[]> {
  const root = new QueryRoot(client);
  const bare: ({ __typename: "Todo" } | { __typename: "User" })[] = await root.search().term("i").execute();
  const hits = await root.search().term("i")
    .select((s) => s.onTodo((t) => t.id().text()).onUser((u) => u.name()))
    .execute();
  const out: string[] = bare.map((b) => b.__typename);
  for (const h of hits) {
    switch (h.__typename) {
      case "Todo": out.push(h.text, h.id); break;
      case "User": out.push(h.name ?? ""); break;
    }
  }
  const node = await root.node().id("1").select((n) => n.id().onUser((u) => u.name())).execute();
  if (node && node.__typename === "User") out.push(node.id, node.name ?? "");
  if (node && node.__typename === "Todo") out.push(node.id);
  return out;
}
`})
}

// TestGenerateScalarAliasInOperations pins that operation files render every
// non-inline scalar through its scalars/ alias, exactly as interface fields
// do, so the alias they import is the alias they use: an unbound scalar, a
// bare-name binding without import ("Date"), and an imported String.
func TestGenerateScalarAliasInOperations(t *testing.T) {
	sdk := generateSDK(t, `
scalar DateTime
scalar Stamp
scalar String
type Query { when(at: DateTime!, s: Stamp, str: String): DateTime stamps: [Stamp!]! }
`, `{ "bindings": { "Stamp": "Date", "String": { "type": "Str", "import": "./str" } } }`)

	op := read(t, sdk, "queries/when.ts")
	mustContain(t, op, "alias args and return",
		`import type { DateTime, GqlString, Stamp } from "../scalars/index.js";`,
		"at(v: DateTime): this", "s(v: Stamp | null): this", "str(v: GqlString | null): this",
		"execute(request?: RequestOptions): Promise<DateTime | null>",
	)
	mustContain(t, read(t, sdk, "queries/stamps.ts"), "alias list return", "execute(request?: RequestOptions): Promise<Stamp[]>")
	typecheckSDK(t, sdk, map[string]string{"sdk/scalars/str.ts": "export type Str = string;\n"})
}

// TestGenerateNullabilityUsage compiles the ways a user meets null: reading a
// nullable field from a typed result, sending an explicit null in an input
// field and an argument, and the narrowed select() result, where a selected
// field is always present but may hold null.
func TestGenerateNullabilityUsage(t *testing.T) {
	sdk := generateSDK(t, `
input Patch { name: String tags: [String] }
type Item { name: String tags: [String]! child: Item }
type Query { item(id: ID, patch: Patch): Item count: Int names: [String] }
`, "")
	typecheckSDK(t, sdk, map[string]string{"usage.ts": `
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";
import type { Item } from "./sdk/types";
import type { Patch } from "./sdk/inputs";

export const item: Item = { name: null, tags: ["a", null], child: null };
export const patch: Patch = { name: null, tags: null };

export async function run(client: GraphQLClient) {
  const root = new QueryRoot(client);
  const count: number | null = await root.count().execute();
  const names: (string | null)[] | null = await root.names().execute();
  const picked: { name: string | null; child: { tags: (string | null)[] } | null } | null = await root
    .item().id(null).patch({ name: null })
    .select((f) => f.name().child((c) => c.tags()))
    .execute();
  return [count, names, picked];
}
`})
}

// tsxBin runs TypeScript directly; used to execute generated code at runtime.
var tsxBin = absFromPackage(repoRoot, "example-ts", "node_modules", ".bin", "tsx")

// runSDKScript runs script (a .ts file next to sdkDir) with tsx and returns
// its combined output. It skips when tsx is not installed.
func runSDKScript(t *testing.T, sdkDir, name, script string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("tsx run skipped in -short mode")
	}
	if _, err := os.Stat(tsxBin); err != nil {
		t.Skipf("tsx run skipped: %s missing (run `task example-ts:install`)", tsxBin)
	}
	p := filepath.Join(filepath.Dir(sdkDir), name)
	if err := os.WriteFile(p, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tsxBin, "--tsconfig", filepath.Join(filepath.Dir(sdkDir), "tsconfig.json"), p)
	cmd.Dir = filepath.Dir(sdkDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tsx %s failed: %v\n%s", name, err, out)
	}
	return string(out)
}

// TestGenerateObjectReturnRequiresSelect pins that an object-returning
// operation cannot run without a selection — the server would reject it
// ("must have a selection of subfields"). It is a compile error to call
// execute() before select(), and the runtime guard stops plain-JS callers
// and batch() before any request is sent.
func TestGenerateObjectReturnRequiresSelect(t *testing.T) {
	sdk := generateSDK(t, `
type User { id: ID! name: String }
type Query { me: User! users: [User!]! }
`, "")
	mustContain(t, read(t, sdk, "queries/me.ts"), "gated builder",
		`import type { RequestOptions, SelectionRequired } from "../builder/index.js";`,
		"export class MeBuilder<TResult = SelectionRequired> {",
		"async execute(this: TResult extends SelectionRequired ? never : this, request?: RequestOptions): Promise<TResult> {",
		"getOpFragment(this: TResult extends SelectionRequired ? never : this, alias: string) {",
	)

	const usage = `
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";
export async function ok(client: GraphQLClient) {
  const me: { id: string } = await new QueryRoot(client).me().select((u) => u.id()).execute();
  const users: { name: string | null }[] = await new QueryRoot(client).users().select((u) => u.name()).execute();
  return [me, users];
}
`
	typecheckSDK(t, sdk, map[string]string{"usage.ts": usage})
	// TS2684: "The 'this' context of type ... is not assignable to method's 'this' of type 'never'".
	typecheckSDKFails(t, sdk, map[string]string{"bad.ts": `
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";
export const p = new QueryRoot(new GraphQLClient("http://unused")).me().execute();
`}, "TS2684")
	if err := os.Remove(filepath.Join(filepath.Dir(sdk), "bad.ts")); err != nil {
		t.Fatal(err)
	}

	// Runtime guard: an empty select() (or a plain-JS caller) throws before
	// the client is asked to send anything.
	out := runSDKScript(t, sdk, "guard.ts", `
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";
class Recording extends GraphQLClient {
  sent = 0;
  override async execute<T>(query: string, variables?: Record<string, unknown>): Promise<T> {
    this.sent++;
    return super.execute<T>(query, variables);
  }
}
const client = new Recording("http://127.0.0.1:9");
new QueryRoot(client).me().select((u) => u).execute().then(
  () => { console.log("UNEXPECTED: sent"); process.exit(1); },
  (e: unknown) => { console.log("guard:", e instanceof Error ? e.message : String(e), "sent=" + client.sent); },
);
`)
	mustContain(t, out, "runtime guard", "guard: gqlkit: me returns an object type; call select() with at least one field before running it", "sent=0")
}

// TestGenerateModuleResolutionMatrix compiles one broad SDK — every kind,
// wrapper, binding shape, union, interface and a usage file — under each
// module setup in tsModuleModes. Before relative imports carried ".js",
// Node ESM (nodenext) rejected every one of them with TS2835.
func TestGenerateModuleResolutionMatrix(t *testing.T) {
	usage := `
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries/index.js";
import type { M } from "./sdk/types/index.js";
import type { MI } from "./sdk/inputs/index.js";
import { Role } from "./sdk/enums/index.js";

export async function run(client: GraphQLClient): Promise<unknown[]> {
  const root = new QueryRoot(client);
  const m: M | null = null;
  const mi: Pick<MI, "enum_n" | "int_l"> = { enum_n: Role.ADMIN, int_l: [1, null] };
  const leaf = await root.q_obj_nn().select((f) => f.id().name()).execute();
  const either = await root.q_gunion_nln().select((u) => u.onLeaf((l) => l.id())).execute();
  return [m, mi, leaf.id, either.map((e) => e.__typename)];
}
`
	for _, mode := range tsModuleModes {
		t.Run(mode.name, func(t *testing.T) {
			sdk := generateSDK(t, matrixSDL(), matrixConfig)
			extra := map[string]string{"sdk/scalars/brand.ts": matrixBrandModule, "usage.ts": usage}
			if out, err := runTSCMode(t, sdk, extra, mode); err != nil {
				t.Fatalf("tsc (%s) failed on the generated SDK: %v\n%s", mode.name, err, out)
			}
		})
	}
}
