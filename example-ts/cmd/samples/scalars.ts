// Custom scalars: a binding with a codec converts values at run time. Here
// `DateTime` is bound to luxon's DateTime with decode/encode functions from
// codecs/datetime.ts, so createdAt arrives as a real DateTime. Also shows
// that executeRaw() stays raw, how a decode failure is reported, and how
// nullable fields are typed.
//
// Guide: docs/typescript/types-scalars-enums.md
// Run:   npx tsx cmd/samples/scalars.ts   (needs mockapi)

import assert from "node:assert/strict";
import { GraphQLClient, GraphQLErrors, ScalarCodecError } from "gqlkit-ts";
import { DateTime } from "luxon";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";

/** Seeded mockapi todo with a createdAt timestamp. */
const SEED_TODO_ID = "1";

/** Seeded mockapi todo that has no priority. */
const TODO_WITHOUT_PRIORITY_ID = "2";

/** Step 1: the codec decodes every DateTime in a response. */
async function decodedValue(qr: QueryRoot): Promise<void> {
  const todo = await qr
    .todo()
    .id(SEED_TODO_ID)
    .select((t) => t.id().createdAt())
    .execute();
  if (!todo) throw new Error(`seed todo ${SEED_TODO_ID} missing`);

  // The type says DateTime, and now the value is one: luxon methods just work.
  console.log("is a luxon DateTime:", DateTime.isDateTime(todo.createdAt));
  console.log("createdAt:", todo.createdAt.toFormat("yyyy-LL-dd HH:mm 'UTC'"));
  assert.ok(DateTime.isDateTime(todo.createdAt));
}

/** Step 2: executeRaw() skips the codecs and returns the wire value. */
async function rawStaysRaw(qr: QueryRoot): Promise<void> {
  const data = await qr.todo().id(SEED_TODO_ID).select((t) => t.createdAt()).executeRaw();
  console.log("executeRaw:", JSON.stringify(data));
}

/**
 * Step 3: a value the decoder rejects becomes a ScalarCodecError naming the
 * field path. A fake fetch supplies the bad value, because mockapi only
 * sends valid timestamps.
 */
async function decodeFailure(): Promise<void> {
  const badFetch: typeof fetch = async () =>
    new Response(JSON.stringify({ data: { todo: { id: "1", createdAt: "not-a-date" } } }), {
      headers: { "Content-Type": "application/json" },
    });
  const qr = new QueryRoot(new GraphQLClient(MOCKAPI_ENDPOINT, { fetch: badFetch }));

  try {
    await qr.todo().id(SEED_TODO_ID).select((t) => t.id().createdAt()).execute();
    assert.fail("an invalid DateTime must fail to decode");
  } catch (err) {
    if (!(err instanceof ScalarCodecError)) throw err;
    console.log("codec error:", err.direction, JSON.stringify(err.path));
    console.log("message:", err.message);
  }
}

/**
 * Step 4: when partial data next to GraphQL errors fails to decode, the
 * GraphQLErrors is still what you catch; err.data is undefined and
 * err.decodeError says why.
 */
async function partialDataDecodeFailure(): Promise<void> {
  const badPartialFetch: typeof fetch = async () =>
    new Response(
      JSON.stringify({
        errors: [{ message: "metadata unavailable", path: ["todo", "metadata"] }],
        data: { todo: { id: "1", createdAt: "not-a-date", metadata: null } },
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  const qr = new QueryRoot(new GraphQLClient(MOCKAPI_ENDPOINT, { fetch: badPartialFetch }));

  try {
    await qr.todo().id(SEED_TODO_ID).select((t) => t.id().createdAt().metadata()).execute();
    assert.fail("a response with errors must reject");
  } catch (err) {
    if (!(err instanceof GraphQLErrors)) throw err;
    console.log("errors:", err.message);
    console.log("data:", err.data);
    console.log("decodeError:", err.decodeError?.message);
    assert.equal(err.data, undefined);
    assert.ok(err.decodeError instanceof ScalarCodecError);
  }
}

/**
 * Step 5: a nullable field is typed `T | null`: once selected it is always
 * present, and null when the server has no value.
 */
async function nullableFields(qr: QueryRoot): Promise<void> {
  const todo = await qr.todo().id(TODO_WITHOUT_PRIORITY_ID).select((t) => t.priority()).execute();
  if (!todo) throw new Error(`seed todo ${TODO_WITHOUT_PRIORITY_ID} missing`);

  const priority: number | null = todo.priority;
  console.log("priority:", priority);
  console.log("priority ?? 0:", priority ?? 0);
  assert.equal(priority, null);
}

export async function run(): Promise<void> {
  console.log("== Custom scalars ==");
  const qr = new QueryRoot(new GraphQLClient(MOCKAPI_ENDPOINT));
  await decodedValue(qr);
  await rawStaysRaw(qr);
  await decodeFailure();
  await partialDataDecodeFailure();
  await nullableFields(qr);
}

if (require.main === module) runSample("scalars", run);
