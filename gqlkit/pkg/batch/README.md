# pkg/batch

Single-request multi-query helper. Merges N generated builders into one GraphQL operation with aliased root fields, posts it once, decodes the response into a single struct.

## Two faces

This package exists in two places:

- **`gqlkit/pkg/batch`** (this directory) — the upstream implementation, used to test the merging logic against `gqlkit/pkg/builder` types.
- **`<your-sdk>/batch`** — generated alongside every SDK that gqlkit emits. Imports the SDK's local `builder` and `graphqlclient` packages, so it's fully self-contained.

End users import the **generated** package. The upstream package keeps the contract honest via tests.

## Usage

```go
import (
    "errors"
    "yourmodule/sdk/batch"
    "yourmodule/sdk/fields"
    "yourmodule/sdk/inputs"
    "yourmodule/sdk/queries"
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
        Select(func(f *fields.TodoFields) { f.ID().Text() }),
    "completed": qr.Todos().
        Filter(&inputs.TodoFilter{Done: boolPtr(true)}).
        Select(func(f *fields.TodoFields) { f.ID().Text() }),
    "users": qr.Users().
        Select(func(u *fields.UserFields) { u.ID().Name().Role() }),
})

if err != nil {
    var berr *batch.Error
    if errors.As(err, &berr) {
        // r.Open / r.Completed are still populated for aliases that resolved.
        for _, e := range berr.Errors {
            log.Printf("alias path=%v: %s", e.Path, e.Message)
        }
    } else {
        return err
    }
}
```

The same shape exists for mutations via `batch.RunMutations` + `batch.MutationItems`.

## API

| Symbol | Purpose |
|---|---|
| `RunQueries(ctx, dest, QueryItems)` | Merges query builders into one operation, decodes response into `dest`. |
| `RunMutations(ctx, dest, MutationItems)` | Mutation-side counterpart. |
| `QueryItems` / `MutationItems` | `map[string]<batchable>` — alias → builder. Aliases become GraphQL response keys; match against `json` tags on `dest`. |
| `QueryBatchable` / `MutationBatchable` | Interfaces every generated builder satisfies (via embedded `builder.QueryMarker` / `builder.MutationMarker`). |
| `Error` | Wraps server-returned `[]GraphQLError` so partial-success batches can still surface diagnostics. Inspect via `errors.As(err, &batch.Error{})`. |

## Behaviour

- **Op type lock at compile time.** Mixing query and mutation builders in a single batch is a compile error — the generated builders embed `builder.QueryMarker` or `builder.MutationMarker`, which gate the two interfaces.
- **Variable namespacing.** Each builder's argument names are prefixed with the alias (e.g. `$open_filter`, not `$filter`) so two builders sharing an argument coexist without colliding.
- **Deterministic query strings.** Aliases are sorted before assembly — the same input map produces a byte-identical query every run.
- **Partial-tolerant decoding.** The package always calls `json.Unmarshal(data, dest)` first, then surfaces `*Error` when the server reports GraphQL errors. Successful aliases survive even when one alias errors out.
- **Client extraction.** The transport is pulled from the first builder via the embedded `*builder.BaseBuilder.GetClient()` — there's no `client` argument on the public API.
- **Strict-client fallback.** Standard partial-tolerant decoding requires the underlying client to implement `ExecuteWithPartialData`. When it doesn't (e.g. tests using a custom client), batch falls back to `Execute` and wraps any returned `GraphQLErrors` in `*Error` — partial data is lost in that path, all-or-nothing semantics apply.

## Generated query

For the dashboard example above:

```graphql
query Batch($completed_filter: TodoFilter, $open_filter: TodoFilter) {
  open:      todos(filter: $open_filter)      { id text }
  completed: todos(filter: $completed_filter) { id text }
  users:                                      { id name role }
}
```

## Tests

Run with:

```bash
go test ./pkg/batch/...
```

The suite covers merging, variable namespacing, no-arg case, partial data, deterministic alias ordering, mutation path, and the strict-client fallback.
