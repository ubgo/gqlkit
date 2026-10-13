package main

import (
	"context"
	"strings"
	"testing"

	"example-go-mockapi/internal/mockapitest"
)

// TestRun runs the example against mockapi and pins what
// docs/go/selecting-fields.md states about self-referencing fields, empty
// nested selectors, interface and union results, and unknown union members.
// The interface lines flipped on purpose when the generator gained
// <Name>Value wrappers; before, node could not be decoded at all.
func TestRun(t *testing.T) {
	endpoint := mockapitest.Require(t)
	var out strings.Builder
	if err := run(context.Background(), &out, endpoint); err != nil {
		t.Fatalf("run: %v\noutput so far:\n%s", err, out.String())
	}
	for _, want := range []string{
		`todo 2 "Create your first todo" -> parent 1 "Try the ping query" -> grandparent <nil>`,
		"query Todo($id: ID!) {\n  todo(id: $id) {\n    id\n  }\n}\nuser=<nil>",
		"== Interface result (node) ==\nTodo 1: \"Try the ping query\" done=false\nUser 2: Bob ADMIN",
		`Todo 2: "Create your first todo" done=false`,
		"User 2: Bob ADMIN\n== Unknown union member ==",
		`unknown member "Comment": {"__typename":"Comment","id":"c1","body":"hi"}`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}
