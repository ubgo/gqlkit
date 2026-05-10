package batch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/khanakia/gqlkit/gqlkit/pkg/builder"
	"github.com/khanakia/gqlkit/gqlkit/pkg/graphqlclient"
)

// ---------------------------------------------------------------------------
// Test fakes
// ---------------------------------------------------------------------------

// fakeClient captures the merged query + variables and returns canned response
// data + GraphQL errors. Implements both builder.GraphQLClient (Execute) and
// the partialExecutor interface (ExecuteWithPartialData) so we can test both
// code paths.
type fakeClient struct {
	// Capture
	query string
	vars  map[string]any

	// Canned partial response
	data    json.RawMessage
	gqlErrs graphqlclient.GraphQLErrors

	// If true, only the strict Execute path is implemented.
	strictOnly bool
}

func (f *fakeClient) Execute(ctx context.Context, query string, vars map[string]any, result any) error {
	f.query, f.vars = query, vars
	if len(f.gqlErrs) > 0 {
		return f.gqlErrs
	}
	if result != nil && len(f.data) > 0 {
		return json.Unmarshal(f.data, result)
	}
	return nil
}

// ExecuteWithPartialData implements partialExecutor unless strictOnly is set
// (in which case the type assertion in run() falls back to Execute).
func (f *fakeClient) ExecuteWithPartialData(ctx context.Context, query string, vars map[string]any) (json.RawMessage, graphqlclient.GraphQLErrors, error) {
	if f.strictOnly {
		// Should not be reached because we hide this method via strictWrapper.
		panic("strictOnly fakeClient hit ExecuteWithPartialData")
	}
	f.query, f.vars = query, vars
	return f.data, f.gqlErrs, nil
}

// strictWrapper wraps fakeClient and exposes ONLY Execute. Used to verify the
// fallback path when the underlying client doesn't implement partialExecutor.
type strictWrapper struct{ inner *fakeClient }

func (s *strictWrapper) Execute(ctx context.Context, query string, vars map[string]any, result any) error {
	return s.inner.Execute(ctx, query, vars, result)
}

// ---------------------------------------------------------------------------
// Test builders — minimal stand-ins for generated query / mutation builders
// ---------------------------------------------------------------------------

// todosTestBuilder mirrors a generated query: takes an optional `done`
// argument and selects id + text from a `todos` root field.
type todosTestBuilder struct {
	*builder.BaseBuilder
	builder.QueryMarker
}

func newTodos(c builder.GraphQLClient) *todosTestBuilder {
	b := builder.NewBaseBuilder(c, "query", "Todos", "todos")
	b.GetSelection().AddField("id")
	b.GetSelection().AddField("text")
	return &todosTestBuilder{BaseBuilder: b}
}

func (b *todosTestBuilder) Done(v bool) *todosTestBuilder {
	b.SetArg("done", v, "Boolean")
	return b
}

// usersTestBuilder mirrors a no-args generated query.
type usersTestBuilder struct {
	*builder.BaseBuilder
	builder.QueryMarker
}

func newUsers(c builder.GraphQLClient) *usersTestBuilder {
	b := builder.NewBaseBuilder(c, "query", "Users", "users")
	b.GetSelection().AddField("id")
	b.GetSelection().AddField("name")
	return &usersTestBuilder{BaseBuilder: b}
}

// createTodoTestBuilder is a mutation stand-in.
type createTodoTestBuilder struct {
	*builder.BaseBuilder
	builder.MutationMarker
}

func newCreateTodo(c builder.GraphQLClient) *createTodoTestBuilder {
	b := builder.NewBaseBuilder(c, "mutation", "CreateTodo", "createTodo")
	b.GetSelection().AddField("id")
	return &createTodoTestBuilder{BaseBuilder: b}
}

func (b *createTodoTestBuilder) Text(v string) *createTodoTestBuilder {
	b.SetArg("text", v, "String!")
	return b
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunQueries_MergesIntoSingleOperation(t *testing.T) {
	c := &fakeClient{data: json.RawMessage(`{
		"open":      [{"id":"1","text":"A"}],
		"completed": [{"id":"2","text":"B"}]
	}`)}

	type result struct {
		Open      []struct{ ID, Text string } `json:"open"`
		Completed []struct{ ID, Text string } `json:"completed"`
	}
	var r result

	err := RunQueries(context.Background(), &r, QueryItems{
		"open":      newTodos(c).Done(false),
		"completed": newTodos(c).Done(true),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !strings.HasPrefix(c.query, "query Batch(") {
		t.Errorf("query should start with 'query Batch('; got:\n%s", c.query)
	}
	for _, want := range []string{
		"$completed_done: Boolean",
		"$open_done: Boolean",
		"completed: todos(done: $completed_done)",
		"open: todos(done: $open_done)",
	} {
		if !strings.Contains(c.query, want) {
			t.Errorf("query missing %q\nfull query:\n%s", want, c.query)
		}
	}

	if c.vars["open_done"] != false || c.vars["completed_done"] != true {
		t.Errorf("variables not namespaced as expected: %v", c.vars)
	}

	if got := r.Open[0].Text; got != "A" {
		t.Errorf("Open[0].Text = %q, want A", got)
	}
	if got := r.Completed[0].Text; got != "B" {
		t.Errorf("Completed[0].Text = %q, want B", got)
	}
}

func TestRunQueries_NamespacesSameNamedArgs(t *testing.T) {
	c := &fakeClient{data: json.RawMessage(`{"a":[],"b":[]}`)}

	var r struct {
		A []any `json:"a"`
		B []any `json:"b"`
	}

	err := RunQueries(context.Background(), &r, QueryItems{
		"a": newTodos(c).Done(false),
		"b": newTodos(c).Done(true),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !strings.Contains(c.query, "$a_done: Boolean") {
		t.Errorf("missing $a_done declaration; query:\n%s", c.query)
	}
	if !strings.Contains(c.query, "$b_done: Boolean") {
		t.Errorf("missing $b_done declaration; query:\n%s", c.query)
	}
	if c.vars["a_done"] != false || c.vars["b_done"] != true {
		t.Errorf("variable namespacing failed: %v", c.vars)
	}
}

func TestRunQueries_NoArgs_OmitsVarBlock(t *testing.T) {
	c := &fakeClient{data: json.RawMessage(`{"u":[]}`)}

	var r struct {
		U []any `json:"u"`
	}
	err := RunQueries(context.Background(), &r, QueryItems{
		"u": newUsers(c),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// No "(" before the opening brace means no variable declarations
	if strings.Contains(c.query, "Batch(") {
		t.Errorf("expected no var block; got:\n%s", c.query)
	}
	if len(c.vars) != 0 {
		t.Errorf("vars should be empty; got %v", c.vars)
	}
}

func TestRunQueries_DeterministicAliasOrder(t *testing.T) {
	// Run twice with the same items; expect byte-identical query strings.
	build := func() string {
		c := &fakeClient{data: json.RawMessage(`{"a":[],"b":[],"c":[]}`)}
		var r struct {
			A []any `json:"a"`
			B []any `json:"b"`
			C []any `json:"c"`
		}
		_ = RunQueries(context.Background(), &r, QueryItems{
			"c": newUsers(c),
			"a": newUsers(c),
			"b": newUsers(c),
		})
		return c.query
	}
	q1, q2 := build(), build()
	if q1 != q2 {
		t.Errorf("query string is non-deterministic\n%s\nvs\n%s", q1, q2)
	}
	// Aliases should appear in sorted order in the output
	idxA := strings.Index(q1, "a:")
	idxB := strings.Index(q1, "b:")
	idxC := strings.Index(q1, "c:")
	if !(idxA >= 0 && idxA < idxB && idxB < idxC) {
		t.Errorf("aliases not in sorted order: a=%d b=%d c=%d\n%s", idxA, idxB, idxC, q1)
	}
}

func TestRunQueries_PartialData_PopulatesDestAndReturnsError(t *testing.T) {
	c := &fakeClient{
		data: json.RawMessage(`{
			"open":  [{"id":"1","text":"A"}],
			"users": null
		}`),
		gqlErrs: graphqlclient.GraphQLErrors{
			{Message: "permission denied", Path: []any{"users"}},
		},
	}

	type result struct {
		Open  []struct{ ID, Text string } `json:"open"`
		Users []any                       `json:"users"`
	}
	var r result

	err := RunQueries(context.Background(), &r, QueryItems{
		"open":  newTodos(c).Done(false),
		"users": newUsers(c),
	})
	if err == nil {
		t.Fatal("expected non-nil error for partial data response")
	}
	var berr *Error
	if !errors.As(err, &berr) {
		t.Fatalf("expected *batch.Error, got %T: %v", err, err)
	}
	if len(berr.Errors) != 1 || berr.Errors[0].Message != "permission denied" {
		t.Errorf("unexpected wrapped errors: %+v", berr.Errors)
	}
	if len(r.Open) != 1 || r.Open[0].Text != "A" {
		t.Errorf("Open should still be populated despite errors; got %+v", r.Open)
	}
}

func TestRunQueries_EmptyItems(t *testing.T) {
	var r struct{}
	err := RunQueries(context.Background(), &r, QueryItems{})
	if err == nil || !strings.Contains(err.Error(), "at least one builder") {
		t.Errorf("expected 'at least one builder' error; got %v", err)
	}
}

func TestRunMutations_MergesIntoMutation(t *testing.T) {
	c := &fakeClient{data: json.RawMessage(`{"a":{"id":"1"},"b":{"id":"2"}}`)}

	type result struct {
		A struct{ ID string } `json:"a"`
		B struct{ ID string } `json:"b"`
	}
	var r result

	err := RunMutations(context.Background(), &r, MutationItems{
		"a": newCreateTodo(c).Text("first"),
		"b": newCreateTodo(c).Text("second"),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(c.query, "mutation Batch(") {
		t.Errorf("expected 'mutation Batch(' prefix; got:\n%s", c.query)
	}
	if r.A.ID != "1" || r.B.ID != "2" {
		t.Errorf("decode mismatch: %+v", r)
	}
}

func TestRunQueries_FallbackToStrictExecute(t *testing.T) {
	// strictWrapper exposes only Execute → batch must take the fallback path.
	inner := &fakeClient{data: json.RawMessage(`{"u":[{"id":"1","name":"A"}]}`)}
	wrapped := &strictWrapper{inner: inner}

	// Build the test builder against the wrapper directly.
	b := builder.NewBaseBuilder(wrapped, "query", "Users", "users")
	b.GetSelection().AddField("id")
	b.GetSelection().AddField("name")
	users := &usersTestBuilder{BaseBuilder: b}

	var r struct {
		U []struct{ ID, Name string } `json:"u"`
	}
	err := RunQueries(context.Background(), &r, QueryItems{"u": users})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.U[0].Name != "A" {
		t.Errorf("decode failed in strict path: %+v", r.U)
	}
}

func TestRunQueries_FallbackWrapsGraphQLErrors(t *testing.T) {
	inner := &fakeClient{
		gqlErrs: graphqlclient.GraphQLErrors{{Message: "boom"}},
	}
	wrapped := &strictWrapper{inner: inner}

	b := builder.NewBaseBuilder(wrapped, "query", "Users", "users")
	b.GetSelection().AddField("id")
	users := &usersTestBuilder{BaseBuilder: b}

	var r struct{}
	err := RunQueries(context.Background(), &r, QueryItems{"u": users})
	var berr *Error
	if !errors.As(err, &berr) {
		t.Fatalf("expected *batch.Error in fallback path, got %T: %v", err, err)
	}
	if berr.Errors[0].Message != "boom" {
		t.Errorf("error not propagated: %+v", berr.Errors)
	}
}

// TestOpFragment_DirectShape pins the fragment shape so the contract between
// BaseBuilder and pkg/batch can't drift silently.
func TestOpFragment_DirectShape(t *testing.T) {
	b := builder.NewBaseBuilder(nil, "query", "Todos", "todos")
	b.SetArg("done", false, "Boolean")
	b.GetSelection().AddField("id")

	frag := b.GetOpFragment("open")

	if frag.OpType != "query" {
		t.Errorf("OpType = %q, want query", frag.OpType)
	}
	if got, want := frag.VarDecls[0], "$open_done: Boolean"; got != want {
		t.Errorf("VarDecls[0] = %q, want %q", got, want)
	}
	if frag.VarValues["open_done"] != false {
		t.Errorf("VarValues[open_done] = %v, want false", frag.VarValues["open_done"])
	}
	if !strings.HasPrefix(frag.AliasedField, "open: todos(done: $open_done)") {
		t.Errorf("AliasedField wrong prefix: %q", frag.AliasedField)
	}
	if !strings.Contains(frag.AliasedField, "id") {
		t.Errorf("AliasedField missing selection: %q", frag.AliasedField)
	}
}
