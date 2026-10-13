# Changelog — gqlkit (CLI)

All notable changes to the `gqlkit` binary. Format: [Keep a Changelog](https://keepachangelog.com/), versioning: [SemVer](https://semver.org/).

Tagged as `gqlkit@vX.Y.Z`. See releases at <https://github.com/ubgo/gqlkit/releases>.

## [Unreleased]

### Added
- `generate-ts` (TypeScript): generated operation builders' `execute()` and `executeRaw()` take an optional `RequestOptions` (`{ signal }`) and pass it to the client, so a request can be cancelled with an `AbortController`. An already-aborted signal sends nothing. `RequestOptions` is re-exported from `builder/index.ts`. This is additive: existing calls are unchanged.
- `generate-ts`: `--prune` deletes leftovers of removed types and operations from the output directory, and `--dry-run` lists what `--prune` would delete without deleting anything. This is the same rule as `generate`: only regular files whose first line is gqlkit's generated banner and that the current run did not write. Hand-written files, symlinks and files from other tools are never touched. Generated subdirectories left empty (e.g. `mutations/` after the schema drops its mutation root) are removed. `--dry-run` without `--prune` is rejected. Every generated `.ts` file, including `builder/index.ts`, now starts with the banner.
- `generate-ts` (TypeScript): `config.jsonc` can loosen how nullable output fields are typed, for code migrating from SDKs that typed them as `field?: T`. Set `"nullable": "null-undefined"` to type nullable output fields, selector results and operation returns as `T | null | undefined`. The default `"null"` keeps `T | null`. `"nullableOverrides"` sets the mode per type (`"Todo"`) or per field (`"Todo.priority"`, `"Query.now"`); a field key beats a type key, and a type key beats the global setting. List elements always stay `T | null`, and inputs and arguments are not affected. An unknown mode, or an override naming no object or interface type or field, is rejected.
- `generate-ts` (TypeScript): scalar codecs. Before, a binding only changed the TS type, so a `DateTime` bound to luxon still arrived as a string. A binding can now name functions that convert values: `"DateTime": { "type": "DateTime", "import": "luxon", "decode": "fromISO", "encode": "toISO", "codecImport": "../../codecs.js" }`. `decode` runs on every value of that scalar in a response: selected fields at any list depth, union and interface fragments, and scalar operation results. `encode` runs on every value sent: scalar arguments and codec fields of input objects, including nested and recursive inputs, through a generated `<Input>EncodePlan`. `codecImport` is a module path resolved from the generated `scalars/` directory, like `import`. A codec without a module, a module without a codec, or a name that is not a plain identifier is rejected when the config loads. When a single operation fails with `GraphQLErrors` that carry partial data, that `err.data` is decoded too before the error is rethrown. If the partial data itself fails to decode, `err.data` is set to `undefined` and `err.decodeError` holds the `ScalarCodecError`. A schema with no codec generates exactly the same SDK as before. Requires the matching gqlkit-ts runtime.
- `generate-ts` (TypeScript): GraphQL unions and interfaces can now be selected. Each one gets a `fields/<type>.ts` selector with an `on<Type>()` method per member or implementation, which adds an inline fragment (`... on Todo { … }`). `__typename` is always requested, so results are a discriminated union you can narrow with `switch (r.__typename)`. Operations and fields returning a union or interface get `select()` / a selector callback like object types. An operation executed without `select()` sends `{ __typename }`, so it is still a valid query (before, `search` with no selection set was rejected by the server). Shared interface fields apply to every member. `types/index.ts` now has `export type SearchResult = Todo | User;` instead of `any`. An interface field that clashes with an `on<Type>()` method gets an `_` suffix.
- `gqlkit generate --prune` deletes files in the output directory that start with the gqlkit generated-code banner but were not produced by this run (e.g. removed types or operations); files without the banner are never touched. It also removes generated subdirectories the prune leaves empty (e.g. `mutations/` after the Mutation root is removed); directories holding anything else are kept. `--dry-run` (with `--prune`) lists them without deleting.
- Generated `graphqlclient.HTTPError{StatusCode, Body}` for non-200 responses whose body is not a GraphQL response.
- `batch.ErrInvalidAlias`, `batch.ErrVariableCollision`, `batch.ErrNilBuilder` returned before any request is sent.
- Nullable input fields can be generated as `inputs.Opt[T]`, which has three states: unset (omitted), `inputs.Null[T]()` (sent as `null`) and `inputs.Some(v)`. It is the only way to clear a field in an update mutation. Configure it in config.jsonc with `"inputs": {"optional": "pointer" | "opt", "overrides": {"Type": mode, "Type.field": mode}}`; a field override beats a type override, which beats the global setting. The default stays `pointer` for this release; `opt` is recommended for new SDKs. Unknown modes, types and fields are rejected. Every SDK now ships `inputs/opt.go`, which needs Go 1.24+ (`omitzero`).
- GraphQL interfaces and unions are now fully usable. Each one gets a `<Name>Value` wrapper whose `Value` is the concrete `*T`, picked by `__typename`. An unknown `__typename` does not fail: `Value` is nil and `Typename` and `Raw` keep the server's JSON. Member types implement `Is<Name>()`. Selectors always request `__typename` and gain `On<Type>(func(*TypeFields))`, which emits an inline fragment `... on Type { ... }`. Query results and fields of these types return the wrapper.
- Generated Go SDKs now enforce Go 1.24+: `builder/` and `inputs/` carry a `go_version_guard.go` (`//go:build !go1.24`) that fails an older toolchain's build with `undefined: gqlkit_SDK_requires_Go_1_24_or_newer` instead of letting `omitzero` be silently ignored.

### Changed
- `generate-ts` (TypeScript): an operation that returns an object type can no longer run without `select()`. The server rejects it ("must have a selection of subfields"), and before this change it compiled and failed at runtime. The builder's result type now starts as `SelectionRequired`, and `execute()`, `executeRaw()` and `getOpFragment()` are a compile error (TS2684) until `select()` is called. An empty selection (`select(f => f)`), plain-JS callers and `batch()` get a clear client-side error before any request is sent. Union and interface returns stay valid without `select()` (they send `{ __typename }`). Operation files no longer import the return type from `../types`.
- `generate-ts` (TypeScript): selections on an interface now always include `__typename`, and the selector's default result type is the discriminated union of its implementations (`{ __typename: "Todo" } | { __typename: "User" }`) instead of `{}`. Code that annotated an interface result without `__typename` still compiles. Code that relied on a union being `any` now gets real types.
- `generate-ts` (TypeScript): generated types now say where the server can send `null`. This is a type-level breaking change; release it as a minor version bump. Nullable output and input fields are `field?: T | null` in `types/index.ts` and `inputs/index.ts` (they were `field?: T`). A field picked with a selector is always present, so its narrowed result is `field: T | null`, not optional. Nullable list elements are `(T | null)[]` (`[Int]` was `number[]`). A non-`select()` operation with a nullable return resolves to `Promise<T | null>`. Nullable arguments and input fields accept an explicit `null`, so a value can be cleared. Code that annotates these values as `T | undefined`, or compares them with `=== undefined`, now fails `tsc`. Use `T | null` or `== null` instead. Binding text that ends in an arrow or conditional type is parenthesised (`(() => string) | null`) so `| null` is not absorbed into it.
- `generate` now fails when the SDK import path cannot be determined (no `--module` and no `go.mod` at or above the working directory), instead of emitting unbuildable `"/types"` imports; `go.mod` is now found in parent directories too.
- `generate` and `generate-ts` reject stray positional arguments (an unquoted `-s ./schema/*.graphql` expanded by the shell) with a hint to quote the glob, and fail when an explicit `-c` file does not exist.
- Every generated Go file now starts with `// Code generated by gqlkit. DO NOT EDIT.` (builder, graphqlclient and batch had no banner; the others wrongly said "ent").
- Type, enum, input and interface descriptions are emitted as attached doc comments; a query/mutation field's description becomes its builder's doc comment.
- In the default `pointer` mode, a non-nil empty slice in a nullable list input field is now sent as `[]` (it used to be dropped) when the SDK is built with Go 1.24+, via a generated `inputs/inputs_json.go` (`//go:build go1.24`). A nil slice is still omitted. With an older toolchain the struct tags are unchanged (`omitempty`), so an unset field is never sent as `null`.
- **Breaking (generated API):** results and fields of interface or union type change from `types.<Iface>` / `interface{}` (which could not be decoded) to `types.<Name>Value`.
- Go identifiers that would collide are suffixed with `_` in schema declaration order, so the SDK compiles. Cases covered: an argument named like a builder method (`select` → `Select_()`, also `execute`, `buildQuery`, `getClient`, ...), two arguments, fields, root operations or files that PascalCase to the same name (`user_name` / `userName` → `UserName` / `UserName_`), a field shadowing a generated `Is<Iface>()` / `On<Type>()` / `MarshalJSON` method, and enum values that differ only by case (`ACTIVE` / `active`) or clash with an enum type name. GraphQL names on the wire are unchanged; schemas without collisions generate exactly as before.

### Fixed
- `generate-ts` (TypeScript): the generated SDK now compiles under Node's native ESM (`"type": "module"` with `moduleResolution` `node16`/`nodenext`). Before, every relative import failed with TS2835. Relative imports now carry explicit `.js` extensions, and directory imports point at the barrel file (`../enums/index.js`, `./user.js`). This works unchanged under `bundler` and `commonjs`/`node10` too. Paths you write yourself in a binding's `import` are emitted verbatim, so under Node ESM write them with the extension (`"./brand.js"`).
- `generate-ts` (TypeScript): a scalar binding with no TypeScript `type` is now rejected with an error that names the scalar and shows the TS form. The usual case is a Go-generator binding such as `{ "model": "time.Time" }` copied into the TS config. Before, it generated `export type DateTime = ;` and exited 0.
- `generate-ts` (TypeScript): a scalar binding that is a union, intersection, function or other compound type (e.g. `"ID": "string | number"`) lost its meaning inside a GraphQL list, because `[]` was appended straight onto the binding text: `[ID!]` rendered as `string | number[]` (a string, or an array of numbers) instead of `(string | number)[]`. Such element types are now parenthesised in object and input fields, field selectors, operation arguments and operation return types, including nullable and nested lists. Simple bindings (`string`, `Record<string, unknown>`, `Array<string | number>`) render unchanged. ([#5](https://github.com/ubgo/gqlkit/issues/5))
- `generate-ts` (TypeScript): operation arguments and return types of an unbound custom scalar (e.g. `when(at: DateTime!): DateTime` with no `DateTime` binding) were typed `any` while the operation file still imported the `DateTime` alias, which left an unused import that fails `tsc` under `noUnusedLocals`. They now use the scalar alias, the same type object and input fields already used. A binding that is a bare type name without `import` (e.g. `"DateTime": "Date"`) and a redeclared `String` scalar with an `import` binding are rendered the same way.
- `generate-ts` (TypeScript): a schema field, argument or operation whose name matched a member of the generated class no longer breaks the SDK. Before, a field named `constructor` or `selection`, an argument named `builder`, `select`, `execute`, `executeRaw`, `getOpFragment` or `constructor`, or a query named `client` produced duplicate-member or second-constructor errors in `tsc`. Generated classes now keep their state in `#private` fields, and a schema name that equals one of the class's own methods gets an `_` suffix (`select` argument → `select_(v)`, `constructor` field → `constructor_()`). Names that don't clash are unchanged. Generated selectors and `select()` also no longer use `return this as any`; they return a correctly typed instance that shares the same selection.
- Generated SDK did not compile for non-null enum, interface or custom-scalar query results, or scalars bound to non-struct types such as `int16` / `encoding/json.Number` ("invalid composite literal").
- A binding whose package name ends in `types`/`inputs` (e.g. `dbtypes.Money`) added an unused import of the SDK's own `types`/`inputs` package.
- Two scalars bound to the same package (`Time` and `DateTime` → `time.Time`) produced a duplicate import; interface fields and `_` placeholder input fields could leave unused imports.
- Bindings from one `clientgen.New` call leaked into later generators in the same process.
- `Client.RawQuery` always returned an empty payload.
- GraphQL errors sent with a non-200 status (gqlgen's 422 validation errors) are now returned as `GraphQLErrors` instead of an opaque status error.
- `batch.RunQueries`/`RunMutations` silently merged two aliases whose namespaced variables collided (alias `a` + arg `b_c` vs alias `a_b` + arg `c`); invalid aliases and nil builders are now rejected.
- Union-typed fields were generated as leaves without a selection set, so the server rejected every query that selected them.
- Interface-typed query results failed to decode (`cannot unmarshal object into ... types.Node`).
- Bindings with pointer or slice prefixes (`*math/big.Int`, `[]*github.com/x/y.T`) produced an invalid import path (`"*math/big"`); the prefixes now stay on the Go type and the import is the package path. A nullable field of a pointer-bound scalar is no longer double-pointered.

### Removed
- Unused exported Go symbols in `clientgen`: `InputTypeDef`, `InputFieldDef`, `InputTypeDefList`, `TypeDefList`, `ErrSchemaNotFound`, `ErrSchemaParseFailed` (no callers anywhere in the repo).

### Documentation
- New documentation index (`docs/README.md`) with a twelve-step learning path, a "start from your goal" table and an index of every guide and runnable example.
- Step-by-step Go guides (`docs/go/`: client, selecting fields, arguments and inputs, types and scalars, errors, batching, low-level builder), each backed by a runnable program under `example-go-mockapi/cmd/`.
- CLI guides (`docs/cli/`: generate Go, generate TypeScript, config.jsonc, fetch schema, filter schema, programmatic use) and the `example-cli/` scripts.
- Decision guides (`docs/choosing/`): query-first vs gqlkit, Go vs TypeScript, schema source, CLI vs Go program, scalar bindings, client setup by app type, which execute method, single vs batch, when to regenerate.
- The README no longer says an unselected field is a compile error in Go (only the TypeScript SDK narrows), no longer tells Go SDK users to `go get` gqlkit or import `gqlkit/pkg/graphqlclient` (the generated SDK is self-contained), passes `-m "$(go list -m)/sdk"`, and documents `batch.RunMutations`.
- The `gqlkit` module README was rewritten: both commands' flags, config, generated layout, Go API and pitfalls.
- Docs cover union/interface selectors, Opt[T] inputs, --prune/--dry-run, and the TypeScript null-mode and codec config.

## [0.13.0] — 2026-10-13

### Changed
- The Go module path is now `github.com/ubgo/gqlkit/gqlkit` (was `github.com/khanakia/gqlkit/gqlkit`). Update imports, `go get` and `go install` lines to the new path. Generated Go SDKs now import `github.com/ubgo/gqlkit/...`.
- Licensed under Apache-2.0 (was MIT).

## [0.10.0] — 2026-07-03

### Fixed
- `generate` (Go): the `-m` / `--module` value was dropped, so cross-package imports in the generated SDK came out as `import ( "" )` / `import ( "/types" )` and the SDK didn't compile. `--module` is now threaded into every local package import (`types`, `enums`, `scalars`, `inputs`, `fields`, `builder`, `graphqlclient`, `batch`), each emitted as `<module>/<pkg>`. `go build ./...` on the generated SDK now succeeds. The `cmd/generate` programmatic API (which sets `Package` directly) was unaffected. ([#4](https://github.com/ubgo/gqlkit/issues/4))
- `generate` (Go): object-typed struct fields were emitted by value, so any GraphQL object cycle (e.g. Shopify's `ProductVariant` → `QuantityRule` → `ProductVariant`) produced an `invalid recursive type` build error. Object and input-object fields are now always pointers (`*T`, and `[]*T` in lists), which both breaks the value cycle and models nullable objects. ([#4](https://github.com/ubgo/gqlkit/issues/4))
- `generate` (Go): schemas whose query root is named non-conventionally (e.g. Shopify's `QueryRoot`) leaked the introspection meta-fields `__schema` / `__type` into the generated `types.go`, referencing the ungenerated builtin `__Schema` / `__Type` types (`undefined: __Schema`). All `__`-prefixed meta-fields are now excluded from generated structs and their import collection. ([#4](https://github.com/ubgo/gqlkit/issues/4))
- `generate` (Go): GraphQL type names that aren't capitalized generated **unexported** Go identifiers, undefined when referenced from another generated package (`undefined: scalars.timestamptz`, `enums.order_by`). Hasura schemas (`timestamptz`, `uuid`, `jsonb`, `order_by`) and Apollo Federation types (`_Service`, `_Entity`) hit this. Generated Go type identifiers are now always exported — the first letter is capitalized and leading underscores are stripped (`timestamptz` → `Timestamptz`, `_Service` → `Service`) at every definition and reference site. GraphQL field/JSON names are unchanged; already-exported names (`JSON`, `DateTime`, `ID`) are untouched. ([#4](https://github.com/ubgo/gqlkit/issues/4))
- `generate` (Go): placeholder fields named `_` (the Apollo Federation empty-type marker, `_: Int`) produced an empty Go identifier — `func (q *QueryRoot) () *Builder`, a syntax error. Fields whose name yields no valid Go identifier are now skipped. ([#4](https://github.com/ubgo/gqlkit/issues/4))
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

[0.10.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.10.0
[0.9.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.9.0
[0.8.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.8.0
[0.7.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.7.0
[0.6.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.6.0
[0.5.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.5.0
[0.4.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.4.0
[0.3.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.3.0
[0.2.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.2.0
[0.1.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit%40v0.1.0
