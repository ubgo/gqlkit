// Command errors shows the error shapes a generated Go SDK returns and how to
// tell them apart: a GraphQL error the server reported (with HTTP 200, or with
// a non-200 status such as 422), partial data from a batch, a non-GraphQL
// error status (*graphqlclient.HTTPError), a transport failure (nothing
// answered), and a 200 body that is not GraphQL.
//
// Run it against mockapi: go run ./cmd/errors
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"example-go-mockapi/internal/mockapi"
	"example-go-mockapi/sdk/batch"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/queries"
)

// failCode is the extensions.code mockapi's `fail` field reports; the
// example passes its own to show the value travels end to end.
const failCode = "OUT_OF_STOCK"

// unreachableEndpoint is a port nothing listens on, for the transport error.
const unreachableEndpoint = "http://127.0.0.1:1/query"

// gatewayBody is what the stand-in proxy answers: plain text, not GraphQL.
const gatewayBody = "upstream unavailable"

// htmlPath is mockapi's playground page: it answers 200 with HTML, which is
// what a wrong endpoint path often looks like in production (a SPA or a
// login page in front of the API).
const htmlPath = "/"

func main() {
	var out strings.Builder
	err := run(context.Background(), &out, mockapi.Endpoint())
	mockapi.Finish(out.String(), err)
}

func run(ctx context.Context, w *strings.Builder, endpoint string) error {
	qr := queries.NewQueryRoot(graphqlclient.NewClient(endpoint))
	if err := resolverError(ctx, w, qr); err != nil {
		return err
	}
	if err := partialData(ctx, w, qr); err != nil {
		return err
	}
	missingArgument(ctx, w, endpoint)
	badGateway(ctx, w)
	unreachable(ctx, w)
	if err := notGraphQL(ctx, w, endpoint); err != nil {
		return err
	}
	return nil
}

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

// unreachable points the client at a port nothing listens on.
func unreachable(ctx context.Context, w *strings.Builder) {
	fmt.Fprintln(w, "== Nothing listening ==")
	qr := queries.NewQueryRoot(graphqlclient.NewClient(unreachableEndpoint))
	_, err := qr.Ping().Execute(ctx)
	classify(w, err)
}

// notGraphQL posts to a path that answers 200 with HTML.
func notGraphQL(ctx context.Context, w *strings.Builder, endpoint string) error {
	fmt.Fprintln(w, "== 200 OK but not GraphQL ==")
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	u.Path = htmlPath
	qr := queries.NewQueryRoot(graphqlclient.NewClient(u.String()))
	_, err = qr.Ping().Execute(ctx)
	classify(w, err)
	return nil
}

// firstLine trims an error message to its first line; the non-200 error
// embeds the whole response body.
func firstLine(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
