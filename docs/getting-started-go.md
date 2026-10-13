# GQLKit — Go SDK Getting Started Guide

GQLKit generates fully typed GraphQL client SDKs from a GraphQL schema. The generated SDK uses a **builder pattern** with type-safe field selection: field names are methods, so a typo is a compile error, and only the fields you select are requested from the server. In Go the result is always the full generated struct, so fields you did not select are present with their zero values (see [Types and scalars](go/types-and-scalars.md#step-4--know-what-unselected-means-in-go)).


## Prerequisites

- Go 1.25+
- A GraphQL API endpoint or `.graphql` schema file


## Table of Contents

- [Step 1: Install the CLI tools](#step-1-install-the-cli-tools)
- [Step 2: Get your GraphQL schema](#step-2-get-your-graphql-schema)
- [Step 3: Configure scalar type mappings](#step-3-configure-scalar-type-mappings)
- [Step 4: Generate the SDK](#step-4-generate-the-sdk)
- [Step 5: Use the generated SDK](#step-5-use-the-generated-sdk)
- [Examples](#examples)
- [Error Handling](#error-handling)
- [Client Options](#client-options)
- [Go guides](#go-guides)


## Step 1: Install the CLI tools

**Option A — Quick install (macOS / Linux):**

```bash
# Install gqlkit (SDK generator)
curl -sL https://raw.githubusercontent.com/ubgo/gqlkit/main/gqlkit/install.sh | sh

# Install gqlkit-sdl (schema fetcher)
curl -sL https://raw.githubusercontent.com/ubgo/gqlkit/main/gqlkit-sdl/install.sh | sh
```

**Option B — Download from GitHub Releases:**

Go to [Releases](https://github.com/ubgo/gqlkit/releases) and download the binary for your platform.


## Step 2: Get your GraphQL schema

If you already have a `.graphql` schema file, skip this step.

To fetch a schema from a running GraphQL endpoint via introspection:

```bash
gqlkit-sdl fetch \
  --url https://your-api.example.com/graphql \
  --output schema.graphql
```

With authentication:

```bash
gqlkit-sdl fetch \
  --url https://your-api.example.com/graphql \
  --output schema.graphql \
  -H "Authorization: Bearer YOUR_TOKEN"
```


## Step 3: Configure scalar type mappings

Create a `config.jsonc` file to map custom GraphQL scalars to Go types:

```jsonc
{
  "bindings": {
    "Time": {
      "model": "time.Time"
    },
    "JSON": {
      "model": "encoding/json.RawMessage"
    },
    "UUID": {
      "model": "github.com/google/uuid.UUID"
    }
  }
}
```

The five spec scalars (`String`, `Int`, `Float`, `Boolean`, `ID`) are mapped automatically, and so are `Int64`, `Int32`, `Uint`, `Uint64`, `Uint32`, `Float64`, `Float32`, `Time` (to `time.Time`) and `JSON` (to `json.RawMessage`), so the `Time` and `JSON` entries above are only there to show the syntax. Any other custom scalar without a binding becomes `any`. The full table and the rules for nullable fields are in [Types and scalars](go/types-and-scalars.md).


## Step 4: Generate the SDK

There are two ways to generate the SDK:

### Option A — CLI (recommended)

Use the `gqlkit` CLI for a quick, no-code setup:

```bash
gqlkit generate \
  --schema schema.graphql \
  --output ./sdk \
  --package myproject/sdk
```

**CLI flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--schema` | `-s` | *(required)* | Path to `.graphql` schema file |
| `--output` | `-o` | `./sdk` | Output directory for generated SDK |
| `--package` | `-p` | `sdk` | Go package name for the generated SDK |
| `--module` | `-m` | | Go module path for the generated SDK (e.g., `github.com/user/myapi`) |
| `--config` | `-c` | | Path to `config.jsonc` file (optional) |

#### Understanding `--package`

The `--package` flag tells the generator the Go import path of the generated SDK. This is the path that generated files use to import each other (e.g., `import "myproject/sdk/builder"`).

**Pass the full import path** — your module name + the output directory:

```bash
# If your go.mod says: module github.com/yourorg/myapi
gqlkit generate \
  --schema schema.graphql \
  --output ./sdk \
  --package github.com/yourorg/myapi/sdk

# If your go.mod says: module myproject
gqlkit generate \
  --schema schema.graphql \
  --output ./sdk \
  --package myproject/sdk
```

**Or just pass a simple name** and let gqlkit auto-detect from your `go.mod`:

```bash
# Reads "module myproject" from go.mod, combines with --output to get "myproject/sdk"
gqlkit generate \
  --schema schema.graphql \
  --output ./sdk \
  --package sdk
```

> **Note:** Auto-detection only works when a `go.mod` file exists in the current directory. If there's no `go.mod`, always pass the full import path.

### Option B — Go script (for more control)

Install the generator library:

```bash
go get github.com/ubgo/gqlkit/gqlkit/pkg/clientgen
```

Create a Go file (e.g., `cmd/generate/main.go`) to invoke the generator programmatically:

```go
package main

import (
    "fmt"

    "github.com/ubgo/gqlkit/gqlkit/pkg/clientgen"
)

func main() {
    config := &clientgen.Config{
        SchemaPath:  "cmd/generate/schema.graphql",
        OutputDir:   "./sdk",
        PackageName: "sdk",
        ModulePath:  "github.com/yourorg/yourproject/sdk",
        ConfigPath:  "cmd/generate/config.jsonc",
        Package:     "yourmodule/sdk",
    }

    gen, err := clientgen.New(config)
    if err != nil {
        fmt.Printf("failed to create generator: %v\n", err)
        return
    }

    if err := gen.Generate(); err != nil {
        fmt.Printf("failed to generate SDK: %v\n", err)
        return
    }

    fmt.Println("SDK generation completed.")
}
```

Then run it:

```bash
go run cmd/generate/main.go
```


Either option creates the `sdk/` directory with the following packages:

```
sdk/
├── builder/         # Runtime builder types
├── graphqlclient/   # HTTP client (NewClient, WithAuthToken, etc.)
├── enums/           # GraphQL enums as Go types
├── fields/          # Field selector types (one per GraphQL type)
├── inputs/          # Input type structs
├── mutations/       # Mutation builders + MutationRoot
├── queries/         # Query builders + QueryRoot
├── scalars/         # Custom scalar type aliases
└── types/           # Go struct definitions for GraphQL types
```


## Step 5: Use the generated SDK

The generated SDK is fully self-contained — no external dependencies needed. The `builder` and `graphqlclient` packages are included in the generated output.

```go
package main

import (
    "context"
    "fmt"
    "log"

    "yourmodule/sdk/graphqlclient"

    "yourmodule/sdk/fields"
    "yourmodule/sdk/inputs"
    "yourmodule/sdk/mutations"
    "yourmodule/sdk/queries"
)

func main() {
    // 1. Create the client
    client := graphqlclient.NewClient(
        "https://your-api.example.com/graphql",
        graphqlclient.WithAuthToken("YOUR_TOKEN"),
    )

    ctx := context.Background()

    // 2. Create query/mutation root
    qr := queries.NewQueryRoot(client)
    mr := mutations.NewMutationRoot(client)

    // 3. Execute queries
    todos, err := qr.Todos().
        Filter(&inputs.TodoFilter{Done: boolPtr(false)}).
        Pagination(&inputs.PaginationInput{Limit: 10, Offset: 0}).
        Select(func(f *fields.TodoFields) {
            f.ID().Text().Done().
                User(func(u *fields.UserFields) {
                    u.ID().Name()
                })
        }).
        Execute(ctx)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Got %d todos\n", len(todos))
}

func boolPtr(b bool) *bool { return &b }
```


## Examples

### Simple scalar queries

```go
// No field selection needed for scalar return types
pingResult, err := qr.Ping().Execute(ctx)

echoResult, err := qr.Echo().Message("Hello from SDK").Execute(ctx)

sumResult, err := qr.Sum().A(40).B(2).Execute(ctx)
```

### Query with nested field selection

```go
users, err := qr.Users().
    Select(func(u *fields.UserFields) {
        u.ID().Name().Email().Role()
    }).
    Execute(ctx)
```

### Pagination with cursor-based connections

```go
conn, err := qr.TodosConnection().
    Filter(&inputs.TodoFilter{Done: boolPtr(false)}).
    Pagination(&inputs.PaginationInput{Limit: 5, Offset: 0}).
    Select(func(c *fields.TodoConnectionFields) {
        c.TotalCount().
            PageInfo(func(p *fields.PageInfoFields) {
                p.HasNextPage().HasPreviousPage().StartCursor().EndCursor()
            }).
            Edges(func(e *fields.TodoEdgeFields) {
                e.Cursor().Node(func(t *fields.TodoFields) {
                    t.ID().Text().Done()
                })
            })
    }).
    Execute(ctx)
```

### Mutations (create, update, delete)

```go
// Create
created, err := mr.CreateTodo().
    Input(inputs.NewTodo{
        Text:   "SDK-created todo",
        UserID: "1",
        Tags:   []string{"sdk", "created"},
    }).
    Select(func(f *fields.TodoFields) {
        f.ID().Text().Done().Tags()
    }).
    Execute(ctx)

// Update
updated, err := mr.UpdateTodo().
    ID(created.ID).
    Input(inputs.UpdateTodoInput{
        Text: stringPtr("Updated via SDK"),
        Done: boolPtr(true),
    }).
    Select(func(f *fields.TodoFields) {
        f.ID().Text().Done().Tags()
    }).
    Execute(ctx)

// Delete
deleted, err := mr.DeleteTodo().ID(created.ID).Execute(ctx)

// Scalar mutation (returns int)
completedCount, err := mr.CompleteAllTodos().Execute(ctx)
```

### Raw JSON access

```go
rawData, err := qr.Ping().ExecuteRaw(ctx)
// rawData is map[string]interface{}
```

### Union types

A union (`search` returns `[SearchResult!]!` where `union SearchResult = Todo | User`) gets a selector with one `On<Type>` method per member. `__typename` is always requested, and each result is a `types.SearchResultValue` whose `Value` is the concrete `*types.Todo` or `*types.User`. Interfaces work the same way. [Selecting fields](go/selecting-fields.md#step-3--query-a-field-that-returns-an-interface) covers both, including what happens when the server sends a member the SDK does not know.

```go
results, err := qr.Search().
    Term("a").
    Select(func(s *fields.SearchResultFields) {
        s.OnTodo(func(t *fields.TodoFields) { t.ID().Text() }).
            OnUser(func(u *fields.UserFields) { u.ID().Name().Role() })
    }).
    Execute(ctx)
if err != nil {
    log.Fatal(err)
}
for _, r := range results {
    switch v := r.Value.(type) {
    case *types.Todo:
        fmt.Println("todo", v.ID, v.Text)
    case *types.User:
        fmt.Println("user", v.ID, v.Name)
    default:
        fmt.Println("unknown member", r.Typename) // r.Raw keeps the JSON
    }
}
```

### Batching multiple queries in one request

`batch.RunQueries` merges several builders into a single GraphQL operation with aliased root fields, posts it once, and decodes the response into a struct keyed by alias via `json` tags. Use it when a screen needs several lists at once (dashboards, side-by-side filtered views).

```go
import (
    "errors"
    "yourmodule/sdk/batch"
    "yourmodule/sdk/fields"
    "yourmodule/sdk/inputs"
    "yourmodule/sdk/types"
)

type Dashboard struct {
    Open      []types.Todo `json:"open"`
    Completed []types.Todo `json:"completed"`
    Users     []types.User `json:"users"`
}

var r Dashboard
err := batch.RunQueries(ctx, &r, batch.QueryItems{
    "open": qr.Todos().
        Filter(&inputs.TodoFilter{Done: boolPtr(false)}).
        Select(func(f *fields.TodoFields) { f.ID().Text().Done() }),
    "completed": qr.Todos().
        Filter(&inputs.TodoFilter{Done: boolPtr(true)}).
        Select(func(f *fields.TodoFields) { f.ID().Text().Done() }),
    "users": qr.Users().
        Select(func(u *fields.UserFields) { u.ID().Name().Role() }),
})

if err != nil {
    var berr *batch.Error
    if errors.As(err, &berr) {
        // Partial-success: r.Open / r.Completed are still populated for
        // aliases that resolved; berr.Errors carries per-alias diagnostics.
        for _, e := range berr.Errors {
            log.Printf("alias path=%v: %s", e.Path, e.Message)
        }
    } else {
        return err
    }
}

fmt.Printf("open=%d completed=%d users=%d\n",
    len(r.Open), len(r.Completed), len(r.Users))
```

Generated query, as sent (aliases are sorted, so the order differs from the map literal):

```graphql
query Batch($completed_filter: TodoFilter, $open_filter: TodoFilter) {
  completed: todos(filter: $completed_filter) {
    id
    text
    done
  }
  open: todos(filter: $open_filter) {
    id
    text
    done
  }
  users: users {
    id
    name
    role
  }
}
```

Rules:

- Argument names are namespaced with the alias to avoid collisions (`$open_filter` vs `$completed_filter`).
- Mixing query and mutation builders in one batch is a compile error — use `batch.RunMutations` for mutations ([Batching](go/batching.md) has a mutation example and the exact compiler message).
- Server-returned errors arrive as `*batch.Error` (use `errors.As`); successful aliases are still decoded into `dest` so partial UI can render.


## Error Handling

Errors the server reports in a GraphQL body arrive as `graphqlclient.GraphQLErrors`, whatever the HTTP status (gqlgen answers validation failures with 422). A non-200 status without a GraphQL body (a gateway or proxy error page) arrives as `*graphqlclient.HTTPError`, and network failures as `*url.Error`. [Errors](go/errors.md) shows all of them and how to keep partial data from a batch.

```go
import (
    "errors"
    "yourmodule/sdk/graphqlclient"
)

result, err := qr.Todos().Select(/* ... */).Execute(ctx)
if err != nil {
    var gqlErrs graphqlclient.GraphQLErrors
    if errors.As(err, &gqlErrs) {
        // Structured GraphQL errors
        for _, e := range gqlErrs {
            fmt.Printf("GraphQL error: %s\n", e.Message)
            fmt.Printf("  Path: %v\n", e.Path)
            fmt.Printf("  Extensions: %v\n", e.Extensions)
        }
    } else {
        // Network or other error
        fmt.Printf("Error: %v\n", err)
    }
}
```


## Client Options

Options apply in order and a later one wins for the same header; timeouts belong on the `*http.Client`. [Client](go/client.md) covers per-call deadlines with `context.Context` and how to see the headers that are actually sent.

```go
client := graphqlclient.NewClient(
    "https://your-api.example.com/graphql",

    // Bearer token authentication
    graphqlclient.WithAuthToken("YOUR_TOKEN"),

    // Custom headers
    graphqlclient.WithHeader("X-Request-ID", "abc123"),
    graphqlclient.WithHeaders(map[string]string{
        "X-Custom": "value",
    }),

    // Custom HTTP client (for timeouts, proxies, TLS, etc.)
    graphqlclient.WithHTTPClient(&http.Client{
        Timeout: 30 * time.Second,
    }),
)
```


## Go guides

One page per topic, each with a runnable example in [`example-go-mockapi/cmd`](../example-go-mockapi/cmd) that is tested against mockapi:

- [Client](go/client.md): auth token, headers, custom `http.Client`, timeouts and cancellation
- [Selecting fields](go/selecting-fields.md): nested objects, self-referencing fields, interfaces and unions
- [Arguments and inputs](go/arguments-and-inputs.md): setters, input structs, pointers, enums as arguments
- [Types and scalars](go/types-and-scalars.md): generated types, built-in and custom scalar mappings, what unselected fields hold
- [Errors](go/errors.md): `GraphQLErrors`, HTTP and transport errors, partial data
- [Batching](go/batching.md): `RunQueries`, `RunMutations`, alias namespacing, compile-time query/mutation separation
- [Low-level builder](go/low-level-builder.md): `BuildQuery`, `GetSelection`, `ExecuteRaw`, `Client.Execute`
