// Command low-level shows the escape hatches under the typed builders: the
// GraphQL document and variables a builder will send, a hand-added field on
// the raw selection, ExecuteRaw, and the client's own Execute, RawQuery and
// ExecuteWithPartialData for operations the generated builders do not cover.
//
// Run it against mockapi: go run ./cmd/low-level
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"example-go-mockapi/internal/mockapi"
	"example-go-mockapi/sdk/fields"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/inputs"
	"example-go-mockapi/sdk/queries"
)

// seededTodoID is a todo mockapi seeds on start ("Try the ping query").
const seededTodoID = "1"

// partialQuery has one field that succeeds and one that always fails.
const partialQuery = `{ ok: ping bad: fail(message: "nope") }`

// rawTodoQuery is a hand-written operation for Client.Execute: two root
// fields with an alias, which a single generated builder cannot express.
const rawTodoQuery = `query TwoTodos($a: ID!, $b: ID!) {
  first: todo(id: $a) { id text }
  second: todo(id: $b) { id text }
}`

func main() {
	var out strings.Builder
	err := run(context.Background(), &out, mockapi.Endpoint())
	mockapi.Finish(out.String(), err)
}

func run(ctx context.Context, w *strings.Builder, endpoint string) error {
	client := graphqlclient.NewClient(endpoint)
	qr := queries.NewQueryRoot(client)

	inspectBuilder(w, qr)
	if err := rawField(ctx, w, qr); err != nil {
		return fmt.Errorf("raw field: %w", err)
	}
	if err := executeRaw(ctx, w, qr); err != nil {
		return fmt.Errorf("ExecuteRaw: %w", err)
	}
	if err := clientExecute(ctx, w, client); err != nil {
		return fmt.Errorf("Client.Execute: %w", err)
	}
	if err := rawData(ctx, w, client); err != nil {
		return fmt.Errorf("raw data: %w", err)
	}
	if err := partial(ctx, w, client); err != nil {
		return fmt.Errorf("partial data: %w", err)
	}
	return nil
}

// inspectBuilder prints what a builder would send, without sending it.
// Useful for logging, snapshot tests, and pasting into a GraphQL playground.
func inspectBuilder(w *strings.Builder, qr *queries.QueryRoot) {
	limit := 2
	b := qr.Todos().
		Pagination(&inputs.PaginationInput{Limit: limit}).
		Select(func(f *fields.TodoFields) { f.ID().Text() })

	fmt.Fprintln(w, "== BuildQuery / GetVariables ==")
	fmt.Fprintln(w, b.BuildQuery())
	vars, _ := json.Marshal(b.GetVariables())
	fmt.Fprintf(w, "variables: %s\n", vars)
}

// rawField adds a field the typed selector does not offer to the builder's
// raw FieldSelection, and reads the result with ExecuteRaw. The case it
// exists for is a nested field that takes arguments, which field selector
// methods cannot pass; __typename stands in for one here because mockapi has
// no such field. Names here are strings: a typo is a server error, not a
// compile error.
func rawField(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	b := qr.Todo().ID(seededTodoID).
		Select(func(f *fields.TodoFields) { f.ID() })
	b.GetSelection().AddField("__typename")

	data, err := b.ExecuteRaw(ctx)
	if err != nil {
		return err
	}
	todo, ok := data["todo"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("todo %s not found", seededTodoID)
	}
	fmt.Fprintln(w, "== Raw field via GetSelection ==")
	fmt.Fprintf(w, "id=%v __typename=%v\n", todo["id"], todo["__typename"])
	return nil
}

// executeRaw returns the response's data object as a map, keyed by root
// field name, without decoding into the generated types.
func executeRaw(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	data, err := qr.Ping().ExecuteRaw(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== ExecuteRaw ==")
	fmt.Fprintf(w, "ping: %v\n", data["ping"])
	return nil
}

// todoSummary is a caller-owned result type for clientExecute; the json tags
// match the aliases in rawTodoQuery.
type todoSummary struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// clientExecute sends a hand-written document. Execute decodes the
// response's "data" object (not the whole envelope) into the result.
func clientExecute(ctx context.Context, w *strings.Builder, client *graphqlclient.Client) error {
	var out struct {
		First  *todoSummary `json:"first"`
		Second *todoSummary `json:"second"`
	}
	vars := map[string]interface{}{"a": "1", "b": "2"}
	if err := client.Execute(ctx, rawTodoQuery, vars, &out); err != nil {
		return err
	}
	if out.First == nil || out.Second == nil {
		// todo(id:) is nullable: a missing todo is null, not an error.
		return fmt.Errorf("seeded todos 1 and 2 not found")
	}
	fmt.Fprintln(w, "== Client.Execute ==")
	fmt.Fprintf(w, "first=%s %q second=%s %q\n", out.First.ID, out.First.Text, out.Second.ID, out.Second.Text)
	return nil
}

// rawData gets the response's "data" object as untouched JSON with
// Client.RawQuery.
func rawData(ctx context.Context, w *strings.Builder, client *graphqlclient.Client) error {
	data, err := client.RawQuery(ctx, "{ ping }", nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== Client.RawQuery ==")
	fmt.Fprintf(w, "%s\n", data)
	return nil
}

// partial sends a hand-written document whose second field fails, with
// Client.ExecuteWithPartialData. Unlike Execute it returns the data and the
// GraphQL errors together; its error result is reserved for requests that
// got no GraphQL answer at all.
func partial(ctx context.Context, w *strings.Builder, client *graphqlclient.Client) error {
	data, gqlErrs, err := client.ExecuteWithPartialData(ctx, partialQuery, nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== Client.ExecuteWithPartialData ==")
	fmt.Fprintf(w, "data=%s errors=%d first=%q\n", data, len(gqlErrs), firstMessage(gqlErrs))
	return nil
}

// firstMessage returns the first error's message, or "" when there is none.
func firstMessage(errs graphqlclient.GraphQLErrors) string {
	if len(errs) == 0 {
		return ""
	}
	return errs[0].Message
}
