# TypeScript: low-level API (raw queries, BaseBuilder, FieldSelection)

Underneath every generated builder are three public pieces of `gqlkit-ts`: `GraphQLClient.execute<T>()`, which sends any document; `BaseBuilder`, which assembles one operation with variables; and `FieldSelection`, which models a selection set. You normally never touch them. Reach for them when the generated SDK cannot express what you need: a union or interface member's fields, a hand-written query copied from a GraphQL IDE, a field from a part of the schema you did not generate, or a debugging session where you want to see the exact document sent.

## Before you start

- `gqlkit-ts` installed. The generated SDK is optional for raw queries.
- mockapi running (`task mockapi:run`) for the sample.

Snippets are quoted from [example-ts/cmd/samples/low-level.ts](../../example-ts/cmd/samples/low-level.ts), which imports:

```ts
import assert from "node:assert/strict";
import { BaseBuilder, FieldSelection, GraphQLClient, OperationType } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";
```

## Step 1 — `executeRaw()`: the untyped `data` of a generated builder

Every generated builder has `executeRaw()`, which sends the same document as `execute()` and resolves to the whole `data` object, typed `Record<string, unknown>`, with no scalar codecs applied:

```ts
async function executeRaw(): Promise<void> {
  const data = await new QueryRoot(client).sum().a(2).b(3).executeRaw();
  console.log("executeRaw:", JSON.stringify(data));
  assert.deepEqual(data, { sum: 5 });
}
```

## Step 2 — `client.execute<T>()`: any document, typed result

Pass the document and the variables. The type argument describes `data`; the client checks only that `data` is an object, not that it matches `T`:

```ts
async function typedRawQuery(): Promise<void> {
  // The type argument is your claim about `data`; the client does not check it.
  type TodoTitles = { todos: { id: string; text: string }[] };

  const data = await client.execute<TodoTitles>(
    `query TodoTitles($limit: Int!) {
      todos(pagination: { limit: $limit, offset: 0 }) { id text }
    }`,
    { limit: 1 },
  );
  console.log("execute<T>:", data.todos[0]?.text);
  assert.equal(data.todos.length, 1);
}
```

`client.rawQuery(query, variables)` is the same call with an `unknown` result, for when you do not want to state a type at all.

## Step 3 — `FieldSelection`: a selection tree

`addField` adds a leaf, `addChild` adds a nested selection under a field name (or under an inline fragment head such as `... on User`), and `build()` renders it:

```ts
function selectionTree(): void {
  const user = new FieldSelection();
  user.addField("name");

  const todo = new FieldSelection();
  todo.addField("id");
  todo.addChild("user", user);
  // An empty child is dropped from the output, because `field {}` is invalid GraphQL.
  todo.addChild("tags", new FieldSelection());

  console.log(todo.build());
}
```

## Step 4 — `BaseBuilder`: a whole operation

`BaseBuilder` takes the client, the operation kind, the operation name and the root field. `setArg` records a value with its GraphQL type, which becomes a variable declaration; `buildQuery()` and `getVariables()` show exactly what will be sent:

```ts
async function baseBuilder(): Promise<void> {
  const builder = new BaseBuilder(client, OperationType.Query, "OneTodo", "todo");
  builder.setArg("id", "1", "ID!");
  builder.getSelection().addField("id");
  builder.getSelection().addField("text");

  console.log(builder.buildQuery());
  console.log("variables:", JSON.stringify(builder.getVariables()));

  const data = await builder.executeRaw();
  console.log("data:", JSON.stringify(data));
}
```

`addField` and `setArg` take optional extra arguments (a decoder, an encoder) that generated code uses to wire [scalar codecs](types-scalars-enums.md#step-2--convert-values-with-a-codec); a hand-built operation like this one sends and returns wire values unless you pass them.

## Try it

```bash
cd example-ts
npx tsx cmd/samples/low-level.ts
```

Output:

```
== Low-level API ==
executeRaw: {"sum":5}
execute<T>: Try the ping query
  id
  user {
    name
  }
query OneTodo($id: ID!) {
  todo(id: $id) {
    id
    text
  }
}
variables: {"id":"1"}
data: {"todo":{"id":"1","text":"Try the ping query"}}

low-level: ok
```

## Pitfalls

- **`T` in `execute<T>()` is not validated.** A wrong type compiles and fails later, wherever the missing field is read. For anything that comes from outside your control, read the result as `unknown` and narrow it, as the union sample does.
- **`setArg`'s third argument must be the exact GraphQL type, including `!`.** It is pasted into the variable declaration. `"ID"` where the schema says `ID!` is a server validation error.
- **Field names and fragment heads are not checked.** `addField("txet")` is a server error at run time.
- **`FieldSelection` keys are emitted verbatim.** Use only field names, aliases (`"short: text"`) and `... on Type` heads you wrote yourself. Never build a key from user input; pass user input through `setArg` so it travels as a variable.
- **`addChild` with an existing key replaces that child** but keeps its position; `addField` twice emits the field twice, which GraphQL merges.
- **`OperationType` has no subscription.** The runtime has no streaming transport, so there is no way to run a subscription through `gqlkit-ts`.
- **Generated builders hide their `BaseBuilder`.** It is a `#private` field, so you cannot add fields to a generated builder from outside; build the whole operation by hand instead.
- **`client.execute()` and `rawQuery()` take `{ signal }` as a third argument** (`RequestOptions`), for cancelling one call; see [Client options](client.md#step-5--cancel-or-time-out-one-call).

## Related

- [Unions](unions.md) — the main reason to use this API today.
- [Errors](errors.md) — these calls throw the same errors as `execute()`.
- [gqlkit-ts README](../../gqlkit-ts/README.md) — full runtime reference.
