# Go field selection: Select, nested objects, self-references, interfaces and unions

Every operation that returns an object takes a `Select` callback. Inside it you call one method per field you want; object fields take another callback for their own fields. gqlkit turns those calls into the GraphQL selection set, so you never write a query string, and a typo in a field name is a compile error.

You want this page for anything past a flat list of scalars: nested objects, connections, a field that points back at its own type, a field that returns an interface or a union.

## Before you start

- A generated Go SDK; the snippets use [`example-go-mockapi/sdk`](../../example-go-mockapi/sdk).
- To run the examples: mockapi on `http://localhost:8081/query` (`task mockapi:run`).
- Know that in Go, selection controls what the server sends, not the shape of the Go type: see [Types and scalars](types-and-scalars.md#step-4--know-what-unselected-means-in-go).

## Step 1 — Select scalar fields and nested objects

Each field method returns the selector, so calls chain. An object field (`User` on `Todo`) takes a `func(*fields.UserFields)`. From `runTodosList` in [`cmd/samples/main.go`](../../example-go-mockapi/cmd/samples/main.go):

```go
	todos, err := qr.Todos().
		Filter(filter).
		Pagination(pagination).
		Select(func(f *fields.TodoFields) {
			f.ID().
				Text().
				Done().
				Priority().
				Tags().
				User(func(u *fields.UserFields) {
					u.ID().Name().Role()
				})
		}).
		Execute(ctx)
```

Connections are the same pattern nested deeper (`Edges` then `Node`); `runTodosConnection` in the same file selects `totalCount`, `pageInfo` and `edges { cursor node { ... } }`.

## Step 2 — Select a self-referencing field to a chosen depth

A field whose type is its own parent type (`Todo.parent: Todo`, a category tree, a comment thread) gets a selector callback like any other object field. You decide how deep to go by nesting callbacks; there is no recursive "select everything", because GraphQL has none. From [`cmd/selecting-fields/main.go`](../../example-go-mockapi/cmd/selecting-fields/main.go):

```go
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
```

The Go type is recursive too: `types.Todo` has `Parent *types.Todo`.

## Step 3 — Query a field that returns an interface

`node(id: ID!): Node` returns the `Node` interface, implemented by `Todo` and `User`. The generated `fields.NodeFields` selector has the interface's own fields (`ID()`) plus one `On<Type>` method per implementation, which adds an inline fragment (`... on Todo { ... }`). The selector always requests `__typename`, and `Execute` returns a `*types.NodeValue` whose `Value` is the concrete `*types.Todo` or `*types.User`. From [`cmd/selecting-fields/main.go`](../../example-go-mockapi/cmd/selecting-fields/main.go):

```go
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
```

`describe` is an ordinary type switch. Its `default` branch matters: when the server sends a `__typename` the SDK was not generated with (a type added to the schema later), decoding does not fail, `Value` is nil, and `Typename` and `Raw` keep exactly what arrived:

```go
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
```

Every implementation also gets a marker method (`func (Todo) IsNode()`), so `*types.Todo` satisfies `types.Node` and you can write functions that accept any `types.Node`.

## Step 4 — Query a field that returns a union

Unions work the same way, except a union has no shared fields, so everything is inside an `On<Type>` fragment. `search(term: String!): [SearchResult!]!` returns `[]types.SearchResultValue`:

```go
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
```

The example also decodes a value with an unknown member offline, to show the `default` branch:

```go
// newerServerResult is what a server could send after adding a third member
// to SearchResult that this SDK was not generated with.
const newerServerResult = `{"__typename":"Comment","id":"c1","body":"hi"}`
```

```go
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
```

Interface- and union-typed fields inside an object work the same way: the field selector takes a callback with the same `On<Type>` methods, and the struct field is a `*<Name>Value`.

## Try it

```bash
task mockapi:run                          # in another terminal
cd example-go-mockapi && go run ./cmd/selecting-fields
```

Output on a freshly started mockapi (the union block lists every todo and user whose text or name contains `o`, so it grows if other programs have added data):

```text
== Self-referencing field ==
todo 2 "Create your first todo" -> parent 1 "Try the ping query" -> grandparent <nil>
== Nested selector with no fields ==
document:
query Todo($id: ID!) {
  todo(id: $id) {
    id
  }
}
user=<nil>
== Interface result (node) ==
Todo 1: "Try the ping query" done=false
User 2: Bob ADMIN
== Union result (search) ==
Todo 2: "Create your first todo" done=false
User 2: Bob ADMIN
== Unknown union member ==
unknown member "Comment": {"__typename":"Comment","id":"c1","body":"hi"}
```

`go test ./cmd/selecting-fields` runs the same program and checks these lines. `go run ./cmd/samples` runs the list and connection queries from Step 1.

## Pitfalls

- **A nested selector that selects nothing is silently dropped.** `t.ID().User(nil)`, or a callback that calls no methods, produces no `user { }` block at all, because GraphQL forbids an empty selection set ([`field_selector.tmpl`](../../gqlkit/pkg/templater/template/field_selector.tmpl) adds the child, and `FieldSelection.Build` in [`builder.go`](../../example-go-mockapi/sdk/builder/builder.go) skips children that render empty). The output above shows the document without `user` and `user=<nil>` with no error.
- **Calling the same nested selector twice keeps only the last call.** Children are stored by field name, so `f.User(func(u *fields.UserFields) { u.ID() }).User(func(u *fields.UserFields) { u.Name() })` asks only for `name`. Put every field in one callback.
- **No `Select` at all on an object-, interface- or union-returning operation is a server error, not a compile error.** The document then has `todo(id: $id)` with no selection set, which the server rejects; see [Errors](errors.md) for how that surfaces. For interfaces and unions, `__typename` is added by the selector, so without `Select` the result could not be decoded into a concrete type either. Call `Select`, even if only to ask for `ID()`.
- **Always handle the `default` case of the type switch.** A server can add a union member or interface implementation at any time; your SDK then gets `Value == nil` for those results until you regenerate. Log `Typename` (and `Raw` if useful) rather than dropping the result silently.
- **Fields from one `On<Type>` fragment are zero on the other types.** `t.Done()` inside `OnTodo` says nothing about users; reading the concrete type from `Value` keeps you from mixing them up.
- **Fields that take arguments inside a selection cannot be given arguments.** Field selector methods take no parameters ([`field_selector.tmpl`](../../gqlkit/pkg/templater/template/field_selector.tmpl)); only root operations have argument setters. For a nested field such as `posts(first: 10)`, add it to the raw selection by name (`sel.AddChild("posts(first: 10)", child)`) and accept that the value is inlined rather than sent as a variable. See [Low-level builder: Step 2](low-level-builder.md#step-2--add-a-field-by-hand).
- **A schema field named like a generated method gets a `_` suffix.** A field or argument that would become `Select`, `Execute`, `OnTodo` and so on is generated as `Select_`, `Execute_`, ... so the SDK's own methods keep their names ([`naming.go`](../../gqlkit/pkg/clientgen/naming.go)).

## Related

- [Types and scalars](types-and-scalars.md): the Go types the selection decodes into, and why unselected fields are zero values.
- [Arguments and inputs](arguments-and-inputs.md): the setters before `Select`.
- [Low-level builder](low-level-builder.md): `GetSelection` and `FieldSelection` for what the typed selectors cannot express.
- Runnable examples: [`cmd/selecting-fields`](../../example-go-mockapi/cmd/selecting-fields), [`cmd/samples`](../../example-go-mockapi/cmd/samples/main.go).
