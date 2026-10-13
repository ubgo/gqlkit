/**
 * Contract tests for GraphQLClient: request shape, header merging, and every
 * response shape a server, proxy or gateway can send back.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import {
  BEARER_PREFIX,
  ERROR_BODY_SNIPPET_LENGTH,
  GRAPHQL_HTTP_METHOD,
  GraphQLClient,
  GraphQLErrors,
  GraphQLHTTPError,
  JSON_CONTENT_TYPE,
  MISSING_ERROR_MESSAGE,
  NO_DATA_MESSAGE,
  type ClientOptions,
  type GraphQLError,
} from "./graphqlclient";
import {
  clientReplying,
  clientReturningData,
  fakeFetch,
  onlyRequest,
  TEST_ENDPOINT,
  type FakeReply,
} from "./testutil/fake-fetch";

const QUERY = "query Q { ping }";

// ---------------------------------------------------------------------------
// Request shape
// ---------------------------------------------------------------------------

test("execute POSTs {query, variables} as JSON to the endpoint", async () => {
  const { client, requests } = clientReturningData({ ping: "pong" });
  await client.execute(QUERY, { a: 1, b: [null, "x"], c: { d: true } });
  const req = onlyRequest(requests);
  assert.equal(req.url, TEST_ENDPOINT);
  assert.equal(req.method, GRAPHQL_HTTP_METHOD);
  assert.equal(req.query, QUERY);
  assert.deepEqual(req.variables, { a: 1, b: [null, "x"], c: { d: true } });
});

test("execute omits the variables key when none are given", async () => {
  const { client, requests } = clientReturningData({ ping: "pong" });
  await client.execute(QUERY);
  assert.equal(onlyRequest(requests).hasVariablesKey, false);
});

test("execute sends an empty variables object as {}", async () => {
  const { client, requests } = clientReturningData({ ping: "pong" });
  await client.execute(QUERY, {});
  assert.deepEqual(onlyRequest(requests).variables, {});
});

test("rawQuery returns the same data as execute", async () => {
  const { client } = clientReturningData({ ping: "pong" });
  assert.deepEqual(await client.rawQuery(QUERY), { ping: "pong" });
});

// ---------------------------------------------------------------------------
// Headers × auth token
// ---------------------------------------------------------------------------

interface HeaderCase {
  name: string;
  headers: Record<string, string> | undefined;
  /** Expected headers by lower-cased name, ignoring the token dimension. */
  expected: Record<string, string>;
}

const HEADER_CASES: HeaderCase[] = [
  { name: "no custom headers", headers: undefined, expected: { "content-type": JSON_CONTENT_TYPE } },
  { name: "empty headers object", headers: {}, expected: { "content-type": JSON_CONTENT_TYPE } },
  {
    name: "extra header",
    headers: { "X-Tenant": "t1" },
    expected: { "content-type": JSON_CONTENT_TYPE, "x-tenant": "t1" },
  },
  {
    name: "same-case Content-Type override",
    headers: { "Content-Type": "application/graphql+json" },
    expected: { "content-type": "application/graphql+json" },
  },
  {
    name: "lower-case content-type override",
    headers: { "content-type": "application/graphql+json" },
    expected: { "content-type": "application/graphql+json" },
  },
  {
    name: "Authorization in headers",
    headers: { Authorization: "Basic abc" },
    expected: { "content-type": JSON_CONTENT_TYPE, authorization: "Basic abc" },
  },
  {
    name: "lower-case authorization in headers",
    headers: { authorization: "Basic abc" },
    expected: { "content-type": JSON_CONTENT_TYPE, authorization: "Basic abc" },
  },
];

const TOKEN_CASES: { name: string; token: string | undefined }[] = [
  { name: "no token", token: undefined },
  { name: "empty token", token: "" },
  { name: "token", token: "tok" },
];

for (const h of HEADER_CASES) {
  for (const t of TOKEN_CASES) {
    test(`execute headers: ${h.name} × ${t.name}`, async () => {
      const options: Omit<ClientOptions, "fetch"> = {};
      if (h.headers !== undefined) options.headers = h.headers;
      if (t.token !== undefined) options.authToken = t.token;
      const { client, requests } = clientReturningData({ ping: "pong" }, options);
      await client.execute(QUERY);
      const req = onlyRequest(requests);

      const expected = { ...h.expected };
      if (t.token) expected.authorization = `${BEARER_PREFIX}${t.token}`;
      assert.deepEqual(req.headers, expected);

      // Exactly one spelling per header name: fetch would join duplicates.
      const lowered = req.headerNames.map((n) => n.toLowerCase());
      assert.equal(new Set(lowered).size, lowered.length, `duplicate header names: ${req.headerNames}`);
    });
  }
}

test("execute reads options per request (token refresh by mutation)", async () => {
  const fake = fakeFetch({ body: { data: { ping: "pong" } } });
  const options: ClientOptions = { fetch: fake.fetch, authToken: "old" };
  const client = new GraphQLClient(TEST_ENDPOINT, options);
  await client.execute(QUERY);
  options.authToken = "new";
  await client.execute(QUERY);
  assert.deepEqual(
    fake.requests.map((r) => r.headers.authorization),
    [`${BEARER_PREFIX}old`, `${BEARER_PREFIX}new`],
  );
});

test("execute does not mutate the caller's headers object", async () => {
  const headers = { "content-type": "text/plain" };
  const { client } = clientReturningData({ ping: "pong" }, { headers, authToken: "t" });
  await client.execute(QUERY);
  assert.deepEqual(headers, { "content-type": "text/plain" });
});

// ---------------------------------------------------------------------------
// Response shapes
// ---------------------------------------------------------------------------

type Outcome =
  | { kind: "data"; data: Record<string, unknown> }
  | {
      kind: "graphql";
      message: string;
      errors?: GraphQLError[];
      data: Record<string, unknown> | undefined;
    }
  | { kind: "http"; status: number }
  | { kind: "nodata" };

interface ResponseCase {
  name: string;
  reply: FakeReply;
  outcome: Outcome;
}

const RESPONSE_CASES: ResponseCase[] = [
  { name: "200 data", reply: { body: { data: { a: 1 } } }, outcome: { kind: "data", data: { a: 1 } } },
  {
    name: "200 data + empty errors",
    reply: { body: { data: { a: 1 }, errors: [] } },
    outcome: { kind: "data", data: { a: 1 } },
  },
  {
    name: "200 data + null errors",
    reply: { body: { data: { a: 1 }, errors: null } },
    outcome: { kind: "data", data: { a: 1 } },
  },
  {
    name: "200 data with null root field",
    reply: { body: { data: { a: null } } },
    outcome: { kind: "data", data: { a: null } },
  },
  {
    name: "200 extra top-level keys ignored",
    reply: { body: { data: { a: 1 }, extensions: { cost: 3 } } },
    outcome: { kind: "data", data: { a: 1 } },
  },
  {
    name: "200 null data + one error",
    reply: { body: { data: null, errors: [{ message: "boom" }] } },
    outcome: { kind: "graphql", message: "boom", data: undefined },
  },
  {
    name: "200 errors only",
    reply: { body: { errors: [{ message: "boom" }] } },
    outcome: { kind: "graphql", message: "boom", data: undefined },
  },
  {
    name: "200 partial data + error",
    reply: {
      body: { data: { a: 1, b: null }, errors: [{ message: "b failed", path: ["b"] }] },
    },
    outcome: {
      kind: "graphql",
      message: "b failed",
      errors: [{ message: "b failed", path: ["b"] }],
      data: { a: 1, b: null },
    },
  },
  {
    name: "200 several errors joined",
    reply: { body: { errors: [{ message: "one" }, { message: "two" }] } },
    outcome: { kind: "graphql", message: "one; two", data: undefined },
  },
  {
    name: "200 full error entry narrowed intact",
    reply: {
      body: {
        errors: [
          {
            message: "bad",
            locations: [{ line: 1, column: 2 }],
            path: ["a", 0, "b"],
            extensions: { code: "BAD_INPUT" },
          },
        ],
      },
    },
    outcome: {
      kind: "graphql",
      message: "bad",
      errors: [
        {
          message: "bad",
          locations: [{ line: 1, column: 2 }],
          path: ["a", 0, "b"],
          extensions: { code: "BAD_INPUT" },
        },
      ],
      data: undefined,
    },
  },
  {
    name: "200 malformed error fields dropped",
    reply: {
      body: {
        errors: [
          {
            message: "bad",
            locations: [{ line: "1", column: 2 }, { line: 3, column: 4 }],
            path: ["a", { x: 1 }],
            extensions: "nope",
          },
        ],
      },
    },
    outcome: {
      kind: "graphql",
      message: "bad",
      errors: [{ message: "bad", locations: [{ line: 3, column: 4 }] }],
      data: undefined,
    },
  },
  {
    name: "200 error without message",
    reply: { body: { errors: [{ extensions: { code: "X" } }] } },
    outcome: { kind: "graphql", message: MISSING_ERROR_MESSAGE, data: undefined },
  },
  {
    name: "200 error entry is a bare string",
    reply: { body: { errors: ["plain"] } },
    outcome: { kind: "graphql", message: "plain", data: undefined },
  },
  {
    name: "200 error entry is a number",
    reply: { body: { errors: [42] } },
    outcome: { kind: "graphql", message: MISSING_ERROR_MESSAGE, data: undefined },
  },
  {
    name: "200 errors is a single object (non-conforming)",
    reply: { body: { errors: { message: "solo" } } },
    outcome: { kind: "graphql", message: "solo", data: undefined },
  },
  {
    name: "200 errors with array data treats data as absent",
    reply: { body: { data: [1], errors: [{ message: "e" }] } },
    outcome: { kind: "graphql", message: "e", data: undefined },
  },
  { name: "200 empty object", reply: { body: {} }, outcome: { kind: "nodata" } },
  { name: "200 null data", reply: { body: { data: null } }, outcome: { kind: "nodata" } },
  { name: "200 array data", reply: { body: { data: [] } }, outcome: { kind: "nodata" } },
  { name: "200 string data", reply: { body: { data: "x" } }, outcome: { kind: "nodata" } },
  { name: "200 non-JSON body", reply: { body: "<html>oops</html>" }, outcome: { kind: "http", status: 200 } },
  { name: "200 empty body", reply: { body: "" }, outcome: { kind: "http", status: 200 } },
  { name: "200 JSON array body", reply: { body: "[1,2]" }, outcome: { kind: "http", status: 200 } },
  { name: "200 JSON null body", reply: { body: "null" }, outcome: { kind: "http", status: 200 } },
  { name: "200 JSON string body", reply: { body: '"hi"' }, outcome: { kind: "http", status: 200 } },
  {
    name: "502 HTML gateway page",
    reply: { status: 502, body: "<html>Bad Gateway</html>" },
    outcome: { kind: "http", status: 502 },
  },
  {
    name: "401 proxy JSON without data/errors",
    reply: { status: 401, body: { message: "Unauthorized" } },
    outcome: { kind: "http", status: 401 },
  },
  {
    name: "400 GraphQL errors (GraphQL-over-HTTP)",
    reply: { status: 400, body: { errors: [{ message: "Syntax Error" }] } },
    outcome: { kind: "graphql", message: "Syntax Error", data: undefined },
  },
  {
    name: "500 GraphQL errors with partial data",
    reply: { status: 500, body: { data: { a: 1 }, errors: [{ message: "x" }] } },
    outcome: { kind: "graphql", message: "x", data: { a: 1 } },
  },
  {
    name: "500 data without errors is still data",
    reply: { status: 500, body: { data: { a: 1 } } },
    outcome: { kind: "data", data: { a: 1 } },
  },
  {
    name: "503 empty JSON object",
    reply: { status: 503, body: {} },
    outcome: { kind: "http", status: 503 },
  },
];

for (const c of RESPONSE_CASES) {
  test(`execute response: ${c.name}`, async () => {
    const { client } = clientReplying(c.reply);
    const run = client.execute<Record<string, unknown>>(QUERY);
    const o = c.outcome;
    switch (o.kind) {
      case "data":
        assert.deepEqual(await run, o.data);
        return;
      case "graphql":
        await assert.rejects(run, (err: unknown) => {
          assert.ok(err instanceof GraphQLErrors, `expected GraphQLErrors, got ${String(err)}`);
          assert.ok(err instanceof Error);
          assert.equal(err.name, "GraphQLErrors");
          assert.equal(err.message, o.message);
          if (o.errors) assert.deepEqual(err.errors, o.errors);
          assert.deepEqual(err.data, o.data);
          return true;
        });
        return;
      case "http":
        await assert.rejects(run, (err: unknown) => {
          assert.ok(err instanceof GraphQLHTTPError, `expected GraphQLHTTPError, got ${String(err)}`);
          assert.equal(err.name, "GraphQLHTTPError");
          assert.equal(err.status, o.status);
          assert.match(err.message, new RegExp(`^GraphQL HTTP ${o.status}: `));
          return true;
        });
        return;
      case "nodata":
        await assert.rejects(run, (err: unknown) => {
          assert.ok(err instanceof Error);
          assert.ok(!(err instanceof GraphQLErrors) && !(err instanceof GraphQLHTTPError));
          assert.equal(err.message, NO_DATA_MESSAGE);
          return true;
        });
        return;
      default: {
        const exhaustive: never = o;
        throw new Error(`unhandled outcome ${String(exhaustive)}`);
      }
    }
  });
}

test("GraphQLHTTPError keeps the full body but quotes only a snippet", async () => {
  const body = "x".repeat(ERROR_BODY_SNIPPET_LENGTH * 5);
  const { client } = clientReplying({ status: 500, body });
  await assert.rejects(client.execute(QUERY), (err: unknown) => {
    assert.ok(err instanceof GraphQLHTTPError);
    assert.equal(err.body, body);
    assert.ok(err.message.length < ERROR_BODY_SNIPPET_LENGTH + 100);
    assert.ok(err.message.endsWith("x".repeat(ERROR_BODY_SNIPPET_LENGTH)));
    return true;
  });
});

test("GraphQLErrors can be constructed directly without data", () => {
  const err = new GraphQLErrors([{ message: "a" }, { message: "b" }]);
  assert.equal(err.message, "a; b");
  assert.equal(err.data, undefined);
  assert.equal(new GraphQLErrors([]).message, "");
});

// ---------------------------------------------------------------------------
// Transport failures
// ---------------------------------------------------------------------------

test("execute propagates a network error unchanged", async () => {
  const netErr = new TypeError("fetch failed");
  const client = new GraphQLClient(TEST_ENDPOINT, {
    fetch: async () => {
      throw netErr;
    },
  });
  await assert.rejects(client.execute(QUERY), (err: unknown) => err === netErr);
});

test("execute propagates an abort from a signal-aware custom fetch", async () => {
  const controller = new AbortController();
  const client = new GraphQLClient(TEST_ENDPOINT, {
    fetch: () =>
      new Promise<Response>((_resolve, reject) => {
        const signal = controller.signal;
        signal.addEventListener("abort", () => reject(signal.reason));
      }),
  });
  const run = client.execute(QUERY);
  controller.abort();
  await assert.rejects(run, (err: unknown) => err instanceof Error && err.name === "AbortError");
});

test("execute propagates a failure while reading the body", async () => {
  const readErr = new Error("socket hang up");
  const stream = new ReadableStream<Uint8Array>({
    start(ctrl) {
      ctrl.error(readErr);
    },
  });
  const client = new GraphQLClient(TEST_ENDPOINT, {
    fetch: async () => new Response(stream, { status: 200 }),
  });
  await assert.rejects(client.execute(QUERY), (err: unknown) => err === readErr);
});

/** Run `fn` with `globalThis.fetch` replaced (or removed when `replacement` is undefined). */
async function withGlobalFetch(
  replacement: typeof fetch | undefined,
  fn: () => Promise<void>,
): Promise<void> {
  const original = Object.getOwnPropertyDescriptor(globalThis, "fetch");
  if (replacement === undefined) {
    Reflect.deleteProperty(globalThis, "fetch");
  } else {
    Object.defineProperty(globalThis, "fetch", { value: replacement, configurable: true, writable: true });
  }
  try {
    await fn();
  } finally {
    if (original) Object.defineProperty(globalThis, "fetch", original);
  }
}

test("execute falls back to globalThis.fetch, read at call time", async () => {
  const client = new GraphQLClient(TEST_ENDPOINT);
  const fake = fakeFetch({ body: { data: { ping: "global" } } });
  await withGlobalFetch(fake.fetch, async () => {
    assert.deepEqual(await client.execute(QUERY), { ping: "global" });
  });
  assert.equal(fake.requests.length, 1);
});

test("execute explains a missing fetch instead of a TypeError", async () => {
  const client = new GraphQLClient(TEST_ENDPOINT);
  await withGlobalFetch(undefined, async () => {
    await assert.rejects(client.execute(QUERY), /no fetch implementation/);
  });
});

test("custom fetch takes precedence over globalThis.fetch", async () => {
  const custom = fakeFetch({ body: { data: { from: "custom" } } });
  const global = fakeFetch({ body: { data: { from: "global" } } });
  const client = new GraphQLClient(TEST_ENDPOINT, { fetch: custom.fetch });
  await withGlobalFetch(global.fetch, async () => {
    assert.deepEqual(await client.execute(QUERY), { from: "custom" });
  });
  assert.equal(global.requests.length, 0);
});
