# Recipes

End-to-end workflows, each one a complete path from nothing to working code. For flag-by-flag detail see the [CLI reference](./cli-reference.md); for step-by-step guides with pitfalls, see [`docs/cli/`](./cli/generate-go.md). Each recipe that uses the CLIs has a runnable, tested script in [`example-cli/`](../example-cli/README.md).

- [Generate a Go SDK for an API you do not control](#generate-a-go-sdk-for-an-api-you-do-not-control)
- [Trim a huge vendor schema down to what you use](#trim-a-huge-vendor-schema-down-to-what-you-use)
- [Generate both a Go and a TypeScript client from one schema](#generate-both-a-go-and-a-typescript-client-from-one-schema)
- [Keep the SDK in sync as the schema changes](#keep-the-sdk-in-sync-as-the-schema-changes)
- [Assemble a schema from several SDL files](#assemble-a-schema-from-several-sdl-files)
- [Batch several queries into one request](#batch-several-queries-into-one-request)
- [Debug a failing introspection fetch](#debug-a-failing-introspection-fetch)

## Generate a Go SDK for an API you do not control

The common case: a third-party GraphQL endpoint, an auth token, and you want typed access from Go.

```bash
go install github.com/ubgo/gqlkit/gqlkit-sdl@latest
go install github.com/ubgo/gqlkit/gqlkit/cmd/gqlkit@latest

gqlkit-sdl fetch \
  --url https://api.example.com/graphql \
  -H "Authorization: Bearer $TOKEN" \
  -o schema.graphql

gqlkit generate -s schema.graphql -o ./sdk -m "$(go list -m)/sdk"
```

`-m` is the import path of the SDK directory, which the generated packages use to import each other: `go list -m` prints your module path, and `/sdk` matches `-o ./sdk`. Passing only `"$(go list -m)"` makes the SDK import `<module>/types` instead of `<module>/sdk/types`, and it will not compile.

Then build. The SDK is self-contained, so there is nothing to `go get`:

```bash
go build ./...
```

Runnable version: [`example-cli/generate-go.sh`](../example-cli/generate-go.sh). Guide: [Generate a Go SDK](./cli/generate-go.md).

## Trim a huge vendor schema down to what you use

Generating an SDK for a full vendor schema produces thousands of types and a package that is slow to compile and impossible to navigate. Keep only the operations you call, then prune everything unreachable:

```bash
gqlkit-sdl fetch \
  --url https://your-shop.myshopify.com/admin/api/2024-10/graphql.json \
  -H "X-Shopify-Access-Token: $SHOPIFY_TOKEN" \
  --only-queries "products,product,orders,order" \
  --only-mutations "productUpdate,orderUpdate" \
  --remove-unused \
  -o shopify.graphql

gqlkit generate -s shopify.graphql -o ./shopify -m "$(go list -m)/shopify"
```

Filters accept exact names or regexes — any value containing regex metacharacters is treated as one, anchored at both ends. Avoid commas inside a regex (the flags split on them), and check the output still has the operations you expect: filters that remove every query field are an error, and filters that remove every mutation drop the `Mutation` type with only a note on stderr. The inverse form is often shorter when you want most of an API:

```bash
gqlkit-sdl fetch --url https://example.com/graphql \
  --exclude-queries "admin.*,internal.*" \
  --exclude-mutations "admin.*,internal.*" \
  --remove-unused
```

`--remove-unused` is the flag that does the real work. Without it the filtered schema still declares every type the removed operations referenced.

Runnable version against the local test server: [`example-cli/fetch-and-filter.sh`](../example-cli/fetch-and-filter.sh). Guide: [Filter a schema](./cli/filter-schema.md).

## Generate both a Go and a TypeScript client from one schema

One schema, two runtimes, no second source of truth:

```bash
gqlkit-sdl fetch --url https://api.example.com/graphql -o schema.graphql

gqlkit generate    -s schema.graphql -o ./backend/sdk -m github.com/you/backend/sdk -c go.config.jsonc
gqlkit generate-ts -s schema.graphql -o ./frontend/src/sdk -c ts.config.jsonc
```

The TypeScript output needs the npm runtime:

```bash
cd frontend && npm install gqlkit-ts
```

Keep one config file per language: the Go and TypeScript binding formats are different ([config.jsonc](./cli/config.md#go-and-typescript-need-different-files)). The guarantees also differ: in TypeScript a field you did not select is a compile error, while in Go the result is the full struct and an unselected field is just a zero value. See [Go or TypeScript](./choosing/go-vs-typescript.md).

Runnable versions: [`example-cli/generate-go.sh`](../example-cli/generate-go.sh) and [`example-cli/generate-ts.sh`](../example-cli/generate-ts.sh).

## Keep the SDK in sync as the schema changes

Generated code is build output. Treat regeneration as a task, not something people remember to do:

```yaml
# Taskfile.yml
version: "3"

tasks:
  schema:fetch:
    desc: Re-fetch the API schema
    cmds:
      - gqlkit-sdl fetch --url {{.API_URL}} -H "Authorization: Bearer {{.TOKEN}}" -o schema.graphql

  sdk:generate:
    desc: Regenerate the typed SDK from schema.graphql
    cmds:
      - gqlkit generate -s schema.graphql -o ./sdk -m $(go list -m)/sdk --prune
      - go build ./...

  sdk:refresh:
    desc: Fetch the schema and regenerate
    cmds:
      - task: schema:fetch
      - task: sdk:generate
```

`--prune` matters: without it the generator overwrites files but never deletes them, so an operation removed from the schema would leave a stale builder behind that no longer compiles. `--prune` deletes only files that start with the gqlkit generated-code banner and were not written by this run, so hand-written files next to the SDK are safe; add `--dry-run` to see the list first.

Commit `schema.graphql` so a schema change shows up as a reviewable diff, and regenerate in CI to catch a drifted SDK before it reaches anyone. What does and does not need a regeneration: [When to regenerate](./choosing/when-to-regenerate.md).

## Assemble a schema from several SDL files

`--schema` takes a glob, so a schema split across files needs no concatenation step:

```bash
gqlkit generate -s './schema/*.graphql' -o ./sdk -m "$(go list -m)/sdk"
```

Quote the glob so the shell passes the pattern through rather than expanding it into several arguments; unquoted, gqlkit stops with `unexpected argument(s)` and a hint to quote it. `**` works too (`'./schema/**/*.graphql'`), and one file may `extend type Query` from another. A pattern that matches nothing fails with `did not match any files` and exit code 1, which is what you want in CI.

Runnable version: [`example-cli/multi-file-schema.sh`](../example-cli/multi-file-schema.sh).

## Batch several queries into one request

Independent queries do not need a round trip each. The generated `batch` package sends them as one document with one alias per query:

```go
var r struct {
    Users []types.User `json:"users"`
    Todos []types.Todo `json:"todos"`
}
err := batch.RunQueries(ctx, &r, batch.QueryItems{
    "users": qr.Users().Select(func(u *fields.UserFields) { u.ID().Name() }),
    "todos": qr.Todos().Select(func(t *fields.TodoFields) { t.ID().Text() }),
})
```

Each alias is decoded into the field of `r` whose `json` tag matches it. Errors come back per alias: a failed `todos` surfaces with `Path: ["todos", ...]`, while `r.Users` is still filled in, and `errors.As` against `*batch.Error` (or `graphqlclient.GraphQLErrors`) tells you which part failed.

Mutations batch the same way with `batch.RunMutations`; putting a mutation builder in `RunQueries` does not compile. In TypeScript, `batch(client, { users: ..., todos: ... })` from `gqlkit-ts` does the same with a result typed per alias. The full sample is `runBatch` in [example-go-mockapi](../example-go-mockapi/cmd/samples/main.go); when batching is worth it: [One request or several](./choosing/single-vs-batch.md).

## Debug a failing introspection fetch

When `fetch` fails, the first question is whether it is your token, your header syntax, or the endpoint. `--debug` prints the equivalent `curl` instead of sending anything, and you can run it directly:

```bash
gqlkit-sdl fetch --url https://api.example.com/graphql -H "Authorization: Bearer $TOKEN" --debug
```

Two things that account for most failures:

- **Header syntax.** `-H` expects `Key:Value`. It is repeatable, so multiple headers are multiple flags, not one comma-separated value.
- **Introspection disabled.** Many production endpoints turn it off. There is no client-side workaround: get the SDL from the API owner, or point `--url` at a staging endpoint that still allows introspection.

To inspect the raw introspection result rather than SDL, ask for JSON:

```bash
gqlkit-sdl fetch --url https://api.example.com/graphql -f json -o schema.json
```

`gqlkit generate` cannot read that JSON file; it reads SDL only. Runnable version of `--debug` and `-f json`: [`example-cli/fetch-and-filter.sh`](../example-cli/fetch-and-filter.sh). The full error table is in [Fetch a schema](./cli/fetch-schema.md#how-a-fetch-fails).
