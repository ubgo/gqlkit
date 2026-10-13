# TypeScript: handling errors

A call to `.execute()` can fail in four different ways, and each one needs different handling: the server rejected or partly failed the operation (`GraphQLErrors`), the URL answered with something that is not GraphQL (`GraphQLHTTPError`), nothing answered at all (the platform's own fetch error), or the call succeeded and the answer is simply `null`. This page shows each case as it really happens and one function that tells them apart.

## Before you start

- A generated SDK and `gqlkit-ts`; see [Getting started — TypeScript](../getting-started-typescript.md).
- mockapi running (`task mockapi:run`) for the sample.

Snippets are quoted from [example-ts/cmd/samples/errors.ts](../../example-ts/cmd/samples/errors.ts), which imports:

```ts
import assert from "node:assert/strict";
import { GraphQLClient, GraphQLErrors, GraphQLHTTPError, batch } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, MOCKAPI_PING_RESPONSE, runSample } from "./mockapi";
```

## The four outcomes

| What happened | What you get | Has partial data |
| --- | --- | --- |
| The response has a non-empty `errors` array (validation error, resolver error, auth error), whatever the HTTP status | throws `GraphQLErrors` | yes, on `err.data`, when the server sent any |
| The response is not GraphQL: HTML error page, non-JSON body, JSON with neither `data` nor `errors` on a non-2xx status | throws `GraphQLHTTPError` with `status` and `body` | no |
| No response: connection refused, DNS failure, abort, timeout | the `fetch` rejection, unchanged (`TypeError: fetch failed`, `AbortError`, `TimeoutError`) | no |
| A scalar codec could not decode a value (see [codecs](types-scalars-enums.md#step-2--convert-values-with-a-codec)) | throws `ScalarCodecError` with `direction` and `path`; inside partial data, `GraphQLErrors` with `err.decodeError` set and `err.data` undefined | no |
| A nullable field resolved to nothing | resolves to `null` | not an error |

## Step 1 — Tell them apart in one place

`GraphQLErrors` and `GraphQLHTTPError` both extend `Error`, so check them first:

```ts
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
```

Each entry of `err.errors` has a `message` (always a string), and optionally `locations`, `path` and `extensions` (where servers put error codes). `err.message` is every message joined with `"; "`, which is what you see in logs.

## Step 2 — GraphQL errors

Here the required `id` argument is missing, so the server rejects the operation before running anything:

```ts
async function graphqlErrors(qr: QueryRoot): Promise<void> {
  console.log("required argument not set:");
  // `todo(id: ID!)` needs .id(...). The builder cannot enforce required
  // arguments, so the server's validation error is what you get.
  const err = await expectFailure(() => qr.todo().select((t) => t.id()).execute());
  assert.ok(err instanceof GraphQLErrors);
  assert.equal(err.data, undefined, "a validation error carries no data");
}
```

## Step 3 — Partial data

When a resolver fails after other fields resolved, the server sends both `errors` and `data`. `execute()` and `batch()` still throw, and the fields that did resolve are on `err.data`, untyped because any field may have been nulled by the error. mockapi's `fail` field always fails with the code you give it, so a [batch](batching.md) of `ping` and `fail` shows all three parts: `path[0]` names the failing alias, `extensions.code` carries the server's error code, and `err.data` keeps the alias that worked:

```ts
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
```

## Step 4 — Not found is `null`, not an error

```ts
async function nullIsNotAnError(qr: QueryRoot): Promise<void> {
  const todo = await qr.todo().id(MISSING_TODO_ID).select((t) => t.id()).execute();
  console.log("missing todo:", todo);
  assert.equal(todo, null);
}
```

The result type is `Narrowed | null` for every nullable field, so the compiler makes you handle it.

## Step 5 — A URL that is not a GraphQL endpoint

mockapi's root path serves an HTML playground. Pointing a client there is the same mistake as a wrong path, a login redirect, or a proxy's 502 page:

```ts
async function notGraphQL(): Promise<void> {
  console.log("wrong URL (HTML page):");
  const qr = new QueryRoot(new GraphQLClient(NON_GRAPHQL_URL));
  const err = await expectFailure(() => qr.ping().execute());
  assert.ok(err instanceof GraphQLHTTPError);
}
```

`err.message` quotes the first 200 characters of the body; `err.body` holds all of it.

## Step 6 — The server is down

```ts
async function networkFailure(): Promise<void> {
  console.log("server down:");
  const qr = new QueryRoot(new GraphQLClient(UNREACHABLE_URL));
  const err = await expectFailure(() => qr.ping().execute());
  assert.ok(err instanceof TypeError);
}
```

The client does not wrap this error, so retry libraries and `AbortError` checks see the platform's own error.

## Try it

```bash
cd example-ts
npx tsx cmd/samples/errors.ts
```

Output:

```
== Errors ==
required argument not set:
  GraphQLErrors (1): Field "todo" argument "id" of type "ID!" is required, but it was not provided. at [{"line":2,"column":3}]
failed alias: bad
error code: FORBIDDEN
partial data: {"ok":"pong from gqlgenapi","bad":null}
missing todo: null
wrong URL (HTML page):
  GraphQLHTTPError: status 200, body starts "<!DOCTYPE html>"
server down:
  TypeError: fetch failed

errors: ok
```

## Pitfalls

- **Check `GraphQLErrors` and `GraphQLHTTPError` before `Error`.** Both are `Error` subclasses, so a plain `err instanceof Error` branch placed first catches them too.
- **A non-2xx status is not always a `GraphQLHTTPError`.** Servers may send `400` or `401` with a proper GraphQL body (`{"errors":[...]}`). The client reports those as `GraphQLErrors`, because the GraphQL errors say more than the status code does. Read the code from `err.errors[0].extensions`, not from the status.
- **`GraphQLErrors` is thrown even when data came back.** gqlkit never resolves with partial data. If you want to show what succeeded, read `err.data` in your `catch` and narrow it yourself.
- **`executeRaw()` throws the same errors as `execute()`.** It skips the typing and the scalar codecs, not the error handling.
- **With codecs, `err.data` is decoded, or `undefined`.** Partial data is run through the same decoders as a successful result. If a value in it cannot be decoded, `err.data` is `undefined` and `err.decodeError` holds the `ScalarCodecError`; the `GraphQLErrors` you catch is otherwise unchanged.
- **Malformed error entries are kept, not dropped.** An `errors` entry with no `message` becomes `"Unknown GraphQL error"`, so a broken server response never turns into an apparent success.
- **A 2xx response with neither `data` nor `errors` throws a plain `Error`** with the message `No data returned from GraphQL query`, not a `GraphQLHTTPError`.
- **Before `gqlkit-ts` gained `GraphQLHTTPError`, a non-JSON response surfaced as a bare `SyntaxError`** from `JSON.parse`, with no status code. If you still see `SyntaxError: Unexpected token '<'`, update `gqlkit-ts`.

## Related

- [Client options](client.md) — timeouts, cancellation and fake responses for testing error paths.
- [Batching](batching.md) — how one failing alias is reported.
- [gqlkit-ts README](../../gqlkit-ts/README.md)
