package main

import (
	"context"
	"strings"
	"testing"

	"example-go-mockapi/internal/mockapitest"
)

// TestRun runs the example against mockapi and pins how each failure is
// classified, as docs/go/errors.md describes it. The 422 line flipped on
// purpose when the client started decoding GraphQL error bodies on non-200
// responses; before that it was a plain "unexpected status code" error.
func TestRun(t *testing.T) {
	endpoint := mockapitest.Require(t)
	var out strings.Builder
	if err := run(context.Background(), &out, endpoint); err != nil {
		t.Fatalf("run: %v\noutput so far:\n%s", err, out.String())
	}
	for _, want := range []string{
		"GraphQL error: sold out path=[fail] code=OUT_OF_STOCK",
		`ping="pong from gqlgenapi" boom=<nil>`,
		`failed alias=boom message="fail called"`,
		"result=0 (zero value on error)",
		`GraphQL error: Field "sum" argument "b" of type "Int!" is required, but it was not provided. path=[] code=GRAPHQL_VALIDATION_FAILED`,
		`HTTP error: status=502 body="upstream unavailable\n"`,
		"transport error (no response): op=Post",
		"other error: failed to unmarshal response: invalid character '<'",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}
