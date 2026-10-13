// Command selecting-fields shows field selection beyond the basics: a
// self-referencing field (Todo.parent: Todo) selected to a chosen depth, the
// silent no-op of a nested selector with no fields, and querying a field that
// returns an interface (node: Node) or a union (search: [SearchResult!]!).
//
// Run it against mockapi: go run ./cmd/selecting-fields
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"example-go-mockapi/internal/mockapi"
	"example-go-mockapi/internal/wiretap"
	"example-go-mockapi/sdk/fields"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/queries"
	"example-go-mockapi/sdk/types"
)

// childTodoID is the seeded todo whose parent is todo "1".
const childTodoID = "2"

// todoNodeID and userNodeID are Relay-style typed ids; mockapi's node(id:)
// takes "<Type>:<id>" because todo and user ids overlap.
const (
	todoNodeID = "Todo:1"
	userNodeID = "User:2"
)

// searchTerm matches one seeded todo ("Create your first todo") and one
// seeded user ("Bob"); search is a case-insensitive substring match.
const searchTerm = "o"

func main() {
	var out strings.Builder
	err := run(context.Background(), &out, mockapi.Endpoint())
	mockapi.Finish(out.String(), err)
}

func run(ctx context.Context, w *strings.Builder, endpoint string) error {
	tap := &wiretap.Transport{}
	qr := queries.NewQueryRoot(graphqlclient.NewClient(endpoint,
		graphqlclient.WithHTTPClient(&http.Client{Transport: tap})))

	if err := selfReference(ctx, w, qr); err != nil {
		return fmt.Errorf("self reference: %w", err)
	}
	if err := emptyNested(ctx, w, qr, tap); err != nil {
		return fmt.Errorf("empty nested: %w", err)
	}
	if err := interfaceResult(ctx, w, qr); err != nil {
		return fmt.Errorf("interface result: %w", err)
	}
	if err := unionResult(ctx, w, qr); err != nil {
		return fmt.Errorf("union result: %w", err)
	}
	return unknownMember(w)
}

// newerServerResult is what a server could send after adding a third member
// to SearchResult that this SDK was not generated with.
const newerServerResult = `{"__typename":"Comment","id":"c1","body":"hi"}`

// unknownMember decodes a union value with a __typename the SDK does not
// know. Decoding does not fail: Value is nil and Typename and Raw keep the
// server's JSON, so describe's default branch handles it.
func unknownMember(w *strings.Builder) error {
	fmt.Fprintln(w, "== Unknown union member ==")
	var r types.SearchResultValue
	if err := json.Unmarshal([]byte(newerServerResult), &r); err != nil {
		return err
	}
	describe(w, r.Typename, r.Value, r.Raw)
	return nil
}

// selfReference selects a todo, its parent, and the parent's parent. A
// self-referencing field gets a selector callback like any object field, so
// you choose the depth explicitly; there is no recursive "select all".
func selfReference(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Self-referencing field ==")
	todo, err := qr.Todo().ID(childTodoID).
		Select(func(t *fields.TodoFields) {
			t.ID().Text().Parent(func(p *fields.TodoFields) {
				p.ID().Text().Parent(func(gp *fields.TodoFields) {
					gp.ID()
				})
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	if todo == nil || todo.Parent == nil {
		return fmt.Errorf("seeded todo %s or its parent is missing", childTodoID)
	}
	// Parent is *types.Todo: nil when the server returned null (a top-level
	// todo) and also nil when you did not select it.
	fmt.Fprintf(w, "todo %s %q -> parent %s %q -> grandparent %v\n",
		todo.ID, todo.Text, todo.Parent.ID, todo.Parent.Text, todo.Parent.Parent)
	return nil
}

// emptyNested shows a trap: a nested selector that selects nothing (or a nil
// callback) is dropped from the document without any error, because GraphQL
// does not allow an object field with an empty selection set.
func emptyNested(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot, tap *wiretap.Transport) error {
	fmt.Fprintln(w, "== Nested selector with no fields ==")
	todo, err := qr.Todo().ID(childTodoID).
		Select(func(t *fields.TodoFields) { t.ID().User(nil) }).
		Execute(ctx)
	if err != nil {
		return err
	}
	last, _ := tap.Last()
	fmt.Fprintf(w, "document:\n%s\nuser=%v\n", last.Query, todo.User)
	return nil
}

// interfaceResult queries node(id:), which returns the Node interface. The
// NodeFields selector always requests __typename and has an On<Type> method
// per implementation, which adds an inline fragment (... on Todo { ... }).
// Execute returns a *types.NodeValue whose Value is the concrete *types.Todo
// or *types.User, so a type switch reads it.
func interfaceResult(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Interface result (node) ==")
	for _, id := range []string{todoNodeID, userNodeID} {
		node, err := qr.Node().ID(id).
			Select(func(n *fields.NodeFields) {
				n.ID().
					OnTodo(func(t *fields.TodoFields) { t.Text().Done() }).
					OnUser(func(u *fields.UserFields) { u.Name().Role() })
			}).
			Execute(ctx)
		if err != nil {
			return err
		}
		if node == nil {
			// node(id:) is nullable: an unknown id is null, not an error.
			return fmt.Errorf("node %s not found", id)
		}
		describe(w, node.Typename, node.Value, node.Raw)
	}
	return nil
}

// unionResult does the same for search, which returns a list of the
// SearchResult union (Todo | User). A union has no shared fields, so every
// field is inside an On<Type> fragment.
func unionResult(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Union result (search) ==")
	results, err := qr.Search().Term(searchTerm).
		Select(func(s *fields.SearchResultFields) {
			s.OnTodo(func(t *fields.TodoFields) { t.ID().Text().Done() }).
				OnUser(func(u *fields.UserFields) { u.ID().Name().Role() })
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	for _, r := range results {
		describe(w, r.Typename, r.Value, r.Raw)
	}
	return nil
}

// describe type-switches on a decoded interface or union value. Value is nil
// when the server sent a __typename this SDK was not generated with (a member
// added to the schema later); Typename and Raw still hold what arrived, so
// the default branch can log it instead of losing it.
func describe(w *strings.Builder, typename string, value any, raw json.RawMessage) {
	switch v := value.(type) {
	case *types.Todo:
		fmt.Fprintf(w, "Todo %s: %q done=%t\n", v.ID, v.Text, v.Done)
	case *types.User:
		fmt.Fprintf(w, "User %s: %s %s\n", v.ID, v.Name, v.Role)
	default:
		fmt.Fprintf(w, "unknown member %q: %s\n", typename, raw)
	}
}
