# example-ts — running and testing the example

This page is the checklist for building, regenerating and testing the TypeScript example. What the SDK does and how to use it is in the guides under [docs/typescript](../docs/typescript/client.md); how the generator works inside is in the contributor reference, [TypeScript codegen](../docs/typescript-codegen.md). An overview of the folder is in the [README](README.md).

## Prerequisites

- Go 1.25+ (the generator is a Go program, even for the TypeScript SDK)
- Node.js 18+ with npm
- [Task](https://taskfile.dev/) for the one-line commands (optional)
- For the samples that call a server: mockapi running, started with `task mockapi:run` from the repo root (port 8081)

## First-time setup

From the repo root:

```bash
task example-ts:setup
```

That builds the `gqlkit-ts` runtime, installs this folder's dependencies, generates `sdk/` and type-checks everything. The same steps by hand:

```bash
cd gqlkit-ts && npm install && npm run build
cd ../example-ts && npm install
go run ./cmd/generate
npx tsc --noEmit
```

The example depends on `gqlkit-ts` through `file:../gqlkit-ts`, which resolves to its built `dist/`. After changing anything in `gqlkit-ts/src`, run `npm run build` there again, or this folder type-checks against the old runtime.

## Regenerate the SDK

```bash
task example-ts:generate          # regenerate in place
task example-ts:generate:clean    # delete sdk/ first, then regenerate
```

`cmd/generate/main.go` reads `cmd/generate/schema.graphql` (a copy of mockapi's schema) and `cmd/generate/config.jsonc` (scalar bindings) and writes `sdk/`. Never edit `sdk/` by hand.

## Test

| Command | Needs mockapi | What it proves |
| --- | --- | --- |
| `task example-ts:test` | no | the generator builds and vets, a clean regenerate succeeds, and the SDK plus every file in `cmd/samples` type-checks (including the narrowing checks in `type-test.ts`) |
| `npx tsc --noEmit` | no | type-check only, without regenerating |
| `npx tsx cmd/samples/run-all.ts` | yes | every guide sample runs and its `node:assert` checks pass; runs them all and exits non-zero if any failed |
| `npm run samples` | yes | the tour in `main.ts` runs (it prints errors but does not fail on them) |
| `npx tsx cmd/samples/<name>.ts` | yes | one guide sample |

After any change to the TypeScript templates in `gqlkit/pkg/clientgents`, run `task example-ts:test`, then `run-all.ts` with mockapi up.

## Type-safe field selection

Moved to [Selecting fields](../docs/typescript/selecting-fields.md), which shows the compile errors you get for unselected fields and how lists, nullable results and nested objects are typed. The generator side (generic field selectors and builders) is in [TypeScript codegen: Field Selectors](../docs/typescript-codegen.md#6-field-selectors).

### Behavior in practice

The examples that used to be here are now compiled on every `task example-ts:test` in [cmd/samples/type-test.ts](cmd/samples/type-test.ts), and explained in [Selecting fields](../docs/typescript/selecting-fields.md). One thing differs from the old text: calling `.execute()` on an object-returning builder without `.select()` no longer compiles (the builder starts as `SelectionRequired`), because the server would reject the query.

## Troubleshooting

- **`mockapi is not reachable at http://localhost:8081/query`** — start it with `task mockapi:run` from the repo root.
- **`Module '"gqlkit-ts"' has no exported member ...`** — the runtime's `dist/` is older than its source; run `npm run build` in `gqlkit-ts`.
- **`DeprecationWarning: module.register() is deprecated`** — printed by `tsx` on recent Node versions; harmless and unrelated to gqlkit.
- **Ids in sample output differ from the guides** — mockapi keeps its data in memory and never reuses an id, so every run that creates records gets new ids. Restart mockapi to reset it.
