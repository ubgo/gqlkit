# Go types, scalars and enums: what the generator emits and why

A generated Go SDK has one Go type per GraphQL type, shared by every operation: objects become structs in `sdk/types`, input objects become structs in `sdk/inputs`, enums become named string types in `sdk/enums`, and custom scalars become type aliases in `sdk/scalars`. This page explains how each GraphQL construct maps to Go, which scalars are mapped for you, how to map your own, and what "unselected" means for a Go struct.

You want this page when you read a generated type and wonder why a field is a pointer, why a scalar is `any`, or why a field you did not select is still there.

## Before you start

- A generated Go SDK; the snippets use [`example-go-mockapi/sdk`](../../example-go-mockapi/sdk).
- To run the example: mockapi on `http://localhost:8081/query` (`task mockapi:run`).

## Step 1 — Read a generated struct

From [`sdk/types/types.go`](../../example-go-mockapi/sdk/types/types.go), generated from `type Todo implements Node { id: ID!  text: String!  done: Boolean!  priority: Int  tags: [String!]!  user: User!  createdAt: DateTime!  metadata: Metadata  parent: Todo }`:

```go
// Todo represents the GraphQL type Todo
type Todo struct {
	ID        string            `json:"id"`
	Text      string            `json:"text"`
	Done      bool              `json:"done"`
	Priority  *int              `json:"priority,omitempty"`
	Tags      []string          `json:"tags"`
	User      *User             `json:"user"`
	CreatedAt time.Time         `json:"createdAt"`
	Metadata  *scalars.Metadata `json:"metadata,omitempty"`
	// Self-reference: the todo this one belongs under, or null for a top-level
	// todo. Seed todo "2" has todo "1" as its parent.
	Parent *Todo `json:"parent,omitempty"`
}
```

The rules ([`types_gen.go`](../../gqlkit/pkg/clientgen/types_gen.go), `graphQLToGoType`):

| GraphQL | Go | Why |
| --- | --- | --- |
| `String!`, `Int!`, `Boolean!`, enum `Role!` | `string`, `int`, `bool`, `enums.Role` | non-null scalars and enums are values |
| `Int`, `String`, `Role` (nullable) | `*int`, `*string`, `*enums.Role` | a pointer tells `null` apart from `0` / `""` |
| any object or input object, nullable or not (`User!`, `Todo`) | `*User`, `*Todo` | always a pointer, so recursive types (`Todo.parent: Todo`) are legal Go |
| lists `[String!]!`, `[TodoEdge!]!` | `[]string`, `[]*TodoEdge` | a nil slice already models null; object elements are pointers |
| interface or union used as a field or result type (`node: Node`, `[SearchResult!]!`) | `*types.NodeValue`, `[]types.SearchResultValue` | a wrapper decoded by `__typename`: `Value` holds the concrete `*Todo` or `*User` |
| interface or union type itself (`interface Node`) | `type Node interface { IsNode() }` | a marker; every implementation or member gets the method (`func (Todo) IsNode()`) |

Operation results follow the same idea: `todo(id: ID!): Todo` returns `*types.Todo` (nil when not found), `todos: [Todo!]!` returns `[]types.Todo`, `ping: String!` returns `string`.

## Step 2 — Know which scalars are mapped for you

These scalars need no configuration ([`typegql.go`](../../gqlkit/pkg/typegql/typegql.go), `builtInTypes`). The first five are the GraphQL spec scalars; the rest are common extended scalars:

| GraphQL scalar | Go type |
| --- | --- |
| `String`, `ID` | `string` |
| `Int` | `int` |
| `Float` | `float64` |
| `Boolean` | `bool` |
| `Int64`, `Int32` | `int64`, `int32` |
| `Uint`, `Uint64`, `Uint32` | `uint`, `uint64`, `uint32` |
| `Float64`, `Float32` | `float64`, `float32` |
| `Time` | `time.Time` |
| `JSON` | `json.RawMessage` |

Any other custom scalar with no binding becomes `any` (`Metadata = any` in [`sdk/scalars/scalars.go`](../../example-go-mockapi/sdk/scalars/scalars.go)) and decodes into whatever `encoding/json` builds: a `map[string]interface{}` for an object, `float64` for a number.

## Step 3 — Map your own scalars

Bind a scalar to any Go type with `bindings` in the config file you pass with `-c`. The value is the type's full import path and name. From [`cmd/generate/config.jsonc`](../../example-go-mockapi/cmd/generate/config.jsonc):

```jsonc
{
  // Default optional-field mode is pointers; these two fields use Opt[T]
  // (Some/Null) so the Go guides can show both input modes against one SDK.
  "inputs": {
    "overrides": {
      "UpdateTodoInput.priority": "opt",
      "UpdateTodoInput.text": "opt"
    }
  },
  "bindings": {
    "Time": {
      "model": "time.Time"
    },
    // Relay cursor — just an opaque string
    "Cursor": {
      "model": "string"
    },
    // ISO-8601 date-time — mapped to time.Time for rich handling
    "DateTime": {
      "model": "time.Time"
    }
    // JSON and Metadata are intentionally NOT mapped here. JSON is a
    // built-in mapping (json.RawMessage); Metadata has no built-in, so it
    // falls back to `any` in the generated Go SDK.
  }
}
```

(The `inputs` section is about input fields, not scalars; see [Arguments and inputs: Step 5](arguments-and-inputs.md#step-5--send-an-explicit-null-with-optt).) A third-party type works the same way: `"UUID": { "model": "github.com/google/uuid.UUID" }` imports `github.com/google/uuid` in the generated code; your module must then require that package. The bound type must decode from the JSON the server sends: `time.Time` expects an RFC 3339 string.

The example reads three kinds of scalar back. From [`cmd/enums-and-inputs/main.go`](../../example-go-mockapi/cmd/enums-and-inputs/main.go):

```go
// scalarTypes prints the Go type each kind of scalar decodes into:
// DateTime is bound to time.Time in cmd/generate/config.jsonc, JSON is a
// built-in mapping to json.RawMessage, and Metadata has no binding, so it
// falls back to `any` and arrives as whatever encoding/json builds.
func scalarTypes(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Scalar types ==")
	todo, err := qr.Todo().ID(seededTodoID).
		Select(func(f *fields.TodoFields) { f.CreatedAt().Metadata() }).
		Execute(ctx)
	if err != nil {
		return err
	}
	if todo == nil || todo.Metadata == nil {
		return fmt.Errorf("seeded todo %s or its metadata is missing", seededTodoID)
	}
	fmt.Fprintf(w, "createdAt: %T year=%d\n", todo.CreatedAt, todo.CreatedAt.Year())
	fmt.Fprintf(w, "metadata:  %T\n", *todo.Metadata)

	info, err := qr.ServerInfo().Execute(ctx)
	if err != nil {
		return err
	}
	// info is a json.RawMessage: the server's JSON, undecoded. (Printing %T
	// would show jsontext.Value on toolchains where RawMessage is an alias.)
	fmt.Fprintf(w, "serverInfo: raw JSON, valid=%t\n", json.Valid(info))
	return nil
}
```

## Step 4 — Know what "unselected" means in Go

In TypeScript the result type narrows to the fields you selected, so reading an unselected field is a compile error. **Go does not do this.** `Execute` always returns the full generated struct; fields you did not select are simply left at their zero value. Nothing stops you reading them:

```go
// unselectedFields shows what Go returns for fields you did not select: the
// full types.Todo struct, with zero values where nothing was requested.
// Nothing stops you reading them, so check your Select before trusting one.
func unselectedFields(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Unselected fields are zero values ==")
	todo, err := qr.Todo().ID(seededTodoID).
		Select(func(f *fields.TodoFields) { f.ID().Text() }).
		Execute(ctx)
	if err != nil {
		return err
	}
	if todo == nil {
		// todo(id:) returns a nullable Todo, so the result is a pointer and
		// nil means "not found", not an error.
		return fmt.Errorf("seeded todo %s not found", seededTodoID)
	}
	fmt.Fprintf(w, "selected:   id=%s text=%q\n", todo.ID, todo.Text)
	fmt.Fprintf(w, "unselected: tags=%v user=%v createdAt.IsZero=%t\n", todo.Tags, todo.User, todo.CreatedAt.IsZero())
	return nil
}
```

So a zero value can mean "the server said zero", "the server said null" (for a pointer: nil) or "you did not ask". Keep the code that reads a result next to the `Select` that fills it, and select every field you read.

## Step 5 — Recognise names and comments from the schema

- **Names are exported.** `Todo` stays `Todo`; lowercase names such as Hasura's `order_by` and `timestamptz` become `Order_by` and `Timestamptz`, and leading underscores are dropped (`_Service` becomes `Service`) so every type is reachable from your package. Enum values become `<Type><PascalCaseValue>`: `order_by` value `asc` is `enums.Order_byAsc`.
- **Names never collide.** When two schema names map to the same Go name, or a schema name matches a method the SDK defines itself (a field or argument named `select` next to the builder's `Select`), the later one gets a `_` suffix: `Select_`, then `Select__` ([`naming.go`](../../gqlkit/pkg/clientgen/naming.go)). The SDK's own method names never change.
- **Descriptions become doc comments.** A type, input, enum or interface description is the doc comment of that declaration, so `go doc` and editors show it; a field or enum-value description is written above that field or constant, as on `Parent` above. A query or mutation's description becomes its builder's doc comment (`NodeBuilder Relay-style lookup by typed ID ...` in [`query_node.go`](../../example-go-mockapi/sdk/queries/query_node.go)).

From a scratch schema with `"Sort order." enum order_by { "Ascending." asc desc }`, the generator emits:

```go
// Order_by Sort order.
type Order_by string

// Order_by enum values
const (
	// Ascending.
	Order_byAsc  Order_by = "asc"
	Order_byDesc Order_by = "desc"
)
```

## Try it

```bash
task mockapi:run                          # in another terminal
cd example-go-mockapi && go run ./cmd/enums-and-inputs
```

The last lines of the output are the scalar types, and the "Unselected" block shows Step 4:

```text
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

`go test ./cmd/enums-and-inputs` checks these lines.

## Pitfalls

- **Scalar descriptions are not emitted.** `scalars.go` is a block of type aliases with no comments, so a scalar's description (`DateTime`'s "ISO-8601 date-time string") is only in the schema.
- **`<Name>Value.Value` is nil for a member the SDK does not know**, and also for a `null` result. Check `Typename` to tell them apart, and handle the `default` case of every type switch; see [Selecting fields: Step 3](selecting-fields.md#step-3--query-a-field-that-returns-an-interface).
- **Upgrading changes interface and union types.** SDKs generated by older gqlkit versions used `types.Node` / `interface{}` for these, which could not be decoded; after regenerating, code that used them must switch to the `<Name>Value` wrappers.
- **`User *User` is a pointer even though `user: User!` is non-null.** Objects are always pointers (Step 1), so check for nil when you did not select the field, or when an error nulled it.
- **An unbound scalar is `any`.** Code that type-asserts it (`(*todo.Metadata).(map[string]interface{})`) breaks silently if the server changes the value's JSON shape. Bind the scalar to a concrete type if you depend on its shape.
- **Enum values that differ only in case or separators get a suffix.** `IN_PROGRESS` and `in_progress` would both be `<Type>InProgress`, so the one declared second becomes `<Type>InProgress_`. Which one gets the suffix depends on schema order, so compare against the string value when it matters.
- **The SDK requires Go 1.24 or newer.** Input encoding uses the `omitzero` tag option; on an older toolchain the build stops at a generated guard with `undefined: gqlkit_SDK_requires_Go_1_24_or_newer`. Set `go 1.24` or later in your `go.mod`.
- **The SDK is self-contained.** `sdk/builder`, `sdk/graphqlclient` and `sdk/batch` are generated into your tree, and the generated code imports nothing from gqlkit; your `go.mod` needs gqlkit only if you run the generator from Go code. Regenerate after upgrading gqlkit to pick up fixes in those packages, and pass `--prune` so files for types and operations the schema no longer has are deleted rather than left behind ([Generate a Go SDK: Step 5](../cli/generate-go.md#step-5-regenerate-with---prune)).

## Related

- [Arguments and inputs](arguments-and-inputs.md): the `inputs` and `enums` packages on the way in.
- [Selecting fields](selecting-fields.md): filling the struct.
- [Getting started — Go: Step 3](../getting-started-go.md#step-3-configure-scalar-type-mappings): the config file in the setup flow.
- Runnable example: [`example-go-mockapi/cmd/enums-and-inputs`](../../example-go-mockapi/cmd/enums-and-inputs).
