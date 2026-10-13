# Go low-level builder and raw client calls

Under every typed builder sits a `*builder.BaseBuilder` that holds the operation name, the arguments and a `*builder.FieldSelection`, and a `*graphqlclient.Client` that posts JSON. Both are public. You can print the exact document and variables a builder will send, add selections the typed API cannot express (a nested field with arguments), read results as untyped maps, or send a hand-written operation through the same client with its auth and transport.

You want this page for logging and snapshot tests of generated queries, for a field the typed selectors cannot express, and for the operation the generated builders do not cover (several root fields with aliases, a persisted query you already have as text).

## Before you start

- A generated Go SDK; the snippets use [`example-go-mockapi/sdk`](../../example-go-mockapi/sdk). The builder lives in `sdk/builder` and the client in `sdk/graphqlclient`; both are generated into your tree.
- To run the example: mockapi on `http://localhost:8081/query` (`task mockapi:run`).

## Step 1 — Print what a builder will send

`BuildQuery()` renders the document and `GetVariables()` returns the variables map, without sending anything. From [`cmd/low-level/main.go`](../../example-go-mockapi/cmd/low-level/main.go):

```go
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
```

The operation is named after the field (`query Todos`), variables are declared in sorted order, and child selections are sorted, so the same builder always produces the same string.

## Step 2 — Add a field by hand

`GetSelection()` returns the builder's root `*FieldSelection`, the same one `Select` writes to. `AddField(name)` and `AddChild(name, child)` add anything, as raw strings. The case this exists for is a nested field that takes arguments, which the typed field selectors cannot pass (`sel.AddChild("posts(first: 10)", child)`); mockapi has no such field, so the example adds `__typename` instead. From [`cmd/low-level/main.go`](../../example-go-mockapi/cmd/low-level/main.go):

```go
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
```

The typed `Execute` would decode into `types.Todo`, which has no field for what you added, so read hand-added fields with `ExecuteRaw` (Step 3) or `Client.Execute` into your own struct (Step 4). Unions and interfaces no longer need this: they have typed `On<Type>` selectors (see [Selecting fields](selecting-fields.md#step-3--query-a-field-that-returns-an-interface)).

## Step 3 — Read a result as a map

`ExecuteRaw(ctx)` sends the same document as `Execute` but returns the response's `data` object as `map[string]interface{}`, keyed by root field name:

```go
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
```

## Step 4 — Send a hand-written operation

`Client.Execute(ctx, query, variables, result)` posts any document through the configured client (same headers, same `http.Client`) and decodes the response's `data` object, not the whole `{"data": ...}` envelope, into `result`. Here two aliased root fields in one operation, decoded into a struct you own:

```go
const rawTodoQuery = `query TwoTodos($a: ID!, $b: ID!) {
  first: todo(id: $a) { id text }
  second: todo(id: $b) { id text }
}`
```

```go
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
```

To keep the `data` object as untouched JSON, use `Client.RawQuery`:

```go
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
```

## Step 5 — Keep data and errors together

`Client.Execute` and `RawQuery` stop at GraphQL errors. `Client.ExecuteWithPartialData(ctx, query, variables)` returns `(json.RawMessage, GraphQLErrors, error)`: the data and the errors together, with the third value reserved for requests that got no GraphQL answer at all. This is what batches use internally:

```go
// partialQuery has one field that succeeds and one that always fails.
const partialQuery = `{ ok: ping bad: fail(message: "nope") }`
```

```go
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
```

## Try it

```bash
task mockapi:run                          # in another terminal
cd example-go-mockapi && go run ./cmd/low-level
```

Output:

```text
== BuildQuery / GetVariables ==
query Todos($pagination: PaginationInput) {
  todos(pagination: $pagination) {
    id
    text
  }
}
variables: {"pagination":{"limit":2,"offset":0}}
== Raw field via GetSelection ==
id=1 __typename=Todo
== ExecuteRaw ==
ping: pong from gqlgenapi
== Client.Execute ==
first=1 "Try the ping query" second=2 "Create your first todo"
== Client.RawQuery ==
{"ping":"pong from gqlgenapi"}
== Client.ExecuteWithPartialData ==
data={"ok":"pong from gqlgenapi","bad":null} errors=1 first="nope"
```

`go test ./cmd/low-level` runs the program and checks these lines.

## Pitfalls

- **`Client.RawQuery` returned an empty payload in SDKs generated by older gqlkit versions.** If yours returns `""` with a nil error, regenerate, or use `Client.Execute` with a `*json.RawMessage` result, which has always worked.
- **Raw selections are not checked.** `AddField`, `AddChild` and the keys you add are strings spliced into the document; a typo is a server validation error, and a child selection with no fields is silently dropped (see [Selecting fields: Pitfalls](selecting-fields.md#pitfalls)).
- **`AddChild` replaces a child with the same key.** Adding `"posts(first: 10)"` twice keeps only the second selection.
- **Typed `Select` and raw `GetSelection` write to the same selection.** Mixing them is how Step 2 works; calling `Select` after adding raw children does not reset them.
- **`ExecuteRaw` numbers are `float64`.** It decodes into `map[string]interface{}`, so `Int` fields arrive as `float64` and large IDs lose precision. Decode into your own struct with `Client.Execute` when types matter.
- **`Client.Execute` with a nil result discards the data** but still returns GraphQL errors; useful for fire-and-forget mutations, surprising if you meant to read the answer.
- **Hand-written documents bypass the compile-time checks** (field names, argument types, query vs mutation in a batch). Keep them for the cases the typed API cannot express.
- **The upstream `github.com/ubgo/gqlkit/gqlkit/pkg/builder` package** is the source of the generated `sdk/builder` and exists for gqlkit's own tests. Use the generated copy in your SDK; importing the upstream one couples your code to the generator's module and its versions ([`gqlkit/pkg/builder/README.md`](../../gqlkit/pkg/builder/README.md) documents its API).

## Related

- [Selecting fields](selecting-fields.md): the typed API this page bypasses.
- [Client](client.md): the options that apply to every call here too.
- [Errors](errors.md): what `Client.Execute` returns on failure.
- Runnable example: [`example-go-mockapi/cmd/low-level`](../../example-go-mockapi/cmd/low-level).
