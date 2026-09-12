# Recipes

End-to-end workflows, each one a complete path from nothing to working code. For flag-by-flag detail see the [CLI reference](./cli-reference.md).

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
go install github.com/khanakia/gqlkit/gqlkit-sdl@latest
go install github.com/khanakia/gqlkit/gqlkit/cmd/gqlkit@latest

gqlkit-sdl fetch \
  --url https://api.example.com/graphql \
  -H "Authorization: Bearer $TOKEN" \
  -o schema.graphql

gqlkit generate -s schema.graphql -o ./sdk -m "$(go list -m)"
```

`go list -m` prints your module path, which is what the generated packages use to import each other — passing it this way means you cannot get `--module` wrong.

Then add the runtime and build:

```bash
go get github.com/khanakia/gqlkit/gqlkit
go build ./...
```

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

gqlkit generate -s shopify.graphql -o ./shopify -m "$(go list -m)"
```

Filters accept exact names or regexes — any value containing regex metacharacters is treated as one. The inverse form is often shorter when you want most of an API:

```bash
gqlkit-sdl fetch --url https://example.com/graphql \
  --exclude-queries "admin.*,internal.*" \
  --exclude-mutations "admin.*,internal.*" \
  --remove-unused
```

`--remove-unused` is the flag that does the real work. Without it the filtered schema still declares every type the removed operations referenced.

## Generate both a Go and a TypeScript client from one schema

One schema, two runtimes, no second source of truth:

```bash
gqlkit-sdl fetch --url https://api.example.com/graphql -o schema.graphql

gqlkit generate    -s schema.graphql -o ./backend/sdk -m github.com/you/backend
gqlkit generate-ts -s schema.graphql -o ./frontend/src/sdk
```

The TypeScript output needs the npm runtime:

```bash
cd frontend && npm install gqlkit-ts
```

Field selection narrows the return type in both languages, so a field the frontend did not select is a compile error there, exactly as it is in Go.

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
      - gqlkit generate -s schema.graphql -o ./sdk -m $(go list -m)
      - go build ./...

  sdk:refresh:
    desc: Fetch the schema and regenerate
    cmds:
      - task: schema:fetch
      - task: sdk:generate
```

Commit `schema.graphql` so a schema change shows up as a reviewable diff, and regenerate in CI to catch a drifted SDK before it reaches anyone.

## Assemble a schema from several SDL files

`--schema` takes a glob, so a schema split across files needs no concatenation step:

```bash
gqlkit generate -s './schema/*.graphql' -o ./sdk -m "$(go list -m)"
```

Quote the glob so the shell passes the pattern through rather than expanding it into several arguments. A pattern that matches nothing fails with `did not match any files` and exit code 1, which is what you want in CI.

## Batch several queries into one request

Independent queries do not need a round trip each. The generated `batch` package sends them as one document with one alias per query:

```go
import "yourmodule/sdk/batch"

results, err := batch.RunQueries(ctx, client, map[string]batch.QueryBatchable{
    "viewer": qr.Viewer().Select(func(u *fields.UserFields) { u.ID().Name() }),
    "todos":  qr.Todos().Select(func(t *fields.TodoFields) { t.ID().Text() }),
})
```

Errors come back per alias: a failed `todos` surfaces with `Path: ["todos", ...]`, so a partial failure tells you which query failed rather than failing the whole batch opaquely. `errors.As` against `graphqlclient.GraphQLErrors` works on a batch error too, so existing error handling keeps working.

Queries only — never mix a mutation into a batch.

## Debug a failing introspection fetch

When `fetch` fails, the first question is whether it is your token, your header syntax, or the endpoint. `--debug` prints the equivalent `curl`, which you can run directly:

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
