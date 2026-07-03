# Contributing to GQLKit

Thanks for your interest in improving **gqlkit**. By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        gqlkit (core)                         │
│                                                              │
│  GraphQL SDL ──→ schemagql ──→ typegql ──→ clientgen (Go)    │
│                                       └──→ clientgents (TS)  │
│                                                              │
│  Runtime: builder + graphqlclient (Go), gqlkit-ts (TS)       │
└─────────────────────────────────────────────────────────────┘

┌──────────────┐   introspection   ┌──────────────┐
│  gqlkit-sdl  │ ────────────────→ │ .graphql SDL  │
│  (CLI tool)  │   fetch + convert │   (schema)    │
└──────────────┘                   └──────┬───────┘
                                          │
                              ┌───────────┴───────────┐
                              ▼                       ▼
                    Go SDK (example-go-*)    TS SDK (example-ts)
```

## Modules

| Module | Description |
|--------|-------------|
| [gqlkit](./gqlkit) | Core SDK generator — parses schema, generates Go and TypeScript client code |
| [gqlkit-ts](./gqlkit-ts) | TypeScript runtime library (npm) — `GraphQLClient`, `BaseBuilder`, `FieldSelection` |
| [gqlkit-sdl](./gqlkit-sdl) | CLI tool — fetches GraphQL schema via introspection, outputs SDL |
| [mockapi](./mockapi) | Test GraphQL API (gqlgen) — todo/user CRUD with filtering and pagination |
| [example-go-chat](./example-go-chat) | Example: Go SDK from a production chatbot schema (~50 queries, ~57 mutations) |
| [example-go-mockapi](./example-go-mockapi) | Example: Go SDK from the test API |
| [example-ts](./example-ts) | Example: TypeScript SDK from the test API |

## Requirements

* Go 1.25+
* Node.js 18+ (for TypeScript SDK)
* [Task](https://taskfile.dev) (optional, for task runner)

## Development — TypeScript SDK

Generates a fully typed TypeScript SDK from a GraphQL schema. Returns **only the selected fields** — unselected fields are compile-time errors.

Full technical docs: [example-ts/DOCS.md](./example-ts/DOCS.md)

### 1. Start the GraphQL server

```bash
cd mockapi
go run server.go
# → running on http://localhost:8081/query
```

### 2. Build the TypeScript runtime library

```bash
cd gqlkit-ts
npm install
npm run build
```

### 3. Install TypeScript dependencies

```bash
cd example-ts
npm install
```

### 4. Build the Go generator

```bash
cd example-ts
go build ./cmd/generate/
```

### 5. Generate the TypeScript SDK

```bash
cd example-ts
go run cmd/generate/main.go
```

Output goes to `example-ts/sdk/` (~30 `.ts` files).

### 6. Type-check the generated SDK

```bash
cd example-ts
npx tsc --noEmit
# no output = no errors
```

### 7. Run sample queries against the live server

```bash
cd example-ts
npm run samples
```

### Quick re-test (after changing generator code)

```bash
cd example-ts
task example-ts:test
# or manually:
go build ./cmd/generate/ && rm -rf sdk && go run cmd/generate/main.go && npx tsc --noEmit
```

### Tasks

```bash
task example-ts:setup           # Steps 2-6 in one command (first-time)
task example-ts:test            # Build + vet + clean generate + typecheck
task example-ts:generate        # Generate SDK
task example-ts:generate:clean  # rm -rf sdk + generate
task example-ts:typecheck       # tsc --noEmit
task example-ts:run             # Run samples
task gqlkit-ts:setup            # Install + build runtime library
```

## Development — Go SDK

### Generate the Go SDK

```bash
cd example-go-chat
go run ./cmd/generate
```

### End-to-end with test API

```bash
# 1) Start API (in one terminal)
task mockapi:run

# 2) Fetch schema from test API
task example-go-mockapi:fetch-schema

# 3) Generate SDK
task example-go-mockapi:generate

# 4) Run sample queries
task example-go-mockapi:run
```

## Testing

Each Go module is tested in isolation (the repo uses a `go.work`, so tests run with `GOWORK=off`):

```bash
cd gqlkit     && GOWORK=off go test ./...
cd gqlkit-sdl && GOWORK=off go test ./...
cd gqlkit-ts  && npm test
```

The generator has a two-layer test suite in `gqlkit/pkg/clientgen` and `gqlkit/pkg/clientgents`: targeted unit tests on the type-mapping functions **plus** end-to-end guards that generate an SDK and then `go build` / `tsc` it. New generator behavior should ship with both where it makes sense. **After any template or generator change, regenerate the examples and confirm they build** (`task example-ts:test`, and rebuild `example-go-mockapi/sdk`) — this catches template regressions the unit tests can't.

## Ways to contribute

- **Report a bug** — open an issue using the bug template; include a minimal schema, the exact command, your OS + `gqlkit --version`, and what you expected.
- **Request a feature** — open an issue using the feature template; describe the problem first, then your proposed solution.
- **Send a pull request** — for anything non-trivial, open an issue first so we can agree on the approach.

## Branches & commits

- Branch off `main`. Use a short descriptive branch name (`fix/...`, `feat/...`, `docs/...`).
- **Commit subjects are sentence case, ≤70 characters, with no prefix.** This repo does **not** use Conventional Commits (`feat:` / `fix:` / `chore:`) — mixing styles pollutes the changelog. Explain the *why* in the body and group by subsystem when a change spans more than one.
- Keep commits focused; one logical change per commit where practical.

## Pull request checklist

- [ ] The change is scoped and described (link the issue it closes).
- [ ] `go test ./...` (each Go module) and `npm test` (gqlkit-ts) pass; `gofmt` and `go vet` are clean.
- [ ] New behavior has tests where it makes sense.
- [ ] Generator/template changes were verified by regenerating the examples.
- [ ] The affected artifact's `CHANGELOG.md` is updated under `[Unreleased]` for user-facing changes.
- [ ] No unrelated files or formatting churn.

## Changelog & releases

The three shippable artifacts (`gqlkit`, `gqlkit-sdl`, `gqlkit-ts`) version independently and each keep their own `CHANGELOG.md` — there is intentionally no combined top-level changelog. Put user-facing changes under `[Unreleased]` in the affected artifact's changelog, following [Keep a Changelog](https://keepachangelog.com/). Releases are tag-triggered (`gqlkit@vX.Y.Z` / `gqlkit-sdl@vX.Y.Z` build via GoReleaser; `gqlkit-ts` publishes to npm); the changelog edit must land before the tag. Maintainers handle tags.

## Questions

Open a [discussion or issue](https://github.com/khanakia/gqlkit/issues). We're happy to help you land your first contribution.
