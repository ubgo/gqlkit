package main

import (
	"context"
	"strings"
	"testing"

	"example-go-mockapi/internal/mockapitest"
)

// TestRun runs the example end to end against mockapi and pins the lines the
// Go guides quote, so a change to the SDK or the server that alters them
// fails here before the docs go stale.
func TestRun(t *testing.T) {
	endpoint := mockapitest.Require(t)
	var out strings.Builder
	if err := run(context.Background(), &out, endpoint); err != nil {
		t.Fatalf("run: %v\noutput so far:\n%s", err, out.String())
	}
	for _, want := range []string{
		"query Todos($pagination: PaginationInput) {",
		`variables: {"pagination":{"limit":2,"offset":0}}`,
		"id=1 __typename=Todo",
		"ping: pong from gqlgenapi",
		`first=1 "Try the ping query" second=2 "Create your first todo"`,
		"== Client.RawQuery ==\n{\"ping\":\"pong from gqlgenapi\"}",
		`data={"ok":"pong from gqlgenapi","bad":null} errors=1 first="nope"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}
