# gqlkit-sdl

A CLI tool that fetches a GraphQL schema from a live endpoint via introspection and converts it to SDL (Schema Definition Language) format.

## Installation

### Quick install (macOS / Linux)

```bash
curl -sL https://raw.githubusercontent.com/khanakia/gqlkit/main/gqlkit-sdl/install.sh | sh
```

### Manual download

Download the latest release for your platform from [GitHub Releases](https://github.com/khanakia/gqlkit/releases).

### From source

```bash
go install gqlkit-sdl@latest
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

### Flags (fetch)

| Flag                   | Required | Default          | Description                                              |
|------------------------|----------|------------------|----------------------------------------------------------|
| `--url`                | Yes      | —                | GraphQL endpoint URL                                     |
| `-o`, `--output`       | No       | `schema.graphql` | Output file path                                         |
| `-f`, `--format`       | No       | `graphql`        | Output format: `graphql` (SDL) or `json`                 |
| `-H`, `--header`       | No       | —                | HTTP header in `Key:Value` format (repeatable)           |
| `--debug`              | No       | —                | Print the curl command for debugging                     |
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

## Go API

The `schema` package can also be used programmatically:

```go
import "gqlkit-sdl/schema"

opts := &schema.FetchOptions{
    Headers: map[string]string{
        "Authorization": "Bearer token",
    },
}

introspectionSchema, err := schema.FetchSchema("https://graphql.anilist.co", opts)
if err != nil {
    log.Fatal(err)
}

// Optional: filter queries/mutations and remove unused types
introspectionSchema = schema.FilterSchema(introspectionSchema, &schema.FilterOptions{
    ExcludeQueries:   []string{"task.*", "space.*"},
    ExcludeMutations: []string{"task.*", "space.*"},
    RemoveUnused:     true,
})

sdl := schema.ConvertToSDL(introspectionSchema)
err = schema.SaveToFile(sdl, "schema.graphql")

// Or save as JSON
err = schema.SaveAsJSON(introspectionSchema, "schema.json")
```
