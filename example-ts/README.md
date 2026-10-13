# example-ts — TypeScript SDK Example

Demonstrates generating and using a TypeScript SDK from the [mockapi](../mockapi) test GraphQL server. The generated SDK provides a fully typed, fluent API where **only selected fields appear in the return type** — unselected fields are compile-time errors.

## Quick Start

```bash
# 1. Start the mock API (in another terminal)
cd ../mockapi && go run server.go

# 2. Build the runtime library
cd ../gqlkit-ts && npm install && npm run build

# 3. Install dependencies + generate + type-check
cd ../example-ts
npm install
go run ./cmd/generate
npx tsc --noEmit

# 4. Run sample queries
npm run samples                    # the tour in main.ts
npx tsx cmd/samples/run-all.ts     # every guide sample; exits non-zero on any failure
```

Every sample talks only to mockapi on `http://localhost:8081/query`. If mockapi is not running, the guide samples stop at once with `mockapi is not reachable at http://localhost:8081/query ... Start it with \`task mockapi:run\``.

Or use Taskfile:

```bash
task example-ts:setup    # First-time setup (steps 2-3)
task example-ts:run      # Run samples
```

## Structure

```
cmd/
  generate/
    main.go              TypeScript SDK generator entry point
    schema.graphql       GraphQL schema (same as mockapi)
    config.jsonc         Scalar type bindings (GraphQL → TypeScript)
  samples/
    main.ts              Tour of queries, mutations and batch
    type-test.ts         Compile-time type safety verification
    mockapi.ts           Shared endpoint constant and the mockapi-is-running check
    run-all.ts           Runs every guide sample below; non-zero exit on failure
    client-options.ts    Guide sample: auth, headers, per-call signal, fake fetch
    selecting.ts         Guide sample: self-referencing field, interface result
    enums.ts             Guide sample: arguments, inputs, enums
    scalars.ts           Guide sample: DateTime codec, executeRaw, nullable fields
    errors.ts            Guide sample: GraphQLErrors, GraphQLHTTPError, network errors
    batch-options.ts     Guide sample: batch, opName, batched mutations
    unions.ts            Guide sample: union selection with on<Type>()
    low-level.ts         Guide sample: client.execute, BaseBuilder, FieldSelection
codecs/
  datetime.ts            DateTime scalar codec (decode/encode), wired in by config.jsonc
sdk/                     Generated SDK (do not edit)
  builder/               Re-exports from gqlkit-ts runtime
  scalars/               TypeScript scalar type aliases
  enums/                 TypeScript enums
  types/                 TypeScript interfaces (Todo, User, etc.)
  inputs/                TypeScript input interfaces
  fields/                Field selector classes
  queries/               Query builder classes + QueryRoot
  mutations/             Mutation builder classes + MutationRoot
```

## Which sample shows what

| Sample | Shows | Guide |
| --- | --- | --- |
| `main.ts` | ping, arguments, nested selection, connections, mutations, JSON and custom scalars, `batch`, `executeRaw` | [Getting started](../docs/getting-started-typescript.md) |
| `type-test.ts` | narrowed result types, nullable results, argument order (compile-time only, no server) | [Selecting fields](../docs/typescript/selecting-fields.md) |
| `selecting.ts` | self-referencing `Todo.parent`, interface result `node(id)` | [Selecting fields](../docs/typescript/selecting-fields.md) |
| `client-options.ts` | `authToken`, `headers` (echoed back by the server), token rotation, per-call `execute({ signal })`, timeout and cancel via custom `fetch`, fake `fetch` for tests | [Client options](../docs/typescript/client.md) |
| `enums.ts` | scalar arguments, input objects, partial updates, filters, enum arguments and results | [Arguments and input types](../docs/typescript/arguments-and-inputs.md) |
| `scalars.ts` | the `DateTime` codec decodes `createdAt` to a luxon `DateTime`; `executeRaw` stays raw; `ScalarCodecError` and `err.decodeError`; nullable fields are `T \| null` | [Types, scalars and enums](../docs/typescript/types-scalars-enums.md) |
| `errors.ts` | `GraphQLErrors`, `null` results, `GraphQLHTTPError`, network failure | [Errors](../docs/typescript/errors.md) |
| `batch-options.ts` | `batch` with `opName`, the document it sends, batched mutations, up-front rejections | [Batching](../docs/typescript/batching.md) |
| `unions.ts` | `onTodo()` / `onUser()` fragments, narrowing on `__typename`, union without `select()` | [Unions and interfaces](../docs/typescript/unions.md) |
| `low-level.ts` | `executeRaw`, `client.execute<T>`, `FieldSelection`, `BaseBuilder` | [Low-level API](../docs/typescript/low-level.md) |

Run one sample with `npx tsx cmd/samples/<name>.ts`. The guide samples check their own results with `node:assert`, so a sample that prints `<name>: ok` has verified what its guide claims.

## SDK Usage Examples

```typescript
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";
import { MutationRoot } from "./sdk/mutations";

const client = new GraphQLClient("http://localhost:8081/query");
const qr = new QueryRoot(client);
const mr = new MutationRoot(client);

// Simple queries
const pong = await qr.ping().execute();
const result = await qr.sum().a(10).b(20).execute();

// Query with nested field selection
const todos = await qr.todosConnection()
    .filter({ done: false })
    .pagination({ limit: 10, offset: 0 })
    .select((conn) =>
        conn
            .totalCount()
            .edges((e) =>
                e.cursor().node((t) =>
                    t.id().text().done().user((u) => u.id().name())
                )
            )
            .pageInfo((p) => p.hasNextPage().endCursor())
    )
    .execute();

// Mutation
const created = await mr.createTodo()
    .input({ text: "Buy milk", userId: "1" })
    .select((t) => t.id().text().done())
    .execute();

// Batch — multiple builders, one HTTP request
import { batch } from "gqlkit-ts";

const { open, done } = await batch(client, {
    open:  qr.todos().filter({ done: false }).select((t) => t.id().text()),
    done:  qr.todos().filter({ done: true  }).select((t) => t.id().text()),
});
// open and done are typed independently from each builder's selection
```

## Type Safety

The SDK narrows return types based on field selection:

```typescript
// Only selected fields exist on the result type
const todo = await qr.todo().id("1")
    .select((t) => t.id().text())
    .execute();

todo.id;    // ✓ string
todo.text;  // ✓ string
todo.done;  // ✗ compile error — not selected
```

See `cmd/samples/type-test.ts` for comprehensive type safety tests.

## Configuration

`cmd/generate/config.jsonc` maps GraphQL scalars to TypeScript types. It binds `Cursor` to `string`, imports `DateTime` from `luxon` with a codec in [codecs/datetime.ts](codecs/datetime.ts) that converts values both ways, imports `JsonValue` from `type-fest`, and leaves `Metadata` unbound on purpose so it falls back to `any`. See [Types, scalars and enums](../docs/typescript/types-scalars-enums.md) for the generated output.
