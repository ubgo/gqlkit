# Generate a TypeScript SDK from the command line

`gqlkit generate-ts` reads a GraphQL schema in SDL and writes a typed TypeScript client SDK. Each builder's `.select()` narrows the result type, so reading a field you did not select is a compile error. The generated files import their runtime (`GraphQLClient`, `BaseBuilder`, `FieldSelection`) from the [`gqlkit-ts`](https://www.npmjs.com/package/gqlkit-ts) npm package. Use it for a browser, Node or edge client of a GraphQL API whose schema you have or can fetch.

The runnable version of this page is [`example-cli/generate-ts.sh`](../../example-cli/generate-ts.sh): it generates the SDK, type-checks a small program against it, compiles it and runs it against the local test server.

## Before you start

- The `gqlkit` binary (`go install github.com/ubgo/gqlkit/gqlkit/cmd/gqlkit@latest`, or see the [README install section](../../README.md#install)).
- A TypeScript project with `gqlkit-ts` installed: `npm install gqlkit-ts`.
- A schema in SDL. Fetch one with [`gqlkit-sdl fetch`](./fetch-schema.md) if you only have an endpoint.

## Step 1: Get the schema

```sh
gqlkit-sdl fetch --url "$GRAPHQL_URL" -o schema.graphql
```

## Step 2: Generate

```sh
gqlkit generate-ts \
	-s schema.graphql \
	-o ./sdk \
	-c "$EXAMPLE_DIR/ts.config.jsonc"
```

There is no `-m` or `-p`: the generated files import each other with relative paths. The example's [`ts.config.jsonc`](../../example-cli/ts.config.jsonc) decodes `DateTime` into a `Date` with a codec, binds `Cursor` to `string` and re-exports `JSON` from `type-fest`; in your project, `-c config.jsonc` or no `-c` at all.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-s`, `--schema` | none, required | SDL file or quoted glob. An unquoted glob is an error. |
| `-o`, `--output` | `./sdk` | Directory to write. Existing files are overwritten; nothing is deleted unless you pass `--prune`. |
| `-c`, `--config` | empty | `config.jsonc` with TypeScript bindings and nullability options. See [config.jsonc](./config.md). A path that does not exist is an error. |
| `--prune` | off | Delete files in `-o` that carry the gqlkit banner but were not written by this run, and remove directories that left empty. Hand-written files are never touched. Works as in [the Go guide](./generate-go.md#step-5-regenerate-with---prune). |
| `--dry-run` | off | With `--prune` only: print `Would prune: ...` instead of deleting. |

## Step 3: Write the codec module, if you bind one

The example binds `DateTime` to a runtime codec, so it needs a module exporting the two functions. `codecImport` in the config is resolved from the generated `sdk/scalars/` directory, so `"../../codecs.js"` points at `codecs.ts` next to `sdk/`:

```ts
// Wire <-> app conversion for the DateTime scalar. The server sends
// ISO-8601 strings; app code works with Date objects.
export function decodeDate(raw: string): Date {
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) throw new RangeError(`DateTime: not a date: ${raw}`);
  return d;
}

export function encodeDate(value: Date): string {
  return value.toISOString();
}
```

A binding without a codec needs no module. See [config.jsonc: codecs](./config.md#typescript-codecs).

## Step 4: Use it

Import the roots from the `queries` and `mutations` directories; there is no `index.ts` at the SDK root. The generated files use `.js` suffixes on their relative imports, so the SDK compiles in CommonJS projects, with bundlers, and in Node ESM projects with `"moduleResolution": "nodenext"`. Use `./sdk/queries/index.js` in your own imports too if your project is Node ESM.

```ts
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries/index.js";

async function main() {
  const client = new GraphQLClient(process.env.GRAPHQL_URL ?? "");
  const qr = new QueryRoot(client);

  console.log("ping:", await qr.ping().execute());

  const todos = await qr
    .todos()
    .pagination({ limit: 2, offset: 0 })
    .select((t) => t.id().text().createdAt())
    .execute();

  for (const t of todos) {
    // createdAt is a Date: the DateTime codec decoded the wire string.
    // t.done would be a compile error here: it was not selected.
    console.log(`todo ${t.id}: ${JSON.stringify(t.text)} created ${t.createdAt.toISOString().slice(0, 10)}`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
```

## Try it

With the test server running (`task mockapi:run`) and the example's npm packages installed (`cd example-cli && npm install`):

```sh
./example-cli/generate-ts.sh
```

Real output:

```
Fetching schema from http://localhost:8081/query...
Converting to SDL format...
Saving to schema.graphql...
Done! Schema saved successfully.
Generating TypeScript SDK from schema.graphql
Output directory: ./sdk
Generated: builder/index.ts
Generated: scalars/index.ts
Generated: enums/index.ts
Generated: types/index.ts
Generated: inputs/index.ts
Generated: fields/*.ts
Generated: queries/ and mutations/
TypeScript SDK generated successfully in ./sdk
ping: pong from gqlgenapi
todo 1: "Try the ping query" created 2025-01-15
todo 2: "Create your first todo" created 2025-01-16
```

And the narrowing guarantee, checked by [`example-cli/test.sh`](../../example-cli/test.sh): adding `console.log(t.done)` to the loop fails `tsc` with

```
out/ts-app/unselected.ts(16,42): error TS2339: Property 'done' does not exist on type '{ id: string; } & { text: string; } & { createdAt: Date; }'.
```

The files written for the mockapi schema: `builder/index.ts`, `enums/index.ts`, `inputs/index.ts`, `scalars/index.ts`, `types/index.ts`, one file per object, interface and union type under `fields/` (kebab-case, e.g. `fields/todo-connection.ts`, `fields/search-result.ts`), and one file per operation plus `root.ts` and `index.ts` under `queries/` and `mutations/`. Every file starts with the `// Code generated by gqlkit. DO NOT EDIT.` banner.

`test.sh` also type-checks the same sources as a Node ESM package (`"type": "module"` with [`tsconfig.nodenext.json`](../../example-cli/tsconfig.nodenext.json)).

## Pitfalls

- **A Go-style config is rejected.** `{ "DateTime": { "model": "time.Time" } }` is the Go generator's format. `generate-ts` stops with `binding "DateTime": missing TypeScript "type"; write "DateTime": "string" or "DateTime": { "type": "DateTime", "import": "luxon" } (a {"model": ...} binding is the Go generator's form)`. Keep separate config files for Go and TypeScript.
- **An import binding needs the package.** `{ "type": "JsonValue", "import": "type-fest" }` writes `export type { JsonValue as JSON } from "type-fest";`, so your project must depend on `type-fest`.
- **A type binding is not a conversion.** Without `decode`, a binding only changes the declared type; the value is still the JSON the server sent. Binding `DateTime` to luxon's `DateTime` with no codec gives you a string typed as a `DateTime`. Add a codec when the runtime value must change.
- **`Time` and `JSON` become `any` without a binding.** The Go generator maps them by default; the TypeScript one does not (see [built-in mappings](./config.md#built-in-mappings)).
- **Your own imports under nodenext need `.js`.** The SDK's internal imports are fine; `import { QueryRoot } from "./sdk/queries"` in your code fails under `nodenext` with `TS2834`. Write `./sdk/queries/index.js`.
- **Stale files after a schema change.** Without `--prune` nothing is deleted. Regenerate with `--prune`.

## Related

- [config.jsonc and scalar bindings](./config.md)
- [Generate a Go SDK](./generate-go.md)
- [CLI reference: gqlkit generate-ts](../cli-reference.md#gqlkit-generate-ts)
- [Getting started — TypeScript](../getting-started-typescript.md)
- [Go or TypeScript](../choosing/go-vs-typescript.md)
