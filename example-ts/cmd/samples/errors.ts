// Errors: what each failure looks like and how to tell them apart —
// GraphQL errors, a missing result that is not an error, a response that is
// not GraphQL at all, and a network failure.
//
// Guide: docs/typescript/errors.md
// Run:   npx tsx cmd/samples/errors.ts   (needs mockapi)

import assert from "node:assert/strict";
import { GraphQLClient, GraphQLErrors, GraphQLHTTPError, batch } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, MOCKAPI_PING_RESPONSE, runSample } from "./mockapi";

/** mockapi's root path serves the HTML playground, not GraphQL JSON. */
const NON_GRAPHQL_URL = "http://localhost:8081/";

/** Port 1 is reserved and never listening, so the connection is refused. */
const UNREACHABLE_URL = "http://localhost:1/query";

/** Error code mockapi's `fail` field puts in `extensions.code`. */
const FAILURE_CODE = "FORBIDDEN";

/** A todo id mockapi never assigns. */
const MISSING_TODO_ID = "does-not-exist";

/**
 * One place that turns any failure into a short description. The order
 * matters: GraphQLErrors and GraphQLHTTPError are both Error subclasses, so
 * the specific checks come before the generic one.
 */
function describe(err: unknown): string {
  if (err instanceof GraphQLErrors) {
    const first = err.errors[0];
    return `GraphQLErrors (${err.errors.length}): ${first?.message} at ${JSON.stringify(first?.locations)}`;
  }
  if (err instanceof GraphQLHTTPError) {
    return `GraphQLHTTPError: status ${err.status}, body starts ${JSON.stringify(err.body.slice(0, 15))}`;
  }
  if (err instanceof Error) {
    return `${err.name}: ${err.message}`;
  }
  return `non-Error rejection: ${String(err)}`;
}

/** Run `fn`, require it to fail, and return the description of the failure. */
async function expectFailure(fn: () => Promise<unknown>): Promise<unknown> {
  try {
    await fn();
  } catch (err) {
    console.log(" ", describe(err));
    return err;
  }
  throw new Error("expected a failure, got a result");
}

/** Step 1: the server rejects the operation — GraphQLErrors. */
async function graphqlErrors(qr: QueryRoot): Promise<void> {
  console.log("required argument not set:");
  // `todo(id: ID!)` needs .id(...). The builder cannot enforce required
  // arguments, so the server's validation error is what you get.
  const err = await expectFailure(() => qr.todo().select((t) => t.id()).execute());
  assert.ok(err instanceof GraphQLErrors);
  assert.equal(err.data, undefined, "a validation error carries no data");
}

/** Step 2: one field fails, the rest resolve — GraphQLErrors with partial data. */
async function partialData(client: GraphQLClient, qr: QueryRoot): Promise<void> {
  try {
    await batch(client, {
      ok: qr.ping(),
      bad: qr.fail().message("not allowed").code(FAILURE_CODE),
    });
    assert.fail("a failing alias must reject the batch");
  } catch (err) {
    if (!(err instanceof GraphQLErrors)) throw err;
    const first = err.errors[0];
    console.log("failed alias:", first?.path?.[0]);
    console.log("error code:", first?.extensions?.code);
    console.log("partial data:", JSON.stringify(err.data));
    assert.equal(first?.path?.[0], "bad");
    assert.equal(err.data?.ok, MOCKAPI_PING_RESPONSE);
  }
}

/** Step 3: "not found" is a null result, not an error. */
async function nullIsNotAnError(qr: QueryRoot): Promise<void> {
  const todo = await qr.todo().id(MISSING_TODO_ID).select((t) => t.id()).execute();
  console.log("missing todo:", todo);
  assert.equal(todo, null);
}

/** Step 4: the URL answers, but not with GraphQL — GraphQLHTTPError. */
async function notGraphQL(): Promise<void> {
  console.log("wrong URL (HTML page):");
  const qr = new QueryRoot(new GraphQLClient(NON_GRAPHQL_URL));
  const err = await expectFailure(() => qr.ping().execute());
  assert.ok(err instanceof GraphQLHTTPError);
}

/** Step 5: nothing answers — the fetch rejection passes through unchanged. */
async function networkFailure(): Promise<void> {
  console.log("server down:");
  const qr = new QueryRoot(new GraphQLClient(UNREACHABLE_URL));
  const err = await expectFailure(() => qr.ping().execute());
  assert.ok(err instanceof TypeError);
}

export async function run(): Promise<void> {
  console.log("== Errors ==");
  const client = new GraphQLClient(MOCKAPI_ENDPOINT);
  const qr = new QueryRoot(client);
  await graphqlErrors(qr);
  await partialData(client, qr);
  await nullIsNotAnError(qr);
  await notGraphQL();
  await networkFailure();
}

if (require.main === module) runSample("errors", run);
