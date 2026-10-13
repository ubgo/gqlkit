# example-cli — the gqlkit CLIs, end to end

Runnable shell scripts for every command-line workflow: fetch a schema with `gqlkit-sdl`, filter it, generate a Go SDK and a TypeScript SDK with `gqlkit`, and prove each result builds and runs. The other examples in this repo call the generators from Go code; this one uses the binaries exactly as a user would.

Each script builds `gqlkit` and `gqlkit-sdl` from this repository's source (never a downloaded release) into `out/bin`, puts them first on `PATH`, and writes everything under `out/`, which is ignored by git. Nothing outside `example-cli/` is touched.

## What each script shows

| Script | Workflow | Needs a server | Guide |
| --- | --- | --- | --- |
| [`fetch-and-filter.sh`](./fetch-and-filter.sh) | `gqlkit-sdl fetch`: SDL, JSON, `--only-*` with `--remove-unused`, regex `--exclude-*`, `-H` headers with `--debug` | yes | [Fetch](../docs/cli/fetch-schema.md), [Filter](../docs/cli/filter-schema.md) |
| [`generate-go.sh`](./generate-go.sh) | `gqlkit generate` into a fresh module with [`go.config.jsonc`](./go.config.jsonc); `go vet`, `go build`, then a program that calls the API | yes | [Generate Go](../docs/cli/generate-go.md) |
| [`generate-ts.sh`](./generate-ts.sh) | `gqlkit generate-ts` with [`ts.config.jsonc`](./ts.config.jsonc) (a `DateTime` codec that decodes to `Date`, a string binding, an npm-import binding); `tsc --noEmit`, compile, run | yes | [Generate TypeScript](../docs/cli/generate-ts.md) |
| [`multi-file-schema.sh`](./multi-file-schema.sh) | `gqlkit generate` with a glob over two SDL files that use `extend type Query` | no | [Recipe](../docs/recipes.md#assemble-a-schema-from-several-sdl-files) |
| [`programmatic/`](./programmatic/README.md) | The same pipeline from Go code: `schema.FetchSchema` + `FilterSchema`, then `clientgen.New` and `clientgents.New`; a Go test runs both programs (`go test -count=1 ./...` inside it) | yes | [Run the generators from Go code](../docs/cli/programmatic.md) |
| [`test.sh`](./test.sh) | Runs all of the above and checks the outputs; also `--prune` (stale generated file removed, hand-written file kept), a Node ESM (`nodenext`) type-check of the TypeScript app with [`tsconfig.nodenext.json`](./tsconfig.nodenext.json), and the failure modes | yes | [Regenerate with --prune](../docs/cli/generate-go.md#step-5-regenerate-with---prune) |

## Run it

From the repository root, start the test GraphQL server in its own terminal:

```sh
task mockapi:run
```

The TypeScript script needs the local `gqlkit-ts` build and this folder's npm packages, once:

```sh
task gqlkit-ts:setup
cd example-cli && npm install
```

Then run one script, or all of them with their checks:

```sh
./example-cli/generate-go.sh
./example-cli/test.sh
```

Real output of `test.sh`:

```
== fetch-and-filter.sh
  ok    fetch-and-filter.sh exited 0
  ok    full.graphql has the Query type
  ok    full.json is an introspection result
  ok    trimmed keeps todo
  ok    trimmed drops ping
  ok    trimmed prunes TodoConnection
  ok    no-todos keeps ping
  ok    no-todos drops todos
  ok    no-todos drops createTodo
  ok    --debug printed a curl command
  ok    --debug carried the -H header
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
== multi-file-schema.sh
  ok    multi-file-schema.sh exited 0
  ok    query from user.graphql generated
  ok    query from post.graphql generated
== failure modes
  ok    a header without a colon is rejected
  ok    a missing schema file is rejected
  ok    an unquoted glob (extra arguments) is rejected
  ok    a missing -c file is rejected
  ok    --dry-run without --prune is rejected
  ok    an introspection JSON file is not accepted as --schema
  ok    scripts refuse to run with no server

example-cli: all checks passed
```

Each script's full output is kept in `out/logs/<script>.log`. With the server stopped, every script that needs it stops at once:

```
error: no GraphQL server answered at http://localhost:8081/query
       start the repo's test server first: task mockapi:run
```

## Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `GRAPHQL_URL` | `http://localhost:8081/query` | Endpoint to introspect and call. Point it at another server to try the scripts there; the checks in `test.sh` assume the mockapi schema. |
| `OUT_DIR` | `example-cli/out` | Where binaries, schemas, apps and logs go. |

## Notes

- The generated Go apps set `GOWORK=off`, only because they sit inside this repository's `go.work` tree. Your project does not need it.
- The TypeScript app is compiled and run with CommonJS module resolution ([`tsconfig.json`](./tsconfig.json)). `test.sh` additionally type-checks the same sources as a Node ESM package with `nodenext` resolution ([`tsconfig.nodenext.json`](./tsconfig.nodenext.json)), which is why `main.ts` imports `./sdk/queries/index.js` with the suffix.
