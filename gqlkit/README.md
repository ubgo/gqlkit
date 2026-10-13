# gqlkit

`gqlkit` is the code generator: it reads a GraphQL schema in SDL and writes a typed client SDK, in Go (`gqlkit generate`) or TypeScript (`gqlkit generate-ts`). The SDK has one builder per query and mutation, one type per GraphQL type, and a field selector per object type, so you compose operations in code and pick fields at the call site instead of writing query strings.

This folder is also a Go module (`github.com/ubgo/gqlkit/gqlkit`) that exposes the same generators as a Go API. A generated SDK never imports this module.

- New to gqlkit? Start with the [documentation index](../docs/README.md) and its [learning path](../docs/README.md#learning-path).
- Every command and flag with captured output: [CLI reference](../docs/cli-reference.md#gqlkit).
- What changed in each release: [CHANGELOG](./CHANGELOG.md).

## Contents

- [Install](#install)
- [Generate a Go SDK](#generate-a-go-sdk)
- [Generate a TypeScript SDK](#generate-a-typescript-sdk)
- [Commands and flags](#commands-and-flags)
- [Scalar bindings (config.jsonc)](#scalar-bindings-configjsonc)
- [What gets generated](#what-gets-generated)
- [Use the generators from Go](#use-the-generators-from-go)
- [Packages in this module](#packages-in-this-module)
- [Pitfalls](#pitfalls)
- [Related](#related)

## Install

Quick install for macOS and Linux:

```bash
curl -sL https://raw.githubusercontent.com/ubgo/gqlkit/main/gqlkit/install.sh | sh
```

Or download a binary for any platform from [GitHub Releases](https://github.com/ubgo/gqlkit/releases), or build it with Go:

```bash
go install github.com/ubgo/gqlkit/gqlkit/cmd/gqlkit@latest   # newest release
go install github.com/ubgo/gqlkit/gqlkit/cmd/gqlkit@v0.14.0  # pinned
```

Releases consumable by Go are tagged in the path-prefixed form Go requires for a nested module (`gqlkit/v0.14.0`), starting at v0.11.0. The `gqlkit@vX.Y.Z` tags are what GitHub Releases and the install script use; both forms point at the same commits, and `gqlkit version` reports the module version it was built from either way.

Check the install:

```bash
gqlkit version
```

## Generate a Go SDK

You need a schema file (fetch one with [`gqlkit-sdl`](../gqlkit-sdl/README.md) if you only have an endpoint) and a Go module to generate into. From the module root:

```bash
gqlkit generate -s schema.graphql -o ./sdk -m "$(go list -m)/sdk"
go build ./...
```

`-m` is the import path of the output directory, not of your module: the generated packages import one another through it. The generated SDK needs Go 1.24 or later; a build guard makes older toolchains fail to compile it rather than misbehave. Then use the SDK; every import comes from the generated code:

```go
import (
	"context"

	"yourmodule/sdk/fields"
	"yourmodule/sdk/graphqlclient"
	"yourmodule/sdk/queries"
)

func listTodos(ctx context.Context) error {
	client := graphqlclient.NewClient("https://api.example.com/graphql",
		graphqlclient.WithAuthToken(token),
	)
	qr := queries.NewQueryRoot(client)

	todos, err := qr.Todos().
		Select(func(f *fields.TodoFields) {
			f.ID().Text().User(func(u *fields.UserFields) { u.Name() })
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	_ = todos // []types.Todo; fields you did not select hold zero values
	return nil
}
```

Step by step, with every flag explained: [Generate a Go SDK](../docs/cli/generate-go.md). Runnable: [`example-cli/generate-go.sh`](../example-cli/generate-go.sh).

## Generate a TypeScript SDK

```bash
npm i gqlkit-ts
gqlkit generate-ts -s schema.graphql -o ./src/sdk
```

```ts
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";

const qr = new QueryRoot(new GraphQLClient("https://api.example.com/graphql", { authToken }));

const todos = await qr.todos().select((t) => t.id().text().user((u) => u.name())).execute();
// todos: { id: string; text: string; user: { name: string } }[]
```

The generated files import their runtime from [`gqlkit-ts`](../gqlkit-ts/README.md), so the app needs that package. Step by step: [Generate a TypeScript SDK](../docs/cli/generate-ts.md). Runnable: [`example-cli/generate-ts.sh`](../example-cli/generate-ts.sh).

## Commands and flags

| Command | What it does |
| --- | --- |
| `gqlkit generate` | Generate a Go SDK |
| `gqlkit generate-ts` | Generate a TypeScript SDK |
| `gqlkit version` | Print the version |
| `gqlkit completion <shell>` | Print a shell completion script (bash, zsh, fish, powershell) |

| Flag | Short | `generate` | `generate-ts` | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `--schema` | `-s` | required | required | — | SDL file, or a quoted glob such as `'./schema/*.graphql'` |
| `--output` | `-o` | yes | yes | `./sdk` | Output directory; created if missing, files overwritten, never deleted |
| `--config` | `-c` | yes | yes | — | `config.jsonc` with scalar bindings and other settings; a path that does not exist is an error |
| `--module` | `-m` | yes | — | nearest `go.mod` (walking up) + `-o` | Import path of the output directory |
| `--package` | `-p` | yes | — | `sdk` | Name shown in the progress output; a value containing `/` is used as the import path |
| `--prune` | — | yes | yes | off | After a successful run, delete files in the output directory that carry the gqlkit banner but were not written by this run; files without the banner are never touched |
| `--dry-run` | — | yes | yes | off | With `--prune`: list what would be deleted, delete nothing (an error without `--prune`) |

Both commands exit 0 on success and 1 on any failure. Which import-path flag to pass in which situation: [decision table](../docs/cli/generate-go.md#which-import-path-flag-to-pass).

## Scalar bindings (config.jsonc)

Custom scalars in the schema (`scalar DateTime`, `scalar Cursor`) need to be told which language type to use; anything unbound becomes `any`. Go and TypeScript use different formats, so keep one file per language:

```jsonc
// Go: go.config.jsonc
{
  "bindings": {
    "DateTime": { "model": "time.Time" },
    "Cursor": { "model": "string" },
    "UUID": { "model": "github.com/google/uuid.UUID" }
  }
}
```

```jsonc
// TypeScript: ts.config.jsonc
{
  "bindings": {
    "DateTime": "string",
    "Cursor": "string",
    "JSON": { "type": "JsonValue", "import": "type-fest" }
  }
}
```

Go already maps `Int64`, `Int32`, `Uint`, `Uint64`, `Uint32`, `Float64`, `Float32`, `Time` (`time.Time`) and `JSON` (`json.RawMessage`) without a config.

The same files hold two more settings:

- **Go `inputs`**: how nullable input-object fields are generated. The default `"pointer"` gives `*T`, which cannot send an explicit `null`. `"opt"` gives `inputs.Opt[T]` with three states (unset, `inputs.Null[T]()`, `inputs.Some(v)`), the only way to clear a field in an update. Set it for everything with `"inputs": { "optional": "opt" }`, or per input type or field with `"overrides": { "UpdateTodoInput.tags": "opt" }`. `Opt` needs Go 1.24 or later.
- **TypeScript `nullable`**: nullable outputs are `T | null` by default. `"nullable": "null-undefined"` (or `nullableOverrides` per type or field) widens them to `T | null | undefined` for code written against older SDKs. A TypeScript binding can also name a codec (`"decode"`, `"encode"`, `"codecImport"`) so values are converted, not just typed. The full key list and the built-in table: [config.jsonc](../docs/cli/config.md). Which binding to pick per scalar: [Choosing a scalar binding](../docs/choosing/scalar-bindings.md).

## What gets generated

| Go package | TypeScript folder | Holds |
| --- | --- | --- |
| `queries`, `mutations` | `queries/`, `mutations/` | The roots (`NewQueryRoot`, `new QueryRoot`) and one builder per root field |
| `fields` | `fields/` | One selector per object, interface and union type (`TodoFields`; `SearchResultFields` with `OnTodo` / `OnUser`) |
| `types` | `types/` | One type per GraphQL object or interface; in Go also a `<Name>Value` wrapper per union and interface (`Typename`, `Value`, `Raw`) |
| `inputs` | `inputs/` | Input object types; in Go also `Opt[T]`, `Some` and `Null` |
| `enums` | `enums/` | Enums (Go: typed string constants; TS: `enum`) |
| `scalars` | `scalars/` | Custom scalars as type aliases, per your bindings |
| `builder` | `builder/` | Go: the builder runtime itself. TS: a re-export of `gqlkit-ts` |
| `graphqlclient` | — | Go: the HTTP client (`NewClient`, `WithAuthToken`, `GraphQLErrors`) |
| `batch` | — | Go: `RunQueries` and `RunMutations`. TS: `batch` comes from `gqlkit-ts` |

The guides for using each part: [Go guides](../docs/README.md#go-guides) and [TypeScript guides](../docs/README.md#typescript-guides).

## Use the generators from Go

Everything the CLI does is available as `clientgen.New` (Go SDK) and `clientgents.New` (TypeScript SDK):

```go
gen, err := clientgen.New(&clientgen.Config{
	SchemaPath: "schema.graphql",
	OutputDir:  "./sdk",
	ModulePath: "example.com/app/sdk",
	ConfigPath: "go.config.jsonc",
})
if err != nil {
	log.Fatal(err)
}
if err := gen.Generate(); err != nil {
	log.Fatal(err)
}
```

The full program, every `Config` field, and fetching from Go: [Run the generators from Go code](../docs/cli/programmatic.md). When to prefer this over the CLI: [CLI or Go program](../docs/choosing/cli-or-go-program.md). Runnable: [`example-go-mockapi/cmd/generate`](../example-go-mockapi/cmd/generate/main.go), [`example-ts/cmd/generate`](../example-ts/cmd/generate/main.go).

## Packages in this module

| Package | Purpose |
| --- | --- |
| `cmd/gqlkit` | The CLI |
| `pkg/clientgen` | The Go SDK generator (`clientgen.New`) |
| `pkg/clientgents` | The TypeScript SDK generator (`clientgents.New`) |
| `pkg/templater` | The Go templates the generators render |
| `pkg/builder`, `pkg/graphqlclient`, `pkg/batch` | Upstream versions of the runtime packages emitted into every Go SDK, kept in sync with the templates and tested here. See [pkg/builder](./pkg/builder/README.md) and [pkg/batch](./pkg/batch/README.md) |
| `pkg/schemagql`, `pkg/typegql`, `pkg/util`, `pkg/writer` | Internal helpers: schema loading, GraphQL to Go type mapping, file writing |

## Pitfalls

- **Wrong `-m`.** Passing your module path (`example.com/app`) instead of the SDK's (`example.com/app/sdk`) generates imports that do not resolve. Without `-m`, gqlkit uses the nearest `go.mod` above the working directory, and fails with an error if there is none or the output directory is outside that module.
- **Stale files.** Without `--prune` the generator overwrites but never deletes, so a builder for a removed operation stays behind and breaks the build. Pass `--prune` (preview with `--dry-run`); see [When to regenerate](../docs/choosing/when-to-regenerate.md).
- **An unquoted glob.** The shell expands `./schema/*.graphql` into several arguments before gqlkit sees it. gqlkit refuses the extra arguments and exits 1 with `unexpected argument(s): ...` and a hint to quote the glob, e.g. `-s './schema/*.graphql'`. Quote it.
- **One config for both languages.** A Go config passed to `generate-ts` writes broken scalar aliases without an error.
- **Go results are not narrowed.** Unselected fields are zero values, not compile errors; only the TypeScript SDK narrows. See [Go or TypeScript](../docs/choosing/go-vs-typescript.md).

## Related

- [Documentation index](../docs/README.md)
- [gqlkit-sdl](../gqlkit-sdl/README.md): fetch and filter a schema
- [gqlkit-ts](../gqlkit-ts/README.md): the TypeScript runtime
- [Contributing to this module](./CONTRIBUTING.md)
