/**
 * Per-request cancellation: `execute` / `rawQuery` / `batch` × (no signal,
 * aborted before send, aborted mid-flight, custom abort reason).
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { BaseBuilder, OperationType } from "./builder";
import { GraphQLClient } from "./graphqlclient";
import { batch } from "./batch";
import { fakeFetch, hangingFetch, TEST_ENDPOINT, type FakeFetch } from "./testutil/fake-fetch";

const QUERY = "query Q { ping }";

/** Name `DOMException` uses for an abort without a custom reason. */
const ABORT_ERROR_NAME = "AbortError";

/** The three entry points that accept a signal, run against one client. */
const ENTRY_POINTS: { name: string; run: (client: GraphQLClient, signal?: AbortSignal) => Promise<unknown> }[] = [
  { name: "execute", run: (c, signal) => c.execute(QUERY, undefined, { signal }) },
  { name: "rawQuery", run: (c, signal) => c.rawQuery(QUERY, {}, { signal }) },
  {
    name: "batch",
    run: (c, signal) => {
      const b = new BaseBuilder(c, OperationType.Query, "Ping", "ping");
      return batch(c, { p: { execute: async () => undefined, getOpFragment: (a: string) => b.getOpFragment(a) } }, { signal });
    },
  },
];

function client(fake: FakeFetch): GraphQLClient {
  return new GraphQLClient(TEST_ENDPOINT, { fetch: fake.fetch });
}

for (const entry of ENTRY_POINTS) {
  test(`${entry.name}: no signal → no signal key in the fetch init`, async () => {
    const fake = fakeFetch({ body: { data: { ping: "pong", p: "pong" } } });
    await entry.run(client(fake));
    assert.equal(fake.requests[0]?.hasSignalKey, false);
  });

  test(`${entry.name}: the signal is passed through to fetch`, async () => {
    const fake = fakeFetch({ body: { data: { ping: "pong", p: "pong" } } });
    const controller = new AbortController();
    await entry.run(client(fake), controller.signal);
    assert.equal(fake.requests[0]?.signal, controller.signal);
  });

  test(`${entry.name}: aborted before send → rejects, nothing sent`, async () => {
    const fake = fakeFetch({ body: { data: { ping: "pong", p: "pong" } } });
    const controller = new AbortController();
    controller.abort();
    await assert.rejects(entry.run(client(fake), controller.signal), (err: unknown) => {
      assert.ok(err instanceof Error);
      assert.equal(err.name, ABORT_ERROR_NAME);
      return true;
    });
    assert.equal(fake.requests.length, 0);
  });

  test(`${entry.name}: aborted before send with a custom reason → that reason, unwrapped`, async () => {
    const fake = fakeFetch({ body: { data: {} } });
    const controller = new AbortController();
    const reason = new Error("user navigated away");
    controller.abort(reason);
    await assert.rejects(entry.run(client(fake), controller.signal), (err: unknown) => err === reason);
    assert.equal(fake.requests.length, 0);
  });

  test(`${entry.name}: aborted mid-flight → rejects with the abort`, async () => {
    const fake = hangingFetch();
    const controller = new AbortController();
    const run = entry.run(client(fake), controller.signal);
    // Let the request reach fetch before cancelling.
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(fake.requests.length, 1, "the request was in flight");
    controller.abort();
    await assert.rejects(run, (err: unknown) => err instanceof Error && err.name === ABORT_ERROR_NAME);
  });
}

test("one signal can cancel several in-flight requests", async () => {
  const fake = hangingFetch();
  const c = client(fake);
  const controller = new AbortController();
  const runs = [c.execute(QUERY, undefined, { signal: controller.signal }), c.execute(QUERY, undefined, { signal: controller.signal })];
  await new Promise((resolve) => setImmediate(resolve));
  controller.abort();
  for (const run of runs) await assert.rejects(run, (err: unknown) => err instanceof Error && err.name === ABORT_ERROR_NAME);
});

test("aborting after the response arrived changes nothing", async () => {
  const fake = fakeFetch({ body: { data: { ping: "pong" } } });
  const controller = new AbortController();
  const data = await client(fake).execute(QUERY, undefined, { signal: controller.signal });
  controller.abort();
  assert.deepEqual(data, { ping: "pong" });
});
