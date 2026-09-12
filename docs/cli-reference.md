# CLI reference

Complete reference for both command-line tools. Every block below is real captured output from the binaries in this repo, not illustrative text — including the error cases and their exit codes.

- [gqlkit](#gqlkit)
  - [gqlkit generate](#gqlkit-generate)
  - [gqlkit generate-ts](#gqlkit-generate-ts)
  - [gqlkit version](#gqlkit-version)
- [gqlkit-sdl](#gqlkit-sdl)
  - [gqlkit-sdl fetch](#gqlkit-sdl-fetch)
  - [gqlkit-sdl version](#gqlkit-sdl-version)
- [Exit codes](#exit-codes)

## gqlkit

Generates a typed client SDK from a GraphQL SDL file. One binary, two generators: Go and TypeScript.

```
$ gqlkit --help
A CLI tool that generates type-safe Go and TypeScript client SDKs from GraphQL SDL files.

Usage:
  gqlkit [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  generate    Generate Go SDK from GraphQL schema
  generate-ts Generate TypeScript SDK from GraphQL schema
  help        Help about any command
  version     Print version and exit

Flags:
  -h, --help   help for gqlkit

Use "gqlkit [command] --help" for more information about a command.
```

### gqlkit generate

Generates the Go SDK.

```
$ gqlkit generate --help
Generates a type-safe Go client SDK from a GraphQL SDL file.

Usage:
  gqlkit generate [flags]

Flags:
  -c, --config string    Path to config.jsonc file (optional)
  -h, --help             help for generate
  -m, --module string    Go module path for generated SDK (e.g., github.com/user/myapi)
  -o, --output string    Output directory for generated SDK (default "./sdk")
  -p, --package string   Go package name for generated SDK (default "sdk")
  -s, --schema string    Path to GraphQL SDL file (required)
```

`--module` matters more than it looks: the generated packages import each other, so the module path is what makes `sdk/queries` able to import `sdk/types`. Pass the module path of the project you are generating into.

A real run against a two-type schema:

```
$ gqlkit generate -s schema.graphql -o ./sdk -m example.com/demo
Generating SDK from schema.graphql
Output directory: ./sdk
Package name: sdk
Generated: scalars.go
Generated: enums.go
Generated: types.go
Generated: inputs.go
Generated: builder.go
Generated: graphqlclient/graphqlclient.go
Generated: fields/field_*.go
Generated: queries/ and mutations/
Generated: batch/batch.go
SDK generated successfully in ./sdk
```

What it wrote:

```
sdk/batch/batch.go
sdk/builder/builder.go
sdk/enums/enums.go
sdk/fields/field_user.go
sdk/graphqlclient/graphqlclient.go
sdk/inputs/inputs.go
sdk/queries/query_user.go
sdk/queries/query_users.go
sdk/queries/root.go
sdk/scalars/scalars.go
sdk/types/types.go
```

One file per root field under `queries/`, one field-selector per object type under `fields/`, and exactly one `types.User` shared by every query that returns a `User`.

Failure modes:

```
$ gqlkit generate
Error: required flag(s) "schema" not set
Usage:
$ echo $?
1
```

```
$ gqlkit generate -s missing.graphql -o ./sdk -m example.com/demo
Error: failed to create generator: failed to parse schema: missing.graphql did not match any files
$ echo $?
1
```

`--schema` accepts a glob, which is why a missing file reports "did not match any files" rather than "no such file": pointing it at `./schema/*.graphql` to assemble a schema from several files is supported.

### gqlkit generate-ts

Generates the TypeScript SDK. Same schema in, a different runtime out — the generated code imports [`gqlkit-ts`](https://www.npmjs.com/package/gqlkit-ts) rather than a Go module, so there is no `--module` or `--package` flag.

```
$ gqlkit generate-ts --help
Generates a type-safe TypeScript client SDK from a GraphQL SDL file.

Usage:
  gqlkit generate-ts [flags]

Flags:
  -c, --config string   Path to config.jsonc file (optional)
  -h, --help            help for generate-ts
  -o, --output string   Output directory for generated SDK (default "./sdk")
  -s, --schema string   Path to GraphQL SDL file (required)
```

```
$ gqlkit generate-ts -s schema.graphql -o ./tssdk
Generating TypeScript SDK from schema.graphql
Output directory: ./tssdk
Generated: builder/index.ts
Generated: scalars/index.ts
Generated: enums/index.ts
Generated: types/index.ts
Generated: inputs/index.ts
Generated: fields/*.ts
Generated: queries/ and mutations/
TypeScript SDK generated successfully in ./tssdk
```

```
tssdk/builder/index.ts
tssdk/enums/index.ts
tssdk/fields/index.ts
tssdk/fields/user.ts
tssdk/inputs/index.ts
tssdk/queries/index.ts
tssdk/queries/root.ts
tssdk/queries/user.ts
```

### gqlkit version

```
$ gqlkit version
gqlkit dev
```

`dev` is what a build from a working tree reports. A release binary reports its tag, and a binary installed with `go install` reports the module version recorded in it — so the answer is never ambiguous about which build you are running.

## gqlkit-sdl

Fetches a schema from a live endpoint by introspection and writes it as SDL, so `gqlkit generate` has something to read.

```
$ gqlkit-sdl --help
Fetch a GraphQL schema and convert it to SDL format

Usage:
  gqlkit-sdl [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  fetch       Fetch GraphQL schema and save as SDL
  help        Help about any command
  version     Print version and exit

Flags:
  -h, --help   help for gqlkit-sdl

Use "gqlkit-sdl [command] --help" for more information about a command.
```

### gqlkit-sdl fetch

```
$ gqlkit-sdl fetch --help
Fetch a GraphQL schema via introspection and save it as SDL.

Filter flags accept exact names or regex patterns (comma-separated).
Any value with regex metacharacters (. * + ? etc.) is treated as a regex.

Usage:
  gqlkit-sdl fetch [flags]

Flags:
      --debug                       Print the curl command for debugging
      --exclude-mutations strings   Remove these mutation fields (comma-separated)
      --exclude-queries strings     Remove these query fields (comma-separated)
  -f, --format string               Output format: "graphql" (SDL) or "json" (default "graphql")
  -H, --header stringArray          HTTP header in "Key:Value" format (repeatable)
  -h, --help                        help for fetch
      --only-mutations strings      Keep only these mutation fields (comma-separated)
      --only-queries strings        Keep only these query fields (comma-separated)
  -o, --output string               Output file path (default "schema.graphql")
      --remove-unused               Remove types/inputs not referenced by remaining operations
      --url string                  GraphQL endpoint URL (required)
```

The filter flags exist for a specific problem: vendor schemas are enormous (Shopify Admin is thousands of types), and generating an SDK for all of it produces a package nobody wants to compile. Keep the operations you use, then `--remove-unused` prunes every type that nothing reachable references.

`--debug` prints the equivalent `curl` command, which is the fastest way to work out whether a 401 is your token or your header syntax.

### gqlkit-sdl version

```
$ gqlkit-sdl version
gqlkit-sdl dev
```

## Exit codes

Both binaries follow the same convention.

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Any failure — a missing required flag, an unreadable or unparseable schema, a failed introspection request, or an unwritable output directory |

There are no distinct codes per failure class. Scripts should branch on the exit code for success versus failure and read stderr for the reason; the message always names the operation that failed (`failed to create generator: failed to parse schema: ...`).
