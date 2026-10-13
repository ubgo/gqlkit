# One request or several: single operations vs batching

Each generated builder's `Execute` / `execute` sends one HTTP request for one root field. The batch helpers merge several builders into one GraphQL document with one alias per builder, so N operations cost one round trip. This page helps you decide when the merge is worth it.

## Before you start

You need a generated SDK. Batching is in the generated Go `batch` package (`batch.RunQueries`, `batch.RunMutations`) and in the `gqlkit-ts` runtime (`batch(client, { ... })`). The step-by-step guides are [Go batching](../go/batching.md) and [TypeScript batching](../typescript/batching.md).

## Decision table

| Situation | Use | Why | Guide | Example |
| --- | --- | --- | --- | --- |
| One operation | `Execute(ctx)` / `.execute()` | Simplest; the typed result is the return value. | [Go: selecting fields](../go/selecting-fields.md), [TS: selecting fields](../typescript/selecting-fields.md) | [Go `selecting-fields`](../../example-go-mockapi/cmd/selecting-fields/main.go), [TS `selecting.ts`](../../example-ts/cmd/samples/selecting.ts) |
| Several independent reads for one screen or job | Batch | One round trip instead of N; latency is the slowest field, not the sum. | [Go](../go/batching.md#step-2--run-the-batch), [TS](../typescript/batching.md#step-1--batch-queries-with-an-operation-name) | `runBatch` in [Go samples](../../example-go-mockapi/cmd/samples/main.go), [TS `batch-options.ts`](../../example-ts/cmd/samples/batch-options.ts) |
| Reads where one needs the result of another | Separate calls, in order | A batch sends everything at once; nothing can depend on another alias's answer. | — | — |
| Several writes that must all succeed or none | Neither on its own | A batch is not a transaction. The server runs top-level mutation fields one after another, and a failure in one does not undo the others. Use a single server-side mutation that does the whole job. | — | — |
| Several independent writes | `batch.RunMutations` (Go) or `batch` with mutation builders (TS) | One round trip; GraphQL runs top-level mutation fields in order. | [Go](../go/batching.md), [TS](../typescript/batching.md#step-2--batch-mutations) | [Go `batch-mutations`](../../example-go-mockapi/cmd/batch-mutations/main.go), [TS `batch-options.ts`](../../example-ts/cmd/samples/batch-options.ts) |
| You need whatever succeeded, even if some parts fail | Either; Go is typed | Go decodes every alias that resolved into your struct and returns a `*batch.Error` for the rest. TypeScript rejects with `GraphQLErrors` and puts the resolved aliases, untyped, on `err.data`. | [Go errors: partial data](../go/errors.md#step-4--keep-partial-data-from-a-batch), [TS errors: partial data](../typescript/errors.md#step-3--partial-data) | [Go `errors`](../../example-go-mockapi/cmd/errors/main.go), [TS `errors.ts`](../../example-ts/cmd/samples/errors.ts) |
| One query, different variants (open and done todos) | Batch with two aliases | Arguments are namespaced per alias (`$open_done`, `$completed_done`), so the same builder with different arguments does not collide. | [Go: read the document](../go/batching.md#step-3--read-the-document-the-batch-built) | `runBatch` in [Go samples](../../example-go-mockapi/cmd/samples/main.go) |

## If X, then Y

- **If you put a mutation builder in `batch.RunQueries` (Go)**, it does not compile: query builders carry a query marker and mutation builders a mutation marker. In TypeScript, mixing kinds compiles and throws `batch: cannot mix operation types` when called. See [Go batching, Step 4](../go/batching.md#step-4--rely-on-the-compiler-to-keep-queries-and-mutations-apart).
- **If an alias is not a valid GraphQL name** (`my-alias`, `1st`), Go returns `batch.ErrInvalidAlias` and TypeScript throws `batch: invalid alias`, both before sending anything ([Go batching, Step 5](../go/batching.md#step-5--let-the-batch-reject-bad-input-before-sending)). Stick to letters, digits and `_`, starting with a letter or `_`.
- **If you need the result typed in Go**, declare a struct whose `json` tags are the aliases; `RunQueries` decodes into it. TypeScript infers the result type from the object you pass.
- **If you want a meaningful operation name in server logs**, TypeScript takes `{ opName }`; Go always sends `Batch`.
- **If a batch grows large**, keep in mind it is one document: the server's query complexity or depth limits apply to the whole thing.

## Pitfalls

- **Partial data in Go looks like success if you skip the error.** Always check `err`; when it is a `*batch.Error`, some aliases are zero values.
- **A validation error fails the whole document.** A missing required argument or an unknown field in one alias means no alias runs, so there is no partial data in either language.
- **Ordering of aliases is deterministic but not yours in Go.** Go sorts aliases when it builds the document; TypeScript keeps the key order of the object you pass. If mutations must run in a specific order, send them separately rather than relying on either.

## Related

- [Go batching](../go/batching.md) and [TypeScript batching](../typescript/batching.md)
- [Go or TypeScript](./go-vs-typescript.md)
- [Which method to call](./execute-methods.md)
- [Feature reference: Go batching](../reference/features.md#go-batching)
