# Feature reference

Every user-facing feature of gqlkit, grouped by area, with the page that explains it and a runnable program that uses it. The list is built from the code (CLI flags, config loaders, code templates and runtime exports), not from the other docs. "—" in the Example column means no example program in this repo uses the feature yet. Examples marked "needs mockapi" call the local test server, so start it first with `task mockapi:run`; `task examples:test` runs all of them. For a guided order through these features, see the [learning path](../README.md#learning-path).

## Contents

- [Schema fetching (gqlkit-sdl)](#schema-fetching-gqlkit-sdl)
- [Code generation (gqlkit CLI)](#code-generation-gqlkit-cli)
- [Configuration (config.jsonc)](#configuration-configjsonc)
- [Generated Go SDK](#generated-go-sdk)
- [Go batching](#go-batching)
- [Go HTTP client](#go-http-client)
- [Generated TypeScript SDK](#generated-typescript-sdk)
- [TypeScript runtime (gqlkit-ts)](#typescript-runtime-gqlkit-ts)
- [Tooling and workflows](#tooling-and-workflows)

## Schema fetching (gqlkit-sdl)

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Fetch a schema by introspection and write SDL (`gqlkit-sdl fetch --url`) | `gqlkit-sdl` | [Fetch a schema, Step 1](../cli/fetch-schema.md#step-1-fetch-as-sdl) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Choose the output file (`-o`, default `schema.graphql`) | `gqlkit-sdl` | [Fetch a schema, Step 1](../cli/fetch-schema.md#step-1-fetch-as-sdl) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Send auth and custom headers (`-H "Key:Value"`, repeatable) | `gqlkit-sdl` | [Fetch a schema, Step 2](../cli/fetch-schema.md#step-2-send-headers) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Save the raw introspection result as JSON (`--format json`) | `gqlkit-sdl` | [Fetch a schema, Step 3](../cli/fetch-schema.md#step-3-fetch-as-json-when-a-tool-wants-it) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Keep or drop root fields by exact name (`--only-queries`, `--only-mutations`, `--exclude-queries`, `--exclude-mutations`) | `gqlkit-sdl` | [Filter a schema, Step 1](../cli/filter-schema.md#step-1-keep-only-what-you-call) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Regex patterns in the filter flags | `gqlkit-sdl` | [Filter a schema, Step 2](../cli/filter-schema.md#step-2-or-drop-what-you-do-not-want-by-regex) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Prune types no remaining operation reaches (`--remove-unused`) | `gqlkit-sdl` | [Filter a schema](../cli/filter-schema.md#how-the-flags-combine) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Print the equivalent curl command (`--debug`) | `gqlkit-sdl` | [Fetch a schema, Step 2](../cli/fetch-schema.md#step-2-send-headers) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Print the version (`gqlkit-sdl version`) | `gqlkit-sdl` | [CLI reference: gqlkit-sdl version](../cli-reference.md#gqlkit-sdl-version) | — |
| Go API: `schema.FetchSchema`, `FilterSchema`, `ConvertToSDL`, `SaveToFile`, `SaveAsJSON` | `gqlkit-sdl/schema` | [Run the generators from Go code: fetch and filter](../cli/programmatic.md#fetch-and-filter-from-go) | — |

## Code generation (gqlkit CLI)

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Generate a Go SDK (`gqlkit generate`) | `gqlkit` | [Generate a Go SDK](../cli/generate-go.md) | [`generate-go.sh`](../../example-cli/generate-go.sh) (needs mockapi) |
| Set the SDK's Go import path (`-m`, `--module`) | `gqlkit` | [Generate a Go SDK: which import-path flag](../cli/generate-go.md#which-import-path-flag-to-pass) | [`generate-go.sh`](../../example-cli/generate-go.sh) (needs mockapi) |
| Set the package name, or pass an import path to `-p` | `gqlkit` | [Generate a Go SDK: which import-path flag](../cli/generate-go.md#which-import-path-flag-to-pass) | — |
| Detect the import path from the nearest `go.mod` (walking up) when `-m` is omitted; no `go.mod` and no `-m` is an error | `gqlkit` | [Generate a Go SDK: which import-path flag](../cli/generate-go.md#which-import-path-flag-to-pass) | — |
| Choose the output directory (`-o`, default `./sdk`) | `gqlkit` | [Generate a Go SDK, Step 3](../cli/generate-go.md#step-3-generate) | [`generate-go.sh`](../../example-cli/generate-go.sh) (needs mockapi) |
| Delete stale generated files after a schema change (`--prune`, banner files only; hand-written files kept; `Prune` from Go) | `gqlkit` | [Generate a Go SDK: Step 5](../cli/generate-go.md#step-5-regenerate-with---prune), [When to regenerate](../choosing/when-to-regenerate.md#step-1-generate-with---prune) | [`test.sh`](../../example-cli/test.sh) (`--prune` checks; needs mockapi) |
| Preview what `--prune` would delete (`--dry-run`, requires `--prune`; `PruneDryRun` from Go) | `gqlkit` | [Generate a Go SDK: Step 5](../cli/generate-go.md#step-5-regenerate-with---prune) | [`test.sh`](../../example-cli/test.sh) (failure-modes check) |
| Build one schema from several SDL files (`--schema` takes a glob) | `gqlkit` | [Recipes: Assemble a schema from several SDL files](../recipes.md#assemble-a-schema-from-several-sdl-files) | [`multi-file-schema.sh`](../../example-cli/multi-file-schema.sh) |
| Generate a TypeScript SDK (`gqlkit generate-ts`) | `gqlkit` | [Generate a TypeScript SDK](../cli/generate-ts.md) | [`generate-ts.sh`](../../example-cli/generate-ts.sh) (needs mockapi) |
| Print the version (`gqlkit version`) | `gqlkit` | [CLI reference: gqlkit version](../cli-reference.md#gqlkit-version) | — |
| Shell completion scripts (`completion` subcommand on both CLIs) | `gqlkit`, `gqlkit-sdl` | [CLI reference: gqlkit](../cli-reference.md#gqlkit) | — |
| Exit codes (0 on success, 1 on any failure) | `gqlkit`, `gqlkit-sdl` | [CLI reference: Exit codes](../cli-reference.md#exit-codes) | failure modes in [`test.sh`](../../example-cli/test.sh) (needs mockapi) |
| Run the Go generator from Go code (`clientgen.New`) | `gqlkit/pkg/clientgen` | [Run the generators from Go code](../cli/programmatic.md) | [example-go-mockapi/cmd/generate](../../example-go-mockapi/cmd/generate/main.go), [example-go-chat/cmd/generate](../../example-go-chat/cmd/generate/main.go) |
| Run the TypeScript generator from Go code (`clientgents.New`) | `gqlkit/pkg/clientgents` | [Run the generators from Go code](../cli/programmatic.md) | [example-ts/cmd/generate](../../example-ts/cmd/generate/main.go) |

## Configuration (config.jsonc)

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Pass a config file (`-c`, `--config`); JSON with comments; a path that does not exist is an error | `gqlkit` | [config.jsonc](../cli/config.md) | [`generate-go.sh`](../../example-cli/generate-go.sh) (needs mockapi) with [`go.config.jsonc`](../../example-cli/go.config.jsonc) |
| Map a custom scalar to a Go type (`bindings.<Scalar>.model`, e.g. `time.Time`) | generated Go SDK | [config.jsonc, Step 2](../cli/config.md#step-2-write-the-go-config), [Go types, Step 3](../go/types-and-scalars.md#step-3--map-your-own-scalars) | [example-go-mockapi config.jsonc](../../example-go-mockapi/cmd/generate/config.jsonc) with `runTodoWithScalars` in [samples](../../example-go-mockapi/cmd/samples/main.go) (needs mockapi) |
| Built-in Go mappings beyond the five spec scalars (`Int64`, `Int32`, `Uint*`, `Float64`, `Float32`, `Time` → `time.Time`, `JSON` → `json.RawMessage`) | generated Go SDK | [config.jsonc: built-in mappings](../cli/config.md#built-in-mappings), [Go types, Step 2](../go/types-and-scalars.md#step-2--know-which-scalars-are-mapped-for-you) | `scalarTypes` in [cmd/enums-and-inputs](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi) |
| Unmapped custom scalars fall back to `any` | generated Go and TS SDKs | [config.jsonc, Step 1](../cli/config.md#step-1-check-the-built-in-mappings-first), [Choosing a scalar binding](../choosing/scalar-bindings.md) | `Metadata` in `runTodoWithScalars`, [example-go-mockapi samples](../../example-go-mockapi/cmd/samples/main.go); [`scalars.ts`](../../example-ts/cmd/samples/scalars.ts) (needs mockapi) |
| Map a custom scalar to a TypeScript type (`"Cursor": "string"`) | generated TS SDK | [config.jsonc, Step 3](../cli/config.md#step-3-write-the-typescript-config), [TS types, Step 1](../typescript/types-scalars-enums.md#step-1--bind-custom-scalars) | [example-ts config.jsonc](../../example-ts/cmd/generate/config.jsonc), [`ts.config.jsonc`](../../example-cli/ts.config.jsonc) |
| Import a scalar's TypeScript type from an npm package (`{ "type": "DateTime", "import": "luxon" }`) | generated TS SDK | [config.jsonc, Step 3](../cli/config.md#step-3-write-the-typescript-config), [TS types, Step 1](../typescript/types-scalars-enums.md#step-1--bind-custom-scalars) | [example-ts config.jsonc](../../example-ts/cmd/generate/config.jsonc) with [`scalars.ts`](../../example-ts/cmd/samples/scalars.ts) (needs mockapi) |
| Scalar codecs (`decode`, `encode`, `codecImport`): values converted at run time in responses and arguments; `executeRaw` stays raw; failures surface as `ScalarCodecError` (or `GraphQLErrors.decodeError`) | generated TS SDK | [TypeScript types, Step 2](../typescript/types-scalars-enums.md#step-2--convert-values-with-a-codec), [config.jsonc: TypeScript codecs](../cli/config.md#typescript-codecs) | [example-ts/codecs/datetime.ts](../../example-ts/codecs/datetime.ts) with [`scalars.ts`](../../example-ts/cmd/samples/scalars.ts) (needs mockapi); [`generate-ts.sh`](../../example-cli/generate-ts.sh) |
| A TS binding without `type` (e.g. a Go `{"model": ...}` binding) is rejected | generated TS SDK | [config.jsonc: Go and TypeScript need different files](../cli/config.md#go-and-typescript-need-different-files) | — |

## Generated Go SDK

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Entry points `queries.NewQueryRoot` and `mutations.NewMutationRoot` | generated Go SDK | [Go client, Step 1](../go/client.md#step-1--create-the-client-with-its-options) | [`client-options`](../../example-go-mockapi/cmd/client-options/main.go) (needs mockapi), [`batch-mutations`](../../example-go-mockapi/cmd/batch-mutations/main.go) (needs mockapi) |
| Typed argument setters on every operation builder (`.ID("1")`, `.Filter(&inputs.TodoFilter{...})`) | generated Go SDK | [Go arguments, Step 1](../go/arguments-and-inputs.md#step-1--know-how-each-argument-maps-to-go) | [`enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi) |
| Arguments sent as typed GraphQL variables, never inlined into the query | generated Go SDK | [Go low-level builder, Step 1](../go/low-level-builder.md#step-1--print-what-a-builder-will-send) | [`low-level`](../../example-go-mockapi/cmd/low-level/main.go) (needs mockapi) |
| Field selection with `.Select(func(f *fields.XFields) { ... })` | generated Go SDK | [Go selecting fields, Step 1](../go/selecting-fields.md#step-1--select-scalar-fields-and-nested-objects) | [`selecting-fields`](../../example-go-mockapi/cmd/selecting-fields/main.go) (needs mockapi) |
| Nested object selection through selector callbacks | generated Go SDK | [Go selecting fields, Step 1](../go/selecting-fields.md#step-1--select-scalar-fields-and-nested-objects) | [`selecting-fields`](../../example-go-mockapi/cmd/selecting-fields/main.go) (needs mockapi), `runTodosConnection` in [samples](../../example-go-mockapi/cmd/samples/main.go) |
| Self-referencing fields (`Item.parent: Item`) get a selector callback, not a leaf | generated Go SDK | [Go selecting fields, Step 2](../go/selecting-fields.md#step-2--select-a-self-referencing-field-to-a-chosen-depth) | [`selecting-fields`](../../example-go-mockapi/cmd/selecting-fields/main.go) (needs mockapi) |
| Interface results: a selector with the interface's fields plus `On<Type>(...)`, decoded into a `types.<Name>Value` wrapper (`Typename`, `Value`, `Raw`); members carry `Is<Name>()` markers | generated Go SDK | [Go selecting fields, Step 3](../go/selecting-fields.md#step-3--query-a-field-that-returns-an-interface) | `interfaceResult` in [`selecting-fields`](../../example-go-mockapi/cmd/selecting-fields/main.go) (needs mockapi) |
| Unions with `On<Type>()` fragments, decoded into `types.<Name>Value`; an unknown member keeps its raw JSON | generated Go SDK | [Go selecting fields, Step 4](../go/selecting-fields.md#step-4--query-a-field-that-returns-a-union) | `unionResult` and `unknownMember` in [`selecting-fields`](../../example-go-mockapi/cmd/selecting-fields/main.go) (needs mockapi) |
| Enums as typed string constants (`enums.RoleAdmin`) | generated Go SDK | [Go arguments, Step 2](../go/arguments-and-inputs.md#step-2--pass-an-enum-or-leave-it-to-the-default) | [`enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi) |
| Input types as structs in `inputs`; nullable fields as pointers by default | generated Go SDK | [Go arguments, Step 3](../go/arguments-and-inputs.md#step-3--fill-an-input-struct-leaving-optional-fields-nil), [empty slices](../go/arguments-and-inputs.md#step-4--clear-a-list-with-an-empty-slice) | [`enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi) |
| Nullable Go input fields as pointers (default) or three-state `inputs.Opt[T]` with `Some(v)` / `Null[T]()` (`inputs.optional`, `inputs.overrides`; `opt` needs Go 1.24+) | generated Go SDK | [Go arguments, Step 5](../go/arguments-and-inputs.md#step-5--send-an-explicit-null-with-optt), [config.jsonc: Step 4](../cli/config.md#step-4-choose-how-nullable-inputs-look-in-go-optional) | `optFields` in [`enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi) with [`config.jsonc`](../../example-go-mockapi/cmd/generate/config.jsonc) |
| One Go type per GraphQL type in `types`; nullable fields as pointers | generated Go SDK | [Go types, Step 1](../go/types-and-scalars.md#step-1--read-a-generated-struct) | [`enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi) |
| Custom scalars as named types in `scalars` | generated Go SDK | [Go types, Step 3](../go/types-and-scalars.md#step-3--map-your-own-scalars) | `scalarTypes` in [cmd/enums-and-inputs](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi) |
| Schema descriptions become Go doc comments, on types, fields, enum values and operation builders | generated Go SDK | [Go types, Step 5](../go/types-and-scalars.md#step-5--recognise-names-and-comments-from-the-schema) | `NodeBuilder` in [example-go-mockapi/sdk/queries/query_node.go](../../example-go-mockapi/sdk/queries/query_node.go) (generated, no server) |
| Lowercase schema names (Hasura `timestamptz`, `order_by`) exported as valid Go identifiers | generated Go SDK | [Go types, Step 5](../go/types-and-scalars.md#step-5--recognise-names-and-comments-from-the-schema) | — |
| Generated names that would collide with builder methods get a `_` suffix (`Select_`) | generated Go SDK | [Go types, Step 5](../go/types-and-scalars.md#step-5--recognise-names-and-comments-from-the-schema) | — |
| Mutations use the same builder shape as queries | generated Go SDK | [Go arguments, Step 3](../go/arguments-and-inputs.md#step-3--fill-an-input-struct-leaving-optional-fields-nil) | [`enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs/main.go) (needs mockapi), [`batch-mutations`](../../example-go-mockapi/cmd/batch-mutations/main.go) (needs mockapi) |
| Typed result from `.Execute(ctx)` | generated Go SDK | [Go types, Step 4](../go/types-and-scalars.md#step-4--know-what-unselected-means-in-go) | [`selecting-fields`](../../example-go-mockapi/cmd/selecting-fields/main.go) (needs mockapi) |
| Untyped result map from `.ExecuteRaw(ctx)` | generated Go SDK | [Go low-level builder, Step 3](../go/low-level-builder.md#step-3--read-a-result-as-a-map) | [`low-level`](../../example-go-mockapi/cmd/low-level/main.go) (needs mockapi) |
| Low-level builder access (`BuildQuery`, `GetVariables`, `GetSelection`) | generated Go SDK | [Go low-level builder, Step 1](../go/low-level-builder.md#step-1--print-what-a-builder-will-send), [Step 2](../go/low-level-builder.md#step-2--add-a-field-by-hand) | `rawField` in [`low-level`](../../example-go-mockapi/cmd/low-level/main.go) (needs mockapi) |
| Self-contained SDK: ships its own `builder`, `graphqlclient` and `batch`, no gqlkit runtime import | generated Go SDK | [Go types: pitfalls](../go/types-and-scalars.md#pitfalls), [Go or TypeScript](../choosing/go-vs-typescript.md#the-differences) | [`test.sh`](../../example-cli/test.sh) (needs mockapi) checks the generated app's `go.mod` has no gqlkit requirement |
| Hand-built queries with the upstream `pkg/builder` | `gqlkit/pkg/builder` | [pkg/builder README](../../gqlkit/pkg/builder/README.md) | — |

## Go batching

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Several queries in one request (`batch.RunQueries`), decoded into one struct by `json` tags | generated Go SDK `batch` | [Go batching, Step 2](../go/batching.md#step-2--run-the-batch) | `runBatch` in [example-go-mockapi samples](../../example-go-mockapi/cmd/samples/main.go) (needs mockapi) |
| Several mutations in one request (`batch.RunMutations`) | generated Go SDK `batch` | [Go batching, Step 2](../go/batching.md#step-2--run-the-batch) | [`batch-mutations`](../../example-go-mockapi/cmd/batch-mutations/main.go) (needs mockapi) |
| Partial success: data for good aliases plus a `*batch.Error` that unwraps to `GraphQLErrors` | generated Go SDK `batch` | [Go errors, Step 4](../go/errors.md#step-4--keep-partial-data-from-a-batch) | `partialData` in [`errors`](../../example-go-mockapi/cmd/errors/main.go) (needs mockapi) |
| Compile-time op-kind gating: a mutation in `RunQueries` (or a query in `RunMutations`) does not compile | generated Go SDK `builder`, `batch` | [Go batching, Step 4](../go/batching.md#step-4--rely-on-the-compiler-to-keep-queries-and-mutations-apart) | [`batch-mutations/testdata/mixed`](../../example-go-mockapi/cmd/batch-mutations/testdata/mixed/main.go), compiled by its `main_test.go` and expected to fail |
| Arguments namespaced per alias (`$alias_arg`) and aliases sorted for a deterministic document | generated Go SDK `batch` | [Go batching, Step 3](../go/batching.md#step-3--read-the-document-the-batch-built) | [`batch-mutations`](../../example-go-mockapi/cmd/batch-mutations/main.go) (needs mockapi) |
| Batch input checked before sending (`ErrInvalidAlias`, `ErrNilBuilder`, `ErrVariableCollision`) | generated Go SDK `batch` | [Go batching, Step 5](../go/batching.md#step-5--let-the-batch-reject-bad-input-before-sending) | `rejected` in [`batch-mutations`](../../example-go-mockapi/cmd/batch-mutations/main.go) (needs mockapi) |

## Go HTTP client

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Create a client (`graphqlclient.NewClient(endpoint, opts...)`) | generated Go SDK `graphqlclient` | [Go client, Step 1](../go/client.md#step-1--create-the-client-with-its-options) | [`client-options`](../../example-go-mockapi/cmd/client-options/main.go) (needs mockapi) |
| Bearer token (`WithAuthToken`) | generated Go SDK `graphqlclient` | [Go client, Step 1](../go/client.md#step-1--create-the-client-with-its-options) | [`client-options`](../../example-go-mockapi/cmd/client-options/main.go) (needs mockapi) |
| Custom headers (`WithHeader`, `WithHeaders`) | generated Go SDK `graphqlclient` | [Go client, Step 1](../go/client.md#step-1--create-the-client-with-its-options) | [`client-options`](../../example-go-mockapi/cmd/client-options/main.go) (needs mockapi) |
| Custom `*http.Client` for timeouts, proxies, TLS (`WithHTTPClient`) | generated Go SDK `graphqlclient` | [Go client, Step 1](../go/client.md#step-1--create-the-client-with-its-options) | [`client-options`](../../example-go-mockapi/cmd/client-options/main.go) (needs mockapi) |
| Structured errors (`GraphQLErrors` with `Message`, `Locations`, `Path`, `Extensions`) via `errors.As`, including validation errors sent with HTTP 422 | generated Go SDK `graphqlclient` | [Go errors](../go/errors.md#step-3--handle-an-error-the-server-reported) | [`errors`](../../example-go-mockapi/cmd/errors/main.go) (needs mockapi) |
| Non-GraphQL error responses (a non-200 status with no GraphQL body) as `*graphqlclient.HTTPError` with `StatusCode` and `Body` | generated Go SDK `graphqlclient` | [Go errors, Step 6](../go/errors.md#step-6--see-an-error-status-that-is-not-graphql) | `badGateway` in [`errors`](../../example-go-mockapi/cmd/errors/main.go) (needs mockapi) |
| Cancellation and deadlines through `context.Context` | generated Go SDK | [Go client, Step 3](../go/client.md#step-3--cancel-or-time-limit-a-single-call) | [`client-options`](../../example-go-mockapi/cmd/client-options/main.go) (needs mockapi) |
| Raw query string to `json.RawMessage` (`Client.RawQuery`) | generated Go SDK `graphqlclient` | [Go low-level builder, Step 4](../go/low-level-builder.md#step-4--send-a-hand-written-operation) | `rawData` in [`low-level`](../../example-go-mockapi/cmd/low-level/main.go) (needs mockapi) |
| Data and errors together (`Client.ExecuteWithPartialData`) | generated Go SDK `graphqlclient` | [Go low-level builder, Step 5](../go/low-level-builder.md#step-5--keep-data-and-errors-together) | `partial` in [`low-level`](../../example-go-mockapi/cmd/low-level/main.go) (needs mockapi) |

## Generated TypeScript SDK

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Entry points `new QueryRoot(client)` and `new MutationRoot(client)` | generated TS SDK | [TypeScript client, Step 1](../typescript/client.md#step-1--send-a-bearer-token-and-custom-headers) | [`client-options.ts`](../../example-ts/cmd/samples/client-options.ts) (needs mockapi), [`enums.ts`](../../example-ts/cmd/samples/enums.ts) (needs mockapi) |
| Typed, chainable argument setters (`.filter({ done: false })`) | generated TS SDK | [TypeScript arguments, Step 1](../typescript/arguments-and-inputs.md#step-1--scalar-arguments) | [`enums.ts`](../../example-ts/cmd/samples/enums.ts) (needs mockapi) |
| `.select()` narrows the result type: unselected fields are compile errors | generated TS SDK | [TypeScript: selecting fields, Step 1](../typescript/selecting-fields.md#step-1--select-scalar-fields) | [type-test.ts](../../example-ts/cmd/samples/type-test.ts), [selecting.ts](../../example-ts/cmd/samples/selecting.ts) (type-test.ts is type-check only) |
| Object-returning builders require `.select()` (`SelectionRequired`: compile error TS2684, plus a run-time guard for JavaScript callers and empty selections) | generated TS SDK | [Selecting fields, Step 4](../typescript/selecting-fields.md#step-4--object-fields-need-select) | `testWithoutSelect` in [`type-test.ts`](../../example-ts/cmd/samples/type-test.ts) (type-check only) |
| Nested object selection through selector callbacks | generated TS SDK | [TypeScript selecting fields, Step 2](../typescript/selecting-fields.md#step-2--select-nested-objects) | `runTodosConnection` in [example-ts samples](../../example-ts/cmd/samples/main.ts) (needs mockapi) |
| Self-referencing fields (`parent<U>(...)`) and interface results (`node(id)`, with `on<Type>(...)`) | generated TS SDK | [TypeScript: selecting fields, Step 6](../typescript/selecting-fields.md#step-6--self-referencing-fields-and-interfaces) | [selecting.ts](../../example-ts/cmd/samples/selecting.ts) (needs mockapi) |
| Nullable outputs are `T \| null` (always present once selected); nullable inputs and arguments accept `null` | generated TS SDK | [Selecting fields, Step 3](../typescript/selecting-fields.md#step-3--lists-nullable-results-and-nullable-fields), [Types, Step 4](../typescript/types-scalars-enums.md#step-4--nullable-values-are-null) | `nullableFields` in [`scalars.ts`](../../example-ts/cmd/samples/scalars.ts) (needs mockapi), [`type-test.ts`](../../example-ts/cmd/samples/type-test.ts) (type-check only) |
| `nullable` / `nullableOverrides` in config.jsonc (`T \| null \| undefined` for migrating older code) | generated TS SDK | [Types, Step 5](../typescript/types-scalars-enums.md#step-5--choose-how-nullable-fields-are-typed), [config.jsonc, Step 5](../cli/config.md#step-5-choose-how-nullable-outputs-look-in-typescript-optional) | — |
| Enums as TypeScript `enum` | generated TS SDK | [TypeScript: types, scalars and enums](../typescript/types-scalars-enums.md#step-3--enums-as-arguments-and-results) | [enums.ts](../../example-ts/cmd/samples/enums.ts) (needs mockapi) |
| Input types as TypeScript interfaces | generated TS SDK | [TypeScript: arguments and inputs](../typescript/arguments-and-inputs.md#step-2--input-objects) | [enums.ts](../../example-ts/cmd/samples/enums.ts) (needs mockapi) |
| Explicit `null` in inputs and arguments (partial updates) | generated TS SDK | [Arguments and inputs, Step 3](../typescript/arguments-and-inputs.md#step-3--partial-updates) | `partialUpdate` in [`enums.ts`](../../example-ts/cmd/samples/enums.ts) (needs mockapi) |
| Names that clash with builder members get a `_` suffix (`select_`, `constructor_`, `onTodo_`) | generated TS SDK | [Arguments: pitfalls](../typescript/arguments-and-inputs.md#pitfalls), [Selecting: pitfalls](../typescript/selecting-fields.md#pitfalls), [Unions: pitfalls](../typescript/unions.md#pitfalls) | — |
| Object and interface types as TypeScript interfaces | generated TS SDK | [TypeScript types: what gets generated](../typescript/types-scalars-enums.md#what-gets-generated) | [example-ts samples](../../example-ts/cmd/samples/main.ts) (needs mockapi) |
| Custom scalars as type aliases or npm re-exports | generated TS SDK | [TypeScript types, Step 1](../typescript/types-scalars-enums.md#step-1--bind-custom-scalars) | [scalars.ts](../../example-ts/cmd/samples/scalars.ts) (needs mockapi) |
| Unions and interfaces selected per member with `on<Type>()`; `__typename` always selected; discriminated result types | generated TS SDK | [TypeScript: unions and interfaces](../typescript/unions.md) | [`unions.ts`](../../example-ts/cmd/samples/unions.ts) (needs mockapi) |
| Relative imports carry `.js`, so the SDK compiles under Node ESM `nodenext`, bundlers and CommonJS | generated TS SDK | [Generate a TypeScript SDK: Step 4](../cli/generate-ts.md#step-4-use-it) | [`test.sh`](../../example-cli/test.sh) (nodenext type-check) |
| Mutations use the same builder shape as queries | generated TS SDK | [TypeScript arguments, Step 2](../typescript/arguments-and-inputs.md#step-2--input-objects) | [`enums.ts`](../../example-ts/cmd/samples/enums.ts) (needs mockapi), [`batch-options.ts`](../../example-ts/cmd/samples/batch-options.ts) (needs mockapi) |
| `.execute()` typed result and `.executeRaw()` untyped result | generated TS SDK | [TypeScript low-level API, Step 1](../typescript/low-level.md#step-1--executeraw-the-untyped-data-of-a-generated-builder) | [`low-level.ts`](../../example-ts/cmd/samples/low-level.ts) (needs mockapi) |
| Several builders in one request (`batch(client, { alias: builder })`), typed per alias | `gqlkit-ts` | [TypeScript batching, Step 1](../typescript/batching.md#step-1--batch-queries-with-an-operation-name) | [`batch-options.ts`](../../example-ts/cmd/samples/batch-options.ts) (needs mockapi) |
| Batch operation name (`batch(..., { opName })`) | `gqlkit-ts` | [TypeScript: batching](../typescript/batching.md#step-1--batch-queries-with-an-operation-name) | [batch-options.ts](../../example-ts/cmd/samples/batch-options.ts) (needs mockapi) |
| Mixing query and mutation builders in one batch throws at run time | `gqlkit-ts` | [TypeScript: batching](../typescript/batching.md#step-3--what-batch-rejects-before-sending) | [batch-options.ts](../../example-ts/cmd/samples/batch-options.ts) (needs mockapi) |

## TypeScript runtime (gqlkit-ts)

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Create a client (`new GraphQLClient(endpoint, options)`) | `gqlkit-ts` | [TypeScript client](../typescript/client.md#step-1--send-a-bearer-token-and-custom-headers) | [`client-options.ts`](../../example-ts/cmd/samples/client-options.ts) (needs mockapi); [gqlkit-ts-demo](../../gqlkit-ts-demo/test.ts) (public endpoint, no mockapi) |
| Raw query string with a typed result (`client.execute<T>(query, vars)`) | `gqlkit-ts` | [TypeScript low-level API, Step 2](../typescript/low-level.md#step-2--clientexecutet-any-document-typed-result) | [`low-level.ts`](../../example-ts/cmd/samples/low-level.ts) (needs mockapi); [gqlkit-ts-demo](../../gqlkit-ts-demo/test.ts) |
| Bearer token (`authToken`) | `gqlkit-ts` | [TypeScript: the client](../typescript/client.md#step-1--send-a-bearer-token-and-custom-headers) | [client-options.ts](../../example-ts/cmd/samples/client-options.ts) (needs mockapi) |
| Custom headers (`headers`) | `gqlkit-ts` | [TypeScript: the client](../typescript/client.md#step-1--send-a-bearer-token-and-custom-headers) | [client-options.ts](../../example-ts/cmd/samples/client-options.ts) (needs mockapi) |
| Custom `fetch` implementation (SSR, tests, middleware) | `gqlkit-ts` | [TypeScript: the client](../typescript/client.md#step-6--test-without-a-server) | [client-options.ts](../../example-ts/cmd/samples/client-options.ts) (needs mockapi) |
| Per-request cancellation (`{ signal }` on generated builders' `.execute()` / `.executeRaw()`, `client.execute`, `client.rawQuery` and `batch`) | `gqlkit-ts` | [TypeScript client, Step 5](../typescript/client.md#step-5--cancel-or-time-out-one-call), [gqlkit-ts README](../../gqlkit-ts/README.md#cancelling-a-request) | `perCallSignal` in [`client-options.ts`](../../example-ts/cmd/samples/client-options.ts) (needs mockapi) |
| Structured errors (`GraphQLErrors` with `.errors[]`) | `gqlkit-ts` | [TypeScript: errors](../typescript/errors.md#step-2--graphql-errors) | [errors.ts](../../example-ts/cmd/samples/errors.ts) (needs mockapi) |
| HTTP failures (`GraphQLHTTPError`) and partial data (`err.data`) | `gqlkit-ts` | [TypeScript: errors](../typescript/errors.md#step-5--a-url-that-is-not-a-graphql-endpoint) | [errors.ts](../../example-ts/cmd/samples/errors.ts) (needs mockapi) |
| Low-level `FieldSelection` and `BaseBuilder` exports | `gqlkit-ts` | [TypeScript: low-level builders](../typescript/low-level.md#step-3--fieldselection-a-selection-tree) | [low-level.ts](../../example-ts/cmd/samples/low-level.ts) |

## Tooling and workflows

| Feature | Package | Guide | Example |
| --- | --- | --- | --- |
| Install the CLIs (`go install`, `install.sh`, release archives) | `gqlkit`, `gqlkit-sdl` | [README: Install](../../README.md#install) | — |
| Local test GraphQL server on port 8081 | `mockapi` | [mockapi README: Running](../../mockapi/README.md#running) | [`mockapi:run` task](../../Taskfile.yml) |
| Generate a Go SDK for an API you do not control | workflow | [Recipes](../recipes.md#generate-a-go-sdk-for-an-api-you-do-not-control) | [`generate-go.sh`](../../example-cli/generate-go.sh) (needs mockapi) |
| Trim a large vendor schema before generating | workflow | [Recipes](../recipes.md#trim-a-huge-vendor-schema-down-to-what-you-use) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) (needs mockapi) |
| Generate Go and TypeScript clients from one schema | workflow | [Recipes](../recipes.md#generate-both-a-go-and-a-typescript-client-from-one-schema) | [`generate-go.sh`](../../example-cli/generate-go.sh) and [`generate-ts.sh`](../../example-cli/generate-ts.sh) (needs mockapi) |
| Keep the SDK in sync as the schema changes | workflow | [Recipes](../recipes.md#keep-the-sdk-in-sync-as-the-schema-changes), [When to regenerate](../choosing/when-to-regenerate.md) | [`test.sh`](../../example-cli/test.sh) (needs mockapi; regenerates from scratch and builds) |
| Machine-readable project summary for code assistants (`llms.txt`) | repo | [Design notes for code assistants](../ai-friendly.md) | [llms.txt](../../llms.txt) |
