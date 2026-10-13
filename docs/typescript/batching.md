# TypeScript: batching several operations in one request

`batch()` from `gqlkit-ts` takes a map of builders, sends them as one GraphQL operation in one HTTP request, and resolves to an object with one result per key, each typed from that builder's selection. Use it for screens that load several root fields at once (a dashboard, two filtered lists side by side), or to run several mutations in a fixed order with one round trip.

## Before you start

- A generated SDK and `gqlkit-ts`; see [Getting started — TypeScript](../getting-started-typescript.md).
- mockapi running (`task mockapi:run`) for the sample.

Snippets are quoted from [example-ts/cmd/samples/batch-options.ts](../../example-ts/cmd/samples/batch-options.ts). It sends requests through a fetch that keeps each body, so it can print the document `batch()` built:

```ts
/** A fetch that keeps each request body, so the sample can show what batch() sent. */
const recordingFetch: typeof fetch = (input, init) => {
  if (typeof init?.body === "string") sentBodies.push(init.body);
  return fetch(input, init);
};

const client = new GraphQLClient(MOCKAPI_ENDPOINT, { fetch: recordingFetch });
const qr = new QueryRoot(client);
const mr = new MutationRoot(client);
```

## Step 1 — Batch queries, with an operation name

Build each operation exactly as you would for `.execute()`, but do not call `.execute()`; pass the builders to `batch()` under the keys you want back. The third argument is optional; `opName` names the operation (default `Batch`), which is what shows up in server logs and tracing:

```ts
async function dashboard(): Promise<void> {
  const { open, done, users } = await batch(
    client,
    {
      open: qr.todos().filter({ done: false }).select((t) => t.id().text()),
      done: qr.todos().filter({ done: true }).select((t) => t.id()),
      users: qr.users().select((u) => u.name().role()),
    },
    { opName: "Dashboard" },
  );

  printLastRequest();
  console.log(`open: ${open.length}, done: ${done.length}, users: ${users.length}`);
  // `open[0].text` type-checks because `text` was selected for that alias;
  // `done[0].text` would not, because only `id` was selected there.
  assert.equal(sentBodies.length, 1);
}
```

The keys become GraphQL aliases, and each builder's arguments are renamed `<alias>_<argument>` so two builders with the same argument (`filter` here) cannot collide. This is the document the call above sent:

```graphql
query Dashboard($open_filter: TodoFilter, $done_filter: TodoFilter) {
  open: todos(filter: $open_filter) {
    id
    text
  }
  done: todos(filter: $done_filter) {
    id
  }
  users: users {
    name
    role
  }
}
```

with variables `{"open_filter":{"done":false},"done_filter":{"done":true}}`.

## Step 2 — Batch mutations

Mutation builders batch the same way. GraphQL runs the root fields of a mutation one after another, in document order, and `batch()` writes them in the order of your object's keys, so the order you write is the order they run:

```ts
async function batchedMutations(): Promise<void> {
  const created = await batch(client, {
    first: mr.createTodo().input({ text: "batched 1", userId: SEED_USER_ID }).select((t) => t.id()),
    second: mr.createTodo().input({ text: "batched 2", userId: SEED_USER_ID }).select((t) => t.id()),
  });
  console.log("created ids:", created.first.id, created.second.id);
  // Sequential ids prove the server ran `first` before `second`.
  assert.ok(Number(created.first.id) < Number(created.second.id));

  const deleted = await batch(client, {
    a: mr.deleteTodo().id(created.first.id),
    b: mr.deleteTodo().id(created.second.id),
  });
  console.log("deleted:", deleted.a, deleted.b);
}
```

## Step 3 — What `batch()` rejects before sending

Some mistakes are caught in `batch()` itself, before any request goes out:

```ts
async function rejectedBeforeSending(): Promise<void> {
  const before = sentBodies.length;

  try {
    await batch(client, {
      list: qr.todos().select((t) => t.id()),
      wipe: mr.completeAllTodos(),
    });
    assert.fail("mixing a query and a mutation must throw");
  } catch (err) {
    console.log("mixed kinds:", err instanceof Error ? err.message : err);
  }

  try {
    await batch(client, {});
    assert.fail("an empty batch must throw");
  } catch (err) {
    console.log("empty map:", err instanceof Error ? err.message : err);
  }

  try {
    await batch(client, { "open-todos": qr.todos().select((t) => t.id()) });
    assert.fail("an alias that is not a GraphQL name must throw");
  } catch (err) {
    console.log("bad alias:", err instanceof Error ? err.message : err);
  }

  assert.equal(sentBodies.length, before, "nothing may be sent for a rejected batch");
}
```

The full list of up-front checks:

- mixing query and mutation builders;
- an empty map;
- an alias or `opName` that is not a valid GraphQL name (letters, digits and `_`, not starting with a digit), such as `"my-alias"` or `"1st"`;
- two aliases whose renamed variables would collide, which only happens in the rare case of alias `a` with argument `b_c` next to alias `a_b` with argument `c`.

## Try it

```bash
cd example-ts
npx tsx cmd/samples/batch-options.ts
```

Output (counts and ids depend on what mockapi holds at the time):

```
== Batching ==
query Dashboard($open_filter: TodoFilter, $done_filter: TodoFilter) {
  open: todos(filter: $open_filter) {
    id
    text
  }
  done: todos(filter: $done_filter) {
    id
  }
  users: users {
    name
    role
  }
}
variables: {"open_filter":{"done":false},"done_filter":{"done":true}}
open: 2, done: 0, users: 3
created ids: 4 5
deleted: true true
mixed kinds: batch: cannot mix operation types — saw "query" and "mutation"
empty map: batch: requires at least one builder
bad alias: batch: invalid alias "open-todos" — must match /^[_A-Za-z][_0-9A-Za-z]*$/

batch-options: ok
```

## Pitfalls

- **Mixing queries and mutations is a run-time error in TypeScript.** Every generated builder has the same structural shape, so the compiler cannot tell a query builder from a mutation builder; `batch()` checks at run time and throws. The Go SDK rejects the same mistake at compile time.
- **One failing alias fails the whole call.** If any alias returns an error, `batch()` rejects with `GraphQLErrors`. The aliases that resolved are on `err.data`, and each error's `path[0]` names the alias that failed. A validation error (a missing required argument, an unknown field) fails the entire document, so then there is no partial data at all. [Errors: partial data](errors.md#step-3--partial-data) runs this case against mockapi.
- **`batch()` never calls the builders' `execute()`.** It uses `execute()` only for the result type. Scalar [codecs](types-scalars-enums.md#step-2--convert-values-with-a-codec) still apply, per alias, because each builder hands its decoders to `batch()` along with its fragment; but a hand-written builder that post-processes inside `execute()` returns unprocessed values from `batch()`.
- **Do not use numeric-looking keys.** JavaScript reorders integer-like keys (`"1"`, `"2"`) ahead of others, which would change the mutation order; `batch()` rejects them as invalid GraphQL names anyway.
- **A builder can be reused, but not changed in between.** The builders are read when `batch()` is called. Changing an argument on a builder after passing it to `batch()` has no effect on that call.
- **One request, one HTTP status.** Batching saves round trips but also means one slow field delays every result in the batch, and one timeout cancels all of them. `batch()` accepts `signal` next to `opName` for that; see [Client options: Cancel or time out one call](client.md#step-5--cancel-or-time-out-one-call).

## Related

- [Errors](errors.md) — partial results and `path[0]`.
- [Selecting fields](selecting-fields.md) — how each alias's type is narrowed.
- [gqlkit-ts README: batch](../../gqlkit-ts/README.md#batchclient-builders-options)
- [Getting started — TypeScript: Batching](../getting-started-typescript.md#batching-multiple-queries-in-one-request)
