# Where the schema comes from: introspection, a local SDL file, or schema.json

`gqlkit generate` and `gqlkit generate-ts` read one thing: GraphQL SDL (`.graphql` text), from a file or a glob. How you get that SDL depends on what the API owner gives you. This page helps you pick, and lists what does not work.

## Before you start

Find out two things about the API: whether introspection is enabled on the endpoint you can reach, and whether the owner publishes the schema as SDL (often a `schema.graphql` in their repo or docs).

## Decision table

| You have | Do this | Guide | Example |
| --- | --- | --- | --- |
| A live endpoint with introspection on | `gqlkit-sdl fetch --url ... -o schema.graphql` | [Fetch a schema](../cli/fetch-schema.md) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) |
| A live endpoint that needs a token | Same, with `-H "Authorization: Bearer ..."` | [Fetch a schema: headers](../cli/fetch-schema.md#step-2-send-headers) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) step 5 |
| A huge vendor endpoint | Fetch with `--only-*` / `--exclude-*` and `--remove-unused` | [Filter a schema](../cli/filter-schema.md) | [`fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh) steps 3-4 |
| A `.graphql` SDL file from the API owner | Pass it to `-s` directly; no fetch needed | [Generate a Go SDK](../cli/generate-go.md) | [`generate-go.sh`](../../example-cli/generate-go.sh) |
| The schema split across several SDL files (your own server's source) | Pass a quoted glob: `-s './schema/*.graphql'` | [Recipe](../recipes.md#assemble-a-schema-from-several-sdl-files) | [`multi-file-schema.sh`](../../example-cli/multi-file-schema.sh) |
| Only an introspection `schema.json` | Convert it to SDL first (below) | this page | none |
| A production endpoint with introspection off | Ask for the SDL, or fetch from a staging endpoint that allows introspection | [Recipe: debug a failing fetch](../recipes.md#debug-a-failing-introspection-fetch) | none |

## If X, then Y

- **If you control the server**, generate from its SDL source files with a glob. You skip the network, and the schema in your build is exactly the one in your repo.
- **If you do not control the server**, fetch and commit the SDL. A schema change then shows up as a reviewable diff, and builds do not depend on the endpoint being up.
- **If you need only part of the API**, fetch with filters. Filtering is a `gqlkit-sdl` feature; `gqlkit generate` has no filter flags, so a local SDL file is generated whole.
- **If all you have is `schema.json`**, gqlkit cannot read it (`invalid schema: schema.json:1:1: Unexpected {`). Convert it with a tool that turns an introspection result into SDL, for example `graphql-js`: `printSchema(buildClientSchema(json.data))`. `gqlkit-sdl` converts only what it fetched itself; it has no "convert a file" command.

## Try it

`example-cli/test.sh` checks both sides of this choice: the fetched SDL and the multi-file glob generate, and an introspection JSON file is refused:

```
== failure modes
  ok    a header without a colon is rejected
  ok    a missing schema file is rejected
  ok    an introspection JSON file is not accepted as --schema
  ok    scripts refuse to run with no server
```

## Pitfalls

- **A schema file name that matches nothing.** `-s` is a glob, so a typo reports `did not match any files` rather than `no such file`.
- **An unquoted glob.** The shell expands `./schema/*.graphql` into several arguments before gqlkit sees it. gqlkit refuses the extra arguments and exits 1 with `unexpected argument(s): ...` and a hint to quote the glob, e.g. `-s './schema/*.graphql'`. Quote it.
- **SDL that also defines built-in scalars.** Some tools write `scalar String` and friends into their SDL. gqlkit accepts that and skips its own copy.
- **A fetched schema is a snapshot.** The endpoint can change under you; see [When to regenerate](./when-to-regenerate.md).

## Related

- [Fetch a schema with gqlkit-sdl](../cli/fetch-schema.md)
- [Filter a schema](../cli/filter-schema.md)
- [Recipes](../recipes.md)
- [CLI or Go program](./cli-or-go-program.md)
- [When to regenerate](./when-to-regenerate.md)
