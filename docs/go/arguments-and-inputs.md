# Go arguments and input types: setters, input structs, pointers and enums

Every argument of a GraphQL operation becomes a setter method on its builder (`.ID("1")`, `.Filter(...)`, `.Role(...)`). Input object types become structs in the `sdk/inputs` package, enums become typed string constants in `sdk/enums`, and anything optional is a pointer, or, if you configure it, an `inputs.Opt[T]` that can also send an explicit `null`. The values are always sent as GraphQL variables, never pasted into the query text.

You want this page when you pass anything beyond a plain string: a filter object, an optional argument, an enum, a value you want the server to treat as "not provided", or a field you want to clear.

## Before you start

- A generated Go SDK; the snippets use [`example-go-mockapi/sdk`](../../example-go-mockapi/sdk), from this schema excerpt:

```graphql
input TodoFilter { textContains: String  done: Boolean  minPriority: Int  hasTag: String }
input UpdateTodoInput { text: String  done: Boolean  priority: Int  tags: [String!] }
input PaginationInput { limit: Int! = 10  offset: Int! = 0 }
enum Role { ADMIN USER GUEST }
type Mutation { upsertUser(id: ID, name: String!, email: String, role: Role = USER): User! }
```

- To run the example: mockapi on `http://localhost:8081/query` (`task mockapi:run`).

## Step 1 — Know how each argument maps to Go

| GraphQL argument | Generated setter | Example |
| --- | --- | --- |
| `name: String!` (required scalar) | `Name(v string)` | `.Name("Eve")` |
| `email: String` (optional scalar) | `Email(v *string)` | `.Email(ptr("eve@example.com"))` |
| `role: Role = USER` (optional enum) | `Role(v *enums.Role)` | `.Role(ptr(enums.RoleGuest))` |
| `filter: TodoFilter` (optional input) | `Filter(v *inputs.TodoFilter)` | `.Filter(&inputs.TodoFilter{...})` |
| `input: UpdateTodoInput!` (required input) | `Input(v inputs.UpdateTodoInput)` | `.Input(inputs.UpdateTodoInput{...})` |
| optional input field, default `pointer` mode (`done: Boolean`) | `Done *bool` | `Done: ptr(true)` |
| optional input field, `opt` mode (`text: String`) | `Text inputs.Opt[string]` | `Text: inputs.Some("x")`, `Text: inputs.Null[string]()` |

Three states are possible for an optional argument: **not calling the setter** leaves it out of the request, so the server applies the schema default; **calling it with a pointer** sends that value; **calling it with `nil`** sends an explicit `null`.

Go has no literal syntax for `&"text"` or `&true`, so the example defines one generic helper. From [`cmd/enums-and-inputs/main.go`](../../example-go-mockapi/cmd/enums-and-inputs/main.go):

```go
// ptr returns a pointer to v. Optional GraphQL inputs and arguments are
// pointers in Go, and Go has no literal syntax for &"text" or &true.
func ptr[T any](v T) *T { return &v }
```

## Step 2 — Pass an enum, or leave it to the default

```go
// enumArgument passes an enum as an optional argument. upsertUser declares
// `role: Role = USER`, so the setter takes *enums.Role: leave it unset to get
// the server default, or pass a pointer to choose.
func enumArgument(ctx context.Context, w *strings.Builder, mr *mutations.MutationRoot) error {
	fmt.Fprintln(w, "== Enum as an argument ==")
	sel := func(u *fields.UserFields) { u.ID().Name().Email().Role() }

	guest, err := mr.UpsertUser().
		Name(newUserName).
		Role(ptr(enums.RoleGuest)).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with Role(GUEST): role=%s isGuest=%t\n", guest.Role, guest.Role == enums.RoleGuest)

	// No Role(...) call: the argument is left out of the request, so the
	// schema default (USER) applies. Email is also optional and unset.
	plain, err := mr.UpsertUser().Name(newUserName).Select(sel).Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "without Role:     role=%s email=%v\n", plain.Role, plain.Email)
	return nil
}
```

The enum constants are `enums.RoleAdmin`, `enums.RoleUser`, `enums.RoleGuest`: the type name followed by the value in PascalCase, split on underscores (`IN_PROGRESS` would become `InProgress`), with the GraphQL spelling kept as the string value. Results use the same type (`types.User.Role` is `enums.Role`), so comparisons are type-checked.

## Step 3 — Fill an input struct, leaving optional fields nil

Optional input fields are pointers with `omitempty`, so a nil field is not sent at all. That is how you say "no filter on this field". The example prints the variables each builder sends, using `GetVariables`:

```go
// optionalFilter shows that a nil pointer field in an input struct is left
// out of the request entirely, so it does not filter at all: Done: nil means
// "any", Done: ptr(false) means "open only". The variables printed are
// exactly what the builder sends.
func optionalFilter(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Optional input fields ==")
	sel := func(f *fields.TodoFields) { f.ID() }

	for _, filter := range []*inputs.TodoFilter{{}, {Done: ptr(false)}} {
		b := qr.Todos().Filter(filter).Select(sel)
		vars, err := json.Marshal(b.GetVariables())
		if err != nil {
			return err
		}
		todos, err := b.Execute(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "variables %s -> %d todos\n", vars, len(todos))
	}
	return nil
}
```

`Done: ptr(false)` is sent: a non-nil pointer to `false` is a value, unlike a plain `bool` field where `false` is indistinguishable from "unset".

## Step 4 — Clear a list with an empty slice

In the default `pointer` mode, an optional list field is a slice. A nil slice is left out of the request ("leave it alone"), and a non-nil empty slice is sent as `[]` ("clear it"):

```go
// emptySliceClears sends an empty list to clear a field. In the default
// pointer mode a nil slice is left out of the request ("leave the tags
// alone") and a non-nil empty slice is sent as [] ("clear the tags").
func emptySliceClears(ctx context.Context, w *strings.Builder, mr *mutations.MutationRoot) error {
	fmt.Fprintln(w, "== Empty slice clears a list ==")
	sel := func(f *fields.TodoFields) { f.ID().Tags() }

	created, err := mr.CreateTodo().
		Input(inputs.NewTodo{Text: "tagged", UserID: seededTodoOwner, Tags: []string{"keep"}}).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	// Delete the todo even if an update fails, so repeated runs leave the
	// shared mockapi store as they found it.
	defer func() { _, _ = mr.DeleteTodo().ID(created.ID).Execute(ctx) }()

	untouched, err := mr.UpdateTodo().ID(created.ID).
		Input(inputs.UpdateTodoInput{Tags: nil}).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	cleared, err := mr.UpdateTodo().ID(created.ID).
		Input(inputs.UpdateTodoInput{Tags: []string{}}).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "before=%v after Tags: nil=%v after Tags: []string{}=%v\n", created.Tags, tagsOf(untouched), tagsOf(cleared))
	return nil
}
```

The SDK does this with a generated `inputs/inputs_json.go` that uses the `omitzero` tag option, which `encoding/json` honours from Go 1.24. The generated SDK requires Go 1.24 or newer: on an older toolchain it does not build, and the error names the requirement (`undefined: gqlkit_SDK_requires_Go_1_24_or_newer`, from the generated `go_version_guard.go` files).

## Step 5 — Send an explicit null with `Opt[T]`

A pointer field has only two states: nil (left out) or a value. To set a field to `null` (for example to remove a due date), generate it as `inputs.Opt[T]`, which has three: the zero value is left out, `inputs.Some(v)` sends `v`, and `inputs.Null[T]()` sends `null`. Choose the mode in the `inputs` section of the config file; a field override beats a type override, which beats the global setting. From [`cmd/generate/config.jsonc`](../../example-go-mockapi/cmd/generate/config.jsonc), which keeps pointers everywhere except two fields:

```jsonc
  "inputs": {
    "overrides": {
      "UpdateTodoInput.priority": "opt",
      "UpdateTodoInput.text": "opt"
    }
  },
```

For a new SDK, `"inputs": { "optional": "opt" }` makes every nullable input field an `Opt`; `"overrides": { "SomeInput": "pointer" }` then opts one type back out. Keys use GraphQL names, and an unknown mode, type or field fails generation instead of being ignored. The generated struct:

```go
// UpdateTodoInput represents the GraphQL type UpdateTodoInput
type UpdateTodoInput struct {
	Text     Opt[string] `json:"text,omitzero"`
	Done     *bool       `json:"done,omitempty"`
	Priority Opt[int]    `json:"priority,omitzero"`
	Tags     []string    `json:"tags,omitempty"`
}
```

The example prints what each state sends:

```go
// optFields shows the three states of an Opt[T] input field. This SDK
// generates UpdateTodoInput.text and .priority as Opt (see the `inputs`
// section of cmd/generate/config.jsonc): the zero value is left out, Some(v)
// sends the value, and Null[T]() sends an explicit null. The variables
// printed are exactly what the builder sends.
func optFields(w *strings.Builder, mr *mutations.MutationRoot) error {
	fmt.Fprintln(w, "== Opt[T]: unset, Some, Null ==")
	for _, in := range []inputs.UpdateTodoInput{
		{},
		{Text: inputs.Some("renamed")},
		{Priority: inputs.Null[int]()},
	} {
		vars, err := json.Marshal(mr.UpdateTodo().ID(seededTodoID).Input(in).GetVariables()["input"])
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "input %s\n", vars)
	}
	return nil
}
```

Read an `Opt` back with `v, ok := o.Get()`, and `o.IsNull()` / `o.IsZero()` for the other two states. `Opt` relies on `omitzero` too, which is one reason the whole SDK requires Go 1.24+.

If you need a field in a mode you did not generate (a `null` for a pointer field), send that one mutation as a hand-written document with [`Client.Execute`](low-level-builder.md#step-4--send-a-hand-written-operation), where the variables map is yours.

## Try it

```bash
task mockapi:run                          # in another terminal
cd example-go-mockapi && go run ./cmd/enums-and-inputs
```

Output:

```text
== Enum constants ==
ADMIN USER GUEST 
== Enum as an argument ==
with Role(GUEST): role=GUEST isGuest=true
without Role:     role=USER email=<nil>
== Optional input fields ==
variables {"filter":{}} -> 2 todos
variables {"filter":{"done":false}} -> 2 todos
== Unselected fields are zero values ==
selected:   id=1 text="Try the ping query"
unselected: tags=[] user=<nil> createdAt.IsZero=true
== Empty slice clears a list ==
before=[keep] after Tags: nil=[keep] after Tags: []string{}=[]
== Opt[T]: unset, Some, Null ==
input {}
input {"text":"renamed"}
input {"priority":null}
== Scalar types ==
createdAt: time.Time year=2025
metadata:  map[string]interface {}
serverInfo: raw JSON, valid=true
```

The todo counts depend on what is in mockapi's in-memory store. `go test ./cmd/enums-and-inputs` runs the program and checks every other line.

## Pitfalls

- **Required arguments are not enforced by the compiler.** A setter is an ordinary method; forgetting `.B(2)` on `qr.Sum().A(1)` compiles and the server rejects the request. See [Errors: Step 5](errors.md#step-5--see-a-request-the-server-refuses).
- **In `pointer` mode there is no way to send `null` for an input field.** A nil pointer means "leave it out". Generate the field in `opt` mode (Step 5) if callers must be able to clear it. Arguments are different: a setter called with `nil` does send `null`.
- **`[]string{}` and `nil` mean different things** (Step 4): clear versus leave alone. Code that builds a slice with `var tags []string` and appends nothing sends nothing; build it with `tags := []string{}` when "none" should clear the field.
- **Changing a field's mode changes its Go type.** Switching `UpdateTodoInput.text` from `pointer` to `opt` turns `*string` into `Opt[string]`, so every caller that sets it must change from `ptr("x")` to `inputs.Some("x")`. Pick the mode before callers multiply; `opt` is the recommended mode for new SDKs.
- **Required input fields with a schema default always send the Go zero value.** `PaginationInput.Limit` is `limit: Int! = 10` in the schema but a plain `int` in Go with no `omitempty`, so `&inputs.PaginationInput{Offset: 20}` sends `limit: 0`, not 10. Set every such field explicitly.
- **Calling a setter twice keeps the last value**; arguments are stored by name.
- **Enum values are strings underneath.** `enums.Role("ADMINN")` compiles. Use the generated constants, and treat values from outside your code (config, user input) as untrusted until compared against them.
- **Mutations share state.** The mutations in this example change mockapi's in-memory store; the example deletes the todo it creates, but `upsertUser` has no delete, so each run adds a user named `Eve`.

## Related

- [Types and scalars](types-and-scalars.md): the result side of the same types.
- [Selecting fields](selecting-fields.md): what goes in `Select`.
- [Low-level builder](low-level-builder.md): `GetVariables`, `BuildQuery`, and sending your own variables.
- Runnable example: [`example-go-mockapi/cmd/enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs).
