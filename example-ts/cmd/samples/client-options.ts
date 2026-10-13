// Client options: auth token, custom headers, and a custom fetch used for
// request logging, timeouts, cancellation and offline tests.
//
// Guide: docs/typescript/client.md
// Run:   npx tsx cmd/samples/client-options.ts   (needs mockapi)

import assert from "node:assert/strict";
import { GraphQLClient, batch, type ClientOptions } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";

/** Placeholder bearer token. mockapi ignores auth, so any value works here. */
const DEMO_TOKEN = "demo-token";

/** How long a request may take before the timeout fetch aborts it. */
const REQUEST_TIMEOUT_MS = 5_000;

/**
 * A fetch that records the headers of every request, then sends it with the
 * real fetch. This is the shape of any fetch middleware: logging, tracing,
 * retries, or a header the server echoes back.
 */
function recordingFetch(seen: Headers[]): typeof fetch {
  return (input, init) => {
    seen.push(new Headers(init?.headers));
    return fetch(input, init);
  };
}

/** Step 1: a bearer token and a custom header on every request. */
async function authAndHeaders(): Promise<void> {
  const seen: Headers[] = [];
  const client = new GraphQLClient(MOCKAPI_ENDPOINT, {
    authToken: DEMO_TOKEN,
    headers: { "X-Request-ID": "req-123" },
    fetch: recordingFetch(seen),
  });

  const pong = await new QueryRoot(client).ping().execute();

  const sent = seen[0];
  console.log("ping:", pong);
  console.log("Authorization:", sent.get("authorization"));
  console.log("X-Request-ID:", sent.get("x-request-id"));
  console.log("Content-Type:", sent.get("content-type"));
  assert.equal(sent.get("authorization"), `Bearer ${DEMO_TOKEN}`);

  // mockapi's requestHeader field echoes a header as the server received it.
  const echoed = await new QueryRoot(client).requestHeader().name("Authorization").execute();
  console.log("server saw:", echoed);
  assert.equal(echoed, `Bearer ${DEMO_TOKEN}`);
}

/** Step 2: rotate the token without building a new client. */
async function rotateToken(): Promise<void> {
  const seen: Headers[] = [];
  // Keep a reference to the options object: the client reads it on every
  // request instead of copying it, so changing a field changes later requests.
  const options: ClientOptions = { authToken: "old-token", fetch: recordingFetch(seen) };
  const qr = new QueryRoot(new GraphQLClient(MOCKAPI_ENDPOINT, options));

  await qr.ping().execute();
  options.authToken = "new-token";
  await qr.ping().execute();

  console.log("first request: ", seen[0].get("authorization"));
  console.log("second request:", seen[1].get("authorization"));
  assert.equal(seen[1].get("authorization"), "Bearer new-token");
}

/** Step 3: a timeout for every request, added by wrapping fetch. */
async function timeout(): Promise<void> {
  const timeoutFetch: typeof fetch = (input, init) =>
    fetch(input, { ...init, signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });

  const client = new GraphQLClient(MOCKAPI_ENDPOINT, { fetch: timeoutFetch });
  console.log("with timeout:", await new QueryRoot(client).ping().execute());
}

/** Step 4: cancel an in-flight request with an AbortController. */
async function cancel(): Promise<void> {
  const controller = new AbortController();
  const cancellableFetch: typeof fetch = (input, init) =>
    fetch(input, { ...init, signal: controller.signal });
  const qr = new QueryRoot(new GraphQLClient(MOCKAPI_ENDPOINT, { fetch: cancellableFetch }));

  const pending = qr.ping().execute();
  controller.abort();

  try {
    await pending;
    assert.fail("an aborted request must reject");
  } catch (err) {
    // The client does not wrap fetch rejections, so the usual check works.
    const name = err instanceof Error ? err.name : String(err);
    console.log("cancelled:", name);
    assert.equal(name, "AbortError");
  }
}

/** Step 5: per-call cancellation and timeouts, with no fetch wrapper. */
async function perCallSignal(): Promise<void> {
  const client = new GraphQLClient(MOCKAPI_ENDPOINT);
  const qr = new QueryRoot(client);

  // Generated builders take `{ signal }` in execute(); it applies to this call only.
  const pong = await qr.ping().execute({ signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });
  console.log("per-call timeout:", pong);

  // client.execute() and batch() take the same option.
  const data = await client.execute<{ ping: string }>("query Ping { ping }", undefined, {
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  console.log("raw query with timeout:", data.ping);

  // An already-aborted signal rejects before anything is sent.
  const controller = new AbortController();
  controller.abort();
  try {
    await batch(client, { pong: qr.ping() }, { signal: controller.signal });
    assert.fail("an aborted batch must reject");
  } catch (err) {
    const name = err instanceof Error ? err.name : String(err);
    console.log("batch cancelled:", name);
    assert.equal(name, "AbortError");
  }
}

/** Step 6: a fake fetch for tests — no server, no network. */
async function fakeFetchForTests(): Promise<void> {
  const fakeFetch: typeof fetch = async () =>
    new Response(JSON.stringify({ data: { ping: "stubbed pong" } }), {
      headers: { "Content-Type": "application/json" },
    });

  const client = new GraphQLClient("http://example.invalid/graphql", { fetch: fakeFetch });
  const pong = await new QueryRoot(client).ping().execute();
  console.log("fake fetch:", pong);
  assert.equal(pong, "stubbed pong");
}

export async function run(): Promise<void> {
  console.log("== Client options ==");
  await authAndHeaders();
  await rotateToken();
  await timeout();
  await cancel();
  await perCallSignal();
  await fakeFetchForTests();
}

if (require.main === module) runSample("client-options", run);
