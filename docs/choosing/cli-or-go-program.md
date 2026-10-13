# Generate with the CLI or from a Go program

The `gqlkit` and `gqlkit-sdl` binaries are thin wrappers over Go packages: `clientgen.New` (Go SDK), `clientgents.New` (TypeScript SDK) and `gqlkit-sdl/schema` (fetch, filter, convert, save). Both routes write the same files. This page helps you pick one for your project's generate step.

## Before you start

Know who runs the generate step (a developer, CI, a TypeScript-only team without Go) and whether it needs anything a flag cannot express. The CLI route is shown in [Generate a Go SDK](../cli/generate-go.md) and [Generate a TypeScript SDK](../cli/generate-ts.md); the Go-program route in [Run the generators from Go code](../cli/programmatic.md).

## Decision table

| Situation | Use | Why | Example |
| --- | --- | --- | --- |
| One SDK, fixed paths, run from a script or Taskfile | CLI | Nothing to build or maintain; flags cover every setting. | [`example-cli/generate-go.sh`](../../example-cli/generate-go.sh), [`example-cli/generate-ts.sh`](../../example-cli/generate-ts.sh) |
| A TypeScript-only team | CLI (release binary or `install.sh`) | No Go toolchain needed to run a binary. | [`example-cli/generate-ts.sh`](../../example-cli/generate-ts.sh) |
| Fetch, filter and generate in one step | Either | Two CLI calls in a script, or `schema.FetchSchema` + `FilterSchema` + `clientgen.New` in one program. | [`example-cli/fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh), [Programmatic: fetch and filter](../cli/programmatic.md#fetch-and-filter-from-go) |
| Go and TypeScript SDKs from one schema | Either | Two CLI calls, or one program that runs both generators. | [Programmatic: both SDKs](../cli/programmatic.md#generate-go-and-typescript-sdks-from-one-program) |
| Paths or bindings computed at build time | Go program | A program can read the environment, a monorepo layout or another config. | [`example-go-mockapi/cmd/generate`](../../example-go-mockapi/cmd/generate/main.go) |
| Pin the generator version in `go.mod` and run it with `go run` | Go program | The generator module is a normal dependency, upgraded with `go get` and reviewed in `go.sum`. | [`example-ts/cmd/generate`](../../example-ts/cmd/generate/main.go) |
| Set the import path independently of `-m`, `-p` and `go.mod` | Go program | `clientgen.Config.Package` has no CLI flag. | [Programmatic: Config fields](../cli/programmatic.md#clientgenconfig-fields) |

## If X, then Y

- **If you are unsure**, start with the CLI. Switching later only means moving the same settings from flags into a `clientgen.Config`.
- **If you choose the Go program**, its `go.mod` gains `github.com/ubgo/gqlkit/gqlkit`; the generated SDK still imports nothing from gqlkit, so your app's runtime dependencies do not change.
- **If you generate several Go SDKs with different scalar bindings in one program**, run them in separate processes for now; see the bindings pitfall in [programmatic](../cli/programmatic.md#pitfalls).
- **If CI should prove the SDK is up to date**, either route works: regenerate, then fail on `git diff --exit-code`. See [When to regenerate](./when-to-regenerate.md).

## Pitfalls

- **Relative paths.** Both routes resolve `schema.graphql` and `./sdk` against the working directory, not the script or program location.
- **Version drift between machines.** `@latest` installs differ over time; pin the CLI version in your install step, or use the Go program route with a pinned module version.
- **Neither route deletes stale files unless asked.** Pass `--prune` (CLI) or set `Prune: true` (Go) when the schema can lose operations; `--dry-run` / `PruneDryRun` previews the deletions.

## Related

- [Generate a Go SDK](../cli/generate-go.md) and [Generate a TypeScript SDK](../cli/generate-ts.md)
- [Run the generators from Go code](../cli/programmatic.md)
- [Where the schema comes from](./schema-source.md)
