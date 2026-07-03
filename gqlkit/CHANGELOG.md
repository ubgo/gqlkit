# Changelog — gqlkit (CLI)

All notable changes to the `gqlkit` binary. Format: [Keep a Changelog](https://keepachangelog.com/), versioning: [SemVer](https://semver.org/).

Tagged as `gqlkit@vX.Y.Z`. See releases at <https://github.com/khanakia/gqlkit/releases>.

## [Unreleased]

### Fixed
- `generate` (Go): the `-m` / `--module` value was dropped, so cross-package imports in the generated SDK came out as `import ( "" )` / `import ( "/types" )` and the SDK didn't compile. `--module` is now threaded into every local package import (`types`, `enums`, `scalars`, `inputs`, `fields`, `builder`, `graphqlclient`, `batch`), each emitted as `<module>/<pkg>`. `go build ./...` on the generated SDK now succeeds. The `cmd/generate` programmatic API (which sets `Package` directly) was unaffected. ([#4](https://github.com/khanakia/gqlkit/issues/4))
- `generate` (Go): object-typed struct fields were emitted by value, so any GraphQL object cycle (e.g. Shopify's `ProductVariant` → `QuantityRule` → `ProductVariant`) produced an `invalid recursive type` build error. Object and input-object fields are now always pointers (`*T`, and `[]*T` in lists), which both breaks the value cycle and models nullable objects. ([#4](https://github.com/khanakia/gqlkit/issues/4))
- `generate` (Go): schemas whose query root is named non-conventionally (e.g. Shopify's `QueryRoot`) leaked the introspection meta-fields `__schema` / `__type` into the generated `types.go`, referencing the ungenerated builtin `__Schema` / `__Type` types (`undefined: __Schema`). All `__`-prefixed meta-fields are now excluded from generated structs and their import collection. ([#4](https://github.com/khanakia/gqlkit/issues/4))
- `generate` (Go): GraphQL type names that aren't capitalized generated **unexported** Go identifiers, undefined when referenced from another generated package (`undefined: scalars.timestamptz`, `enums.order_by`). Hasura schemas (`timestamptz`, `uuid`, `jsonb`, `order_by`) and Apollo Federation types (`_Service`, `_Entity`) hit this. Generated Go type identifiers are now always exported — the first letter is capitalized and leading underscores are stripped (`timestamptz` → `Timestamptz`, `_Service` → `Service`) at every definition and reference site. GraphQL field/JSON names are unchanged; already-exported names (`JSON`, `DateTime`, `ID`) are untouched. ([#4](https://github.com/khanakia/gqlkit/issues/4))
- `generate` (Go): placeholder fields named `_` (the Apollo Federation empty-type marker, `_: Int`) produced an empty Go identifier — `func (q *QueryRoot) () *Builder`, a syntax error. Fields whose name yields no valid Go identifier are now skipped. ([#4](https://github.com/khanakia/gqlkit/issues/4))
- `generate-ts` (TypeScript): the same `_` placeholder field produced a broken operation builder — `queries/_.ts` with a generic `Builder` class (colliding across multiple placeholders) and an empty GraphQL operation name. Such fields are now skipped, mirroring the Go generator. Non-conventional query-root names (e.g. `QueryRoot`) and Hasura/Federation lowercase type names are valid TypeScript and continue to generate as before.
- Generator internals: a scalar binding in `config.jsonc` with an empty or unresolvable `model` used to panic the generator (nil dereference in the Go-type resolver). It now degrades to `any` / a same-package type. `ToSnakeCase` (used for generated file names) is now UTF-8 safe.

### Changed
- `generate` (Go): generated `types.go` now emits object/input-object fields as pointers (see Fixed above). This changes the shape of generated structs — regenerate consumers after upgrading.

## [0.9.0] — 2026-05-10

### Added
- Generated Go SDKs now ship a `batch/` package — `RunQueries` / `RunMutations` merge multiple builders into a single GraphQL operation with aliased root fields, decoded into one struct via `json` tags. One HTTP round trip for N queries.
- `OpFragment` + `BaseBuilder.GetOpFragment(alias)` on the runtime builder so each generated op can be spliced into a batched document.
- `QueryMarker` / `MutationMarker` zero-size embeds emitted by the operation template — give the type system a compile-time discriminator so mixing query + mutation builders in `RunQueries` is rejected at compile time.
- `ExecuteWithPartialData` on the generated `graphqlclient.Client` so partial-success batches still populate the destination struct alongside any GraphQL errors.

### Changed
- Generated query / mutation builders now embed a kind marker (`builder.QueryMarker` for queries, `builder.MutationMarker` for mutations) in addition to `*BaseBuilder`. Existing call sites are unaffected; the marker only matters when passing a builder to the new `batch` package.
- Generator orchestration adds a `generateBatchFiles` step (writes `<sdk>/batch/batch.go`).

### Documentation
- New "Batching multiple queries in one request" section in `docs/getting-started-go.md`.
- New `gqlkit/pkg/batch/README.md` documenting the upstream and generated faces of the package.
- WHY-comments across the new code explaining the marker pattern, partial-tolerant decoding, deterministic alias sort, and the optional `partialExecutor` interface.

## [0.8.0] — 2026-04-27

### Fixed
- Self-referencing object fields (e.g. `Item.parent: Item`, `KpiSnapshot.comparison: KpiSnapshot`) were generating as scalar leaves — `addField("parent")` with no selector callback — producing invalid GraphQL when used. The over-eager `baseName != def.Name` guard in `clientgents/field_sel_gen.go` and `clientgen/field_sel_gen.go` is removed; self-imports remain skipped (the only thing the guard was actually needed for). Affects both Go and TypeScript SDK generators.

## [0.7.0] — 2026-03-22

### Added
- `gqlkit-sdl` companion CLI gains a `--format json` flag for JSON SDL output (paired release).

### Changed
- Documentation clarifies the `--package` flag — passing a full import path (`example/sdk`) sets both the Go package name and the import root.
- `example-go-chat` updated to track current generator output.

## [0.6.0] — 2026-03-18

### Added
- `--package` flag accepts a full Go import path. The generator splits it into a package name (last segment) and import path (all of it), so generated `import "<root>/builder"` etc. stay consistent across consumers.

## [0.5.0] — 2026-03-18

### Added
- Auto-detect SDK import path from the consumer's `go.mod`. Eliminates the need to pass `--package` for the common case where the generated SDK lives inside the same module that's invoking `go run`.

## [0.4.0] — 2026-03-18

### Added
- `graphqlclient` package is now generated *into* the SDK — generated SDKs are fully self-contained, no runtime dependency on the gqlkit module.
- `--config` flag is now optional. When omitted, gqlkit ships with a sensible default scalar binding set (built-in primitives only).

### Changed
- Generated imports use full GitHub module paths (`github.com/.../sdk/builder`) instead of relative paths.

## [0.3.0] — 2026-03-18

### Added
- `-o` shorthand for `--output`, `-c` shorthand for `--config`.

### Changed
- `--config` flag is optional (default scalar bindings used when absent).

## [0.2.0] — 2026-03-18

### Added
- TypeScript codegen support — `gqlkit generate-ts` emits a typed builder SDK consuming the [`gqlkit-ts`](https://www.npmjs.com/package/gqlkit-ts) npm runtime.
- Custom scalar examples — `Cursor`, `DateTime`, `JSON`, `Metadata` — wired through mockapi and the example SDKs.
- AI-friendly documentation (`docs/ai-friendly.md`) explaining why the builder pattern works well for AI-assisted coding.
- "Comparison vs genqlient / GraphQL Code Generator" section in the README.
- `CONTRIBUTING.md` with development setup.

## [0.1.0] — 2026-03-17

### Added
- Initial release. Cobra CLI with `generate` and `version` commands.
- GoReleaser-driven monorepo release workflow — tags of the form `gqlkit@vX.Y.Z` build via `.goreleaser.yml`; `install.sh` pulls the latest matching release.
- Stable download URLs (no version in artifact names).
- Generated SDK structure: `types/`, `enums/`, `inputs/`, `scalars/`, `fields/`, `queries/`, `mutations/`, `builder/`.

[0.9.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.9.0
[0.8.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.8.0
[0.7.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.7.0
[0.6.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.6.0
[0.5.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.5.0
[0.4.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.4.0
[0.3.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.3.0
[0.2.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.2.0
[0.1.0]: https://github.com/khanakia/gqlkit/releases/tag/gqlkit%40v0.1.0
