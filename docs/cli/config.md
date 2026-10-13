# config.jsonc and scalar bindings

A GraphQL schema can declare custom scalars (`scalar DateTime`, `scalar Cursor`) without saying what they look like on the wire. The config file tells gqlkit which language type to use for each one, in a top-level `bindings` object. It also holds two per-language options: how the Go SDK represents nullable input fields (`inputs`), and how the TypeScript SDK types nullable output fields (`nullable`, `nullableOverrides`). You need it when the generated SDK shows `any` where you want a real type, or when the defaults for null handling do not fit.

Pass it with `-c` (`--config`) to `gqlkit generate` or `gqlkit generate-ts`, or as `ConfigPath` when [running the generators from Go](./programmatic.md). The two runnable example files are [`example-cli/go.config.jsonc`](../../example-cli/go.config.jsonc) and [`example-cli/ts.config.jsonc`](../../example-cli/ts.config.jsonc).

## Before you start

- A schema with at least one custom scalar. List them with `grep '^scalar' schema.graphql`.
- To pick a binding style, see [Choosing a scalar binding](../choosing/scalar-bindings.md).

## Step 1: Check the built-in mappings first

Some scalars need no config at all. These come from [`gqlkit/pkg/typegql/typegql.go`](../../gqlkit/pkg/typegql/typegql.go) (Go) and [`gqlkit/pkg/clientgents/ts_type_map.go`](../../gqlkit/pkg/clientgents/ts_type_map.go) (TypeScript).

### Built-in mappings

| GraphQL scalar | Go type | TypeScript type |
| --- | --- | --- |
| `String`, `ID` | `string` | `string` |
| `Int` | `int` | `number` |
| `Int64`, `Int32` | `int64`, `int32` | `number` |
| `Float`, `Float64` | `float64` | `number` |
| `Float32` | `float32` | `number` |
| `Uint`, `Uint64`, `Uint32` | `uint`, `uint64`, `uint32` | `number` |
| `Boolean` | `bool` | `boolean` |
| `Time` | `time.Time` | `any` (no built-in) |
| `JSON` | `json.RawMessage` | `any` (no built-in) |
| any other scalar | `any` | `any` |

A binding in the config file replaces the built-in mapping for that scalar.

## Step 2: Write the Go config

Each value is an object with a `model` key holding a Go type. A type outside the universe scope is written as `<import path>.<TypeName>`: `time.Time`, `encoding/json.RawMessage`, `github.com/google/uuid.UUID`. The generator splits on the last dot, imports the path and uses the last path segment as the package name.

```jsonc
// Scalar bindings for `gqlkit generate` (the Go SDK).
// Each value is an object with a "model": a Go type, written as
// <import path>.<TypeName> for anything outside the universe scope.
{
  "bindings": {
    // A standard-library type: imported as "time" in the generated code.
    "DateTime": { "model": "time.Time" },
    // A Go builtin: no import.
    "Cursor": { "model": "string" }
    // Not listed, so the built-in mapping applies: JSON -> json.RawMessage.
    // Not listed and not built in, so it falls back to any: Metadata.
  }
}
```

The result in `sdk/scalars/scalars.go`, for the mockapi schema:

```go
	Cursor = string
	DateTime = time.Time
	JSON = json.RawMessage
	Metadata = any
```

These are type aliases, so a `scalars.DateTime` is a `time.Time` and needs no conversion.

## Step 3: Write the TypeScript config

Each value is either a string (an inline type alias) or an object. The object needs a `type`, and can add `import` (re-export the type from an npm package or a path) and a runtime codec (`decode`, `encode`, `codecImport`, see [TypeScript codecs](#typescript-codecs)).

```jsonc
// Scalar bindings for `gqlkit generate-ts` (the TypeScript SDK).
// Each value is either a TypeScript type as a string, or an object with a
// "type" and optionally an npm "import" and a runtime codec. This is NOT the
// Go format: a {"model": ...} object here is rejected.
{
  "bindings": {
    // A codec: the SDK calls decodeDate on every DateTime in a response and
    // encodeDate on every DateTime it sends, so app code sees real Date
    // objects. codecImport is resolved from the generated sdk/scalars/
    // directory; generate-ts.sh writes codecs.ts next to sdk/.
    "DateTime": {
      "type": "Date",
      "decode": "decodeDate",
      "encode": "encodeDate",
      "codecImport": "../../codecs.js"
    },
    // An inline type alias.
    "Cursor": "string",
    // Re-exported from an npm package: the app must depend on type-fest.
    "JSON": { "type": "JsonValue", "import": "type-fest" }
    // Not listed and not built in, so it falls back to any: Metadata.
  }
}
```

The result in `sdk/scalars/index.ts`:

```ts
export type Boolean = boolean;
export type Cursor = string;
export type DateTime = Date;
export { decodeDate as decodeDateTime, encodeDate as encodeDateTime } from "../../codecs.js";
export type Float = number;
export type GqlString = string;
export type ID = string;
export type Int = number;
export type { JsonValue as JSON } from "type-fest";
export type Metadata = any;
```

A string value, and the `type` of an object, is copied into the generated code as a TypeScript type expression, so it must be valid TypeScript.

### TypeScript codecs

A binding with only a `type` changes the declared type, not the value: the SDK still hands you the JSON the server sent. A codec converts it. `decode` names a function that turns each wire value of the scalar in a response into the bound type; `encode` names one that turns each value you send (an argument or an input field) back into its wire form. Either may be set alone. Both are exports of `codecImport`, a module path resolved like `import`: a relative path is relative to the generated `sdk/scalars/` directory, and should end in `.js` like the SDK's own imports. `null` never reaches either function.

With the example config above, `fields/todo.ts` registers the decoder on the field (`this.#selection.addField("createdAt", decodeDateTime, 0)`), `t.createdAt` is a `Date` at run time, and [`generate-ts.sh`](../../example-cli/generate-ts.sh) prints it with `toISOString()`. The codec module the example writes is shown in [Generate a TypeScript SDK](./generate-ts.md#step-3-write-the-codec-module-if-you-bind-one). A fuller codec, luxon's `DateTime`, is in [`example-ts/codecs/datetime.ts`](../../example-ts/codecs/datetime.ts).

Rules checked at generation time: a codec needs `codecImport`, a `codecImport` needs a codec, and `decode`/`encode` must be plain identifiers.

## Step 4: Choose how nullable inputs look in Go (optional)

By default a nullable field of an input type is a pointer with `omitempty`: `nil` leaves the field out of the request, and there is no way to send an explicit `null`. The `inputs` section switches fields to `inputs.Opt[T]`, which has three states: unset (omitted), `inputs.Null[T]()` (sent as `null`) and `inputs.Some(v)` (sent as `v`).

```jsonc
{
  "inputs": {
    "optional": "opt",
    "overrides": { "TodoFilter": "pointer" }
  }
}
```

`optional` sets the default for every nullable input field (`"pointer"` or `"opt"`). `overrides` takes GraphQL names, either a type (`"TodoFilter"`) or one field (`"UpdateTodoInput.tags"`); a field key wins over a type key, which wins over `optional`. Against the mockapi schema that gives:

```go
type TodoFilter struct {
	TextContains *string `json:"textContains,omitempty"`
	Done         *bool   `json:"done,omitempty"`
	MinPriority  *int    `json:"minPriority,omitempty"`
	HasTag       *string `json:"hasTag,omitempty"`
}
```

```go
type UpdateTodoInput struct {
	Text     Opt[string]   `json:"text,omitzero"`
	Done     Opt[bool]     `json:"done,omitzero"`
	Priority Opt[int]      `json:"priority,omitzero"`
	Tags     Opt[[]string] `json:"tags,omitzero"`
}
```

and `inputs.UpdateTodoInput{Text: inputs.Some("rename"), Priority: inputs.Null[int]()}` marshals to `{"text":"rename","priority":null}`. `Opt` relies on encoding/json's `omitzero`, so code using `"opt"` needs a Go 1.24 or newer toolchain; `opt.go` carries a `go1.24` build constraint, so an older toolchain fails with `undefined: Opt` instead of silently sending `null` for unset fields. Operation arguments are not affected: they already have three states (setter not called, called with `nil`, called with a value).

Errors are reported before anything is written, for example:

```
Error: failed to create generator: invalid inputs config: inputs.overrides["NewTodo.text"]: field is non-null, so it has no optional representation
Error: failed to create generator: invalid inputs config: inputs.optional "maybe" (valid: [pointer opt])
```

## Step 5: Choose how nullable outputs look in TypeScript (optional)

By default a nullable output field is typed `T | null`, which is exactly what the server sends. `"nullable": "null-undefined"` widens it to `T | null | undefined`, for code written against older SDKs that compared fields with `=== undefined`. `nullableOverrides` sets the mode for one type (`"Todo"`) or one field (`"Todo.priority"`, `"Query.todo"`):

```jsonc
{
  "nullable": "null-undefined",
  "nullableOverrides": { "Todo.priority": "null" },
  "bindings": { "DateTime": "string" }
}
```

```ts
export interface Todo {
  id: string;
  text: string;
  done: boolean;
  priority?: number | null;
  tags: string[];
  user: User;
  createdAt: string;
  metadata?: Metadata | null | undefined;
  parent?: Todo | null | undefined;
}
```

The mode applies to object and interface fields and to operation results; list elements stay `| null`, and inputs and arguments are unaffected. An override key that names no type or field fails generation: `"nullableOverrides" "Todo.nope": unknown type or field: Todo has no field "nope"`.

## Try it

Both example scripts pass the two config files to the CLI, and [`example-cli/test.sh`](../../example-cli/test.sh) checks the result (`DateTime = time.Time`, `Metadata = any`, the decoded dates, the `type-fest` re-export):

```sh
./example-cli/test.sh
```

```
== generate-go.sh
  ok    generate-go.sh exited 0
  ok    Go program printed ping
  ok    Go program printed todos
  ok    go.mod has no gqlkit requirement
  ok    DateTime bound to time.Time
  ok    Metadata fell back to any
  ok    --prune reported the stale file
  ok    --prune deleted the stale file
  ok    --prune kept the hand-written file
== generate-ts.sh
  ok    generate-ts.sh exited 0
  ok    TS program printed ping
  ok    TS program printed todos with decoded dates
  ok    JSON re-exported from type-fest
  ok    reading an unselected field fails tsc with TS2339
  ok    DateTime codec re-exported
  ok    SDK uses .js relative imports
  ok    SDK type-checks as Node ESM (nodenext)
```

The `inputs` and `nullable` outputs above were generated from the same mockapi schema with the configs shown; they are not part of `test.sh`.

## Pitfalls

### Go and TypeScript need different files

The two generators read the same `bindings` key with different value shapes, and each rejects the other's. A TypeScript config passed to `gqlkit generate`:

```
Error: failed to create generator: failed to load client config: failed to unmarshal config: json: cannot unmarshal string into Go struct field ClientConfig.bindings.Cursor of type typegql.TypeMapEntry
```

A Go config passed to `gqlkit generate-ts`:

```
Error: failed to create TypeScript generator: failed to load client config: invalid config ../../go.config.jsonc: binding "Cursor": missing TypeScript "type"; write "Cursor": "string" or "Cursor": { "type": "DateTime", "import": "luxon" } (a {"model": ...} binding is the Go generator's form)
```

`inputs` is read only by the Go generator and `nullable`/`nullableOverrides` only by the TypeScript one. Keep one file per language.

### Other traps

- **A missing `-c` file is an error** (`config file nope.jsonc: stat nope.jsonc: no such file or directory`). Leave `-c` out if you have no config.
- **Comments and trailing commas are fine.** The file is JSONC: `//` and `/* */` comments and trailing commas are accepted.
- **A third-party Go type needs `go get`.** Binding `UUID` to `github.com/google/uuid.UUID` generates `import "github.com/google/uuid"`; `go build` then fails with `no required module provides package github.com/google/uuid` until you add it to `go.mod`.
- **An empty `model` means `any`.** `{ "model": "" }` is treated like no binding in Go. In TypeScript an object without `type` is an error.
- **Unknown top-level keys are ignored.** A misspelled `"nullabel"` or `"input"` does nothing and prints no warning; check the generated output after adding an option.

## Related

- [Choosing a scalar binding](../choosing/scalar-bindings.md)
- [Generate a Go SDK](./generate-go.md)
- [Generate a TypeScript SDK](./generate-ts.md)
- [Feature reference: Configuration](../reference/features.md#configuration-configjsonc)
