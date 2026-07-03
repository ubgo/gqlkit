package schemagql

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile creates a file with the given SDL content inside dir and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

const validQuerySDL = `
type Query {
  hello: String!
  count: Int
}

type User {
  id: ID!
  name: String!
}
`

const queryAndMutationSDL = `
schema {
  query: Query
  mutation: Mutation
}

type Query {
  me: User
}

type Mutation {
  createUser(name: String!): User!
}

type User {
  id: ID!
  name: String!
}
`

// SDL that already declares the builtin scalar String, exercising the
// hasBuiltins == true branch (prelude is NOT merged).
const withBuiltinsSDL = `
scalar String
scalar Int
scalar Float
scalar Boolean
scalar ID

type Query {
  hello: String!
}
`

func TestGetSchema_ValidSingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "schema.graphql", validQuerySDL)

	schema, err := GetSchema(StringList{filepath.Join(dir, "schema.graphql")})
	require.NoError(t, err)
	require.NotNil(t, schema)

	// Query root is wired up.
	require.NotNil(t, schema.Query)
	assert.Equal(t, "Query", schema.Query.Name)

	// The user-defined type is present.
	require.Contains(t, schema.Types, "User")
	assert.Equal(t, "User", schema.Types["User"].Name)

	// Builtin scalars got injected via the prelude (hasBuiltins == false path).
	assert.Contains(t, schema.Types, "String")
	assert.Contains(t, schema.Types, "Int")

	// Query fields survived.
	q := schema.Types["Query"]
	require.NotNil(t, q.Fields.ForName("hello"))
	require.NotNil(t, q.Fields.ForName("count"))
}

func TestGetSchema_QueryAndMutation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "schema.graphql", queryAndMutationSDL)

	schema, err := GetSchema(StringList{filepath.Join(dir, "schema.graphql")})
	require.NoError(t, err)
	require.NotNil(t, schema)

	require.NotNil(t, schema.Query)
	assert.Equal(t, "Query", schema.Query.Name)
	require.NotNil(t, schema.Mutation)
	assert.Equal(t, "Mutation", schema.Mutation.Name)

	require.NotNil(t, schema.Mutation.Fields.ForName("createUser"))
}

func TestGetSchema_MultiFileMerge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "query.graphql", `
type Query {
  hello: String!
}
`)
	writeFile(t, dir, "types.graphql", `
type Product {
  id: ID!
  title: String!
}
`)

	schema, err := GetSchema(StringList{
		filepath.Join(dir, "query.graphql"),
		filepath.Join(dir, "types.graphql"),
	})
	require.NoError(t, err)
	require.NotNil(t, schema)

	require.NotNil(t, schema.Query)
	assert.Contains(t, schema.Types, "Product")
	assert.Contains(t, schema.Types, "Query")
}

func TestGetSchema_GlobPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.graphql", `
type Query {
  a: String!
}
`)
	writeFile(t, dir, "b.graphql", `
type B {
  id: ID!
}
`)

	// Single glob matching multiple files.
	schema, err := GetSchema(StringList{filepath.Join(dir, "*.graphql")})
	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Contains(t, schema.Types, "B")
	require.NotNil(t, schema.Query)
}

func TestGetSchema_WithBuiltinsPresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "schema.graphql", withBuiltinsSDL)

	// This exercises the hasBuiltins == true branch: the prelude is NOT merged
	// because the SDL already declares the builtin scalars.
	schema, err := GetSchema(StringList{filepath.Join(dir, "schema.graphql")})
	require.NoError(t, err)
	require.NotNil(t, schema)
	require.NotNil(t, schema.Query)
	assert.Contains(t, schema.Types, "String")
}

func TestGetSchema_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	// An explicit (non-glob) path that doesn't exist. doublestar treats it as a
	// literal pattern that matches nothing -> "did not match any files".
	_, err := GetSchema(StringList{filepath.Join(dir, "nope.graphql")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not match any files")
}

func TestGetSchema_GlobMatchesNothing(t *testing.T) {
	dir := t.TempDir()
	_, err := GetSchema(StringList{filepath.Join(dir, "*.graphql")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not match any files")
}

func TestGetSchema_InvalidSDL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.graphql", `type Query { this is not valid SDL @@@ }`)

	_, err := GetSchema(StringList{filepath.Join(dir, "bad.graphql")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid schema")
}

func TestGetSchema_SemanticallyInvalidSchema(t *testing.T) {
	dir := t.TempDir()
	// Parses fine but fails schema validation: field references an unknown type.
	writeFile(t, dir, "schema.graphql", `
type Query {
  broken: DoesNotExist
}
`)

	_, err := GetSchema(StringList{filepath.Join(dir, "schema.graphql")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid schema")
}

func TestGetSchema_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// Create a directory that matches the glob but cannot be read as a file's
	// content in a normal way. Instead, we point directly at a path we then make
	// unreadable to trigger the os.ReadFile error branch.
	p := writeFile(t, dir, "secret.graphql", validQuerySDL)
	require.NoError(t, os.Chmod(p, 0o000))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	if os.Getuid() == 0 {
		t.Skip("running as root; permission bits do not restrict reads")
	}

	_, err := GetSchema(StringList{p})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreadable schema file")
}

func TestGetSchema_EmptyGlobList(t *testing.T) {
	// No patterns at all -> no files -> empty document. The prelude gets merged
	// (no builtins) but there is no Query root, which is a valid-ish empty schema
	// in gqlparser terms. Assert it doesn't panic and returns without a Query.
	schema, err := GetSchema(StringList{})
	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Nil(t, schema.Query)
}

func TestExpandFilenames_Dedup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.graphql", `type Query { a: String }`)

	// Same file referenced via two overlapping patterns should dedup to one.
	names, err := expandFilenames([]string{
		filepath.Join(dir, "a.graphql"),
		filepath.Join(dir, "*.graphql"),
	})
	require.NoError(t, err)
	assert.Len(t, names, 1)
}

func TestExpandFilenames_NoMatch(t *testing.T) {
	dir := t.TempDir()
	_, err := expandFilenames([]string{filepath.Join(dir, "missing.graphql")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not match any files")
}

func TestExpandFilenames_MalformedPattern(t *testing.T) {
	// An unterminated character class is a syntax error in the glob engine,
	// exercising the doublestar.Glob error branch.
	_, err := expandFilenames([]string{"/tmp/["})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't expand file-glob")
}

func TestGetSchema_MalformedPattern(t *testing.T) {
	_, err := GetSchema(StringList{"/tmp/["})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't expand file-glob")
}

func TestParseSchemaFile_Valid(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "schema.graphql", validQuerySDL)

	schema, err := ParseSchemaFile(p)
	require.NoError(t, err)
	require.NotNil(t, schema)
	require.NotNil(t, schema.Query)
	assert.Equal(t, "Query", schema.Query.Name)
	assert.Contains(t, schema.Types, "User")
}

func TestParseSchemaFile_Nonexistent(t *testing.T) {
	_, err := ParseSchemaFile(filepath.Join(t.TempDir(), "nope.graphql"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read schema file")
}

func TestParseSchemaFile_InvalidSDL(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "bad.graphql", `type Query { @@@ invalid }`)

	_, err := ParseSchemaFile(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse schema")
}

func TestStringList_Type(t *testing.T) {
	// Basic sanity: StringList is usable as a []string.
	sl := StringList{"a", "b"}
	assert.Equal(t, []string{"a", "b"}, []string(sl))
	assert.Len(t, sl, 2)
}
