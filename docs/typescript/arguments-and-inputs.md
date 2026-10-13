# TypeScript: arguments and input types

Every argument of a GraphQL field becomes a typed setter on its builder (`.id("1")`, `.filter({ done: false })`), and every GraphQL input type becomes a TypeScript interface in `sdk/inputs`. Values are always sent as GraphQL variables, never pasted into the query text, so user input cannot change the query.

You want this page when you call a field that takes arguments, send an input object to a mutation, do a partial update, or need to know what happens with optional and required arguments.

## Before you start

- A generated TypeScript SDK; see [Getting started — TypeScript](../getting-started-typescript.md).
- mockapi running (`task mockapi:run` from the repo root) to run the sample.

Snippets are quoted from [example-ts/cmd/samples/enums.ts](../../example-ts/cmd/samples/enums.ts). Its setup:

```ts
const client = new GraphQLClient(MOCKAPI_ENDPOINT);
const qr = new QueryRoot(client);
const mr = new MutationRoot(client);
```

## Step 1 — Scalar arguments

One setter per argument, named after it and typed from the schema (`Int!` → `number`). Chain them in any order:

```ts
async function scalarArguments(): Promise<void> {
  const sum = await qr.sum().a(10).b(20).execute();
  console.log("sum(a: 10, b: 20):", sum);
  assert.equal(sum, 30);
}
```

The document sent is `query Sum($a: Int!, $b: Int!) { sum(a: $a, b: $b) }` with variables `{"a":10,"b":20}`.

## Step 2 — Input objects

An input argument takes the generated interface. Nullable input fields are optional properties, so you can leave them out. Typing the object as `NewTodo` (imported from `sdk/inputs`) catches a misspelt or missing field where you build it:

```ts
async function createWithInput(): Promise<string> {
  const input: NewTodo = {
    text: "Write the TypeScript guide",
    userId: SEED_USER_ID,
    priority: 2,
    tags: [SAMPLE_TAG, "docs"],
  };

  const created = await mr
    .createTodo()
    .input(input)
    .select((t) => t.id().text().priority().tags())
    .execute();

  console.log("created:", JSON.stringify(created));
  return created.id;
}
```

## Step 3 — Partial updates

Fields you leave out of an input object are not sent at all, so a patch-style mutation changes only what you set. An empty list is sent as `[]`, and nullable fields also accept an explicit `null`; both are different from leaving the field out. What `null` means is up to the server (mockapi ignores it):

```ts
async function partialUpdate(id: string): Promise<void> {
  const updated = await mr
    .updateTodo()
    .id(id)
    .input({ done: true })
    .select((t) => t.id().done().priority().tags())
    .execute();

  // updateTodo returns `Todo` (nullable), so the narrowed result can be null.
  if (!updated) throw new Error(`todo ${id} disappeared`);
  console.log("after { done: true }:", JSON.stringify(updated));
  assert.deepEqual(updated.tags, [SAMPLE_TAG, "docs"]); // untouched

  const cleared = await mr
    .updateTodo()
    .id(id)
    .input({ tags: [] })
    .select((t) => t.tags())
    .execute();
  console.log("after { tags: [] }:", JSON.stringify(cleared));

  // Nullable input fields accept an explicit null. What null means is up to
  // the server: mockapi treats it like an omitted field and keeps the value.
  const nulled = await mr
    .updateTodo()
    .id(id)
    .input({ priority: null })
    .select((t) => t.priority())
    .execute();
  console.log("after { priority: null }:", JSON.stringify(nulled));
}
```

## Step 4 — Inputs as query filters

Query arguments use the same interfaces:

```ts
async function filterByInput(): Promise<void> {
  const mine = await qr
    .todos()
    .filter({ done: true, minPriority: 2 })
    .select((t) => t.id().text())
    .execute();
  console.log("done todos with priority >= 2:", mine.length);
}
```

Enum arguments are covered in [Types, scalars and enums](types-scalars-enums.md#step-3--enums-as-arguments-and-results).

## Try it

```bash
cd example-ts
npx tsx cmd/samples/enums.ts
```

Output (ids grow on every run, because mockapi never reuses one):

```
== Arguments, inputs and enums ==
sum(a: 10, b: 20): 30
created: {"id":"3","text":"Write the TypeScript guide","priority":2,"tags":["sample-enums","docs"]}
after { done: true }: {"id":"3","done":true,"priority":2,"tags":["sample-enums","docs"]}
after { tags: [] }: {"tags":[]}
after { priority: null }: {"priority":2}
done todos with priority >= 2: 1
deleted: true
created user: {"id":"3","name":"Enum Sample","role":"ADMIN"}
role is GUEST: true

enums: ok
```

## Pitfalls

- **Required arguments are not enforced by the compiler.** `todo(id: ID!)` generates an `.id()` setter, but nothing stops you from calling `.execute()` without it. The server then rejects the whole operation with `Field "todo" argument "id" of type "ID!" is required, but it was not provided.` See [Errors](errors.md).
- **`null`, omitted and `undefined` are three different things on the way in.** Nullable input fields and arguments accept `null` (`priority?: number | null`, `email(v: string | null)`), which is sent as JSON `null`. Leaving a field out sends nothing. A property set to `undefined` is dropped by `JSON.stringify`, so it behaves like leaving it out. Whether `null` clears a value is the server's decision: mockapi ignores it (see the output above); many APIs clear the field.
- **Setting an argument twice keeps the last value.** `.a(1).a(2)` sends `2`. Arguments are declared in the order they were first set.
- **Argument names that clash with generated methods get an `_` suffix.** An argument named `select`, `execute`, `executeRaw`, `getOpFragment` or `constructor` becomes `select_(v)` and so on, so the builder still compiles. Other names are unchanged.
- **Server-side default values apply only when the argument is omitted.** `upsertUser(role: Role = USER)` uses `USER` when you never call `.role()`. A setter is always sent once called.
- **List arguments take arrays of the bound type.** If you bind a scalar to a union type in `config.jsonc` (for example `"ID": "string | number"`), a list of it is `(string | number)[]`. Releases before the fix for old issue #5 generated `string | number[]`, which accepted the wrong values; regenerate if you see that.

## Related

- [Selecting fields](selecting-fields.md)
- [Types, scalars and enums](types-scalars-enums.md)
- [Batching](batching.md) — how arguments are renamed when builders share one request.
- [Errors](errors.md)
