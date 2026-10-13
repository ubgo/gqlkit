# gqlkit-sdl

`gqlkit-sdl` fetches a GraphQL schema from a live endpoint by introspection and writes it as SDL (Schema Definition Language), the format [`gqlkit`](../gqlkit/README.md) generates SDKs from. It can also filter the schema down to the operations you use and prune the types nothing reaches any more, which keeps an SDK for a large vendor API small.

- Step-by-step guides: [Fetch a schema](../docs/cli/fetch-schema.md) and [Filter a schema](../docs/cli/filter-schema.md).
- Runnable: [`example-cli/fetch-and-filter.sh`](../example-cli/fetch-and-filter.sh) runs every flag on this page against the local test server.
- Every flag with captured output: [CLI reference](../docs/cli-reference.md#gqlkit-sdl). Release history: [CHANGELOG](./CHANGELOG.md).
- Not sure whether to fetch at all? [Where the schema comes from](../docs/choosing/schema-source.md).

## Installation

### Quick install (macOS / Linux)

```bash
curl -sL https://raw.githubusercontent.com/ubgo/gqlkit/main/gqlkit-sdl/install.sh | sh
```

### Manual download

Download the latest release for your platform from [GitHub Releases](https://github.com/ubgo/gqlkit/releases).

### From source

```bash
go install github.com/ubgo/gqlkit/gqlkit-sdl@latest
```

## Usage

```bash
gqlkit-sdl <command> [flags]
```

### Commands

| Command   | Description                  |
|-----------|------------------------------|
| `fetch`   | Fetch schema and save as SDL |
| `version` | Print version and exit       |
| `completion` | Print a shell completion script |

### Flags (fetch)

| Flag                   | Required | Default          | Description                                              |
|------------------------|----------|------------------|----------------------------------------------------------|
| `--url`                | Yes      | —                | GraphQL endpoint URL                                     |
| `-o`, `--output`       | No       | `schema.graphql` | Output file path                                         |
| `-f`, `--format`       | No       | `graphql`        | Output format: `graphql` (SDL) or `json`                 |
| `-H`, `--header`       | No       | —                | HTTP header as `Key: Value` (a space after the colon is optional); repeatable |
| `--debug`              | No       | —                | Print the equivalent curl command and exit without fetching |
| `--only-queries`       | No       | —                | Keep only these query fields (comma-separated)           |
| `--only-mutations`     | No       | —                | Keep only these mutation fields (comma-separated)        |
| `--exclude-queries`    | No       | —                | Remove these query fields (comma-separated)              |
| `--exclude-mutations`  | No       | —                | Remove these mutation fields (comma-separated)           |
| `--remove-unused`      | No       | —                | Remove types/inputs not referenced by remaining operations |

### Examples

```bash
# Check version
gqlkit-sdl version

# Basic usage
gqlkit-sdl fetch --url "https://graphql.anilist.co"

# With custom output file
gqlkit-sdl fetch --url "https://graphql.anilist.co" --output my-schema.graphql

# With authentication
gqlkit-sdl fetch --url "https://graphql.anilist.co" \
  -H "Authorization: Bearer your-token"

# With multiple headers
gqlkit-sdl fetch --url "http://localhost:2310/sa/query_playground" \
  -H "Authorization: Bearer your-token" \
  -H "Referer: http://localhost:2310/sa/gql?pkey=1234" \
  -H "Origin: http://localhost:2310"

# Save schema as JSON
gqlkit-sdl fetch --url "https://graphql.anilist.co" -f json -o schema.json
```

### Filtering Queries & Mutations

You can include or exclude specific queries and mutations by name. All filter flags accept comma-separated values and support **regex patterns**.

#### Exact names

```bash
# Keep only two specific queries, drop everything else
gqlkit-sdl fetch --url "https://example.com/graphql" \
  --only-queries "users,posts"

# Remove specific mutations
gqlkit-sdl fetch --url "https://example.com/graphql" \
  --exclude-mutations "deleteUser,deletePost"
```

#### Regex patterns

Any value containing a regex metacharacter (`. * + ? ^ $ { } ( ) | [ ] \`) is automatically treated as a regex. Patterns are anchored to match the full field name (`^pattern$`).

```bash
# Exclude all mutations starting with "task"
--exclude-mutations "task.*"

# Exclude all queries starting with "space" or "task"
--exclude-queries "space.*,task.*"

# Keep only queries ending with "List"
--only-queries ".*List"

# Keep only mutations containing "segment" (case-sensitive)
--only-mutations ".*segment.*"

# Mix exact names and regex patterns freely
--exclude-mutations "ping,task.*,space.*"
```

#### Common regex patterns

| Pattern        | Matches                                    | Example matches                            |
|----------------|--------------------------------------------|--------------------------------------------|
| `task.*`       | Any name starting with `task`              | `taskCreate`, `taskDelete`, `taskSetupGa4` |
| `.*List`       | Any name ending with `List`                | `userList`, `taskList`, `spaceList`        |
| `.*segment.*`  | Any name containing `segment`              | `segmentCreate`, `executeSegment`          |
| `get(User\|Post)` | Exactly `getUser` or `getPost`          | `getUser`, `getPost`                       |
| `.*ById`       | Any name ending with `ById`                | `getUserById`, `getPostById`               |

#### Removing unused types

When you filter out queries or mutations, their associated input types, return types, and enums may become orphaned. Use `--remove-unused` to automatically prune any type not reachable from the remaining operations.

```bash
# Exclude task/space operations and clean up orphaned types
gqlkit-sdl fetch --url "https://example.com/graphql" \
  --exclude-mutations "task.*,space.*" \
  --exclude-queries "task.*,space.*" \
  --remove-unused

# Keep only segment-related queries with a clean schema
gqlkit-sdl fetch --url "https://example.com/graphql" \
  --only-queries ".*segment.*" \
  --only-mutations ".*segment.*" \
  --remove-unused
```

> **Tip:** `--only-*` is an allowlist — if set, only matching fields survive. `--exclude-*` is a denylist — matching fields are removed. When both are set on the same root type, the allowlist is applied first, then exclusions are applied to the result.

## Pitfalls

- **`--debug` fetches nothing.** It prints the curl command and exits; drop it to fetch. It is the fastest way to see exactly which headers are sent.
- **A header needs a colon.** `-H "Authorization Bearer x"` is rejected. A repeated header keeps the last value, and a `Host` header sets the request's host rather than being sent as an ordinary header.
- **Commas split filter values, including inside a regex.** `--only-queries "todos{1,2}"` becomes two patterns. Use alternation (`todo|todos`) instead of `{m,n}`.
- **An invalid regex stops the fetch.** Nothing is written: `Error: filtering schema: invalid filter pattern "todo(": ...`.
- **Filters remove root fields, not types.** Without `--remove-unused`, every type the removed operations used is still declared, and still generated.
- **Emptying a root type.** Removing every query field is an error. Removing every mutation field drops the `Mutation` type and succeeds with only a note on stderr; check the output in scripts.
- **JSON output is for other tools.** `gqlkit generate` reads SDL only, so keep the default format for the file you generate from.
- **The CLI timeout is fixed at 2 minutes.** The Go API takes `FetchOptions.Timeout`.

More, with the exact error text: [Fetch a schema: pitfalls](../docs/cli/fetch-schema.md#pitfalls) and [Filter a schema: pitfalls](../docs/cli/filter-schema.md#pitfalls).

## Go API

The `schema` package does everything the CLI does, from Go:

```bash
go get github.com/ubgo/gqlkit/gqlkit-sdl
```

```go
import "github.com/ubgo/gqlkit/gqlkit-sdl/schema"

introspection, err := schema.FetchSchema("https://api.example.com/graphql", &schema.FetchOptions{
	Headers: map[string]string{"Authorization": "Bearer " + os.Getenv("API_TOKEN")},
	Timeout: 30 * time.Second, // zero means schema.DefaultFetchTimeout (2 minutes)
})
if err != nil {
	log.Fatal(err)
}

// Optional: filter queries/mutations and remove unused types.
trimmed, err := schema.FilterSchema(introspection, &schema.FilterOptions{
	ExcludeQueries:   []string{"task.*", "space.*"},
	ExcludeMutations: []string{"task.*", "space.*"},
	RemoveUnused:     true,
})
if err != nil {
	log.Fatal(err) // invalid regex, or every query field removed
}

if err := schema.SaveToFile(schema.ConvertToSDL(trimmed), "schema.graphql"); err != nil {
	log.Fatal(err)
}
// Or save the introspection result as JSON.
if err := schema.SaveAsJSON(trimmed, "schema.json"); err != nil {
	log.Fatal(err)
}
```

| Function | Notes |
| --- | --- |
| `FetchSchema(url, *FetchOptions)` | `Debug: true` prints a curl command and returns `(nil, nil)`, so check for a nil schema. |
| `FilterSchema(s, *FilterOptions)` | Returns a new schema; the input is not modified. A nil options pointer returns the input unchanged. |
| `ConvertToSDL(s)` | SDL as a string, without built-in scalars and directives. |
| `SaveToFile(sdl, path)`, `SaveAsJSON(s, path)` | Write with mode `0644`, replacing the file. |

The complete program, and generating an SDK in the same run: [Run the generators from Go code](../docs/cli/programmatic.md#fetch-and-filter-from-go).

## Related

- [gqlkit](../gqlkit/README.md): generate a Go or TypeScript SDK from the SDL this tool writes
- [Recipes: trim a huge vendor schema](../docs/recipes.md#trim-a-huge-vendor-schema-down-to-what-you-use) and [debug a failing fetch](../docs/recipes.md#debug-a-failing-introspection-fetch)
- [Documentation index](../docs/README.md)
- [Contributing to this module](./CONTRIBUTING.md)
