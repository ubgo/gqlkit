# Filter a schema before generating

Vendor schemas are large: thousands of types, hundreds of root fields. Generating an SDK for all of it gives you a package that is slow to compile and hard to navigate, when your app calls a dozen operations. `gqlkit-sdl fetch` can keep or drop root query and mutation fields while it fetches, and `--remove-unused` then prunes every type nothing left can reach. Use it whenever you only need part of an API.

The runnable version is steps 3 and 4 of [`example-cli/fetch-and-filter.sh`](../../example-cli/fetch-and-filter.sh).

## Before you start

- Read [Fetch a schema with gqlkit-sdl](./fetch-schema.md); filtering is a set of extra flags on the same command.
- List the operations your app calls. Root field names are what the filters match: `todos`, `createTodo`, not type names.

## Step 1: Keep only what you call

```sh
gqlkit-sdl fetch --url "$GRAPHQL_URL" \
	--only-queries "todo,todos" \
	--only-mutations "createTodo" \
	--remove-unused \
	-o schema/trimmed.graphql
```

A plain name is an exact match: `todo` keeps `todo` but not `todos`, which is why both are listed.

## Step 2: Or drop what you do not want, by regex

```sh
gqlkit-sdl fetch --url "$GRAPHQL_URL" \
	--exclude-queries "todo.*" \
	--exclude-mutations ".*Todo.*" \
	--remove-unused \
	-o schema/no-todos.graphql
```

A value containing any of `\ . * + ? ^ $ { } ( ) | [ ]` is compiled as a Go regular expression and anchored at both ends, so `todo.*` means `^(?:todo.*)$`: it removes `todo`, `todos` and `todosConnection`, but would not remove a field named `myTodo`. Use `.*todo.*` to match anywhere.

## How the flags combine

From [`gqlkit-sdl/schema/filter.go`](../../gqlkit-sdl/schema/filter.go):

| Rule | Effect |
| --- | --- |
| `--only-queries` set | Every query field that matches none of the values is removed. |
| `--exclude-queries` set | Every query field that matches any value is removed. |
| Both set | A field must match `--only-*` and not match `--exclude-*`; exclude wins on a tie. |
| Only query flags set | Mutations are untouched, and the other way round. |
| Subscriptions | Never filtered. |
| `--remove-unused` | Applied after the field filters. Keeps every type reachable from the remaining query, mutation and subscription fields (through fields, arguments, input fields, interfaces and union members) and from directive arguments; drops the rest. Works without any field filter too. |

## Try it

```sh
./example-cli/fetch-and-filter.sh
```

Real output for the two filtered fetches:

```
Fetching schema from http://localhost:8081/query...
Applying filters...
Converting to SDL format...
Saving to schema/trimmed.graphql...
Done! Schema saved successfully.
Fetching schema from http://localhost:8081/query...
Applying filters...
Converting to SDL format...
Saving to schema/no-todos.graphql...
Done! Schema saved successfully.

Schema files:
     149 schema/full.graphql
      78 schema/trimmed.graphql
      92 schema/no-todos.graphql
     319 total
```

The roots of `trimmed.graphql`. `TodoConnection`, `TodoEdge`, `PageInfo`, the `SearchResult` union and the `Cursor` and `JSON` scalars are gone, because none of the three remaining operations reaches them. The `Node` interface stays: `Todo` and `User` implement it, and `--remove-unused` follows interfaces:

```graphql
type Mutation {
  createTodo(input: NewTodo!): Todo!
}

type Query {
  todo(id: ID!): Todo
  todos(filter: TodoFilter, pagination: PaginationInput): [Todo!]!
}
```

## Pitfalls

- **Emptying a root type.** If the filters remove every query field, the fetch fails and writes nothing: `Error: filtering schema: filters removed every query field; GraphQL requires at least one`. If they remove every mutation field (`--exclude-mutations ".*"`), the `Mutation` type is dropped and the fetch succeeds with `Note: filters removed every mutation field; dropped the empty Mutation type from the output.` on stderr. That is easy to miss in a script, so check the output has the operations you expect.
- **Commas split values, including inside a regex.** The filter flags are comma-separated lists, so `--only-queries "todos{1,2}"` becomes the two patterns `todos{1` and `2}`. Avoid `{m,n}` repetition; list the names separately or use alternation: `--only-queries "todo|todos"`. Alternation is anchored as a whole, so `todo|echo` keeps exactly `todo` and `echo`.
- **An invalid regex is an error.** A value with a metacharacter that does not compile stops the fetch before anything is written: `Error: filtering schema: invalid filter pattern "todo(": error parsing regexp: missing closing ): ...`.
- **Without `--remove-unused` the types stay.** Field filters only remove root fields. Every type the removed operations used is still declared, and still generated.
- **Field filters do not reach nested fields.** Only fields of the root `Query` and `Mutation` types are filtered. A type you keep keeps all its fields.

## Related

- [Fetch a schema with gqlkit-sdl](./fetch-schema.md)
- [Recipe: trim a huge vendor schema](../recipes.md#trim-a-huge-vendor-schema-down-to-what-you-use)
- [gqlkit-sdl README: filtering](../../gqlkit-sdl/README.md#regex-patterns)
- [Run the filter from Go](./programmatic.md#fetch-and-filter-from-go)
