// Package mockapitest is the test-only half of package mockapi: it proves the
// server is up before an example runs against it. Kept apart so the example
// binaries never link the testing package.
package mockapitest

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"example-go-mockapi/internal/mockapi"
)

// probeTimeout bounds the reachability check so a missing server fails a
// test in about a second instead of hanging until the test deadline.
const probeTimeout = 2 * time.Second

// pingQuery is the cheapest operation mockapi answers; it has no arguments
// and touches no state, so probing with it never changes what a test sees.
const pingQuery = `{"query":"{ ping }"}`

// Require fails the test (never skips it) when mockapi does not answer a
// ping. Skipping would let an example rot silently: a green run with no
// server proves nothing about the program the docs quote.
func Require(t *testing.T) string {
	t.Helper()
	endpoint := mockapi.Endpoint()
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(pingQuery))
	if err != nil {
		t.Fatalf("mockapi probe: build request for %s: %v", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("mockapi is not reachable at %s (%v). Start it with `task mockapi:run`, or set %s.", endpoint, err, mockapi.EndpointEnv)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("mockapi probe: close response body: %v", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mockapi at %s answered %s to a ping; is something else on that port?", endpoint, resp.Status)
	}
	return endpoint
}
