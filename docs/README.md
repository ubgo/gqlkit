# gqlkit documentation

gqlkit generates typed GraphQL client SDKs for Go and TypeScript from a schema. You fetch the schema once (`gqlkit-sdl`), generate an SDK from it (`gqlkit`), and then write queries in code with builders that pick fields at the call site. This page is the map of every guide and runnable example in the repo.

![How gqlkit fits together: generated SDK request/response stack at run time, and the gqlkit-sdl to gqlkit generate pipeline at build time](assets/architecture.svg)

## Contents

- [Learning path](#learning-path)
- [Start from your goal](#start-from-your-goal)
- [Getting started](#getting-started)
- [Go guides](#go-guides)
- [TypeScript guides](#typescript-guides)
- [CLI guides](#cli-guides)
- [Choosing guides](#choosing-guides)
- [Reference](#reference)
- [Runnable examples](#runnable-examples)
- [Contributing](#contributing)

## Learning path

Twelve steps from a first query to a production setup. Each step names what you learn, the guide that teaches it, and a program you can run. Go and TypeScript share the path; pick your column at each step. Most examples call the local test server, so start it once with `task mockapi:run` from the repo root.

| # | What you learn | Guide | Runnable example |
| --- | --- | --- | --- |
| 1 | Install the two CLIs (`gqlkit`, `gqlkit-sdl`) | [README: Install](../README.md#install) | [`example-cli/test.sh`](../example-cli/test.sh) runs both |
| 2 | Fetch a schema from a live endpoint, with auth headers if needed | [Fetch a schema](cli/fetch-schema.md) | [`example-cli/fetch-and-filter.sh`](../example-cli/fetch-and-filter.sh) |
| 3 | Generate an SDK: Go (`-m` import path) or TypeScript | [Generate Go](cli/generate-go.md), [Generate TypeScript](cli/generate-ts.md) | [`generate-go.sh`](../example-cli/generate-go.sh), [`generate-ts.sh`](../example-cli/generate-ts.sh) |
| 4 | Send a first query with the generated client | [Getting started — Go](getting-started-go.md#step-5-use-the-generated-sdk), [Getting started — TypeScript](getting-started-typescript.md#step-6-use-the-generated-sdk) | `runPing` in [Go samples](../example-go-mockapi/cmd/samples/main.go), [TS `main.ts`](../example-ts/cmd/samples/main.ts) |
| 5 | Select fields: nested objects, self-references, interfaces, and how result types narrow | [Go](go/selecting-fields.md), [TypeScript](typescript/selecting-fields.md) | [Go `selecting-fields`](../example-go-mockapi/cmd/selecting-fields/main.go), [TS `selecting.ts`](../example-ts/cmd/samples/selecting.ts) |
| 6 | Pass arguments, input objects and enums | [Go](go/arguments-and-inputs.md), [TypeScript](typescript/arguments-and-inputs.md) | [Go `enums-and-inputs`](../example-go-mockapi/cmd/enums-and-inputs/main.go), [TS `enums.ts`](../example-ts/cmd/samples/enums.ts) |
| 7 | Understand generated types, map custom scalars, write `config.jsonc` | [Go types](go/types-and-scalars.md), [TS types](typescript/types-scalars-enums.md), [config.jsonc](cli/config.md), [choosing a binding](choosing/scalar-bindings.md) | `runTodoWithScalars` in [Go samples](../example-go-mockapi/cmd/samples/main.go), [TS `scalars.ts`](../example-ts/cmd/samples/scalars.ts) |
| 8 | Handle errors: GraphQL errors, HTTP failures, partial data | [Go](go/errors.md), [TypeScript](typescript/errors.md) | [Go `errors`](../example-go-mockapi/cmd/errors/main.go), [TS `errors.ts`](../example-ts/cmd/samples/errors.ts) |
| 9 | Batch several queries or mutations into one request | [Go](go/batching.md), [TypeScript](typescript/batching.md), [single vs batch](choosing/single-vs-batch.md) | [Go `batch-mutations`](../example-go-mockapi/cmd/batch-mutations/main.go), [TS `batch-options.ts`](../example-ts/cmd/samples/batch-options.ts) |
| 10 | Configure the client: auth, headers, timeouts, cancellation, tests | [Go](go/client.md), [TypeScript](typescript/client.md), [client setup by app](choosing/client-setup.md) | [Go `client-options`](../example-go-mockapi/cmd/client-options/main.go), [TS `client-options.ts`](../example-ts/cmd/samples/client-options.ts) |
| 11 | Query unions and interfaces with `On<Type>` / `on<Type>`, and drop to the low-level builder for what the builders cannot express | [Go selecting fields](go/selecting-fields.md#step-4--query-a-field-that-returns-a-union), [TS unions](typescript/unions.md), [Go low-level](go/low-level-builder.md), [TS low-level](typescript/low-level.md) | [Go `selecting-fields`](../example-go-mockapi/cmd/selecting-fields/main.go), [TS `unions.ts`](../example-ts/cmd/samples/unions.ts), [Go `low-level`](../example-go-mockapi/cmd/low-level/main.go) |
| 12 | Keep the SDK in sync: filter big schemas, regenerate cleanly, check it in CI | [Filter a schema](cli/filter-schema.md), [When to regenerate](choosing/when-to-regenerate.md), [Recipes](recipes.md#keep-the-sdk-in-sync-as-the-schema-changes) | [`example-cli/test.sh`](../example-cli/test.sh) |

## Start from your goal

| I want to... | Read | Run |
| --- | --- | --- |
| Decide whether gqlkit fits my project | [gqlkit or a query-first generator](choosing/vs-query-first.md) | — |
| Call a third-party API from Go | [Recipe](recipes.md#generate-a-go-sdk-for-an-api-you-do-not-control), [Generate Go](cli/generate-go.md) | [`generate-go.sh`](../example-cli/generate-go.sh) |
| Call an API from a TypeScript app | [Getting started — TypeScript](getting-started-typescript.md), [Generate TypeScript](cli/generate-ts.md) | [`generate-ts.sh`](../example-cli/generate-ts.sh) |
| Generate Go and TypeScript from one schema | [Recipe](recipes.md#generate-both-a-go-and-a-typescript-client-from-one-schema), [Go or TypeScript](choosing/go-vs-typescript.md) | [`example-cli/test.sh`](../example-cli/test.sh) |
| Use only part of a huge vendor schema (Shopify, GitHub) | [Filter a schema](cli/filter-schema.md), [Recipe](recipes.md#trim-a-huge-vendor-schema-down-to-what-you-use) | [`fetch-and-filter.sh`](../example-cli/fetch-and-filter.sh) |
| Build from a schema split across files | [Recipe](recipes.md#assemble-a-schema-from-several-sdl-files), [Where the schema comes from](choosing/schema-source.md) | [`multi-file-schema.sh`](../example-cli/multi-file-schema.sh) |
| Load one screen's data in one round trip | [Single vs batch](choosing/single-vs-batch.md), [Go batching](go/batching.md), [TS batching](typescript/batching.md) | `runBatch` in [Go samples](../example-go-mockapi/cmd/samples/main.go), [TS `batch-options.ts`](../example-ts/cmd/samples/batch-options.ts) |
| Send the signed-in user's token | [Client setup by app](choosing/client-setup.md) | [Go `client-options`](../example-go-mockapi/cmd/client-options/main.go), [TS `client-options.ts`](../example-ts/cmd/samples/client-options.ts) |
| Map `DateTime`, `UUID` or `JSON` scalars | [Choosing a scalar binding](choosing/scalar-bindings.md), [config.jsonc](cli/config.md) | [Go config](../example-go-mockapi/cmd/generate/config.jsonc), [TS config](../example-ts/cmd/generate/config.jsonc) |
| Clear a field with an explicit `null` from Go | [Go arguments and inputs, Step 5](go/arguments-and-inputs.md#step-5--send-an-explicit-null-with-optt), [config.jsonc, Step 4](cli/config.md#step-4-choose-how-nullable-inputs-look-in-go-optional) | [Go `enums-and-inputs`](../example-go-mockapi/cmd/enums-and-inputs/main.go) |
| Remove files for operations the schema dropped | [When to regenerate](choosing/when-to-regenerate.md#step-1-generate-with---prune) | [`example-cli/test.sh`](../example-cli/test.sh) |
| Show a partial page when one field fails | [Go errors](go/errors.md#step-4--keep-partial-data-from-a-batch), [TS errors](typescript/errors.md#step-3--partial-data) | [Go `errors`](../example-go-mockapi/cmd/errors/main.go), [TS `errors.ts`](../example-ts/cmd/samples/errors.ts) |
| Send an operation the builders cannot express | [Which method to call](choosing/execute-methods.md) | [Go `low-level`](../example-go-mockapi/cmd/low-level/main.go), [TS `low-level.ts`](../example-ts/cmd/samples/low-level.ts) |
| Test my code without a GraphQL server | [Client setup by app](choosing/client-setup.md) | [TS `client-options.ts`](../example-ts/cmd/samples/client-options.ts) |
| Run the generators from my own Go program | [Run the generators from Go code](cli/programmatic.md), [CLI or Go program](choosing/cli-or-go-program.md) | [`example-go-mockapi/cmd/generate`](../example-go-mockapi/cmd/generate/main.go) |
| Debug a fetch that fails | [Recipe](recipes.md#debug-a-failing-introspection-fetch), [Fetch a schema](cli/fetch-schema.md#how-a-fetch-fails) | [`fetch-and-filter.sh`](../example-cli/fetch-and-filter.sh) |

## Getting started

- [Getting started — Go](getting-started-go.md): install, fetch, generate and a first tour of the Go SDK.
- [Getting started — TypeScript](getting-started-typescript.md): the same for the TypeScript SDK and the `gqlkit-ts` runtime.
- [Recipes](recipes.md): end-to-end workflows (third-party APIs, trimming vendor schemas, both languages, keeping in sync, multi-file schemas, batching, debugging a fetch).

## Go guides

| Page | Covers |
| --- | --- |
| [Client](go/client.md) | `NewClient`, `WithAuthToken`, `WithHeader(s)`, `WithHTTPClient`, context deadlines |
| [Selecting fields](go/selecting-fields.md) | `Select`, nested objects, self-referencing fields, interfaces, unions |
| [Arguments and inputs](go/arguments-and-inputs.md) | argument setters, input structs, pointers or `Opt[T]` for optional fields, enums |
| [Types, scalars and enums](go/types-and-scalars.md) | generated structs, built-in and custom scalar mappings, what "unselected" means in Go |
| [Errors](go/errors.md) | `GraphQLErrors`, `HTTPError`, transport errors, partial data from a batch |
| [Batching](go/batching.md) | `batch.RunQueries`, `batch.RunMutations`, compile-time gating, alias namespacing |
| [Low-level builder](go/low-level-builder.md) | `BuildQuery`, `GetVariables`, hand-built selections, `ExecuteRaw`, `Client.Execute`, `RawQuery` |

## TypeScript guides

| Page | Covers |
| --- | --- |
| [Client](typescript/client.md) | `authToken`, `headers`, custom `fetch`, token rotation, timeouts, cancellation, tests without a server |
| [Selecting fields](typescript/selecting-fields.md) | `.select()`, narrowed result types, nested objects, nullable results, self-references, interfaces |
| [Arguments and inputs](typescript/arguments-and-inputs.md) | setters, input interfaces, partial updates, filters |
| [Types, scalars and enums](typescript/types-scalars-enums.md) | generated interfaces, scalar bindings, enums, `null` vs `undefined` |
| [Errors](typescript/errors.md) | `GraphQLErrors`, `GraphQLHTTPError`, partial data, network failures |
| [Batching](typescript/batching.md) | `batch()`, `opName`, batched mutations, what is rejected before sending |
| [Unions](typescript/unions.md) | `on<Type>` selectors and narrowing on `__typename` |
| [Low-level API](typescript/low-level.md) | `executeRaw`, `client.execute<T>`, `FieldSelection`, `BaseBuilder` |

## CLI guides

| Page | Covers |
| --- | --- |
| [Generate a Go SDK](cli/generate-go.md) | `gqlkit generate`, which import-path flag to pass (`-m`, `-p`, `go.mod`) |
| [Generate a TypeScript SDK](cli/generate-ts.md) | `gqlkit generate-ts`, module resolution |
| [config.jsonc](cli/config.md) | every config key for Go and TypeScript, built-in mappings |
| [Fetch a schema](cli/fetch-schema.md) | `gqlkit-sdl fetch`, headers, JSON output, failure modes |
| [Filter a schema](cli/filter-schema.md) | exact and regex filters, `--remove-unused`, how flags combine |
| [Run the generators from Go code](cli/programmatic.md) | `clientgen.New`, `clientgents.New`, the `gqlkit-sdl/schema` Go API |
| [CLI reference](cli-reference.md) | every command and flag with captured output and exit codes |

## Choosing guides

Decision pages: which option fits which situation, with a guide and an example per option.

| Page | Decision |
| --- | --- |
| [gqlkit or a query-first generator](choosing/vs-query-first.md) | whether gqlkit fits your project |
| [Go or TypeScript](choosing/go-vs-typescript.md) | what each SDK promises, and what differs |
| [Where the schema comes from](choosing/schema-source.md) | introspection, an SDL file, a glob, or `schema.json` |
| [CLI or Go program](choosing/cli-or-go-program.md) | how to run the generate step |
| [Choosing a scalar binding](choosing/scalar-bindings.md) | built-in, `any`, a simple type or a library type, per scalar |
| [Client setup by app](choosing/client-setup.md) | which client options for a service, web server, SPA, SSR, CLI or tests |
| [Which method to call](choosing/execute-methods.md) | typed `Execute`, raw result, batch or a hand-written document |
| [Single vs batch](choosing/single-vs-batch.md) | when to merge operations into one request |
| [When to regenerate](choosing/when-to-regenerate.md) | what needs a regeneration, and how to do it cleanly |

## Reference

- [Feature reference](reference/features.md): every user-facing feature with its guide and example.
- [CLI reference](cli-reference.md): every command and flag.
- [gqlkit README](../gqlkit/README.md), [gqlkit-sdl README](../gqlkit-sdl/README.md), [gqlkit-ts README](../gqlkit-ts/README.md): per-artifact overviews and API summaries.
- [pkg/builder README](../gqlkit/pkg/builder/README.md) and [pkg/batch README](../gqlkit/pkg/batch/README.md): the builder and batch runtime as they are generated into every Go SDK.
- [Design notes for code assistants](ai-friendly.md) and [llms.txt](../llms.txt).
- Changelogs: [gqlkit](../gqlkit/CHANGELOG.md), [gqlkit-sdl](../gqlkit-sdl/CHANGELOG.md), [gqlkit-ts](../gqlkit-ts/CHANGELOG.md).

## Runnable examples

All of these run against the local test server ([mockapi](../mockapi/README.md), `task mockapi:run`) unless noted. `task examples:test` runs every live suite and fails when mockapi is down.

| Example | What it shows | Run |
| --- | --- | --- |
| [`example-cli/`](../example-cli/README.md) | the CLIs end to end: fetch, filter, generate Go and TS, build and call | `task example-cli:test` |
| [`example-go-mockapi/cmd/samples`](../example-go-mockapi/cmd/samples/main.go) | a tour: roots, arguments, nested selection, connections, scalars, mutations, batch | `task example-go-mockapi:run` |
| [`example-go-mockapi/cmd/selecting-fields`](../example-go-mockapi/cmd/selecting-fields/README.md) | self-references, interfaces, unions | `go run ./cmd/selecting-fields` |
| [`example-go-mockapi/cmd/enums-and-inputs`](../example-go-mockapi/cmd/enums-and-inputs/README.md) | enums, input structs, optional pointers and `Opt[T]` | `go run ./cmd/enums-and-inputs` |
| [`example-go-mockapi/cmd/errors`](../example-go-mockapi/cmd/errors/README.md) | every error shape and partial data | `go run ./cmd/errors` |
| [`example-go-mockapi/cmd/batch-mutations`](../example-go-mockapi/cmd/batch-mutations/README.md) | `batch.RunMutations` and the merged document | `go run ./cmd/batch-mutations` |
| [`example-go-mockapi/cmd/client-options`](../example-go-mockapi/cmd/client-options/README.md) | auth, headers, `http.Client`, deadlines | `go run ./cmd/client-options` |
| [`example-go-mockapi/cmd/low-level`](../example-go-mockapi/cmd/low-level/README.md) | `BuildQuery`, hand-built selections, `Client.Execute`, `RawQuery` | `go run ./cmd/low-level` |
| [`example-go-mockapi/cmd/generate`](../example-go-mockapi/cmd/generate/main.go) | `clientgen.New` with scalar bindings (no server needed) | `task example-go-mockapi:generate` |
| [`example-ts/cmd/samples`](../example-ts/README.md) | `main.ts` tour plus one file per guide (`selecting`, `enums`, `scalars`, `errors`, `batch-options`, `client-options`, `unions`, `low-level`) | `task example-ts:samples` |
| [`example-ts/cmd/samples/type-test.ts`](../example-ts/cmd/samples/type-test.ts) | compile-time narrowing (type-check only, no server) | `task example-ts:test` |
| [`example-ts/cmd/generate`](../example-ts/cmd/generate/main.go) | `clientgents.New` with string and npm-import bindings (no server needed) | `task example-ts:generate` |
| [`gqlkit-ts-demo/`](../gqlkit-ts-demo/README.md) | the published npm package against a public endpoint (needs internet, not mockapi) | see its README |
| [`example-go-chat/`](../example-go-chat/README.md) | generation against a large production schema; its samples need a private server | read-only |

The Go commands run from `example-go-mockapi/`; each folder also has a `main_test.go` (`go test ./cmd/<name>`) that checks the output the guide quotes.

## Contributing

- [CONTRIBUTING.md](../CONTRIBUTING.md): setup, repo layout, checks to run before a pull request.
- [TypeScript codegen internals](typescript-codegen.md): how the TypeScript generator is built.
- [Releasing](releasing.md): tag conventions and release steps for each artifact.
- Per-module guides: [gqlkit](../gqlkit/CONTRIBUTING.md), [gqlkit-sdl](../gqlkit-sdl/CONTRIBUTING.md), [gqlkit-ts](../gqlkit-ts/CONTRIBUTING.md).
