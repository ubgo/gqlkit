# Changelog — gqlkit-sdl (CLI)

All notable changes to the `gqlkit-sdl` binary. Format: [Keep a Changelog](https://keepachangelog.com/), versioning: [SemVer](https://semver.org/).

Tagged as `gqlkit-sdl@vX.Y.Z`. See releases at <https://github.com/ubgo/gqlkit/releases>.

## [Unreleased]

### Changed
- `fetch --format` now rejects an unknown value (for example `-f yaml` or `-f JSON`) with an error listing the valid formats, `graphql` and `json`. Previously any other value silently wrote SDL.
- A non-200 response's body is quoted in the error only up to 512 bytes, with "... (truncated)" appended when there was more, so a gateway's HTML page no longer floods the terminal.
- `fetch --debug` writes the curl command through the command's output like the other progress lines (still stdout). Library callers can set `FetchOptions.DebugOutput`.
- An invalid regex in a filter flag (for example `--only-queries "user("`) is now an error naming the pattern. It used to be treated as an exact name and silently matched nothing.
- When the filters remove every mutation field, the empty `Mutation` type is dropped from the output and a note is printed on stderr. Removing every query field is an error, since GraphQL requires at least one. Previously both wrote an empty type and exited 0, and gqlkit then failed to parse the file.
- Library: `schema.FilterSchema` now returns `(*IntrospectionSchema, error)`.

### Fixed
- Filter patterns with alternation now match whole field names only: `--only-queries "todo|echo"` keeps `todo` and `echo`, no longer also `todos` and `todosConnection`.
- Deprecation reasons are now written with GraphQL string escaping. Previously control characters produced Go-style escapes (such as `\x01`) that made the SDL fail to parse.
- `fetch` no longer hangs forever on an endpoint that accepts the connection but never answers; the request now fails after 2 minutes.
- `fetch` now fails with "response contains no introspection schema" when the endpoint answers 200 without `data.__schema` (for example `{"data":null}` from a proxy), instead of writing an empty schema file and reporting success.
- `-H "Host: ..."` is now sent as the request's Host; it was silently dropped before.
- Descriptions containing `"""` or a lone carriage return now produce valid SDL; previously the generated file failed to parse.

### Documentation
- The 0.8.0 entry below names the filter flags `--include-query` / `--exclude-query`. The real flags are `--only-queries`, `--exclude-queries`, `--only-mutations` and `--exclude-mutations`.
- README: fixed the `go install` path (`github.com/ubgo/gqlkit/gqlkit-sdl@latest`) and the Go API example (`FilterSchema` returns an error; full module import path), added a Pitfalls section, and linked the fetch and filter guides and `example-cli/fetch-and-filter.sh`.

## [0.11.0] — 2026-10-13

### Changed
- The Go module path is now `github.com/ubgo/gqlkit/gqlkit-sdl` (was `github.com/khanakia/gqlkit/gqlkit-sdl`). Update imports, `go get` and `go install` lines to the new path.
- Licensed under Apache-2.0 (was MIT).

## [0.8.0] — 2026-03-31

### Added
- Query / mutation filtering on `fetch` — `--include-query` / `--exclude-query` (and the mutation duals) let you scope the fetched SDL to a subset of operations. Useful when introspecting large schemas where only a handful of root fields are relevant to your client.
- Unused-type removal pass — types unreachable from the included operations are dropped from the output. Produces a smaller, focused SDL file.

## [0.7.0] — 2026-03-22

### Added
- `--format json` flag — emits the introspection result as raw JSON instead of SDL. Useful for piping into other tooling that expects the GraphQL introspection response shape.

## [0.6.0] — 2026-03-18

### Added
- `--package` flag accepts a full Go import path (paired release with `gqlkit@v0.6.0`).

## [0.5.0] — 2026-03-18

### Added
- Auto-detect SDK import path from `go.mod` (paired release with `gqlkit@v0.5.0`).

## [0.4.0] — 2026-03-18

### Changed
- Generated `graphqlclient` is bundled into the SDK; full GitHub module paths in generated imports (paired release with `gqlkit@v0.4.0`).

## [0.3.0] — 2026-03-18

### Added
- `-o` / `-c` shorthand flags; `--config` is optional (paired release with `gqlkit@v0.3.0`).

## [0.2.0] — 2026-03-18

### Added
- TypeScript codegen support, custom scalar examples, AI-friendly docs (paired release with `gqlkit@v0.2.0`).

## [0.1.0] — 2026-03-17

### Added
- Initial release. Cobra-based `gqlkit-sdl fetch` command — performs GraphQL introspection against a running endpoint and writes SDL to disk.
- `--debug-curl` flag for troubleshooting introspection requests.
- GoReleaser-driven release workflow tagged as `gqlkit-sdl@vX.Y.Z`.

[0.8.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.8.0
[0.7.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.7.0
[0.6.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.6.0
[0.5.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.5.0
[0.4.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.4.0
[0.3.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.3.0
[0.2.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.2.0
[0.1.0]: https://github.com/ubgo/gqlkit/releases/tag/gqlkit-sdl%40v0.1.0
