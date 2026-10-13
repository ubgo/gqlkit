# Choosing a scalar binding

Every custom scalar in your schema becomes some type in the generated SDK. You have four options, and they differ in type safety, in what your app must install, and in what the JSON on the wire has to look like. This page helps you pick one per scalar; [config.jsonc](../cli/config.md) shows how to write each.

## Before you start

List the custom scalars and check what the server actually sends for each one. A scalar named `DateTime` might be an ISO string, a Unix number, or an object; the binding must decode what arrives.

```sh
grep '^scalar' schema.graphql
```

## The options

| Option | Go config | TypeScript config | Result | Choose it when | Guide and example |
| --- | --- | --- | --- | --- | --- |
| Built-in mapping | nothing | nothing | `Int64` → `int64` / `number`; in Go also `Time` → `time.Time`, `JSON` → `json.RawMessage` | The scalar's name is in the [built-in table](../cli/config.md#built-in-mappings) and the default fits. | [Go Step 2](../go/types-and-scalars.md#step-2--know-which-scalars-are-mapped-for-you); `runServerInfo` in [Go samples](../../example-go-mockapi/cmd/samples/main.go) |
| Fall back to `any` | nothing | nothing | `Metadata = any` | You do not read the value, or its shape varies. You give up type checking for it. | [config.jsonc Step 1](../cli/config.md#step-1-check-the-built-in-mappings-first); `Metadata` in [`scalars.ts`](../../example-ts/cmd/samples/scalars.ts) |
| A simple type | `{ "model": "string" }` | `"string"` | `Cursor = string` | The value is a plain JSON string, number or boolean: cursors, IDs, emails, URLs, decimals sent as strings. | [TS Step 1](../typescript/types-scalars-enums.md#step-1--bind-custom-scalars); [`example-ts` config](../../example-ts/cmd/generate/config.jsonc) |
| A library or stdlib type | `{ "model": "time.Time" }` or `{ "model": "github.com/google/uuid.UUID" }` | `{ "type": "JsonValue", "import": "type-fest" }` | Go: an import of that package. TS: `export type { JsonValue as JSON } from "type-fest";` | You want a rich type, and it can decode the wire format. | [Go Step 3](../go/types-and-scalars.md#step-3--map-your-own-scalars), [TS Step 1](../typescript/types-scalars-enums.md#step-1--bind-custom-scalars); [Go config](../../example-go-mockapi/cmd/generate/config.jsonc), [TS config](../../example-ts/cmd/generate/config.jsonc) |
| A union type expression (TS only) | not available | `"string \| number"` | `export type X = string \| number;` | The server sends more than one JSON type for the scalar. | [config.jsonc Step 3](../cli/config.md#step-3-write-the-typescript-config); none |
| A type with a codec (TS only) | not available (Go decodes through the type's own `UnmarshalJSON`) | `{ "type": "DateTime", "import": "luxon", "decode": "parseDate", "encode": "formatDate", "codecImport": "./codecs.js" }` | Responses are decoded into the type and arguments encoded back, including inside lists, fragments and `batch()` results. | You want a real `DateTime` / `Date` object, not a string typed as one. | [TS types, Step 2](../typescript/types-scalars-enums.md#step-2--convert-values-with-a-codec); [gqlkit-ts README: codecs](../../gqlkit-ts/README.md#custom-scalar-codecs) |

## If X, then Y

- **If the value is an RFC 3339 date-time string**, Go: `time.Time` (its JSON decoding accepts RFC 3339). TypeScript: `string` and parse it where you use it, or bind it to luxon's `DateTime` with a codec so the SDK converts it for you. A plain `{ "type", "import" }` re-export without a codec describes the type only.
- **If the value is a Unix timestamp number**, do not bind it to `time.Time`; decoding a number into `time.Time` fails. Use `int64` / `number`.
- **If the value is arbitrary JSON**, Go's built-in `json.RawMessage` keeps it undecoded for you to unmarshal later. In TypeScript, bind it to a JSON type such as `type-fest`'s `JsonValue` rather than leaving `any`.
- **If you bind to a third-party Go type**, run `go get` for its module; the generated code imports it.
- **If you bind to an npm type**, add the package to your app's `dependencies`; the generated `scalars/index.ts` re-exports from it.
- **If the scalar is an ID-like string** (`Cursor`, `UUID` sent as text), `string` is the honest binding in TypeScript. In Go, `github.com/google/uuid.UUID` works for UUIDs because it decodes from a JSON string.

## Try it

[`example-cli/go.config.jsonc`](../../example-cli/go.config.jsonc) and [`example-cli/ts.config.jsonc`](../../example-cli/ts.config.jsonc) use the built-in, `any`, simple-type and library-type options against the test server's `DateTime`, `Cursor`, `JSON` and `Metadata` scalars. Run `./example-cli/test.sh` and look at `example-cli/out/go-app/sdk/scalars/scalars.go` and `example-cli/out/ts-app/sdk/scalars/index.ts`.

## Pitfalls

- **A binding alone is not a converter.** In Go, the JSON value is decoded straight into the bound type, so a type whose JSON decoding does not accept the wire value fails at run time with a decode error. In TypeScript, a plain binding changes only the type: unless you add a codec (`decode`, `encode`, `codecImport` in the binding), a `DateTime` bound to luxon still arrives as a string, and the type lies.
- **The same scalar may need different bindings in Go and TypeScript**, and the two config files use different formats. See [Go and TypeScript need different files](../cli/config.md#go-and-typescript-need-different-files).
- **`Time` and `JSON` are only built in for Go.** In TypeScript they are `any` until you bind them.

## Related

- [config.jsonc and scalar bindings](../cli/config.md)
- [Go types, scalars and enums](../go/types-and-scalars.md) and [TypeScript types, scalars and enums](../typescript/types-scalars-enums.md)
- [Go or TypeScript](./go-vs-typescript.md)
- [Feature reference: Configuration](../reference/features.md#configuration-configjsonc)
