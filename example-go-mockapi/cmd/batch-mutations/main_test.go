package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"example-go-mockapi/internal/mockapitest"
)

// TestRun runs the example against mockapi and pins the merged document the
// Go batching guide quotes: aliases sorted, variables prefixed with the alias.
func TestRun(t *testing.T) {
	endpoint := mockapitest.Require(t)
	var out strings.Builder
	if err := run(context.Background(), &out, endpoint); err != nil {
		t.Fatalf("run: %v\noutput so far:\n%s", err, out.String())
	}
	for _, want := range []string{
		"alias \"buy-milk\": ErrInvalidAlias=true",
		"nil builder:     ErrNilBuilder=true",
		"requests sent: 0",
		"mutation Batch($bread_input: NewTodo!, $milk_input: NewTodo!) {",
		"  bread: createTodo(input: $bread_input) {",
		"  milk: createTodo(input: $milk_input) {",
		"variables: milk_input.text=Buy milk bread_input.text=Buy bread",
		`milk="Buy milk" bread="Buy bread"`,
		"milk=true bread=true requests=2",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}

// TestMixedKindsDoNotCompile proves the guide's claim that a mutation builder
// in batch.RunQueries is a compile error, by compiling testdata/mixed and
// requiring the compiler to reject it for the op-kind marker. A claim in prose
// is not verified until something fails when it stops being true.
func TestMixedKindsDoNotCompile(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go toolchain not on PATH: %v", err)
	}
	cmd := exec.Command(goBin, "build", "-o", t.TempDir()+"/mixed", "./testdata/mixed")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("testdata/mixed compiled, but a mutation builder in RunQueries must not:\n%s", out)
	}
	if !strings.Contains(string(out), "missing method IsQueryOp") {
		t.Fatalf("testdata/mixed failed to compile for the wrong reason:\n%s", out)
	}
	t.Logf("compiler output (quoted in docs/go/batching.md):\n%s", out)
}
