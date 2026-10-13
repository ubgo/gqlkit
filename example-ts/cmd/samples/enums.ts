// Arguments, input types and enums: typed setters, optional input fields,
// partial updates, and enum values sent as arguments and read back.
//
// Guides: docs/typescript/arguments-and-inputs.md, docs/typescript/types-scalars-enums.md
// Run:    npx tsx cmd/samples/enums.ts   (needs mockapi)
//
// Every todo and user this sample creates is cleaned up or reused, so running
// it twice leaves mockapi in an equivalent state (one extra user per run:
// mockapi has no deleteUser).

import assert from "node:assert/strict";
import { GraphQLClient } from "gqlkit-ts";
import { Role } from "../../sdk/enums";
import type { NewTodo } from "../../sdk/inputs";
import { MutationRoot } from "../../sdk/mutations";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";

/** Seeded mockapi user that owns the todos this sample creates. */
const SEED_USER_ID = "1";

/** Tag that marks todos created by this sample, so the filter finds only them. */
const SAMPLE_TAG = "sample-enums";

const client = new GraphQLClient(MOCKAPI_ENDPOINT);
const qr = new QueryRoot(client);
const mr = new MutationRoot(client);

/** Step 1: scalar arguments through typed setters. */
async function scalarArguments(): Promise<void> {
  const sum = await qr.sum().a(10).b(20).execute();
  console.log("sum(a: 10, b: 20):", sum);
  assert.equal(sum, 30);
}

/** Step 2: an input object; optional fields may be left out. */
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

/** Step 3: a partial update — only the fields you set are sent. */
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

/** Step 4: an input object as a filter on a query. */
async function filterByInput(): Promise<void> {
  const mine = await qr
    .todos()
    .filter({ done: true, minPriority: 2 })
    .select((t) => t.id().text())
    .execute();
  console.log("done todos with priority >= 2:", mine.length);
}

/** Step 5: enums as arguments and as typed results. */
async function enumArgument(): Promise<void> {
  const created = await mr
    .upsertUser()
    .name("Enum Sample")
    .role(Role.ADMIN)
    .select((u) => u.id().name().role())
    .execute();
  console.log("created user:", JSON.stringify(created));

  // The same id updates in place; the enum value on the wire is the string "GUEST".
  const updated = await mr
    .upsertUser()
    .id(created.id)
    .name("Enum Sample")
    .role(Role.GUEST)
    .select((u) => u.id().role())
    .execute();

  // `updated.role` is typed `Role`, so comparing to the enum member type-checks.
  console.log("role is GUEST:", updated.role === Role.GUEST);
  assert.equal(updated.role, Role.GUEST);
}

async function cleanup(id: string): Promise<void> {
  const deleted = await mr.deleteTodo().id(id).execute();
  console.log("deleted:", deleted);
}

export async function run(): Promise<void> {
  console.log("== Arguments, inputs and enums ==");
  await scalarArguments();
  const id = await createWithInput();
  try {
    await partialUpdate(id);
    await filterByInput();
  } finally {
    await cleanup(id);
  }
  await enumArgument();
}

if (require.main === module) runSample("enums", run);
