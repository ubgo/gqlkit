package batch

import (
	"errors"
	"testing"

	"github.com/khanakia/gqlkit/gqlkit/pkg/graphqlclient"
)

// Pins batch.Error.Error (batch.go:68) — the only arm of the error
// interface batch exposes to callers. It must delegate to the underlying
// GraphQLErrors rather than formatting its own message, because the alias
// prefix in each Path is what tells a caller WHICH batched query failed.
func TestErrorMessageDelegatesToGraphQLErrors(t *testing.T) {
	inner := graphqlclient.GraphQLErrors{
		{Message: "user not found", Path: []interface{}{"users", 0}},
	}
	err := &Error{Errors: inner}

	if got, want := err.Error(), inner.Error(); got != want {
		t.Fatalf("Error() = %q, want the underlying message %q", got, want)
	}
	if got := err.Error(); got != "user not found" {
		t.Fatalf("Error() = %q, want %q", got, "user not found")
	}
}

// Pins batch.Error.Unwrap (batch.go:76). The documented contract is that
// callers who already match on graphqlclient.GraphQLErrors keep working
// unchanged after migrating from Execute to RunQueries — that promise is
// only true if Unwrap exposes the inner slice to errors.As. A plain
// wrapper without Unwrap would compile, pass every other test, and
// silently break those call sites.
func TestErrorUnwrapKeepsErrorsAsWorking(t *testing.T) {
	inner := graphqlclient.GraphQLErrors{
		{Message: "forbidden", Path: []interface{}{"viewer"}},
	}
	var err error = &Error{Errors: inner}

	var target graphqlclient.GraphQLErrors
	if !errors.As(err, &target) {
		t.Fatal("errors.As did not match GraphQLErrors through batch.Error — Unwrap is not wired")
	}
	if len(target) != 1 || target[0].Message != "forbidden" {
		t.Fatalf("unwrapped errors = %+v, want the original single error", target)
	}
}

// Pins the empty case: an Error carrying no GraphQL errors must not panic
// and must produce the empty message GraphQLErrors documents, so a
// zero-value batch error is safe to log.
func TestErrorEmptyIsSafe(t *testing.T) {
	err := &Error{}
	if got := err.Error(); got != "" {
		t.Fatalf("Error() on empty = %q, want empty string", got)
	}
}
