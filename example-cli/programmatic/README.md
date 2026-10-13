# example-cli/programmatic — the generators and gqlkit-sdl from Go code

The two programs quoted in [Run the generators from Go code](../../docs/cli/programmatic.md), as a module you can run and test:

| Program | What it does |
| --- | --- |
| [`sdlapi/main.go`](./sdlapi/main.go) | Fetches the mockapi schema with `schema.FetchSchema`, keeps the `todo` and `todos` queries with `schema.FilterSchema` (pruning unused types), and writes `schema.graphql` and `schema.json`. |
| [`gen/main.go`](./gen/main.go) | Reads `schema.graphql`, `go.jsonc` and `ts.jsonc` from the working directory and generates a Go SDK into `./sdk` (`clientgen.New`) and a TypeScript SDK into `./web/src/sdk` (`clientgents.New`). |

The docs page quotes both files byte for byte. [`testdata/go.jsonc`](./testdata/go.jsonc) and [`testdata/ts.jsonc`](./testdata/ts.jsonc) are the two config files `gen` reads.

## Run it

Start the test server from the repository root (`task mockapi:run`), then:

```sh
cd example-cli/programmatic
go test -count=1 ./...
```

[`programmatic_test.go`](./programmatic_test.go) builds both programs and runs them in a temporary directory: `sdlapi` first, then `gen` on the schema `sdlapi` wrote. It checks the trimmed schema (`todo` kept, `ping` gone), the bindings in both SDKs (`DateTime = time.Time` in Go, `export type DateTime = string;` in TypeScript), and that the generated Go SDK compiles on its own with `GOWORK=off`. Nothing is written inside the repository. `-count=1` is there because the test talks to a live server, which Go's test cache cannot see.

With the server stopped the test fails, rather than skipping:

```
mockapi is not running at http://localhost:8081/query (...); start it with: task mockapi:run
```

## Notes

- The module requires the published `gqlkit` and `gqlkit-sdl` modules but builds against the local source through the repository's `go.work`, like the other examples. `sdlapi` uses the two-value `FilterSchema` signature, which is newer than the pinned `gqlkit-sdl` v0.11.0, so building it outside the workspace needs a newer release.
- Running a program by hand from this directory writes its outputs here; [`.gitignore`](./.gitignore) keeps them out of git.
