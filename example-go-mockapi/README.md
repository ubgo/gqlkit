# example-go-mockapi — Go SDK examples against the mock API

A generated Go SDK for the [mockapi](../mockapi) test server, plus one small program per feature. Every program here is quoted by a page in [`docs/go/`](../docs/go), and every program except `generate` and `samples` has a test that runs it against mockapi and checks the output the docs show.

## Quick start

```bash
# 1. Start the mock API (in another terminal), from the repo root
task mockapi:run

# 2. Run any example
cd example-go-mockapi
go run ./cmd/client-options

# 3. Run every example's test (fails, never skips, if mockapi is down)
go test -count=1 ./...
```

`-count=1` matters here: the tests depend on a live server, and Go's test cache cannot see that. To point the examples at another address, set `MOCKAPI_URL` (default `http://localhost:8081/query`).

## Which program shows what

| Program | Shows | Guide |
| --- | --- | --- |
| [`cmd/client-options`](cmd/client-options/main.go) | `WithAuthToken`, `WithHeader`, `WithHeaders`, `WithHTTPClient` with a timeout and a recording transport, the header as the server received it, a per-call `context` deadline | [Client](../docs/go/client.md) |
| [`cmd/selecting-fields`](cmd/selecting-fields/main.go) | self-referencing field (`Todo.parent`) to a chosen depth, an empty nested selector being dropped, a `Node` interface and a `SearchResult` union with `On<Type>` fragments and `<Name>Value` wrappers, an unknown union member | [Selecting fields](../docs/go/selecting-fields.md) |
| [`cmd/enums-and-inputs`](cmd/enums-and-inputs/main.go) | enum constants, an enum argument with and without the schema default, optional input fields as pointers, unselected fields as zero values, clearing a list with `[]`, `inputs.Opt[T]` with `Some` / `Null`, scalar types (`time.Time`, `json.RawMessage`, `any`) | [Arguments and inputs](../docs/go/arguments-and-inputs.md), [Types and scalars](../docs/go/types-and-scalars.md) |
| [`cmd/errors`](cmd/errors/main.go) | `GraphQLErrors` with `extensions.code`, partial data from a batch with `*batch.Error`, a 422 validation failure (also `GraphQLErrors`), `*graphqlclient.HTTPError` from a non-GraphQL 502, a transport error, a non-JSON 200 | [Errors](../docs/go/errors.md) |
| [`cmd/batch-mutations`](cmd/batch-mutations/main.go) | `batch.RunMutations`, the merged document (sorted aliases, `$alias_arg` variables), `ErrInvalidAlias` / `ErrNilBuilder` before sending, and a test that proves a mutation in `RunQueries` does not compile | [Batching](../docs/go/batching.md) |
| [`cmd/low-level`](cmd/low-level/main.go) | `BuildQuery` / `GetVariables`, a field added through `GetSelection`, `ExecuteRaw`, `Client.Execute`, `Client.RawQuery`, `Client.ExecuteWithPartialData` | [Low-level builder](../docs/go/low-level-builder.md) |
| [`cmd/samples`](cmd/samples/main.go) | a tour: ping, arguments, users, list with filter and pagination, connection, typed union, JSON scalar, custom scalars, todo create/update/delete, `batch.RunQueries` | [Getting started — Go](../docs/getting-started-go.md) |
| [`cmd/generate`](cmd/generate/main.go) | generating this SDK from Go code with `clientgen.New` and a `config.jsonc` | [Getting started — Go: Option B](../docs/getting-started-go.md#option-b--go-script-for-more-control) |

Each example writes its output into a `strings.Builder` (the `w` parameter in the quoted functions), which cannot fail, and `mockapi.Finish` prints it in one checked write and exits non-zero on any error. Two helper packages are shared by the examples: [`internal/mockapi`](internal/mockapi/mockapi.go) (the endpoint, `MOCKAPI_URL` and `Finish`) and [`internal/wiretap`](internal/wiretap/wiretap.go) (an `http.RoundTripper` that records what the client sends, used to print headers and batch documents). Tests use [`internal/mockapitest`](internal/mockapitest/mockapitest.go) to fail with a clear message when mockapi is not reachable.

## Shared state

mockapi keeps its data in memory and every client shares it. The examples delete the todos they create; `upsertUser` has no delete, so `enums-and-inputs` adds two users named `Eve` per run. `cmd/samples` marks every todo as done (`completeAllTodos`). Restart mockapi to get the seed data back.

## Structure

```
cmd/
  generate/              SDK generator (clientgen.New), schema.graphql, config.jsonc
  samples/               Tour of the SDK
  client-options/        One feature per folder, each with main.go + main_test.go
  selecting-fields/
  enums-and-inputs/
  errors/
  batch-mutations/       testdata/mixed/ holds the program that must not compile
  low-level/
internal/
  mockapi/               Endpoint + MOCKAPI_URL
  mockapitest/           Require(t): fail when mockapi is down
  wiretap/               Recording http.RoundTripper
sdk/                     Generated SDK (do not edit)
  builder/               FieldSelection + BaseBuilder runtime
  graphqlclient/         HTTP client and options
  batch/                 RunQueries / RunMutations
  scalars/ enums/ types/ inputs/ fields/ queries/ mutations/
```

## Regenerating the SDK

```bash
task example-go-mockapi:fetch-schema  # re-fetch the schema from a running mockapi
task example-go-mockapi:generate      # regenerate sdk/
```

After regenerating, run `go test -count=1 ./...` here: the tests pin the behaviour the guides describe, so a generator change that alters any of it shows up as a failing test with the guide page named in the test's comment.
