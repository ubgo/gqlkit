package clientgents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kitchenSink exercises the full generator surface: objects, an interface, an
// input object, an enum, custom scalars (Pascal + Hasura lowercase), lists,
// nested non-null, recursion (User.friends, Category.parent), a union member
// set, mutations, and arguments of every flavor.
const kitchenSink = `
scalar DateTime
scalar jsonb
enum Role { ADMIN USER GUEST }
interface Node { id: ID! }
union SearchResult = User | Post
input PostFilter { q: String tags: [String!] author: ID role: Role after: DateTime }
type Category { id: ID! name: String! parent: Category children: [Category!]! }
type Post {
  id: ID!
  title: String!
  body: String
  author: User!
  tags: [String!]!
  meta: jsonb
  createdAt: DateTime!
}
type User {
  id: ID!
  name: String!
  role: Role!
  friends: [User!]
  posts(filter: PostFilter, limit: Int): [Post!]!
  bestFriend: User
  node: Node
}
type Query {
  me: User!
  user(id: ID!): User
  posts(filter: PostFilter): [Post!]!
  search(q: String!): [SearchResult!]!
  scalarThing: DateTime
}
type Mutation {
  createPost(title: String!, tags: [String!]): Post!
  deletePost(id: ID!): Boolean!
}
`

// runGenerate writes the SDL to a temp file and runs Generate into a temp output
// dir, returning that dir. The whole test runs from a temp cwd so any schema.json
// side effect does not pollute the repo.
func runGenerate(t *testing.T, sdl string) string {
	t.Helper()
	work := t.TempDir()
	t.Chdir(work)

	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(sdl), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	outDir := filepath.Join(work, "out")
	g, err := New(&Config{SchemaPath: schemaPath, OutputDir: outDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return outDir
}

// read returns the content of a generated file, failing if missing.
func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// mustContain asserts every want substring is present in s.
func mustContain(t *testing.T, s, label string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%s: missing %q\n---\n%s", label, w, s)
		}
	}
}

// TestGenerateKitchenSink is the end-to-end guard: a rich schema must generate
// the full file matrix and every file's content must match the expected shapes.
func TestGenerateKitchenSink(t *testing.T) {
	dir := runGenerate(t, kitchenSink)

	// --- file matrix exists ---
	for _, f := range []string{
		"builder/index.ts",
		"scalars/index.ts",
		"enums/index.ts",
		"types/index.ts",
		"inputs/index.ts",
		"fields/index.ts",
		"fields/user.ts",
		"fields/post.ts",
		"fields/category.ts",
		"queries/index.ts",
		"queries/root.ts",
		"queries/me.ts",
		"queries/user.ts",
		"mutations/index.ts",
		"mutations/root.ts",
		"mutations/create-post.ts",
	} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected generated file %s: %v", f, err)
		}
	}

	// --- builder re-export ---
	mustContain(t, read(t, dir, "builder/index.ts"), "builder/index.ts",
		`export { FieldSelection, BaseBuilder } from "gqlkit-ts";`,
		`export { GraphQLClient } from "gqlkit-ts";`)

	// --- enums ---
	enums := read(t, dir, "enums/index.ts")
	mustContain(t, enums, "enums", "export enum Role {", `ADMIN = "ADMIN",`, `GUEST = "GUEST",`)

	// --- scalars: DateTime + jsonb custom scalars mapped to `any` (unbound) ---
	scalars := read(t, dir, "scalars/index.ts")
	mustContain(t, scalars, "scalars", "export type DateTime =", "export type jsonb =")

	// --- types: interfaces with correct TS field types + optionality ---
	types := read(t, dir, "types/index.ts")
	mustContain(t, types, "types",
		"export interface User {",
		"id: string;",        // ID! -> non-optional string
		"role: Role;",        // enum, non-null
		"friends?: User[];",  // [User!] nullable -> optional list
		"posts: Post[];",     // [Post!]! -> required list
		"bestFriend?: User;", // nullable object -> optional
		"export interface Post {",
		"body?: string;",       // nullable String
		"createdAt: DateTime;", // custom scalar
		"meta?: jsonb;",        // custom lowercase scalar
		"export interface Category {",
		"parent?: Category;", // recursion
		"children: Category[];",
	)
	// Query/Mutation roots must NOT leak into types.
	if strings.Contains(types, "interface Query") || strings.Contains(types, "interface Mutation") {
		t.Errorf("types/index.ts leaked a root type:\n%s", types)
	}
	// enum + scalar imports present
	mustContain(t, types, "types imports",
		`from "../enums"`, `from "../scalars"`)

	// --- inputs ---
	inputs := read(t, dir, "inputs/index.ts")
	mustContain(t, inputs, "inputs",
		"export interface PostFilter {",
		"q?: string;",       // nullable String
		"tags?: string[];",  // [String!] nullable
		"role?: Role;",      // enum ref
		"after?: DateTime;", // custom scalar ref
	)

	// --- field selector: scalar leaf vs nested object callback vs recursion ---
	userFields := read(t, dir, "fields/user.ts")
	mustContain(t, userFields, "fields/user.ts",
		"export class UserFields",
		`import { FieldSelection } from "../builder";`,
		"id():", // scalar leaf
		`this.selection.addField("id");`,
		"role():",                   // enum scalar leaf
		"friends<U extends object>", // self-ref object -> callback
		`this.selection.addChild("friends", child);`,
		"posts<U extends object>",               // object list callback
		`import { PostFields } from "./post";`,  // cross-type import
		`import type { Role } from "../enums";`, // enum import for scalar field
	)
	// self-reference must NOT self-import (bug 871c081 guard).
	if strings.Contains(userFields, `from "./user"`) {
		t.Errorf("fields/user.ts self-imports UserFields:\n%s", userFields)
	}

	categoryFields := read(t, dir, "fields/category.ts")
	mustContain(t, categoryFields, "fields/category.ts",
		"parent<U extends object>", // recursion as object callback
		"children<U extends object>",
	)
	if strings.Contains(categoryFields, `from "./category"`) {
		t.Errorf("fields/category.ts self-imports:\n%s", categoryFields)
	}

	// --- fields barrel ---
	mustContain(t, read(t, dir, "fields/index.ts"), "fields/index.ts",
		"UserFields", "PostFields", "CategoryFields")

	// --- query builder with select (object return) ---
	meQuery := read(t, dir, "queries/me.ts")
	mustContain(t, meQuery, "queries/me.ts",
		"export class MeBuilder",
		"select<T extends object>",
		"UserFields",
		`import { BaseBuilder, GraphQLClient } from "../builder";`,
	)

	// user(id: ID!): User -> nullable object return, arg present
	userQuery := read(t, dir, "queries/user.ts")
	mustContain(t, userQuery, "queries/user.ts",
		"export class UserBuilder",
		"id(v: string): this",
		`this.builder.setArg("id", v, "ID!");`,
		"User | null", // nullable object default generic
	)

	// posts query: list return -> select wrapper T[]
	postsQuery := read(t, dir, "queries/posts.ts")
	mustContain(t, postsQuery, "queries/posts.ts",
		"PostFilter", // input import + arg type
		"Post[]",     // list return
	)

	// scalar-return query has NO select method (HasSelect=false path)
	scalarQuery := read(t, dir, "queries/scalar-thing.ts")
	if strings.Contains(scalarQuery, "select<T extends object>") {
		t.Errorf("scalar-return query must not have select():\n%s", scalarQuery)
	}
	mustContain(t, scalarQuery, "queries/scalar-thing.ts", "async execute(): Promise<")

	// --- query root + index ---
	mustContain(t, read(t, dir, "queries/root.ts"), "queries/root.ts",
		"class QueryRoot", "MeBuilder", "UserBuilder")
	mustContain(t, read(t, dir, "queries/index.ts"), "queries/index.ts", "QueryRoot")

	// --- mutations ---
	createPost := read(t, dir, "mutations/create-post.ts")
	mustContain(t, createPost, "mutations/create-post.ts",
		"CreatePostMutationBuilder",
		`"mutation"`,
		"title(v: string): this",
	)
	mustContain(t, read(t, dir, "mutations/root.ts"), "mutations/root.ts",
		"class MutationRoot", "CreatePostMutationBuilder", "DeletePostMutationBuilder")
	mustContain(t, read(t, dir, "mutations/index.ts"), "mutations/index.ts", "MutationRoot")

	// deletePost returns Boolean! (scalar) -> no select, ReturnType boolean
	deletePost := read(t, dir, "mutations/delete-post.ts")
	mustContain(t, deletePost, "mutations/delete-post.ts", "Promise<boolean>")

	// --- introspection / __ fields skipped everywhere ---
	if strings.Contains(types, "__schema") || strings.Contains(userFields, "__typename") {
		t.Error("introspection fields leaked into generated output")
	}
}

// TestGenerateQueryOnly covers the branch where the schema has no Mutation type:
// mutations/ files must be absent and Generate must still succeed.
func TestGenerateQueryOnly(t *testing.T) {
	dir := runGenerate(t, `
type Query { ping: String! }
`)
	if _, err := os.Stat(filepath.Join(dir, "mutations")); !os.IsNotExist(err) {
		t.Errorf("mutations/ should not exist for a query-only schema (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "queries/ping.ts")); err != nil {
		t.Errorf("queries/ping.ts missing: %v", err)
	}
}

// TestGenerateWithScalarBinding covers the external-import binding path through
// scalars/index.ts (import re-export) and config loading in New.
func TestGenerateWithScalarBinding(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	schemaPath := filepath.Join(work, "schema.graphql")
	if err := os.WriteFile(schemaPath, []byte(`
scalar DateTime
scalar JSON
type Thing { at: DateTime! blob: JSON }
type Query { thing: Thing }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(work, "config.jsonc")
	if err := os.WriteFile(cfgPath, []byte(`{
  // DateTime comes from luxon; JSON is an inline alias
  "bindings": {
    "DateTime": { "type": "DateTime", "import": "luxon" },
    "JSON": { "type": "Record<string, unknown>" }
  }
}`), 0o644); err != nil {
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
	scalars := read(t, outDir, "scalars/index.ts")
	mustContain(t, scalars, "scalars with binding",
		`from "luxon"`, // external import re-export
		"export type JSON = Record<string, unknown>;", // inline alias
	)
}
