# GQLKit — GraphQL SDK generator CLI, schema fetcher, and Go + TypeScript runtimes

[![Go Reference](https://pkg.go.dev/badge/github.com/khanakia/gqlkit/gqlkit.svg)](https://pkg.go.dev/github.com/khanakia/gqlkit/gqlkit) [![Go Report Card](https://goreportcard.com/badge/github.com/khanakia/gqlkit/gqlkit)](https://goreportcard.com/report/github.com/khanakia/gqlkit/gqlkit) [![test](https://github.com/khanakia/gqlkit/actions/workflows/test.yml/badge.svg)](https://github.com/khanakia/gqlkit/actions/workflows/test.yml) [![lint](https://github.com/khanakia/gqlkit/actions/workflows/lint.yml/badge.svg)](https://github.com/khanakia/gqlkit/actions/workflows/lint.yml) ![coverage gqlkit](https://img.shields.io/badge/coverage%20gqlkit-91.7%25-brightgreen) ![coverage gqlkit-sdl](https://img.shields.io/badge/coverage%20gqlkit--sdl-96.9%25-brightgreen) [![release](https://img.shields.io/github/v/tag/khanakia/gqlkit?sort=semver&filter=gqlkit@*&label=release)](https://github.com/khanakia/gqlkit/tags) [![license](https://img.shields.io/badge/license-MIT-blue)](./LICENSE) ![Go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)

Generate fully typed GraphQL client SDKs for **Go** and **TypeScript** from any GraphQL schema. Built on a **builder pattern** with type-safe field selection — only the fields you select appear in the return type.

**Schema-first, not query-first:** generate builders once, then compose queries and mutations in code with dynamic, compile-time-checked field selection — one shared type per GraphQL type, no per-query codegen loop. Works with any GraphQL API, including large real-world schemas (Shopify Admin, Hasura, Apollo Federation).

## What ships here

This repo publishes four artifacts. The two CLIs are what you run; the two runtimes are what the code they generate imports.

| Deliverable | What it is | Install | Docs |
|---|---|---|---|
| **`gqlkit`** CLI | Generates a typed Go **or** TypeScript SDK from a GraphQL schema | `curl -sL https://raw.githubusercontent.com/khanakia/gqlkit/main/gqlkit/install.sh \| sh` | [CLI reference](./docs/cli-reference.md) |
| **`gqlkit-sdl`** CLI | Fetches a live endpoint's schema by introspection and writes it as SDL | `curl -sL https://raw.githubusercontent.com/khanakia/gqlkit/main/gqlkit-sdl/install.sh \| sh` | [CLI reference](./docs/cli-reference.md#gqlkit-sdl) |
| **`gqlkit`** Go module | Runtime the generated **Go** SDK imports — builder, client, batching | `go get github.com/khanakia/gqlkit/gqlkit` | [Getting started — Go](./docs/getting-started-go.md) |
| **`gqlkit-ts`** npm package | Runtime the generated **TypeScript** SDK imports | `npm i gqlkit-ts` | [Getting started — TypeScript](./docs/getting-started-typescript.md) |

## Contents

- [What ships here](#what-ships-here)
- [Install](#install)
- [Quick Start — Go](#quick-start--go)
- [Quick Start — TypeScript](#quick-start--typescript)
- [Why GQLKit?](#why-gqlkit)
- [CLI reference](./docs/cli-reference.md)
- [Recipes](./docs/recipes.md)
- [Key Features](#key-features)
- [FAQ](#faq)
- [Tools](#tools)
- [Testing and coverage](#testing-and-coverage)
- [Contributing](#contributing)

## Install

```bash
# macOS / Linux
curl -sL https://raw.githubusercontent.com/khanakia/gqlkit/main/gqlkit/install.sh | sh
curl -sL https://raw.githubusercontent.com/khanakia/gqlkit/main/gqlkit-sdl/install.sh | sh
```

Or download binaries from [Releases](https://github.com/khanakia/gqlkit/releases).

## Quick Start — Go

**1. Fetch your schema** (skip if you already have a `.graphql` file):

```bash
gqlkit-sdl fetch --url https://your-api.example.com/graphql -o schema.graphql
```

**2. Generate the SDK:**

```bash
gqlkit generate --schema schema.graphql --output ./sdk --package sdk
```

**3. Use it:**

```go
import (
    "gqlkit/pkg/graphqlclient"
    "yourmodule/sdk/queries"
    "yourmodule/sdk/fields"
)

client := graphqlclient.NewClient("https://your-api.example.com/graphql",
    graphqlclient.WithAuthToken("YOUR_TOKEN"),
)

qr := queries.NewQueryRoot(client)

todos, err := qr.Todos().
    Filter(&inputs.TodoFilter{Done: boolPtr(false)}).
    Select(func(f *fields.TodoFields) {
        f.ID().Text().Done().User(func(u *fields.UserFields) {
            u.ID().Name()
        })
    }).
    Execute(ctx)
```

Full guide: [docs/getting-started-go.md](./docs/getting-started-go.md)

## Quick Start — TypeScript

**1. Fetch your schema** (skip if you already have a `.graphql` file):

```bash
gqlkit-sdl fetch --url https://your-api.example.com/graphql -o schema.graphql
```

**2. Install the runtime:**

```bash
npm install gqlkit-ts
```

**3. Generate the SDK:**

```bash
gqlkit generate-ts --schema schema.graphql --output ./sdk --config config.jsonc
```

**4. Use it:**

```typescript
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";

const client = new GraphQLClient("https://your-api.example.com/graphql", {
  authToken: "YOUR_TOKEN",
});

const qr = new QueryRoot(client);

const todos = await qr
  .todos()
  .filter({ done: false })
  .select((t) =>
    t.id().text().done().user((u) => u.id().name())
  )
  .execute();
```

Full guide: [docs/getting-started-typescript.md](./docs/getting-started-typescript.md)

## Why GQLKit?

Existing GraphQL code generators like [genqlient](https://github.com/Khan/genqlient) (Go) and [GraphQL Code Generator](https://the-guild.dev/graphql/codegen) (TypeScript) take a **query-first** approach: you write every query as a static string upfront, then run codegen to produce types for each one. This creates real problems as your project grows.

### The duplicate types problem

In genqlient, every query generates its own unique types — even when they return the same GraphQL type. Two queries that both fetch a `User` produce two completely separate Go structs:

```graphql
# Two queries, same underlying User type
query GetUser($id: ID!)   { user(id: $id)   { id name email } }
query GetViewer            { viewer           { id name email } }
```

```go
// genqlient generates two unrelated structs — you can't pass one where the other is expected
type GetUserUser struct { Id string; Name string; Email string }
type GetViewerViewerUser struct { Id string; Name string; Email string }
```

With GQLKit, `User` is just `User`. One shared type across all queries:

```go
// GQLKit — one type, used everywhere
users, _ := qr.Users().Select(func(u *fields.UserFields) { u.ID().Name().Email() }).Execute(ctx)
user, _  := qr.User().ID("1").Select(func(u *fields.UserFields) { u.ID().Name().Email() }).Execute(ctx)
// Both return types.User
```

### Static queries vs. dynamic field selection

With genqlient and GraphQL Code Generator, field selection is locked at build time. Want the same query with different fields? Write another query, run codegen again, get another set of types.

GQLKit lets you choose fields at call time with full type safety:

```go
// Lightweight list view — only fetch what you need
qr.Todos().Select(func(f *fields.TodoFields) { f.ID().Text() }).Execute(ctx)

// Detail view — same query, more fields, no codegen step
qr.Todos().Select(func(f *fields.TodoFields) {
    f.ID().Text().Done().Priority().Tags().User(func(u *fields.UserFields) {
        u.ID().Name().Email().Role()
    })
}).Execute(ctx)
```

```typescript
// TypeScript — same flexibility, compile-time narrowed return types
const light = await qr.todos().select((t) => t.id().text()).execute();
const full  = await qr.todos().select((t) => t.id().text().done().user((u) => u.id().name())).execute();
// light.done → compile error (not selected)
// full.done  → boolean (selected)
```

### Which approach fits the job

Compared by category rather than by product, since the trade-offs belong to the approach and not to any one tool. ✅ built in · ⚠️ possible with work · ❌ not available.

| The job | **Schema-first builder** (GQLKit) | Query-first codegen | Hand-written queries + manual types | Untyped/generic client |
|---|---|---|---|---|
| Add a query you did not anticipate | ✅ call the builder | ❌ write the operation, re-run codegen | ❌ write query + types by hand | ✅ but untyped |
| Vary which fields a call selects | ✅ per call site, type-checked | ❌ a new operation per variant | ⚠️ edit the string and the struct together | ✅ but untyped |
| Pass one entity type between layers | ✅ one type per GraphQL type | ❌ one type per operation path | ⚠️ whatever you declared | ❌ maps/`any` |
| Catch a removed schema field at compile time | ✅ | ✅ after regenerating | ❌ fails at run time | ❌ fails at run time |
| Target Go and TypeScript from one schema | ✅ same generator | ⚠️ a separate tool per language | ❌ written twice | ⚠️ |
| Start from a live endpoint with no SDL on disk | ✅ `gqlkit-sdl fetch` | ⚠️ depends on the tool | ⚠️ manual introspection | ✅ nothing to generate |
| Work with a schema of thousands of types | ✅ filter + prune before generating | ⚠️ per-operation output stays small | ✅ you write only what you need | ✅ |
| Avoid a codegen step in daily work | ✅ regenerate only when the schema changes | ❌ regenerate per query change | ✅ no codegen at all | ✅ |

The honest trade-off: query-first codegen knows each operation ahead of time, so it can validate the exact document you will send and produce a type shaped precisely to it. A builder validates the schema and narrows types at the call site instead. If your queries are a fixed, small set that rarely changes, that difference matters little; if you are composing queries across many call sites, the builder is what stops the type explosion.

### Comparison by tool

| | GQLKit | [genqlient](https://github.com/Khan/genqlient) (Go) | [GraphQL Code Generator](https://the-guild.dev/graphql/codegen) (TS) |
|---|---|---|---|
| **Approach** | Schema-first — generates builders from the schema | Query-first — generates types from predefined `.graphql` operations | Query-first — generates types from predefined operations |
| **Field selection** | Dynamic at call time, type-safe | Static, locked at codegen time | Static, locked at codegen time |
| **Type per GraphQL type** | One shared type (e.g., `User`) | One per query path (e.g., `GetUserUser`, `GetViewerViewerUser`) | One per operation (e.g., `GetUserQuery`, `GetViewerQuery`) |
| **Adding a new query** | Just call the builder — no codegen step | Write `.graphql` file, re-run codegen | Write query string, re-run codegen |
| **Changing field selection** | Change the `.Select()` call | Write a new query variant, re-run codegen | Write a new query variant, re-run codegen |
| **Config complexity** | One `config.jsonc` for scalar mappings | `genqlient.yaml` + `@genqlient` directives per field | 60+ plugins; typical project needs 2–5 configured together |
| **Output structure** | Organized packages (`queries/`, `fields/`, `types/`, etc.) | Single `generated.go` file | Varies by plugin; often one large file |
| **Schema introspection** | Built-in (`gqlkit-sdl fetch`) | Not supported — must provide local SDL files | Supported via config |
| **Runtime overhead** | Minimal — lightweight HTTP client | Minimal | Generated code includes duplicate query strings; needs Babel/SWC plugin to optimize |
| **Languages** | Go + TypeScript from same schema | Go only | TypeScript/JavaScript only |

### TL;DR

- **genqlient / GraphQL Code Generator**: You write queries as static strings → codegen produces types for each one → types proliferate → changing fields means re-running codegen.
- **GQLKit**: You generate builders once from the schema → compose queries in code with dynamic field selection → one type per GraphQL type → no codegen loop for day-to-day work.

### AI-friendly by design

GQLKit's builder pattern works naturally with AI coding assistants (Copilot, Cursor, Claude). With query-first tools, the AI has to write raw GraphQL strings in a separate file, then you manually run codegen before the types exist — breaking the AI's flow. With GQLKit, the AI just writes code:

```
"fetch todos with user names" →

qr.Todos().Select(func(f *fields.TodoFields) {
    f.ID().Text().User(func(u *fields.UserFields) { u.ID().Name() })
}).Execute(ctx)
```

No `.graphql` files to create, no codegen to run, no types that don't exist yet. The builder API is fully discoverable from method signatures — the AI sees `.ID()`, `.Text()`, `.Done()`, `.User()` and chains them directly.

Deep dive: [docs/ai-friendly.md](./docs/ai-friendly.md)

## Key Features

- **Type-safe field selection** — only selected fields exist on the return type; unselected fields are compile-time errors
- **Builder pattern** — fluent API for queries, mutations, arguments, and nested field selection
- **Go + TypeScript** — generate SDKs for both languages from the same schema
- **Single-request batching (Go + TypeScript)** — merge multiple builders into one GraphQL operation with aliases via `batch.RunQueries` (Go) / `batch()` (TS) — one HTTP round trip for N queries, partial-success aware
- **Schema introspection** — fetch schemas from any GraphQL endpoint with `gqlkit-sdl`
- **Custom scalar mappings** — configure how GraphQL scalars map to language types via `config.jsonc`
- **Zero runtime overhead** — generated code with minimal dependencies

## Tools

| Tool | Description | Changelog |
|------|-------------|-----------|
| `gqlkit` | CLI — generates Go and TypeScript SDKs from GraphQL schemas | [`gqlkit/CHANGELOG.md`](./gqlkit/CHANGELOG.md) |
| `gqlkit-sdl` | CLI — fetches GraphQL schemas via introspection | [`gqlkit-sdl/CHANGELOG.md`](./gqlkit-sdl/CHANGELOG.md) |
| [`gqlkit-ts`](https://www.npmjs.com/package/gqlkit-ts) | npm package — lightweight TypeScript runtime for generated SDKs | [`gqlkit-ts/CHANGELOG.md`](./gqlkit-ts/CHANGELOG.md) |

Each artifact is versioned independently; release notes live in its own changelog. GitHub release pages at <https://github.com/khanakia/gqlkit/releases> mirror the same entries.

## FAQ

**What is gqlkit?**
A code generator that turns a GraphQL schema (SDL) into a fully typed client SDK for Go and TypeScript. Instead of writing queries as static strings, you compose them in code with a fluent builder and pick fields at call time — the return type narrows to exactly the fields you selected.

**How is gqlkit different from genqlient or GraphQL Code Generator?**
Those tools are query-first: you write every operation as a `.graphql` string, run codegen, and get a separate type per operation — so two queries that both return a `User` produce two unrelated structs, and changing fields means editing the query and re-running codegen. gqlkit is schema-first: it generates builders once, `User` is always `User`, and you change field selection by changing the `.Select(...)` call. See [Why GQLKit?](#why-gqlkit) for the full comparison.

**Does gqlkit work with large or unconventional schemas (Shopify, Hasura, Apollo Federation)?**
Yes. The Go generator is verified to produce compiling SDKs for the Shopify Admin API (2,400+ types), Hasura schemas (lowercase scalar/enum names like `timestamptz`, `order_by`), and Apollo Federation types (`_Service`, `_Entity`) — including recursive object graphs and non-conventional query-root names like `QueryRoot`.

**Is the generated Go SDK self-contained?**
Yes. Each generated Go SDK ships its own `builder/`, `graphqlclient/`, and `batch/` packages — there is no runtime dependency on gqlkit itself. Consumers only need the generated package plus their app code.

**Does it support mutations, batching, and custom scalars?**
Yes to all three. Mutations use the same builder pattern as queries; `batch.RunQueries` (Go) / `batch()` (TS) merge multiple builders into a single aliased request (one HTTP round trip, partial-success aware); and scalar-to-language-type mappings are configured in a `config.jsonc`.

**Which languages does it generate?**
Go and TypeScript, from the same schema. The Go and TypeScript SDKs expose the same builder API shape.

**How do I get the schema?**
Either point gqlkit at an existing `.graphql` SDL file, or fetch it from any live endpoint via introspection with `gqlkit-sdl fetch --url <endpoint>`.

**Is it production-ready?**
The generators have a two-layer test suite (unit tests on the type mapping plus end-to-end "generate and compile" guards) and CI on every push. The artifacts version independently via SemVer; see each [changelog](#tools).

## Testing and coverage

Coverage is measured per module, because each one is released and consumed separately. Run `task cover` to reproduce these numbers; they are read from the profile, not rounded by hand.

| Module | Statement coverage | Command |
|---|---|---|
| `gqlkit` | **91.7%** | `cd gqlkit && GOWORK=off go test -cover ./...` |
| `gqlkit-sdl` | **96.9%** | `cd gqlkit-sdl && GOWORK=off go test -cover ./...` |

`GOWORK=off` is deliberate: it resolves each module exactly the way CI and `go install` do, rather than through the workspace. A stale root `vendor/` directory (untracked, gitignored) will otherwise force `-mod=vendor` and fail the run with a missing test dependency.

Four functions report 0% and stay that way for reasons that no additional test can change:

| Function | Why it cannot be covered |
|---|---|
| `gqlkit/cmd/gqlkit/root.go:main` | Process entry point — calls `os.Exit`, so executing it ends the test binary. Everything it dispatches to is covered through `cmd/gqlkit`'s own tests. |
| `gqlkit-sdl/main.go:main` | Same: `os.Exit` wrapper around the covered `fetch` command. |
| `builder.QueryMarker.IsQueryOp` | Empty marker method (`func (QueryMarker) IsQueryOp() {}`). It has zero statements, and statement coverage counts statements — the figure cannot rise above 0% even though `pkg/builder`'s tests call it on every run. |
| `builder.MutationMarker.IsMutationOp` | Same — the dual marker, also a zero-statement body. |

The markers are still pinned: `pkg/builder/marker_test.go` asserts, at compile time, that each type satisfies the interface `pkg/batch` requires, and that the two markers remain distinct types. That check matters because the interface is satisfied in *generated* SDKs — renaming a marker would leave this module green while every generated batch call site stopped compiling.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for development setup, architecture, and how to run the examples.
