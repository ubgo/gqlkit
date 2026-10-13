# Changelog — gqlkit-ts

All notable changes to the [`gqlkit-ts`](https://www.npmjs.com/package/gqlkit-ts) npm package. Format: [Keep a Changelog](https://keepachangelog.com/), versioning: [SemVer](https://semver.org/).

## [Unreleased]

### Added
- `GraphQLHTTPError` (with `status` and the full response `body`) — thrown when the response is not a GraphQL response, such as an HTML 502 page or a proxy's JSON error.
- `GraphQLErrors.data` — partial data the server returned alongside the errors; in a batch where one alias failed, the aliases that resolved are here and each error's `path[0]` names the failing alias.
- `OperationType` / `OPERATION_TYPES`, `DEFAULT_BATCH_OP_NAME`, `GRAPHQL_NAME_PATTERN`, and the `GraphQLError` and `OpFragment` types are exported from the package root.
- Opt-in scalar codecs: `FieldSelection.addField(name, decode?, listDepth?)`, `addChild(name, child, possibleTypes?)`, `decode()` / `hasDecoders()`, and `BaseBuilder.setArg(..., encode?)`, `setResultDecoder()` / `decodeResult()` turn custom-scalar wire values into app values and back (lists, nested objects, aliases, and `... on X` fragments routed by `__typename`); with no codec configured nothing changes.
- `batch()` applies each builder's decoders to its alias (and to partial data on `GraphQLErrors.data`) through the new optional `OpFragment.decode`.
- `BaseBuilder.decodePartial(err)` decodes the partial data of a single operation's `GraphQLErrors` (generated `execute()` calls it before rethrowing), and `GraphQLErrors.decodeError` holds the `ScalarCodecError` when partial data fails to decode (`data` is then `undefined`); `batch()` sets it the same way.
- `ScalarCodecError` (with `direction`, `path`, `cause`) names the exact response or variable path when a codec throws; `ScalarDecoder`, `ScalarEncoder`, `ArgEncoder`, `InputEncodePlan` types and `CodecDirection` are exported.
- Per-request cancellation: `execute(query, variables?, { signal })`, `rawQuery(query, variables?, { signal })` and `batch(client, builders, { signal })` take an `AbortSignal`; an already-aborted signal rejects before anything is sent, and the rejection is the signal's own reason. New `RequestOptions` type export.

### Changed
- `BaseBuilder`'s `opType` constructor parameter and `OpFragment.opType` are now typed `OperationType` (`"query" | "mutation"`) instead of `string`; generated SDKs pass literals and are unaffected, but code passing a plain `string` must narrow it.
- The response body is read with `response.text()` instead of `response.json()`, so a custom `fetch` fake must return an object with `text()` (a real `Response` already does).
- A non-2xx response with neither `data` nor `errors` now throws `GraphQLHTTPError` instead of `Error("No data returned from GraphQL query")`.
- `batch()` now throws before sending on an invalid alias or operation name, or on a variable-name collision (see Fixed).

### Fixed
- A nested selection with no fields is omitted instead of being emitted as invalid `field {}` GraphQL, matching the Go runtime.
- `batch()` rejects aliases that would produce the same variable (alias `a` + argument `b_c` and alias `a_b` + argument `c` both made `$a_b_c`) instead of silently overwriting one value.
- `batch()` validates aliases and the operation name against the GraphQL name grammar instead of sending a document the server rejects with a syntax error.
- A non-JSON response body raises `GraphQLHTTPError` with the status code instead of a bare `SyntaxError`.
- A non-array `errors` value no longer crashes the client, and an error entry without a string `message` gets a placeholder instead of `"undefined"`.
- Request headers merge case-insensitively, so a custom `content-type` or `authorization` is no longer sent next to the default spelling (which `fetch` joins into an invalid value); `authToken` still wins.
- Partial data returned with GraphQL errors is kept on `GraphQLErrors.data` instead of being discarded.
- `OpFragment` is actually exported from the package root (0.2.0 listed it but it was missing).
- The `gqlkit-ts-demo` smoke test now depends on `gqlkit-ts@^0.2.0` instead of `^0.1.0`, so it exercises the current release line.

### Documentation
- Step-by-step TypeScript guides (`docs/typescript/`), each backed by a runnable sample under `example-ts/cmd/samples/`.
- The README gained a "With a generated SDK" section, a "Cancelling a Request" section and a Pitfalls section (one client per identity in SSR, timeouts, `execute<T>` is a cast, batch failures), and links to the guides.

## [0.2.0] — 2026-05-10

### Added
- `batch(client, builders, options?)` — merges multiple generated builders into a single GraphQL operation with aliased root fields. One HTTP round trip instead of N. Result is keyed by alias, each value typed from that builder's `execute()` return type via the `BatchResult<T>` mapped type.
- `BatchableBuilder<TResult>`, `BatchResult<T>`, `BatchOptions`, and `OpFragment` exports for callers building custom batch helpers on top of the runtime.
- `BaseBuilder.getOpFragment(alias)` — emits an alias-namespaced fragment that `batch()` splices into a merged operation. Argument names are prefixed with the alias (`$open_filter` instead of `$filter`) so two builders sharing an argument name coexist without colliding.

### Changed
- Generated TypeScript SDKs (via `gqlkit generate-ts`) now include a `getOpFragment(alias)` forwarder on every operation builder, so they satisfy `BatchableBuilder` structurally without inheriting from `BaseBuilder` directly.

### Fixed
- Self-referencing object fields (e.g. `Item.parent: Item`) no longer generate as scalar leaves — they get a proper selector callback (`parent<U>(...)`) like every other object field. Affects schemas with tree / graph types. Generator-side fix in `gqlkit@v0.8.0`; this release ships the runtime side.

## [0.1.0] — earlier

### Added
- Initial release. Lightweight, zero-dependency runtime library consumed by SDKs generated via `gqlkit generate-ts`.
- `GraphQLClient` — HTTP client with bearer-token auth, custom headers, custom-fetch injection.
- `GraphQLErrors` — structured error class wrapping the server's GraphQL error array.
- `FieldSelection` + `BaseBuilder` — query-assembly primitives that generated SDKs extend.
- Build pipeline: `tsc` → `dist/` (CommonJS + `.d.ts`).

[0.2.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-ts%40v0.2.0
