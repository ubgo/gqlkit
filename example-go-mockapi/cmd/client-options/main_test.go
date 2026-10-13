package main

import (
	"context"
	"strings"
	"testing"

	"example-go-mockapi/internal/mockapitest"
)

// TestRun runs the example against mockapi and pins the headers the client
// sends and the deadline behaviour docs/go/client.md describes.
func TestRun(t *testing.T) {
	endpoint := mockapitest.Require(t)
	var out strings.Builder
	if err := run(context.Background(), &out, endpoint); err != nil {
		t.Fatalf("run: %v\noutput so far:\n%s", err, out.String())
	}
	for _, want := range []string{
		"ping: pong from gqlgenapi",
		"Authorization: Bearer demo-token",
		"X-Request-Id: req-1",
		"X-Tenant: acme",
		"X-Client: client-options-example",
		"Content-Type: application/json",
		"Accept: application/json",
		"== Header the server received ==\nAuthorization: Bearer demo-token",
		"errors.Is(err, context.DeadlineExceeded) = true",
		"context deadline exceeded",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}
