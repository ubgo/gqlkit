# TypeScript: selecting fields and narrowed result types

Every generated builder for a field that returns an object has a `.select()` method. You pick fields by calling methods on a field selector, and the result type of `.execute()` contains exactly the fields you picked. Reading a field you did not select is a compile error, not a silent `undefined` at run time.

You want this page to understand what `.select()` returns, how nested objects, lists and nullable values are typed, why an object field cannot run without `.select()`, and how self-referencing fields and interfaces are selected.

## Before you start

- A generated TypeScript SDK. See [Getting started — TypeScript](../getting-started-typescript.md).
- `strict: true` in your `tsconfig.json`. Narrowing works without it, but `null` is only enforced with `strictNullChecks`.

The runnable snippets are quoted from [example-ts/cmd/samples/main.ts](../../example-ts/cmd/samples/main.ts) and [example-ts/cmd/samples/selecting.ts](../../example-ts/cmd/samples/selecting.ts) (both run against mockapi) and [example-ts/cmd/samples/type-test.ts](../../example-ts/cmd/samples/type-test.ts) (compile-time checks, no server).

## Step 1 — Select scalar fields

Each field selector method adds one field and returns a selector whose type parameter has grown by that field. `.execute()` then resolves to that accumulated type. From `type-test.ts`:

```ts
async function testMutationSelect() {
  const result = await mr
    .createTodo()
    .input({ text: "Test", userId: "1" })
    .select((t) => t.id().text().done())
    .execute();

  const id: string = result.id;
  const text: string = result.text;
  const done: boolean = result.done;
  console.log(id, text, done);
}
```

Reading a field that was not selected fails the type-check. With `.select((t) => t.id().text())`, `todo.done` gives:

```
error TS2339: Property 'done' does not exist on type '{ id: string; } & { text: string; }'.
```

## Step 2 — Select nested objects

A field that returns an object takes a callback that receives that object's selector. The callback's result type becomes the nested type, to any depth. From `main.ts`:

```ts
async function runTodosConnection(qr: QueryRoot) {
  const result = await qr
    .todosConnection()
    .filter({ done: false })
    .pagination({ limit: 10, offset: 0 })
    .select((conn) =>
      conn
        .totalCount()
        .edges((e) =>
          e
            .cursor()
            .node((t) =>
              t
                .id()
                .text()
                .done()
                .priority()
                .user((u) => u.id().name().email())
            )
        )
        .pageInfo((p) =>
          p.hasNextPage().hasPreviousPage().startCursor().endCursor()
        )
    )
    .execute();



  console.log("Todos connection:", JSON.stringify(result, null, 2));
}
```

`result.edges[0].node.user.name` is a `string`; `result.edges[0].node.user.role` is a compile error because `role` was not selected.

## Step 3 — Lists, nullable results and nullable fields

The shape of the GraphQL return type is kept around the narrowed type:

| Schema | `.execute()` resolves to |
| --- | --- |
| `todos: [Todo!]!` | `Narrowed[]` |
| `todo(id: ID!): Todo` (nullable) | `Narrowed \| null` |
| `requestHeader(name: String!): String` (nullable scalar) | `string \| null` |
| `todosConnection: TodoConnection!` | `Narrowed` |
| `ping: String!` (scalar, no `.select()`) | `string` |

A nullable root result must be checked before use. From `type-test.ts`:

```ts
async function testNullableSelect() {
  const result = await qr
    .todo()
    .id("1")
    .select((t) => t.id().text())
    .execute();

  // result is narrowed type | null
  if (result) {
    const id: string = result.id;
    const text: string = result.text;
    console.log(id, text);
  }
}
```

A nullable field inside the selection (`email: String`, `priority: Int`) is typed `T | null`, and it is always present once selected: `email: string | null`, never `email?: string`. That matches what the server sends (`"email": null`). Code written for older SDKs, which typed these as optional, fails to compile:

```
error TS2322: Type 'string | null' is not assignable to type 'string | undefined'.
```

Use `T | null` in annotations and `== null` or `??` in checks. A nullable list element is `(T | null)[]`. If you need the old optional shape while migrating, see [`nullable` in config.jsonc](types-scalars-enums.md#step-5--choose-how-nullable-fields-are-typed).

## Step 4 — Object fields need `.select()`

GraphQL requires a selection set on every object field, so a builder for one starts with the result type `SelectionRequired`, and `execute()`, `executeRaw()` and `batch()` refuse to compile until `.select()` is called. `qr.todosConnection().execute()` gives:

```
error TS2684: The 'this' context of type 'TodosConnectionBuilder<SelectionRequired>' is not assignable to method's 'this' of type 'never'.
```

`type-test.ts` pins that default type:

```ts
// Test: an object-returning builder starts as SelectionRequired, so
// `qr.todosConnection().execute()` without select() does not compile
// (TS2684: the 'this' context ... is not assignable to method's 'this' of
// type 'never'). The server would reject it anyway. This annotation pins the
// default type; the negative case is documented in
// docs/typescript/selecting-fields.md.
function testWithoutSelect() {
  const unselected: TodosConnectionBuilder<SelectionRequired> = qr.todosConnection();
  console.log(unselected);
}
```

Plain JavaScript callers, and an empty selection such as `.select((t) => t)`, are stopped at run time instead, before anything is sent: `gqlkit: todosConnection returns an object type; call select() with at least one field before running it`. Scalar-returning fields such as `ping` have no `.select()` and run as they are; union and interface fields may skip `.select()` because they always request `__typename` (see [Unions](unions.md)).

## Step 5 — Set arguments before or after `.select()`

Argument setters return the same builder, so their position does not matter. From `type-test.ts`:

```ts
  // Args after select (should also work)
  const r2 = await qr
    .todosConnection()
    .select((c) => c.totalCount())
    .filter({ done: true })
    .execute();
```

## Step 6 — Self-referencing fields and interfaces

A field whose type is the type it sits on, such as `Todo.parent: Todo`, is an object field like any other: it takes a selector callback, to any depth. Being nullable, it is `{ ... } | null` and must be checked:

```ts
async function selfReferencing(): Promise<void> {
  const todo = await qr
    .todo()
    .id(CHILD_TODO_ID)
    .select((t) => t.id().text().parent((p) => p.id().text().parent((g) => g.id())))
    .execute();
  if (!todo) throw new Error(`seed todo ${CHILD_TODO_ID} missing`);

  console.log("todo:", JSON.stringify(todo));
  // `parent` is nullable: typed `{ ... } | null`, and present once selected.
  if (todo.parent === null) throw new Error(`todo ${CHILD_TODO_ID} lost its parent`);
  console.log("parent text:", todo.parent.text);
  console.log("grandparent:", todo.parent.parent ?? "none");
  assert.equal(todo.parent.id, "1");
}
```

A field that returns an interface, such as `node(id: ID!): Node`, gets the interface's shared fields plus one `on<Type>()` per implementation. `__typename` is always selected, so the result narrows like a [union](unions.md):

```ts
async function interfaceResult(): Promise<void> {
  const node = await qr
    .node()
    .id(TODO_NODE_ID)
    .select((n) => n.id().onTodo((t) => t.text()).onUser((u) => u.name()))
    .execute();
  console.log("node:", JSON.stringify(node));
  if (node === null) throw new Error(`${TODO_NODE_ID} not found`);

  // `id` comes from the interface, so it exists on every member; `text`
  // only after narrowing on __typename.
  if (node.__typename === "Todo") console.log(`todo ${node.id}: ${node.text}`);
  assert.equal(node.__typename, "Todo");
}
```

## Try it

```bash
cd example-ts
npx tsc --noEmit          # type-checks type-test.ts and every sample, no server needed
npx tsx cmd/samples/main.ts   # runs the tour against mockapi
npx tsx cmd/samples/selecting.ts
```

Output of `selecting.ts`:

```
== Selecting fields ==
todo: {"id":"2","text":"Create your first todo","parent":{"id":"1","text":"Try the ping query","parent":null}}
parent text: Try the ping query
grandparent: none
node: {"__typename":"Todo","id":"1","text":"Try the ping query"}
todo 1: Try the ping query

selecting: ok
```

`npx tsc --noEmit` prints nothing when every narrowing check passes. To see a narrowing error yourself, add `console.log(result.edges[0].node.user.role);` inside `testNestedSelect` in `type-test.ts` and run it again.

## Pitfalls

- **Nullable means `null`, not `undefined`.** A selected nullable field is always present; test it with `== null` or `?? fallback`. `=== undefined` is never true for a value the server sent.
- **Calling `.select()` twice adds up the fields but not the types.** The second call adds its fields to the first call's, while the result type is only the second callback's type. Call it once.
- **Selecting an object field with no subfields drops it.** A nested callback that selects nothing (`.user((u) => u)`) is left out of the query, because `user {}` is not valid GraphQL, while the type still claims `user: {}`. At the root, an empty selection throws before sending (Step 4).
- **Schema names that clash with generated methods get an `_` suffix.** A field named `constructor` or `select` becomes `constructor_()` / `select_()` on the selector, so the generated class still compiles. Names that do not clash are unchanged.
- **Only the type is narrowed, not the object.** `JSON.stringify(result)` shows exactly what the server sent, which is what you selected; there is no run-time filtering or validation beyond scalar [codecs](types-scalars-enums.md#step-2--convert-values-with-a-codec).

## Related

- [Arguments and input types](arguments-and-inputs.md)
- [Types, scalars and enums](types-scalars-enums.md)
- [Unions](unions.md)
- [TypeScript codegen: type-safe field selection](../typescript-codegen.md) — how the generics are generated.
