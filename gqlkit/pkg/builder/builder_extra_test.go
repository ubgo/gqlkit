package builder

import (
	"context"
	"errors"
	"testing"
)

// mockClient is a GraphQLClient that records the last call and returns a canned
// result or error.
type mockClient struct {
	lastQuery string
	lastVars  map[string]interface{}
	fillWith  map[string]interface{}
	err       error
}

func (m *mockClient) Execute(ctx context.Context, query string, vars map[string]interface{}, resp any) error {
	m.lastQuery = query
	m.lastVars = vars
	if m.err != nil {
		return m.err
	}
	if out, ok := resp.(*map[string]interface{}); ok && m.fillWith != nil {
		*out = m.fillWith
	}
	return nil
}

func TestGetClient(t *testing.T) {
	c := &mockClient{}
	b := NewBaseBuilder(c, "query", "Me", "me")
	if b.GetClient() != c {
		t.Error("GetClient did not return the injected client")
	}
}

func TestExecuteRawSuccess(t *testing.T) {
	c := &mockClient{fillWith: map[string]interface{}{"me": map[string]interface{}{"id": "1"}}}
	b := NewBaseBuilder(c, "query", "Me", "me")
	b.GetSelection().AddField("id")

	got, err := b.ExecuteRaw(context.Background())
	if err != nil {
		t.Fatalf("ExecuteRaw: %v", err)
	}
	if got["me"] == nil {
		t.Errorf("expected decoded response, got %v", got)
	}
	if c.lastQuery == "" {
		t.Error("client was not called with a query")
	}
}

func TestExecuteRawError(t *testing.T) {
	c := &mockClient{err: errors.New("boom")}
	b := NewBaseBuilder(c, "query", "Me", "me")
	if _, err := b.ExecuteRaw(context.Background()); err == nil {
		t.Fatal("expected ExecuteRaw to propagate the client error")
	}
}

func TestGetOpFragment(t *testing.T) {
	c := &mockClient{}
	b := NewBaseBuilder(c, "query", "Users", "users")
	b.SetArg("first", 10, "Int")
	b.SetArg("role", "ADMIN", "String")
	b.GetSelection().AddField("id")

	frag := b.GetOpFragment("u0")
	if frag.OpType != "query" {
		t.Errorf("OpType = %q, want query", frag.OpType)
	}
	// Args are alias-prefixed to avoid collisions in a batched document.
	if frag.VarValues["u0_first"] != 10 || frag.VarValues["u0_role"] != "ADMIN" {
		t.Errorf("VarValues not alias-prefixed: %+v", frag.VarValues)
	}
	if len(frag.VarDecls) != 2 {
		t.Errorf("expected 2 var decls, got %v", frag.VarDecls)
	}
	// The aliased field must lead with the alias and include the selection.
	if got := frag.AliasedField; got == "" || got[:3] != "u0:" {
		t.Errorf("AliasedField = %q, want it to start with 'u0:'", got)
	}
}

func TestOpMarkers(t *testing.T) {
	// The marker methods are compile-time discriminators; calling them is a
	// no-op but must be covered so the query/mutation gating stays wired.
	QueryMarker{}.IsQueryOp()
	MutationMarker{}.IsMutationOp()
}
