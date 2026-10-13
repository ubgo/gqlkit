# Fetch a schema with gqlkit-sdl

`gqlkit-sdl fetch` sends the standard introspection query to a live GraphQL endpoint and writes the schema as SDL, the format `gqlkit generate` and `gqlkit generate-ts` read. Use it when you have an endpoint but no `.graphql` file, or to refresh a committed schema. To trim what it writes, see [Filter a schema](./filter-schema.md).

The runnable version is [`example-cli/fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh), which runs every command below against the local test server.

## Before you start

- The `gqlkit-sdl` binary: `go install github.com/ubgo/gqlkit/gqlkit-sdl@latest`, or see the [README install section](../../README.md#install).
- An endpoint with introspection enabled. Many production APIs turn it off; see [Where the schema comes from](../choosing/schema-source.md) for the alternatives.

## Step 1: Fetch as SDL

```sh
gqlkit-sdl fetch --url "$GRAPHQL_URL" -o schema/full.graphql
```

`--url` is required. `-o` defaults to `schema.graphql` in the current directory; the file is replaced if it exists, and its directory must already exist.

## Step 2: Send headers

Most APIs need a token. `-H` takes `Key:Value` and is repeatable; spaces around the key and value are trimmed, so `"Authorization: Bearer x"` and `"Authorization:Bearer x"` are the same.

```sh
gqlkit-sdl fetch --url "$GRAPHQL_URL" \
	-H "Authorization: Bearer example-token" \
	-H "X-Request-Source: example-cli" \
	--debug >schema/curl.txt
```

`--debug` prints the equivalent `curl` command and stops: nothing is sent and no schema file is written. Run the printed command yourself to see the raw response, which is the quickest way to tell a bad token from a bad header. The example run prints (query body shortened here):

```
curl -X 'POST' -d '{"query":"\nquery IntrospectionQuery { ... }"}' -H 'Authorization: Bearer example-token' -H 'Content-Type: application/json' -H 'X-Request-Source: example-cli' 'http://localhost:8081/query'
```

## Step 3: Fetch as JSON when a tool wants it

```sh
gqlkit-sdl fetch --url "$GRAPHQL_URL" -f json -o schema/full.json
```

This writes the introspection response, `{"data": {"__schema": ...}}`, pretty-printed. Some tools (IDE plugins, other code generators) read that format. gqlkit itself does not: `gqlkit generate -s schema.json` fails with `invalid schema: schema.json:1:1: Unexpected {`.

## Try it

```sh
./example-cli/fetch-and-filter.sh
```

Real output of the first two steps:

```
Fetching schema from http://localhost:8081/query...
Converting to SDL format...
Saving to schema/full.graphql...
Done! Schema saved successfully.
Fetching schema from http://localhost:8081/query...
Saving JSON to schema/full.json...
Done! Schema saved successfully.
```

The SDL starts with the custom scalars and their descriptions, then the types in the order the server returns them. The first fields of `type Query` (the test server has a few more after these, with descriptions):

```graphql
type Query {
  ping: String!
  todo(id: ID!): Todo
  todos(filter: TodoFilter, pagination: PaginationInput): [Todo!]!
  todosConnection(filter: TodoFilter, pagination: PaginationInput): TodoConnection!
  user(id: ID!): User
  users: [User!]!
  search(term: String!): [SearchResult!]!
  echo(message: String!): String!
  sum(a: Int!, b: Int!): Int!
  serverInfo: JSON!
```

## How a fetch fails

Every failure exits with code 1 and one `Error:` line, followed by the usage text. From [`gqlkit-sdl/schema/fetcher.go`](../../gqlkit-sdl/schema/fetcher.go):

| Situation | Error line |
| --- | --- |
| Header without a colon | `invalid header format "NoColon", expected Key:Value` |
| Network failure or no answer within 2 minutes | `fetching schema: failed to execute request: ...` |
| Non-200 status | `fetching schema: unexpected status code 401: <response body, first 512 bytes>` |
| GraphQL errors, for example introspection disabled | `fetching schema: GraphQL errors: <messages joined by "; ">` |
| A 200 with an HTML page, usually a wrong path such as `/` instead of `/query` | `fetching schema: failed to unmarshal response: invalid character '<' looking for beginning of value` |
| A 200 with JSON but no `data.__schema.queryType` (a proxy, a non-GraphQL JSON API) | `fetching schema: response contains no introspection schema (data.__schema.queryType missing)` |
| Output directory does not exist | `saving file: open schema/x.graphql: no such file or directory` |

## Pitfalls

- **`--debug` writes nothing.** It is a dry run. Drop it to fetch.
- **Only two formats.** `-f` takes `graphql` (the default) or `json`; anything else is an error: `unknown --format "yaml" (valid: graphql, json)`.
- **A repeated header keeps the last value.** Two `-H "Authorization: ..."` flags send only the second.
- **`Host` is special.** A `-H "Host: api.internal"` sets the request's host instead of being sent as an ordinary header, which is what you want behind a proxy that routes by host.
- **The timeout is fixed at 2 minutes on the CLI.** A slow endpoint with a very large schema can hit it. The Go API takes `FetchOptions.Timeout` (see [programmatic use](./programmatic.md#fetch-and-filter-from-go)).
- **Commas inside values.** Header values may contain commas (`-H` is not comma-split), but the filter flags are; see [Filter a schema](./filter-schema.md#pitfalls).

## Related

- [Filter a schema](./filter-schema.md)
- [Where the schema comes from](../choosing/schema-source.md)
- [CLI reference: gqlkit-sdl fetch](../cli-reference.md#gqlkit-sdl-fetch)
- [Recipe: debug a failing introspection fetch](../recipes.md#debug-a-failing-introspection-fetch)
