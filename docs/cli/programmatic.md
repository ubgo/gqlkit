# Run the generators from Go code

Everything the two CLIs do is also a Go API: `clientgen.New` (the Go SDK generator behind `gqlkit generate`), `clientgents.New` (the TypeScript generator behind `gqlkit generate-ts`) and the `gqlkit-sdl/schema` package (fetch, filter, convert, save). Use the API when a shell command is not enough: generating several SDKs in one step, computing paths or bindings, wrapping generation in your own tool, or fetching with a custom timeout. For a one-off or a Taskfile line, the CLI is simpler.

Both programs on this page are runnable in [`example-cli/programmatic`](../../example-cli/programmatic/README.md). Other examples in this repo that use the API: [`example-go-mockapi/cmd/generate`](../../example-go-mockapi/cmd/generate/main.go), [`example-go-chat/cmd/generate`](../../example-go-chat/cmd/generate/main.go) and [`example-ts/cmd/generate`](../../example-ts/cmd/generate/main.go).

## Before you start

- A Go module for the generator program. Its `go.mod` gains `github.com/ubgo/gqlkit/gqlkit` (and `github.com/ubgo/gqlkit/gqlkit-sdl` if you fetch from Go). This is a dependency of the generator program only; the generated Go SDK still imports nothing from gqlkit.

```sh
go get github.com/ubgo/gqlkit/gqlkit
go get github.com/ubgo/gqlkit/gqlkit-sdl
```

## Generate Go and TypeScript SDKs from one program

```go
// Command gen regenerates the Go and TypeScript SDKs from schema.graphql.
// Run it from the module root: go run ./gen
package main

import (
	"log"

	"github.com/ubgo/gqlkit/gqlkit/pkg/clientgen"
	"github.com/ubgo/gqlkit/gqlkit/pkg/clientgents"
)

func main() {
	goGen, err := clientgen.New(&clientgen.Config{
		SchemaPath: "schema.graphql",
		OutputDir:  "./sdk",
		ModulePath: "example.com/prog/sdk",
		ConfigPath: "go.jsonc",
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := goGen.Generate(); err != nil {
		log.Fatal(err)
	}

	tsGen, err := clientgents.New(&clientgents.Config{
		SchemaPath: "schema.graphql",
		OutputDir:  "./web/src/sdk",
		ConfigPath: "ts.jsonc",
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := tsGen.Generate(); err != nil {
		log.Fatal(err)
	}
}
```

`New` validates the config, reads the bindings file, parses the schema and compiles the templates; `Generate` writes the files. `go.jsonc` and `ts.jsonc` are the two [config file formats](./config.md).

### clientgen.Config fields

| Field | CLI flag | Meaning |
| --- | --- | --- |
| `SchemaPath` | `-s` | SDL file or glob. Required. |
| `OutputDir` | `-o` | Defaults to `./sdk`. |
| `ModulePath` | `-m` | Import path of the output directory. Copied into `Package` when `Package` is empty. |
| `PackageName` | `-p` | Defaults to `sdk`. Used as the import path only when it contains `/` and `Package` and `ModulePath` are empty. |
| `ConfigPath` | `-c` | [Config file](./config.md); empty means none, a path that does not exist is an error. |
| `Package` | none | The import path actually used. Set it directly to bypass `ModulePath`, `PackageName` and `go.mod` detection. |
| `Prune` | `--prune` | Delete banner-carrying files in `OutputDir` that this run did not write. `gen.PrunedFiles()` lists what went. |
| `PruneDryRun` | `--dry-run` | With `Prune`: list in `PrunedFiles()` and print, delete nothing. Without `Prune`, `New` returns an error. |

`example-go-mockapi/cmd/generate` sets both `ModulePath` and `Package`; `Package` wins, which is why that example's imports are `example-go-mockapi/sdk/...`.

`clientgents.Config` has `SchemaPath`, `OutputDir` (default `./sdk`), `ConfigPath`, `Prune` and `PruneDryRun`, and its generator has the same `PrunedFiles()`.

## Fetch and filter from Go

```go
// Command sdlapi fetches a schema, keeps two queries, prunes unused types,
// and writes both SDL and JSON, using the gqlkit-sdl Go API instead of the CLI.
package main

import (
	"log"
	"os"
	"time"

	"github.com/ubgo/gqlkit/gqlkit-sdl/schema"
)

func main() {
	introspection, err := schema.FetchSchema("http://localhost:8081/query", &schema.FetchOptions{
		Headers: map[string]string{"Authorization": "Bearer " + os.Getenv("API_TOKEN")},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	trimmed, err := schema.FilterSchema(introspection, &schema.FilterOptions{
		OnlyQueries:  []string{"todo", "todos"},
		RemoveUnused: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := schema.SaveToFile(schema.ConvertToSDL(trimmed), "schema.graphql"); err != nil {
		log.Fatal(err)
	}
	if err := schema.SaveAsJSON(trimmed, "schema.json"); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote schema.graphql and schema.json")
}
```

| Function | Notes |
| --- | --- |
| `FetchSchema(url, *FetchOptions)` | `Timeout` zero means `schema.DefaultFetchTimeout` (2 minutes). `Debug: true` prints a curl command and returns `(nil, nil)`, so check for a nil schema. Returns `schema.ErrNoSchema` for a 200 with no schema. |
| `FilterSchema(s, *FilterOptions)` | Returns `(*IntrospectionSchema, error)`: a new schema (the input is not modified), or an error for an invalid regex or a filter that removes every query field. A nil options pointer returns the input unchanged. Same rules as the [CLI filters](./filter-schema.md#how-the-flags-combine). |
| `ConvertToSDL(s)` | Returns SDL as a string. Built-in scalars and directives are left out. |
| `SaveToFile(sdl, path)`, `SaveAsJSON(s, path)` | Write with mode `0644`, replacing the file. JSON is wrapped as `{"data": {"__schema": ...}}`. |

## Try it

Both programs are checked in as [`example-cli/programmatic/gen/main.go`](../../example-cli/programmatic/gen/main.go) and [`example-cli/programmatic/sdlapi/main.go`](../../example-cli/programmatic/sdlapi/main.go), identical to the listings above. Their test runs `sdlapi` against the local test server, then `gen` on the schema it wrote, in a temporary directory:

```sh
cd example-cli/programmatic
go test -count=1 -v ./
```

```
=== RUN   TestSdlapiThenGen
--- PASS: TestSdlapiThenGen (0.98s)
PASS
ok  	example-cli/programmatic	0.991s
```

It checks that `type Query` holds `todo` and not `ping`, that `sdk/scalars/scalars.go` has `DateTime = time.Time` and `web/src/sdk/scalars/index.ts` has `export type DateTime = string;`, and that the Go SDK builds on its own. Setup and notes: [example-cli/programmatic/README.md](../../example-cli/programmatic/README.md).

## Pitfalls

- **Generators print to stdout.** `Generate` writes progress lines with `fmt.Printf`; there is no option to silence them.
- **Nothing is deleted unless you ask.** Without `Prune`, `Generate` overwrites files but leaves files from removed operations in place. Set `Prune: true` rather than `os.RemoveAll(OutputDir)`: it removes only files carrying the gqlkit banner, so hand-written files in the directory survive.
- **Relative paths are relative to the working directory**, not to the generator's source file. `go run ./gen` from the module root and `go run .` from inside `gen/` read different files, and `go.mod` detection starts from the working directory too.
- **No `ModulePath`, no `Package` and no `go.mod` in the working directory or above** is an error from `New` (`cannot determine the generated SDK's import path`). Set `ModulePath` in generator programs so the result does not depend on where they run.
- **Several generators in one process are independent.** Each `clientgen.New` starts from a fresh copy of the built-in scalar map, so bindings from one SDK do not leak into the next (checked: `sdk-a` with `DateTime: time.Time`, then `sdk-b` with no config, gives `sdk-b` `DateTime = any`).

## Related

- [Generate a Go SDK (CLI)](./generate-go.md)
- [Generate a TypeScript SDK (CLI)](./generate-ts.md)
- [config.jsonc](./config.md)
- [gqlkit-sdl README: Go API](../../gqlkit-sdl/README.md#go-api)
