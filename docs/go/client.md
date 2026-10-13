# Go client: auth, headers, timeouts and cancellation

Every generated Go SDK ships its own HTTP client in `sdk/graphqlclient`. You create one `*graphqlclient.Client` per endpoint, configure it once with options (a bearer token, extra headers, your own `*http.Client`), and pass it to `queries.NewQueryRoot` and `mutations.NewMutationRoot`. Per-call cancellation and deadlines come from the `context.Context` you hand to `Execute`.

You want this page when you point the SDK at a real API: one that needs a token, a tenant header, a timeout, a proxy, or a test transport.

## Before you start

- A generated Go SDK. The snippets use the one in [`example-go-mockapi/sdk`](../../example-go-mockapi/sdk), generated from the [mockapi](../../mockapi) schema; your import paths will differ.
- To run the example: mockapi running on `http://localhost:8081/query` (`task mockapi:run` from the repo root).

## Step 1 — Create the client with its options

`NewClient(endpoint, opts...)` applies each option in order. There are four:

| Option | What it does |
| --- | --- |
| `WithAuthToken(token)` | Sets `Authorization: Bearer <token>` on every request |
| `WithHeader(key, value)` | Sets one header on every request |
| `WithHeaders(map[string]string)` | Sets several headers on every request |
| `WithHTTPClient(*http.Client)` | Replaces `http.DefaultClient`: timeouts, proxies, TLS, custom transports |

The options only write into the client's header map or swap its `*http.Client`, so a later option with the same header key wins. This is the whole of [`cmd/client-options/main.go`](../../example-go-mockapi/cmd/client-options/main.go)'s `run` function:

```go
func run(ctx context.Context, w *strings.Builder, endpoint string) error {
	tap := &wiretap.Transport{}

	client := graphqlclient.NewClient(endpoint,
		// Sets "Authorization: Bearer <token>" on every request.
		graphqlclient.WithAuthToken(demoToken),
		// One header, then several at once. Options apply in order, so a
		// later option overwrites an earlier one with the same key.
		graphqlclient.WithHeader("X-Request-Id", "req-1"),
		graphqlclient.WithHeaders(map[string]string{
			"X-Tenant": "acme",
			"X-Client": "client-options-example",
		}),
		// Timeouts, proxies, TLS and transports all live on the http.Client.
		graphqlclient.WithHTTPClient(&http.Client{
			Timeout:   requestTimeout,
			Transport: tap,
		}),
	)
	qr := queries.NewQueryRoot(client)

	pong, err := qr.Ping().Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "ping: %s\n", pong)

	last, _ := tap.Last()
	fmt.Fprintln(w, "== Headers sent ==")
	for _, h := range shownHeaders {
		fmt.Fprintf(w, "%s: %s\n", h, last.Header.Get(h))
	}

	// mockapi's requestHeader query echoes a header as the server received
	// it, proving the token survived the trip, not just that it was set.
	auth, err := qr.RequestHeader().Name("Authorization").Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== Header the server received ==")
	fmt.Fprintf(w, "Authorization: %s\n", deref(auth))

	return deadline(ctx, w, qr)
}
```

The constants it uses:

```go
// demoToken is a placeholder; mockapi does not check tokens. Load a real
// token from your environment or secret store, never from source.
const demoToken = "demo-token"

// requestTimeout caps every request made through this client, including
// reading the response body. It belongs on the http.Client: the generated
// client has no timeout option of its own.
const requestTimeout = 10 * time.Second
```

## Step 2 — See exactly what the client sends

`WithHTTPClient` accepts any `*http.Client`, so you can put your own `http.RoundTripper` in front of the network: for logging, metrics, retries, request signing, or tests. The example uses a small recording transport, [`internal/wiretap`](../../example-go-mockapi/internal/wiretap/wiretap.go), which captures the headers and the decoded GraphQL body and then forwards the request unchanged. (The example also asks mockapi's `requestHeader` query which `Authorization` value arrived, so both ends are checked.)

```go
// RoundTrip implements http.RoundTripper. A RoundTripper must not modify
// the caller's request, so the body is read once and the real transport
// gets a clone carrying an identical copy of it.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := Request{Header: req.Header.Clone()}
	out := req
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("wiretap: read body: %w", err)
		}
		_ = req.Body.Close()
		out = req.Clone(req.Context())
		out.Body = io.NopCloser(bytes.NewReader(body))
		if err := json.Unmarshal(body, &rec); err != nil {
			return nil, fmt.Errorf("wiretap: decode GraphQL body: %w", err)
		}
	}
	t.mu.Lock()
	t.reqs = append(t.reqs, rec)
	t.mu.Unlock()

	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(out)
}
```

## Step 3 — Cancel or time-limit a single call

Every `Execute`, `ExecuteRaw` and batch call takes a `context.Context`. The client builds its request with `http.NewRequestWithContext`, so when the context is cancelled or its deadline passes, the request is abandoned and the error matches the context's error with `errors.Is`. The example uses a deadline that has already passed so the result is deterministic:

```go
func deadline(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Context deadline ==")
	expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()

	_, err := qr.Ping().Execute(expired)
	if err == nil {
		return errors.New("expected a deadline error, got none")
	}
	fmt.Fprintf(w, "errors.Is(err, context.DeadlineExceeded) = %t\n", errors.Is(err, context.DeadlineExceeded))
	fmt.Fprintf(w, "err: %v\n", err)
	return nil
}
```

In a server, pass the incoming request's context (`r.Context()`) so a client that disconnects also cancels the GraphQL call. Use `context.WithTimeout` for a per-call budget and `http.Client.Timeout` for a hard ceiling on every call.

## Try it

```bash
task mockapi:run                          # in another terminal
cd example-go-mockapi && go run ./cmd/client-options
```

Output:

```text
ping: pong from gqlgenapi
== Headers sent ==
Authorization: Bearer demo-token
X-Request-Id: req-1
X-Tenant: acme
X-Client: client-options-example
Content-Type: application/json
Accept: application/json
== Header the server received ==
Authorization: Bearer demo-token
== Context deadline ==
errors.Is(err, context.DeadlineExceeded) = true
err: failed to execute request: Post "http://localhost:8081/query": context deadline exceeded
```

`go test ./cmd/client-options` runs the same program and checks these lines.

## Pitfalls

- **Your headers override the client's own.** The client sets `Content-Type: application/json` and `Accept: application/json`, then applies your headers with `Header.Set` ([`graphqlclient.tmpl`](../../gqlkit/pkg/templater/template/graphqlclient.tmpl), `do`, shared by every call). `WithHeader("Content-Type", ...)` therefore replaces the JSON content type, and most servers will then reject the body.
- **Last option wins.** `WithAuthToken("a")` followed by `WithHeader("Authorization", "Basic ...")` sends the Basic header; the bearer token is gone. Order options deliberately.
- **Headers are fixed at construction.** There is no per-call header option. For a token that rotates, or a header that depends on the request (a trace id, the end user's tenant), set it in a custom `http.RoundTripper` passed through `WithHTTPClient`, as `wiretap` does above, or build a new client per tenant.
- **The default client has no timeout.** Without `WithHTTPClient` the SDK uses `http.DefaultClient`, which waits forever on a server that accepts the connection and never answers. Set `http.Client.Timeout`, pass a context with a deadline, or both.
- **`WithHTTPClient` with a nil transport is fine** (Go uses `http.DefaultTransport`), but a nil `*http.Client` is not: `WithHTTPClient(nil)` makes every call panic.
- **Never commit a real token.** Read it from the environment or a secret store; the example's `demoToken` is a placeholder that mockapi ignores.

## Related

- [Errors](errors.md): what `Execute` returns when the server, the network or the body is wrong.
- [Low-level builder and raw client calls](low-level-builder.md): `Client.Execute` with a hand-written query.
- [Batching](batching.md): one HTTP request for several operations, through the same client.
- Runnable example: [`example-go-mockapi/cmd/client-options`](../../example-go-mockapi/cmd/client-options).
