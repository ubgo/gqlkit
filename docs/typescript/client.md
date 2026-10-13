# TypeScript client: auth, headers and custom fetch

`GraphQLClient` from `gqlkit-ts` is the one object every generated builder sends its request through. You give it the endpoint URL and, optionally, a bearer token, extra headers and your own `fetch` function. This page shows each option, how to cancel or time out a request, and how a custom `fetch` covers request logging and offline tests.

You want this page when your API needs a login token, when you must control timeouts or cancel requests, or when you want to unit-test code that uses the generated SDK without a running server.

## Before you start

- A generated TypeScript SDK and the `gqlkit-ts` runtime installed. If you do not have one yet, follow [Getting started — TypeScript](../getting-started-typescript.md) first.
- Node.js 18 or later (for the global `fetch`, `Headers`, `Response` and `AbortSignal.timeout`), or any browser.
- To run the sample on this page: the repo's test server, started with `task mockapi:run` from the repo root. It listens on `http://localhost:8081/query`.

Every snippet below is quoted from [example-ts/cmd/samples/client-options.ts](../../example-ts/cmd/samples/client-options.ts), which runs against that server. `fetch`, `Headers` and `Response` are globals; the sample's imports are:

```ts
import assert from "node:assert/strict";
import { GraphQLClient, batch, type ClientOptions } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";
```

## Step 1 — Send a bearer token and custom headers

`authToken` becomes an `Authorization: Bearer <token>` header and `headers` are added to every request. The client always sets `Content-Type: application/json` itself. The sample wraps `fetch` so it can print what was actually sent (this wrapper is the general shape of any fetch middleware), then asks mockapi's `requestHeader` field to echo the header back, which proves it reached the server:

```ts
function recordingFetch(seen: Headers[]): typeof fetch {
  return (input, init) => {
    seen.push(new Headers(init?.headers));
    return fetch(input, init);
  };
}
```

```ts
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
```

## Step 2 — Rotate the token without a new client

The client keeps a reference to the options object you pass and reads it on every request; it does not copy it. Hold on to the object and change `authToken` when your token refreshes, and every builder created from that client picks up the new value:

```ts
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
```

## Step 3 — Add a timeout to every request

For one call, pass a signal to `execute()` (Step 5). To give every request a client sends a timeout without touching each call site, wrap `fetch` and attach an `AbortSignal.timeout`:

```ts
async function timeout(): Promise<void> {
  const timeoutFetch: typeof fetch = (input, init) =>
    fetch(input, { ...init, signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });

  const client = new GraphQLClient(MOCKAPI_ENDPOINT, { fetch: timeoutFetch });
  console.log("with timeout:", await new QueryRoot(client).ping().execute());
}
```

`REQUEST_TIMEOUT_MS` is `5_000` in the sample. A request that runs out of time rejects with a `DOMException` named `TimeoutError`.

## Step 4 — Cancel every request on a client

A controller wired into the fetch wrapper cancels everything that client has in flight, for example when a page unmounts: put an `AbortController`'s signal on the request from inside your fetch. The client passes the rejection through unchanged, so the usual `err.name === "AbortError"` check works:

```ts
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
```

The controller belongs to the client here, so aborting it cancels every request in flight on that client. To cancel a single call, use the per-call signal from Step 5.

## Step 5 — Cancel or time out one call

Every generated builder's `execute()` and `executeRaw()`, as well as `client.execute()`, `client.rawQuery()` and `batch()`, take a per-call `{ signal }` (the `RequestOptions` type). A signal that is already aborted rejects before anything is sent; an abort mid-flight rejects with the signal's own reason (`AbortError`, or `TimeoutError` for `AbortSignal.timeout`):

```ts
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
```

## Step 6 — Test without a server

Because the client calls whatever `fetch` you give it, a function that returns a canned `Response` is a complete test double. No server, no network, and no mocking library:

```ts
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
```

To test error handling the same way, return `{ errors: [{ message: "..." }] }` and assert that `execute()` rejects with `GraphQLErrors` (see [Errors](errors.md)).

## Try it

From the repo root, with mockapi running:

```bash
cd example-ts
npx tsx cmd/samples/client-options.ts
```

Output:

```
== Client options ==
ping: pong from gqlgenapi
Authorization: Bearer demo-token
X-Request-ID: req-123
Content-Type: application/json
server saw: Bearer demo-token
first request:  Bearer old-token
second request: Bearer new-token
with timeout: pong from gqlgenapi
cancelled: AbortError
per-call timeout: pong from gqlgenapi
raw query with timeout: pong from gqlgenapi
batch cancelled: AbortError
fake fetch: stubbed pong

client-options: ok
```

On recent Node versions `tsx` may print a `DeprecationWarning` about `module.register()` first; it comes from `tsx`, not from gqlkit.

## Pitfalls

- **`authToken` wins over an `Authorization` header.** If you set both, the bearer token replaces the header, whatever its spelling (`authorization`, `AUTHORIZATION`). Header names are matched case-insensitively, so a `content-type` entry in `headers` replaces the default `Content-Type` rather than sending two. See `buildHeaders` in [gqlkit-ts/src/graphqlclient.ts](../../gqlkit-ts/src/graphqlclient.ts).
- **An empty `authToken` sends no header.** `authToken: ""` is treated as "no token", not as `Bearer ` with nothing after it.
- **Mutating the options object affects every builder on that client.** That is what makes token rotation work, and it also means two parts of your app sharing one options object share one token. Give each identity its own client.
- **Per-call signal or fetch wrapper: pick by scope.** `execute({ signal })` affects one call (Step 5). A signal created once in a fetch wrapper (Step 4) is shared by every request on that client, while `AbortSignal.timeout` called inside the wrapper (Step 3) is fresh per request, which is what you want for a client-wide timeout. If both are set, the wrapper's signal replaces the per-call one, because the wrapper overwrites `init.signal`; combine them with `AbortSignal.any([init.signal, ...])` in the wrapper if you need both.
- **Pass `fetch` unbound, or bound to `globalThis`.** The client calls your function without a `this`. A method taken off an object (`fetch: someLib.fetch`) can throw "Illegal invocation" in browsers; wrap it in an arrow function as the samples do.
- **The global `fetch` is looked up on every request**, so a polyfill or test stub installed after the client was built still takes effect. If no `fetch` exists at all (very old Node), `execute()` throws `GraphQLClient: no fetch implementation`; pass `options.fetch`.
- **Network failures are not wrapped.** A refused connection is the platform's own `TypeError: fetch failed`; see [Errors](errors.md) for telling it apart from GraphQL errors.

## Related

- [Errors](errors.md) — what `execute()` throws and how to tell failures apart.
- [Batching](batching.md) — several builders through one client in one request.
- [gqlkit-ts README](../../gqlkit-ts/README.md) — the runtime API reference.
- [Getting started — TypeScript](../getting-started-typescript.md)
