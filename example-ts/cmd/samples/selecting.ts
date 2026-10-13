// Selecting fields: a self-referencing field (Todo.parent: Todo) selected
// like any other object field, and a field returning an interface
// (node(id): Node), selected through its shared fields and on<Type>().
//
// Guide: docs/typescript/selecting-fields.md
// Run:   npx tsx cmd/samples/selecting.ts   (needs mockapi)

import assert from "node:assert/strict";
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";

/** Seeded todo whose parent is todo "1"; todo "1" itself has no parent. */
const CHILD_TODO_ID = "2";

/** mockapi's node ids are typed "<Type>:<id>", because todo and user ids overlap. */
const TODO_NODE_ID = "Todo:1";

const qr = new QueryRoot(new GraphQLClient(MOCKAPI_ENDPOINT));

/** Step 1: a field whose type is its own parent type takes a selector callback too. */
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

/** Step 2: an interface result: shared fields plus on<Type>() per implementation. */
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

export async function run(): Promise<void> {
  console.log("== Selecting fields ==");
  await selfReferencing();
  await interfaceResult();
}

if (require.main === module) runSample("selecting", run);
