# Go errors: GraphQL errors, HTTP failures and partial data

A GraphQL call can fail in three different places, and a generated Go SDK reports each one differently: the server answered in GraphQL and reported errors (`graphqlclient.GraphQLErrors`, whatever the HTTP status), something answered with an error status but no GraphQL body (`*graphqlclient.HTTPError`), or no answer came back at all (a transport error from `net/http`). Batches add a fourth case: some aliases succeeded and some did not (`*batch.Error`, with the good data still decoded).

You want this page when you write the `if err != nil` branch for real: to show a validation message to a user, retry on a network failure, or render whatever part of a batch came back.

## Before you start

- A generated Go SDK; the snippets use [`example-go-mockapi/sdk`](../../example-go-mockapi/sdk).
- To run the example: mockapi on `http://localhost:8081/query` (`task mockapi:run`).

## Step 1 — Know what `Execute` returns on failure

A typed builder's `Execute(ctx)` returns `(result, error)`. When `error` is non-nil, `result` is the zero value of the return type: `0`, `""`, `nil`, or an empty struct. There is no partial result from a single typed call, even when the server sent `data` next to `errors`: the client stops at the first sign of `errors` ([`graphqlclient.tmpl`](../../gqlkit/pkg/templater/template/graphqlclient.tmpl), `Execute` returns the `GraphQLErrors` before decoding `data`).

| What happened | Error you get | How to detect it |
| --- | --- | --- |
| Server answered with an `errors` array, with HTTP 200 (resolver error, permission denied) or another status (gqlgen answers validation failures with 422) | `graphqlclient.GraphQLErrors` (a slice of `GraphQLError` with `Message`, `Locations`, `Path`, `Extensions`) | `errors.As(err, &gqlErrs)` |
| Non-200 status with a body that is not GraphQL (a gateway's 502 page, an auth proxy's 401) | `*graphqlclient.HTTPError` with `StatusCode` and `Body` | `errors.As(err, &httpErr)` |
| No response: refused connection, DNS failure, timeout, cancelled context | `*url.Error` wrapped by the client (`failed to execute request: ...`) | `errors.As(err, &urlErr)`, `errors.Is(err, context.DeadlineExceeded)` |
| Response was 200 but not JSON (an HTML login page, a proxy error page) | plain error: `failed to unmarshal response: ...` | inspect the message |

## Step 2 — Classify the error

This helper from [`cmd/errors/main.go`](../../example-go-mockapi/cmd/errors/main.go) makes the decision every caller makes:

```go
// classify is the decision every caller makes: is this a GraphQL error the
// server reported (inspect Message, Path, Extensions), or did the request
// never get a GraphQL answer at all?
func classify(w *strings.Builder, err error) {
	var gqlErrs graphqlclient.GraphQLErrors
	if errors.As(err, &gqlErrs) {
		for _, e := range gqlErrs {
			fmt.Fprintf(w, "GraphQL error: %s path=%v code=%v\n", e.Message, e.Path, e.Extensions["code"])
		}
		return
	}
	var httpErr *graphqlclient.HTTPError
	if errors.As(err, &httpErr) {
		fmt.Fprintf(w, "HTTP error: status=%d body=%q\n", httpErr.StatusCode, httpErr.Body)
		return
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		fmt.Fprintf(w, "transport error (no response): op=%s\n", urlErr.Op)
		return
	}
	fmt.Fprintf(w, "other error: %s\n", firstLine(err))
}
```

Check `GraphQLErrors` first: it is the case you can show to a user. `*HTTPError` usually means something in front of the API (a proxy, a gateway, an auth layer) answered instead of it; `StatusCode` is what you retry or alert on.

`Extensions` is a `map[string]interface{}`; servers put machine-readable codes there (gqlgen, which mockapi uses, puts one under `code`). Read it with a type assertion rather than assuming a type: `code, _ := e.Extensions["code"].(string)`.

## Step 3 — Handle an error the server reported

mockapi's `fail(message, code)` field always fails in its resolver. The server answers HTTP 200 with `"data": {"fail": null}` and an `errors` array, so `Execute` returns `GraphQLErrors` and the typed result is the zero value (`nil` here, since `fail` returns a nullable `String`):

```go
// resolverError calls a field whose resolver fails. The server answers
// HTTP 200 with an `errors` array, which the client returns as
// graphqlclient.GraphQLErrors; the typed result is the zero value.
func resolverError(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Resolver error (HTTP 200 + errors) ==")
	msg, code := "sold out", failCode
	res, err := qr.Fail().Message(&msg).Code(&code).Execute(ctx)
	if err == nil {
		return fmt.Errorf("fail returned no error")
	}
	fmt.Fprintf(w, "result=%v\n", res)
	classify(w, err)
	return nil
}
```

## Step 4 — Keep partial data from a batch

`batch.RunQueries` and `batch.RunMutations` do not stop at the first error. They use `Client.ExecuteWithPartialData`, decode `data` into your destination struct for every alias that resolved, and return a `*batch.Error` holding the GraphQL errors. `*batch.Error` unwraps to `graphqlclient.GraphQLErrors`, so the classifier above still matches it. Batch a good field next to the failing one:

```go
// batchResult is the destination for partialData: one field per alias.
type batchResult struct {
	Ping string  `json:"ping"`
	Boom *string `json:"boom"`
}

// partialData batches a good field with a failing one. The batch decodes the
// good alias into dest and returns *batch.Error for the failing one, whose
// Path names the alias.
func partialData(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Partial data from a batch ==")
	var r batchResult
	err := batch.RunQueries(ctx, &r, batch.QueryItems{
		"ping": qr.Ping(),
		"boom": qr.Fail(),
	})
	var berr *batch.Error
	if !errors.As(err, &berr) {
		return fmt.Errorf("want *batch.Error, got %v", err)
	}
	fmt.Fprintf(w, "ping=%q boom=%v\n", r.Ping, r.Boom)
	for _, e := range berr.Errors {
		// Path[0] is the alias. Errors not tied to a field (rare) have no Path.
		if len(e.Path) > 0 {
			fmt.Fprintf(w, "failed alias=%v message=%q\n", e.Path[0], e.Message)
		}
	}
	return nil
}
```

`e.Path[0]` tells you which part of the screen to replace with an error state. For a hand-written query, `client.ExecuteWithPartialData(ctx, query, vars)` gives the same behaviour and returns `(json.RawMessage, GraphQLErrors, error)`: the third value is non-nil only when there was no GraphQL answer at all.

## Step 5 — See a request the server refuses

Leaving out a required argument compiles, because argument setters are ordinary optional method calls. The server rejects the document before running it, with HTTP 422 and a GraphQL errors body, and the client returns those errors as `GraphQLErrors`, the same as an error with HTTP 200:

```go
// missingArgument leaves out a required argument. Setters are optional
// method calls, so the compiler cannot catch this; the server rejects the
// document with HTTP 422 and a GraphQL errors body, which the client returns
// as GraphQLErrors like any other GraphQL error.
func missingArgument(ctx context.Context, w *strings.Builder, endpoint string) {
	fmt.Fprintln(w, "== Missing required argument (HTTP 422) ==")
	qr := queries.NewQueryRoot(graphqlclient.NewClient(endpoint))
	sum, err := qr.Sum().A(1).Execute(ctx) // B(...) never called
	fmt.Fprintf(w, "result=%d (zero value on error)\n", sum)
	classify(w, err)
}
```

## Step 6 — See an error status that is not GraphQL

mockapi always answers in GraphQL, so the example stands up an in-process server that behaves like a failing gateway:

```go
// badGateway stands in for a proxy or gateway in front of the API that
// answers 502 with a plain-text body. A non-200 status whose body is not a
// GraphQL response comes back as *graphqlclient.HTTPError, with the status
// and body kept for diagnosis. (An in-process server, because mockapi itself
// always answers in GraphQL.)
func badGateway(ctx context.Context, w *strings.Builder) {
	fmt.Fprintln(w, "== Non-GraphQL error status (HTTP 502) ==")
	proxy := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		http.Error(rw, gatewayBody, http.StatusBadGateway)
	}))
	defer proxy.Close()

	qr := queries.NewQueryRoot(graphqlclient.NewClient(proxy.URL))
	_, err := qr.Ping().Execute(ctx)
	classify(w, err)
}
```

The example also points a client at a port nothing listens on, and at mockapi's HTML playground page, to show the last two rows of the table.

## Try it

```bash
task mockapi:run                          # in another terminal
cd example-go-mockapi && go run ./cmd/errors
```

Output:

```text
== Resolver error (HTTP 200 + errors) ==
result=<nil>
GraphQL error: sold out path=[fail] code=OUT_OF_STOCK
== Partial data from a batch ==
ping="pong from gqlgenapi" boom=<nil>
failed alias=boom message="fail called"
== Missing required argument (HTTP 422) ==
result=0 (zero value on error)
GraphQL error: Field "sum" argument "b" of type "Int!" is required, but it was not provided. path=[] code=GRAPHQL_VALIDATION_FAILED
== Non-GraphQL error status (HTTP 502) ==
HTTP error: status=502 body="upstream unavailable\n"
== Nothing listening ==
transport error (no response): op=Post
== 200 OK but not GraphQL ==
other error: failed to unmarshal response: invalid character '<' looking for beginning of value
```

`go test ./cmd/errors` runs the same program and checks these lines.

## Pitfalls

- **Don't branch on the HTTP status to find GraphQL errors.** A 422 (or any status) with a GraphQL `errors` body is `GraphQLErrors`, not `*HTTPError`; the client decides by the body, not the status ([`graphqlclient.tmpl`](../../gqlkit/pkg/templater/template/graphqlclient.tmpl), `isGraphQLBody`). SDKs generated by older gqlkit versions returned a plain `unexpected status code` error for these; regenerate to get the current behaviour.
- **A single typed call never returns partial data.** If the server sends `data` next to `errors`, `Execute` drops the data. If you need the fields that did resolve, put the operation in a batch (even a batch of one) or call `client.ExecuteWithPartialData` yourself.
- **A null result is not an error.** A nullable root field (`todo(id: ID!): Todo`) comes back as a nil pointer with a nil error when the server finds nothing. Check for `nil` before dereferencing; see `unselectedFields` in [`cmd/enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs/main.go).
- **`HTTPError.Body` is the whole response body**, and `HTTPError.Error()` includes it, so it can be large or contain data you do not want in logs. Log `StatusCode` and a truncated body.
- **`errors.Is(err, context.Canceled)` and `context.DeadlineExceeded` work** because the client wraps with `%w`; prefer them over matching the message text. See [Client](client.md#step-3--cancel-or-time-limit-a-single-call).
- **A batch can return a `*batch.Error` with an empty destination** when the server sent `"data": null` (for example, a non-null field failed at the root). Check the fields you need, not only the error type.

## Related

- [Batching](batching.md): `RunQueries`, `RunMutations` and the partial-success contract.
- [Client](client.md): timeouts and cancellation.
- [Low-level builder and raw client calls](low-level-builder.md): `Client.Execute` and `ExecuteWithPartialData` with your own query.
- Runnable example: [`example-go-mockapi/cmd/errors`](../../example-go-mockapi/cmd/errors).
