// batch(): several builders in one HTTP request — the document it sends,
// a custom operation name, batched mutations, and the checks that throw
// before anything is sent.
//
// Guide: docs/typescript/batching.md
// Run:   npx tsx cmd/samples/batch-options.ts   (needs mockapi)

import assert from "node:assert/strict";
import { GraphQLClient, batch } from "gqlkit-ts";
import { MutationRoot } from "../../sdk/mutations";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";

/** Seeded mockapi user that owns the todos this sample creates. */
const SEED_USER_ID = "1";

/** Request bodies seen by the recording fetch below, for printing the document. */
const sentBodies: string[] = [];

/** A fetch that keeps each request body, so the sample can show what batch() sent. */
const recordingFetch: typeof fetch = (input, init) => {
  if (typeof init?.body === "string") sentBodies.push(init.body);
  return fetch(input, init);
};

const client = new GraphQLClient(MOCKAPI_ENDPOINT, { fetch: recordingFetch });
const qr = new QueryRoot(client);
const mr = new MutationRoot(client);

/** Print the query and variables of the most recent request. */
function printLastRequest(): void {
  const body: unknown = JSON.parse(sentBodies[sentBodies.length - 1] ?? "{}");
  if (typeof body === "object" && body !== null && "query" in body && "variables" in body) {
    console.log(String(body.query));
    console.log("variables:", JSON.stringify(body.variables));
  }
}

/** Step 1: one request, typed per alias, with a custom operation name. */
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

/** Step 2: mutations batch too; root mutation fields run in map order. */
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

/** Step 3: mistakes batch() rejects before sending anything. */
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

export async function run(): Promise<void> {
  console.log("== Batching ==");
  await dashboard();
  await batchedMutations();
  await rejectedBeforeSending();
}

if (require.main === module) runSample("batch-options", run);
