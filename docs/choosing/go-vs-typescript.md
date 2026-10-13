# Go or TypeScript: what differs between the two SDKs

Both SDKs come from the same schema with the same builder shape: a root, one builder per operation, typed argument setters, a selector callback, and `execute`. The differences are in what the type system can promise and in how batching and runtime packaging work. Read this before you rely on a guarantee from one language in the other, or when you generate both from one schema.

## Before you start

Generate one of each with [`example-cli/generate-go.sh`](../../example-cli/generate-go.sh) and [`example-cli/generate-ts.sh`](../../example-cli/generate-ts.sh) to compare the output side by side; both run against the local test server.

## The differences

| Area | Go SDK | TypeScript SDK | Guides |
| --- | --- | --- | --- |
| Result type after a selection | The full `types.X` struct. Fields you did not select are present and hold zero values (`""`, `false`, `nil`). | Narrowed to the selected fields. Reading an unselected field is a compile error (`TS2339`). | [Go](../go/types-and-scalars.md#step-4--know-what-unselected-means-in-go), [TS](../typescript/selecting-fields.md) |
| Nullable fields | Nullable scalars are pointers (`Priority *int`). Object fields are pointers whether nullable or not (`User *User` for `user: User!`). | `T \| null` in the narrowed type, matching the `null` the server sends. Config `"nullable": "null-undefined"` (globally or per type/field with `nullableOverrides`) loosens it to `T \| null \| undefined` for code written against older SDKs. | [Go](../go/types-and-scalars.md#step-1--read-a-generated-struct), [TS](../typescript/types-scalars-enums.md#step-4--nullable-values-are-null) |
| Interfaces | The selector has the interface's own fields plus `On<Type>(...)` per implementation; `Execute` returns a `*types.NodeValue` wrapper whose `Value` you type-switch on. | The selector has the interface's own fields plus `on<Type>(...)`; the result is a union discriminated on `__typename`. | [Go](../go/selecting-fields.md#step-3--query-a-field-that-returns-an-interface), [TS](../typescript/selecting-fields.md#step-6--self-referencing-fields-and-interfaces) |
| Unions | `OnTodo(...)`, `OnUser(...)` on the union's selector; results are `types.SearchResultValue` wrappers (`Typename`, `Value`, `Raw`), and an unknown member keeps its raw JSON instead of failing. | `onTodo(...)`, `onUser(...)` on the union's selector; the result type is narrowed per member and discriminated on `__typename`. | [Go](../go/selecting-fields.md#step-4--query-a-field-that-returns-a-union), [TS](../typescript/unions.md) |
| Batching | `batch.RunQueries` and `batch.RunMutations` decode into your struct by `json` tags. Putting a mutation in `RunQueries` does not compile. | `batch(client, { alias: builder })` returns a result typed per alias. Mixing a query and a mutation throws at run time. | [Go](../go/batching.md), [TS](../typescript/batching.md) |
| Errors in a batch | Partial success: aliases that resolved are decoded, and a `*batch.Error` (unwrapping to `GraphQLErrors`) lists the failures. | The call rejects with `GraphQLErrors`; the aliases that resolved are on `err.data`, untyped. | [Go](../go/errors.md#step-4--keep-partial-data-from-a-batch), [TS](../typescript/errors.md#step-3--partial-data) |
| Runtime | Self-contained: the SDK ships its own `builder`, `graphqlclient` and `batch` packages. `go.mod` gains nothing. Needs Go 1.24 or later (a build guard stops older toolchains). | Needs the `gqlkit-ts` npm package at run time. | [Go client](../go/client.md), [TS client](../typescript/client.md) |
| Import paths | Absolute; the generator must know the SDK's import path (`-m`). | Relative; no flag needed. | [Generate Go](../cli/generate-go.md#which-import-path-flag-to-pass), [Generate TS](../cli/generate-ts.md) |
| Module systems | Any Go module. | Imports carry `.js` suffixes, so Node ESM (`nodenext`), bundler and CommonJS resolution all work. | [Generate TS](../cli/generate-ts.md) |
| Built-in scalars | Also maps `Int64`, `Int32`, `Uint*`, `Float32/64`, `Time` → `time.Time`, `JSON` → `json.RawMessage`. | Maps the numeric extras to `number`; `Time` and `JSON` fall back to `any`. | [Built-in mappings](../cli/config.md#built-in-mappings) |
| Config file format | `bindings` as `{ "model": "time.Time" }` objects, plus `inputs` (pointer or `Opt[T]`). | `bindings` as a string or `{ "type", "import", "decode", "encode", "codecImport" }`, plus `nullable` / `nullableOverrides`. The two formats are not interchangeable. | [config.jsonc](../cli/config.md) |
| Timeouts and cancellation | `Execute(ctx)` honours the context's deadline and cancellation; a client-wide timeout goes on the `http.Client`. | `execute({ signal })` on generated builders, and `{ signal }` on `client.execute`, `client.rawQuery` and `batch`; `AbortSignal.timeout(ms)` gives a deadline. A `fetch` wrapper sets one for every request. | [Go client, Step 3](../go/client.md#step-3--cancel-or-time-limit-a-single-call), [TS client](../typescript/client.md#step-5--cancel-or-time-out-one-call) |
| Changing the token later | Headers are fixed when the client is built; use a `RoundTripper` or a new client. | Mutate the options object you passed; the client reads it on every request. | [Go client: pitfalls](../go/client.md#pitfalls), [TS client, Step 2](../typescript/client.md#step-2--rotate-the-token-without-a-new-client) |

## If X, then Y

- **If you need "I can only read what I fetched" enforced by the compiler**, that holds in TypeScript only. In Go, code review is the guard: a zero value from an unselected field looks like real data.
- **If you query unions or interfaces heavily**, both SDKs have typed `On<Type>` / `on<Type>` selectors. TypeScript narrows the result to each member's selected fields; Go gives you a wrapper to type-switch on, with the full member struct inside.
- **If you batch mutations and want a mistake caught before run time**, Go catches it at compile time; TypeScript catches it when the batch runs.
- **If you need typed partial results from a batch**, Go gives them to you; in TypeScript, read `err.data` and cast it yourself, or split independent work into separate requests.
- **If you need to send an explicit `null` in a Go input** (to clear a field in an update), set `"inputs": { "optional": "opt" }` in the Go config; nullable input fields then become `inputs.Opt[T]` with `Some(v)` / `Null[T]()`. The default pointer mode cannot express null. TypeScript inputs accept `null` directly.
- **If you generate both from one schema**, keep two config files (one per language) and regenerate both together. The [both-clients recipe](../recipes.md#generate-both-a-go-and-a-typescript-client-from-one-schema) shows the commands.

## Pitfalls

- Copying a TypeScript habit into Go: `todo.Done` after selecting only `ID()` and `Text()` compiles and is `false`, not "missing".
- Copying a Go config into TypeScript: `generate-ts` writes `export type DateTime = ;` without an error. See [config.jsonc](../cli/config.md#go-and-typescript-need-different-files).
- Expecting `Time` to be typed in TypeScript because it is in Go: bind it explicitly.

## Related

- [Generate a Go SDK](../cli/generate-go.md) and [Generate a TypeScript SDK](../cli/generate-ts.md)
- [One request or several](./single-vs-batch.md)
- [Go guides](../go/client.md) and [TypeScript guides](../typescript/client.md), indexed in [the docs index](../README.md)
- [Feature reference](../reference/features.md)
