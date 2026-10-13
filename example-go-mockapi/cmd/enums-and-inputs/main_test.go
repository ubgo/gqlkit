package main

import (
	"context"
	"strings"
	"testing"

	"example-go-mockapi/internal/mockapitest"
)

// TestRun runs the example against mockapi and pins every behaviour the Go
// guides state about enums, optional inputs, Opt[T] and unselected fields.
// The empty-slice line flipped on purpose when the generator started sending
// a non-nil empty slice as [] (it used to be dropped by omitempty).
func TestRun(t *testing.T) {
	endpoint := mockapitest.Require(t)
	var out strings.Builder
	if err := run(context.Background(), &out, endpoint); err != nil {
		t.Fatalf("run: %v\noutput so far:\n%s", err, out.String())
	}
	for _, want := range []string{
		"ADMIN USER GUEST",
		"with Role(GUEST): role=GUEST isGuest=true",
		"without Role:     role=USER email=<nil>",
		`variables {"filter":{}} -> `,
		`variables {"filter":{"done":false}} -> `,
		`selected:   id=1 text="Try the ping query"`,
		"unselected: tags=[] user=<nil> createdAt.IsZero=true",
		"before=[keep] after Tags: nil=[keep] after Tags: []string{}=[]",
		"input {}\ninput {\"text\":\"renamed\"}\ninput {\"priority\":null}",
		"createdAt: time.Time year=2025",
		"metadata:  map[string]interface {}",
		"serverInfo: raw JSON, valid=true",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}
