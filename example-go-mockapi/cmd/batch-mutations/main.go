// Command batch-mutations runs several mutations in one HTTP request with
// batch.RunMutations, prints the merged document the batch sent (aliases
// sorted, arguments namespaced per alias), then deletes what it created in a
// second batch so the shared mockapi store is left as it was found. It also
// shows the inputs a batch refuses before sending anything.
//
// Run it against mockapi: go run ./cmd/batch-mutations
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"example-go-mockapi/internal/mockapi"
	"example-go-mockapi/internal/wiretap"
	"example-go-mockapi/sdk/batch"
	"example-go-mockapi/sdk/fields"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/inputs"
	"example-go-mockapi/sdk/mutations"
	"example-go-mockapi/sdk/types"
)

// todoOwner is a seeded mockapi user id.
const todoOwner = "1"

func main() {
	var out strings.Builder
	err := run(context.Background(), &out, mockapi.Endpoint())
	mockapi.Finish(out.String(), err)
}

// created is the batch destination: one field per alias, matched by json tag.
// A missing or misspelled tag leaves that field at its zero value silently.
type created struct {
	Milk  types.Todo `json:"milk"`
	Bread types.Todo `json:"bread"`
}

// deleted is the destination for the clean-up batch.
type deleted struct {
	Milk  bool `json:"milk"`
	Bread bool `json:"bread"`
}

func run(ctx context.Context, w *strings.Builder, endpoint string) error {
	// wiretap records each request so the example can print the merged
	// document; it is not needed to use batching.
	tap := &wiretap.Transport{}
	client := graphqlclient.NewClient(endpoint,
		graphqlclient.WithHTTPClient(&http.Client{Transport: tap}))
	mr := mutations.NewMutationRoot(client)

	sel := func(f *fields.TodoFields) { f.ID().Text() }

	rejected(ctx, w, mr, tap)

	var c created
	err := batch.RunMutations(ctx, &c, batch.MutationItems{
		"milk":  mr.CreateTodo().Input(inputs.NewTodo{Text: "Buy milk", UserID: todoOwner}).Select(sel),
		"bread": mr.CreateTodo().Input(inputs.NewTodo{Text: "Buy bread", UserID: todoOwner}).Select(sel),
	})
	if err != nil {
		// *batch.Error means the server answered with GraphQL errors; c is
		// still filled for every alias that succeeded. Anything else is a
		// transport failure and c is untouched.
		var berr *batch.Error
		if !errors.As(err, &berr) {
			return err
		}
		fmt.Fprintf(w, "partial success: %v\n", berr)
	}

	last, _ := tap.Last()
	fmt.Fprintln(w, "== Document sent ==")
	fmt.Fprintln(w, last.Query)
	fmt.Fprintf(w, "variables: milk_input.text=%v bread_input.text=%v\n",
		textOf(last.Variables["milk_input"]), textOf(last.Variables["bread_input"]))

	fmt.Fprintln(w, "== Created ==")
	fmt.Fprintf(w, "milk=%q bread=%q\n", c.Milk.Text, c.Bread.Text)

	var d deleted
	if err := batch.RunMutations(ctx, &d, batch.MutationItems{
		"milk":  mr.DeleteTodo().ID(c.Milk.ID),
		"bread": mr.DeleteTodo().ID(c.Bread.ID),
	}); err != nil {
		return fmt.Errorf("clean-up batch: %w", err)
	}
	fmt.Fprintln(w, "== Deleted ==")
	fmt.Fprintf(w, "milk=%t bread=%t requests=%d\n", d.Milk, d.Bread, tap.Count())
	return nil
}

// textOf reads "text" from a recorded input variable, which arrives as the
// map json.Unmarshal builds for a JSON object.
func textOf(v interface{}) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	return m["text"]
}

// rejected shows the checks a batch makes before sending anything: an alias
// that is not a GraphQL name and a nil builder are refused with sentinel
// errors you can test with errors.Is, and no request goes out.
func rejected(ctx context.Context, w *strings.Builder, mr *mutations.MutationRoot, tap *wiretap.Transport) {
	fmt.Fprintln(w, "== Rejected before sending ==")
	var c created
	err := batch.RunMutations(ctx, &c, batch.MutationItems{
		"buy-milk": mr.CompleteAllTodos(),
	})
	fmt.Fprintf(w, "alias \"buy-milk\": ErrInvalidAlias=%t\n", errors.Is(err, batch.ErrInvalidAlias))

	err = batch.RunMutations(ctx, &c, batch.MutationItems{"milk": nil})
	fmt.Fprintf(w, "nil builder:     ErrNilBuilder=%t\n", errors.Is(err, batch.ErrNilBuilder))
	fmt.Fprintf(w, "requests sent: %d\n", tap.Count())
}
