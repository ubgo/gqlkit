# Setting up the client for your kind of app

Every generated builder sends its request through one client: `graphqlclient.NewClient` in Go, `new GraphQLClient` from `gqlkit-ts` in TypeScript. The options are few (a bearer token, headers, your own `http.Client` or `fetch`), but which ones you need, and how long one client should live, depends on where your code runs. This page maps the common kinds of app to a setup.

## Before you start

Read the option reference once: [Go client](../go/client.md) and [TypeScript client](../typescript/client.md). The runnable versions are [`example-go-mockapi/cmd/client-options`](../../example-go-mockapi/cmd/client-options/main.go) and [`example-ts/cmd/samples/client-options.ts`](../../example-ts/cmd/samples/client-options.ts).

## Decision table: Go

| Kind of app | Client lifetime | Options | Why | Guide |
| --- | --- | --- | --- | --- |
| Backend service calling an API with one service token | One client for the process | `WithAuthToken(token)`, `WithHTTPClient(&http.Client{Timeout: ...})` | The default `http.DefaultClient` has no timeout; a hung upstream would hang your handler. | [Go client, Step 1](../go/client.md#step-1--create-the-client-with-its-options) |
| Web server calling the API as the signed-in user | One `*http.Client` for the process, a new `graphqlclient.Client` per request (or per user) | `WithAuthToken(userToken)` plus the shared `WithHTTPClient` | Headers are fixed when the client is built, so a per-user token needs its own client. Sharing the `http.Client` keeps connection pooling. | [Go client: pitfalls](../go/client.md#pitfalls) |
| Token that rotates (OAuth refresh) | One client | `WithHTTPClient` with a `RoundTripper` that sets `Authorization` on each request | No per-call header option exists; the transport is the hook that runs on every request. | [Go client, Step 2](../go/client.md#step-2--see-exactly-what-the-client-sends) |
| Per-request values (trace id, tenant header) | One client | A `RoundTripper` that reads them from `req.Context()` | Same reason; the context passed to `Execute(ctx)` reaches the transport. | [Go client: pitfalls](../go/client.md#pitfalls) |
| CLI tool or batch job | One client | `WithHTTPClient` with a timeout, and `context.WithTimeout` around each call | A per-call deadline stops one slow query from eating the whole run. | [Go client, Step 3](../go/client.md#step-3--cancel-or-time-limit-a-single-call) |
| Behind a corporate proxy or with a private CA | One client | `WithHTTPClient` with a custom `Transport` (`Proxy`, `TLSClientConfig`) | Everything below HTTP lives on the `http.Client`. | [Go client](../go/client.md) |
| Tests | One per test | `httptest.NewServer` as the endpoint, or a `RoundTripper` that answers in memory | The client only needs a URL and a transport; no mocking library. | [Go client, Step 2](../go/client.md#step-2--see-exactly-what-the-client-sends) |

## Decision table: TypeScript

| Kind of app | Client lifetime | Options | Why | Guide |
| --- | --- | --- | --- | --- |
| Browser single-page app | One client for the app | `authToken`, and mutate the options object when the token refreshes | The client reads the options object on every request, so a refresh needs no new client. | [TS client, Step 2](../typescript/client.md#step-2--rotate-the-token-without-a-new-client) |
| Browser app using cookie sessions | One client | A `fetch` wrapper that adds `credentials: "include"` | The default `fetch` call does not send cookies cross-origin. | [TS client](../typescript/client.md) |
| Server-side rendering (Next.js, Remix, SvelteKit) | A new client per incoming request | `authToken` or `headers` taken from that request | A module-level client with a user's token would be shared by every visitor. | [TS client: pitfalls](../typescript/client.md#pitfalls) |
| Node service or worker with one service token | One client | `authToken`; `{ signal: AbortSignal.timeout(ms) }` per call, or a `fetch` wrapper with `AbortSignal.timeout` for every request | There is no built-in timeout; a per-call signal or the wrapper gives each request a deadline. | [TS client, Step 3](../typescript/client.md#step-3--add-a-timeout-to-every-request) |
| UI that cancels stale requests (typeahead search) | One client | An `AbortController` per request, passed as `.execute({ signal })` on the generated builder, or `{ signal }` on `client.execute` / `batch` | Every call takes its own signal; aborting the previous controller cancels the stale request. | [TS client, Step 5](../typescript/client.md#step-5--cancel-or-time-out-one-call) |
| Other runtimes with a global `fetch` (edge functions, Deno, Bun) | One client | none, or a `fetch` wrapper as above | `gqlkit-ts` has no dependencies and calls only the global `fetch`. This repo's tests run it on Node only. | [gqlkit-ts README](../../gqlkit-ts/README.md) |
| Tests | One per test | `fetch` set to a function that returns a canned `Response` | No server and no network; the generated builders run unchanged. | [TS client, Step 6](../typescript/client.md#step-6--test-without-a-server) |

## If X, then Y

- **If two users can share a process**, never put a user's token on a long-lived client. In Go build a client per user; in TypeScript server code build one per request.
- **If you set both a token and an `Authorization` header**, Go keeps whichever option came last; TypeScript always lets `authToken` win.
- **If you need a timeout**, Go: `http.Client.Timeout` for every call and `context.WithTimeout` for one call. TypeScript: `AbortSignal.timeout` inside a `fetch` wrapper for every call, or `{ signal: AbortSignal.timeout(ms) }` on one `.execute()`, `client.execute` or `batch` call.
- **If you are calling several APIs**, make one client per endpoint; a client has exactly one URL.

## Pitfalls

- **Overriding `Content-Type`.** Both clients send JSON; a `Content-Type` header of your own replaces it and most servers then reject the body.
- **Passing a bound method as `fetch`.** In browsers `fetch: someLib.fetch` can throw "Illegal invocation"; wrap it in an arrow function.
- **`WithHTTPClient(nil)` in Go** makes every call panic; pass a real `*http.Client`.
- **Committing a token.** Read it from the environment or a secret store.

## Related

- [Go client](../go/client.md) and [TypeScript client](../typescript/client.md)
- [Go errors](../go/errors.md) and [TypeScript errors](../typescript/errors.md): what each client returns when the request fails
- [Which method to call](./execute-methods.md)
