# gqlkit or a query-first generator

Most GraphQL code generators are query-first: you write each operation as a `.graphql` document, run the generator, and get a type for that operation. [genqlient](https://github.com/Khan/genqlient) (Go) and [GraphQL Code Generator](https://the-guild.dev/graphql/codegen) (TypeScript) are the common ones. gqlkit is schema-first: it generates a builder for every operation in the schema once, and you choose fields in code at the call site. Neither approach is better everywhere. This page helps you decide which fits your project.

## Before you start

Look at how your app uses the API: how many distinct operations, how often field selections change, whether the same entity (a `User`) is passed between layers, and whether you need Go, TypeScript or both.

## Decision table

| If your project... | Lean towards | Because |
| --- | --- | --- |
| Composes queries across many call sites, with selections that change often | gqlkit | A new selection is a code change, not a new document plus a codegen run. |
| Passes one entity type between layers (`User` from three different queries into one function) | gqlkit | One Go/TS type per GraphQL type. Query-first tools generate one type per operation path (`GetUserUser`, `GetViewerViewerUser`). |
| Needs Go and TypeScript clients from one schema | gqlkit | One generator, the same builder shape in both. |
| Starts from a live endpoint with no SDL on disk, or a huge vendor schema | gqlkit | `gqlkit-sdl` fetches by introspection and can filter and prune before generating. |
| Has a small, fixed set of operations that rarely change | query-first | The documents are the source of truth, and each type is shaped exactly to its operation. |
| Wants the server to validate the exact documents at build time, or uses persisted queries | query-first | The documents exist ahead of time; a builder assembles them at run time. |
| Relies on named fragments, directives (`@include`, `@skip`) or subscriptions in client code | query-first | gqlkit builders have no named-fragment or directive API (inline fragments only, through the `On<Type>` / `on<Type>` selectors on unions and interfaces), and the generators emit no subscription builders. |
| Needs exact result typing in Go (only the selected fields exist) | query-first | gqlkit's Go results are the full struct with zero values for unselected fields; only gqlkit's TypeScript SDK narrows. |

## Side by side

| | gqlkit | genqlient (Go) | GraphQL Code Generator (TS) |
| --- | --- | --- | --- |
| Input | Schema SDL | Schema SDL plus `.graphql` operations | Schema plus operations in code or files |
| Field selection | At the call site, in code | Fixed per operation at generation time | Fixed per operation at generation time |
| Types | One per GraphQL type | One per operation path | One per operation (plus shared base types depending on plugins) |
| Adding a query | Call the builder | Write the operation, regenerate | Write the operation, regenerate |
| Result typing | TS: narrowed to the selection. Go: full struct | Exactly the selection | Exactly the selection |
| Schema fetching | Built in (`gqlkit-sdl`) | Bring your own SDL | Supported through config |
| Languages | Go and TypeScript | Go | TypeScript / JavaScript |

## Try it

Generate both SDKs and look at what a new query costs: [`example-cli/generate-go.sh`](../../example-cli/generate-go.sh) and [`example-cli/generate-ts.sh`](../../example-cli/generate-ts.sh) each write a small program against the generated SDK; adding a field to its `.select(...)` is the whole change, with no regeneration.

## Pitfalls

- **"Type-safe selection" means different things in Go and TypeScript in gqlkit.** See [Go or TypeScript](./go-vs-typescript.md) before you rely on it.
- **A builder can still send an invalid document** for things the schema types do not express, such as server-side complexity limits. Query-first tools that validate documents at build time catch some of these earlier.
- **Mixing both approaches is possible.** gqlkit's generated client also runs raw documents (`ExecuteRaw`, `RawQuery` in Go; `executeRaw`, `client.execute` in TypeScript) for the odd operation the builders do not cover.

## Related

- [README: Why GQLKit?](../../README.md#why-gqlkit)
- [Go or TypeScript](./go-vs-typescript.md)
- [Feature reference](../reference/features.md)
- [Which method to call](./execute-methods.md): raw documents next to the builders
- [Documentation index](../README.md)
