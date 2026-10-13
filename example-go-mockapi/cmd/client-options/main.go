// Command client-options configures the generated graphqlclient: a bearer
// token, extra headers, a custom *http.Client with a timeout, and a
// per-call deadline through context.Context. A recording transport
// (internal/wiretap) shows the headers exactly as they leave the client, and
// mockapi's requestHeader query shows what the server received.
//
// Run it against mockapi: go run ./cmd/client-options
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"example-go-mockapi/internal/mockapi"
	"example-go-mockapi/internal/wiretap"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/queries"
)

// demoToken is a placeholder; mockapi does not check tokens. Load a real
// token from your environment or secret store, never from source.
const demoToken = "demo-token"

// requestTimeout caps every request made through this client, including
// reading the response body. It belongs on the http.Client: the generated
// client has no timeout option of its own.
const requestTimeout = 10 * time.Second

// shownHeaders are the headers this example prints, in a fixed order.
var shownHeaders = []string{"Authorization", "X-Request-Id", "X-Tenant", "X-Client", "Content-Type", "Accept"}

func main() {
	var out strings.Builder
	err := run(context.Background(), &out, mockapi.Endpoint())
	mockapi.Finish(out.String(), err)
}

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

// deadline shows per-call cancellation. Every Execute takes a context; when
// it expires the request is abandoned and the error matches
// context.DeadlineExceeded with errors.Is. A deadline that has already passed
// makes the outcome deterministic for this example.
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

// deref prints a nullable String result; requestHeader returns null when the
// header was not sent.
func deref(s *string) string {
	if s == nil {
		return "<not sent>"
	}
	return *s
}
