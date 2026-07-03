# GQLKit — GraphQL SDK Generator

[![Go Reference](https://pkg.go.dev/badge/github.com/khanakia/gqlkit/gqlkit.svg)](https://pkg.go.dev/github.com/khanakia/gqlkit/gqlkit) [![Go Report Card](https://goreportcard.com/badge/github.com/khanakia/gqlkit/gqlkit)](https://goreportcard.com/report/github.com/khanakia/gqlkit/gqlkit) [![test](https://github.com/khanakia/gqlkit/actions/workflows/test.yml/badge.svg)](https://github.com/khanakia/gqlkit/actions/workflows/test.yml) [![lint](https://github.com/khanakia/gqlkit/actions/workflows/lint.yml/badge.svg)](https://github.com/khanakia/gqlkit/actions/workflows/lint.yml) ![coverage](https://img.shields.io/badge/coverage-91%25-brightgreen) [![release](https://img.shields.io/github/v/tag/khanakia/gqlkit?sort=semver&filter=gqlkit@*&label=release)](https://github.com/khanakia/gqlkit/tags) [![license](https://img.shields.io/badge/license-MIT-blue)](./LICENSE) ![Go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)

Generate fully typed GraphQL client SDKs for **Go** and **TypeScript** from any GraphQL schema. Built on a **builder pattern** with type-safe field selection — only the fields you select appear in the return type.

**Schema-first, not query-first:** generate builders once, then compose queries and mutations in code with dynamic, compile-time-checked field selection — one shared type per GraphQL type, no per-query codegen loop. Works with any GraphQL API, including large real-world schemas (Shopify Admin, Hasura, Apollo Federation).

## Contents

- [Install](#install)
- [Quick Start — Go](#quick-start--go)
- [Quick Start — TypeScript](#quick-start--typescript)
- [Why GQLKit?](#why-gqlkit)
- [Key Features](#key-features)
- [FAQ](#faq)
- [Tools](#tools)
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

### Comparison

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

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for development setup, architecture, and how to run the examples.
