// Low-level runtime API: inspect what a generated builder sends, run a
// hand-written document with a typed result, and assemble an operation from
// BaseBuilder and FieldSelection directly.
//
// Guide: docs/typescript/low-level.md
// Run:   npx tsx cmd/samples/low-level.ts   (needs mockapi)

import assert from "node:assert/strict";
import { BaseBuilder, FieldSelection, GraphQLClient, OperationType } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";

const client = new GraphQLClient(MOCKAPI_ENDPOINT);

/** Step 1: the raw `data` object of a generated builder, untyped. */
async function executeRaw(): Promise<void> {
  const data = await new QueryRoot(client).sum().a(2).b(3).executeRaw();
  console.log("executeRaw:", JSON.stringify(data));
  assert.deepEqual(data, { sum: 5 });
}

/** Step 2: a hand-written document with a typed result. */
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

/** Step 3: build a selection tree on its own and print it. */
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

/** Step 4: a full operation from BaseBuilder: document, variables, result. */
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

export async function run(): Promise<void> {
  console.log("== Low-level API ==");
  await executeRaw();
  await typedRawQuery();
  selectionTree();
  await baseBuilder();
}

if (require.main === module) runSample("low-level", run);
