# Go batching: several operations in one HTTP request

The generated `sdk/batch` package merges several builders into one GraphQL document with aliased root fields, posts it once, and decodes the answer into a struct you define, one field per alias. `batch.RunQueries` takes query builders and `batch.RunMutations` takes mutation builders; passing the wrong kind is a compile error, not a runtime surprise.

You want this when one screen or job needs several root fields at once (a dashboard with three lists, two filtered views of the same list, a set of related writes) and the round trips add up.

## Before you start

- A generated Go SDK; the snippets use [`example-go-mockapi/sdk`](../../example-go-mockapi/sdk). Every generated SDK has its own `batch` package; there is nothing extra to install.
- To run the example: mockapi on `http://localhost:8081/query` (`task mockapi:run`).

## Step 1 — Define the destination struct

The batch decodes the response's `data` object into your struct with `encoding/json`, so each field's `json` tag must equal an alias. From [`cmd/batch-mutations/main.go`](../../example-go-mockapi/cmd/batch-mutations/main.go):

```go
type created struct {
	Milk  types.Todo `json:"milk"`
	Bread types.Todo `json:"bread"`
}
```

The field type is whatever the operation returns: `types.Todo` for `createTodo: Todo!`, `[]types.Todo` for a list, `bool` for `deleteTodo: Boolean!`, a pointer for a nullable result.

## Step 2 — Run the batch

Build each operation exactly as you would for a single call, but stop before `.Execute`, and put the builders in a `batch.MutationItems` (or `batch.QueryItems`) map keyed by alias. The batch takes its HTTP client from the builders, so there is no client argument:

```go
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
```

Queries work the same way with `batch.RunQueries` and `batch.QueryItems`; `runBatch` in [`cmd/samples/main.go`](../../example-go-mockapi/cmd/samples/main.go) fetches open todos, completed todos and users in one request.

## Step 3 — Read the document the batch built

The batch renames every variable to `<alias>_<argument>` so two builders with the same argument (`input` above, or `filter` on two `todos` queries) do not collide, and it sorts aliases so the same map always produces a byte-identical document (useful for caching, snapshots and logs). The operation is always named `Batch`. See the output under Try it.

## Step 4 — Rely on the compiler to keep queries and mutations apart

Each generated query builder embeds `builder.QueryMarker` and each mutation builder embeds `builder.MutationMarker`. `batch.QueryItems` only accepts types with an `IsQueryOp()` method, so this program, kept in [`cmd/batch-mutations/testdata/mixed/main.go`](../../example-go-mockapi/cmd/batch-mutations/testdata/mixed/main.go), does not compile:

```go
package main

import (
	"context"

	"example-go-mockapi/sdk/batch"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/mutations"
	"example-go-mockapi/sdk/queries"
)

func main() {
	client := graphqlclient.NewClient("http://localhost:8081/query")
	qr := queries.NewQueryRoot(client)
	mr := mutations.NewMutationRoot(client)

	var dest struct{}
	_ = batch.RunQueries(context.Background(), &dest, batch.QueryItems{
		"ping":  qr.Ping(),
		"clear": mr.CompleteAllTodos(), // compile error: not a query builder
	})
}
```

```text
cmd/batch-mutations/testdata/mixed/main.go:24:12: cannot use mr.CompleteAllTodos() (value of type *mutations.CompleteAllTodosMutationBuilder) as batch.QueryBatchable value in map literal: *mutations.CompleteAllTodosMutationBuilder does not implement batch.QueryBatchable (missing method IsQueryOp)
```

`TestMixedKindsDoNotCompile` in the example's test file compiles that program and fails if it ever builds.

## Step 5 — Let the batch reject bad input before sending

A batch checks its map before building the document and returns a sentinel error, without sending anything, for an alias that is not a GraphQL name (`batch.ErrInvalidAlias`), a nil builder (`batch.ErrNilBuilder`), and two aliases whose namespaced variables would collide (`batch.ErrVariableCollision`, see Pitfalls). `rejected` in the example (called first in `run` above) triggers the first two:

```go
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
```

## Try it

```bash
task mockapi:run                          # in another terminal
cd example-go-mockapi && go run ./cmd/batch-mutations
```

Output:

```text
== Rejected before sending ==
alias "buy-milk": ErrInvalidAlias=true
nil builder:     ErrNilBuilder=true
requests sent: 0
== Document sent ==
mutation Batch($bread_input: NewTodo!, $milk_input: NewTodo!) {
  bread: createTodo(input: $bread_input) {
    id
    text
  }
  milk: createTodo(input: $milk_input) {
    id
    text
  }
}
variables: milk_input.text=Buy milk bread_input.text=Buy bread
== Created ==
milk="Buy milk" bread="Buy bread"
== Deleted ==
milk=true bread=true requests=2
```

Two rejected batches with no request, then four mutations in two HTTP requests. `go test ./cmd/batch-mutations` checks the document, the rejections and the compile failure.

## Pitfalls

- **Mutations run in alias order, not map order.** GraphQL executes mutation root fields one after another in document order, and the batch sorts aliases alphabetically ([`batch.tmpl`](../../gqlkit/pkg/templater/template/batch.tmpl), `sort.Strings(aliases)` in `run`). Above, `bread` is created before `milk`. When order matters (create, then link), pick aliases that sort in the order you need (`a_create`, `b_link`) or use separate calls.
- **A json tag that does not match an alias is silently ignored.** The field stays at its zero value and no error is returned. Keep the alias strings and the tags next to each other, or use constants for both.
- **Aliases must be valid GraphQL names** (letters, digits, underscore, not starting with a digit). The batch checks before sending and returns an error wrapping `batch.ErrInvalidAlias` (Step 5). SDKs generated by older gqlkit versions sent the document and let the server reject it; regenerate to get the check.
- **All builders must share one client.** The batch sends through the client of the first alias in sorted order ([`batch.tmpl`](../../gqlkit/pkg/templater/template/batch.tmpl), `items[aliases[0]].(clientHolder)`); builders created from a different client (another endpoint or token) are silently sent through that one.
- **Namespaced variable names can collide**: alias `a` with argument `b_c` and alias `a_b` with argument `c` both become `$a_b_c`. The batch detects this before sending and returns an error wrapping `batch.ErrVariableCollision` that names both aliases; rename one of them.
- **An empty map is an error** (`batch.RunQueries: requires at least one builder`), not a no-op.
- **Partial success is the normal error path.** `*batch.Error` means the server answered and some aliases may have data; any other error means nothing was decoded. See [Errors](errors.md#step-4--keep-partial-data-from-a-batch).
- **Queries in a batch may run concurrently on the server** (gqlgen resolves query root fields in parallel). Do not batch queries that depend on each other's side effects.

## Related

- [Errors](errors.md): `*batch.Error`, `GraphQLErrors` and partial data.
- [Selecting fields](selecting-fields.md): building the `Select` callbacks you pass in.
- [Low-level builder](low-level-builder.md): `GetOpFragment`, the method the batch uses to render each builder.
- Runnable example: [`example-go-mockapi/cmd/batch-mutations`](../../example-go-mockapi/cmd/batch-mutations); queries: `runBatch` in [`cmd/samples`](../../example-go-mockapi/cmd/samples/main.go).
