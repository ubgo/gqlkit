// This file verifies that the generated SDK types compile correctly.
// It should compile with zero errors against the current schema.

import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MutationRoot } from "../../sdk/mutations";
import type { SelectionRequired } from "../../sdk/builder";
import type { TodosConnectionBuilder } from "../../sdk/queries/todos-connection";

const client = new GraphQLClient("http://localhost/graphql");
const qr = new QueryRoot(client);
const mr = new MutationRoot(client);

// Test: simple scalar queries (no select, no generic)
async function testPing() {
  const result: string = await qr.ping().execute();
  console.log(result);
}

// Test: todosConnection with select → narrowed return type
async function testTodosConnection() {
  const result = await qr
    .todosConnection()
    .filter({ done: false })
    .pagination({ limit: 5, offset: 0 })
    .select((conn) =>
      conn
        .totalCount()
        .edges((e) =>
          e
            .cursor()
            .node((t) => t.id().text().done().priority().tags())
        )
        .pageInfo((p) => p.hasNextPage().endCursor())
    )
    .execute();

  // These compile because they were selected:
  const count: number = result.totalCount;
  const cursor: string = result.edges[0].cursor;
  const id: string = result.edges[0].node.id;
  const text: string = result.edges[0].node.text;
  const done: boolean = result.edges[0].node.done;
  const tags: string[] = result.edges[0].node.tags;
  const hasNext: boolean = result.pageInfo.hasNextPage;

  // Nullable field (priority is nullable in schema): always present once
  // selected, and null when the server has no value.
  const priority: number | null = result.edges[0].node.priority;

  console.log(count, cursor, id, text, done, tags, hasNext, priority);
}

// Test: nested field selection (todo -> user)
async function testNestedSelect() {
  const result = await qr
    .todosConnection()
    .select((conn) =>
      conn.edges((e) =>
        e.node((t) =>
          t
            .id()
            .text()
            .user((u) => u.id().name().email().role())
        )
      )
    )
    .execute();

  const node = result.edges[0].node;
  const userName: string = node.user.name;
  const userEmail: string | null = node.user.email;
  console.log(userName, userEmail);
}

// Test: list return type (todos query returns Todo[])
async function testTodosListSelect() {
  const result = await qr
    .todos()
    .filter({ textContains: "buy" })
    .select((t) => t.id().text().done())
    .execute();

  // result is an array of narrowed type
  const first = result[0];
  const id: string = first.id;
  const text: string = first.text;
  const done: boolean = first.done;
  console.log(id, text, done);
}

// Test: nullable return type (todo query returns Todo | null)
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

// Test: mutation with select
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

// Test: simple mutations (no select)
async function testSimpleMutations() {
  const deleted: boolean = await mr.deleteTodo().id("1").execute();
  const count: number = await mr.completeAllTodos().execute();
  console.log(deleted, count);
}

// Test: args before and after select both work
async function testArgOrder() {
  // Args before select
  const r1 = await qr
    .todosConnection()
    .filter({ done: true })
    .select((c) => c.totalCount())
    .execute();

  // Args after select (should also work)
  const r2 = await qr
    .todosConnection()
    .select((c) => c.totalCount())
    .filter({ done: true })
    .execute();

  const c1: number = r1.totalCount;
  const c2: number = r2.totalCount;
  console.log(c1, c2);
}

testPing();
testTodosConnection();
testNestedSelect();
testTodosListSelect();
testNullableSelect();
testWithoutSelect();
testMutationSelect();
testSimpleMutations();
testArgOrder();
