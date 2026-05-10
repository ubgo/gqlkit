# gqlkit-ts

A lightweight, zero-dependency TypeScript runtime library for executing GraphQL queries. Provides a typed HTTP client, error handling, and builder primitives for generated GraphQL SDKs.

## Installation

```bash
npm install gqlkit-ts
```

## Quick Start

```typescript
import { GraphQLClient } from "gqlkit-ts";

const client = new GraphQLClient("https://api.example.com/graphql");

const data = await client.execute<{ users: { id: string; name: string }[] }>(
  `query { users { id name } }`
);

console.log(data.users);
```

## Usage

### Creating a Client

```typescript
import { GraphQLClient } from "gqlkit-ts";

const client = new GraphQLClient("https://api.example.com/graphql", {
  authToken: "your-token",             // Sets Authorization: Bearer header
  headers: { "X-Custom": "value" },    // Additional headers
  fetch: customFetch,                  // Custom fetch implementation (optional)
});
```

### Typed Queries

```typescript
interface UsersResponse {
  users: { id: string; name: string; email: string }[];
}

const data = await client.execute<UsersResponse>(
  `query GetUsers($limit: Int!) { users(limit: $limit) { id name email } }`,
  { limit: 10 }
);

// data.users is fully typed
```

### Raw Queries

```typescript
const data = await client.rawQuery(
  `query { users { id name } }`
);
// Returns unknown — useful for ad-hoc/untyped queries
```

### Batching Multiple Queries Into One Request

Use `batch` to send several builders as a single GraphQL operation with aliases — one HTTP round trip, not N. All builders must share the same operation type (all queries or all mutations).

```typescript
import { batch } from "gqlkit-ts";

const { open, completed } = await batch(client, {
  open: qr.tasks(),
  completed: qr.tasks().status("completed"),
});
// → POST /graphql once
// query Batch($completed_status: Status) {
//   open: tasks { ... }
//   completed: tasks(status: $completed_status) { ... }
// }
```

The result is keyed by the alias you supplied; each value is typed from that builder's `execute()` return type. Argument names are namespaced with the alias (`$completed_status`), so two builders sharing an argument name don't collide.

Pass `{ opName }` to override the default `Batch` operation name:

```typescript
await batch(client, { ... }, { opName: "DashboardLoad" });
```

### Error Handling

```typescript
import { GraphQLClient, GraphQLErrors } from "gqlkit-ts";

try {
  const data = await client.execute(query);
} catch (err) {
  if (err instanceof GraphQLErrors) {
    // Access structured GraphQL errors
    for (const e of err.errors) {
      console.error(e.message);       // Error description
      console.error(e.locations);     // Source locations in the query
      console.error(e.path);          // Response field path
      console.error(e.extensions);    // Vendor-specific metadata
    }
  }
}
```

## API Reference

### `GraphQLClient`

HTTP client for executing GraphQL operations.

| Method | Description |
|---|---|
| `execute<T>(query, variables?)` | Execute a query and return typed `data` |
| `rawQuery(query, variables?)` | Execute a query and return untyped `data` |

### `batch(client, builders, options?)`

Merges multiple builders into a single GraphQL operation. Returns an alias-keyed result object, each value typed from the corresponding builder's `execute()` return type.

| Argument | Type | Description |
|---|---|---|
| `client` | `GraphQLClient` | Client used to send the merged operation |
| `builders` | `Record<string, BatchableBuilder>` | Alias → builder map; aliases become GraphQL response keys |
| `options.opName` | `string` | Operation name (default: `"Batch"`) |

Throws if the input is empty or if builders mix `query` and `mutation`.

### `ClientOptions`

| Option | Type | Description |
|---|---|---|
| `headers` | `Record<string, string>` | Additional HTTP headers for every request |
| `authToken` | `string` | Bearer token for the `Authorization` header |
| `fetch` | `typeof fetch` | Custom fetch implementation (SSR, testing) |

### `GraphQLErrors`

Error class thrown when the server returns GraphQL errors. Extends `Error` with an `errors` property containing the raw error objects.

### `FieldSelection`

Tracks selected fields in a GraphQL selection set. Used internally by generated SDK builders.

```typescript
const sel = new FieldSelection();
sel.addField("id");
sel.addField("name");

const nested = new FieldSelection();
nested.addField("city");
sel.addChild("address", nested);

sel.build();
// id
// name
// address {
//   city
// }
```

### `BaseBuilder`

Base class for generated query/mutation builders. Handles argument storage, field selection, query assembly, and execution.

```typescript
// Generated SDK code extends BaseBuilder:
class GetUserBuilder extends BaseBuilder {
  constructor(client: GraphQLClient) {
    super(client, "query", "GetUser", "user");
  }
  id(value: string) { this.setArg("id", value, "ID!"); return this; }
  async exec() { return (await this.executeRaw()).user; }
}
```

## Requirements

- Node.js 18+ (uses native `fetch`)
- TypeScript 5.4+ (for development)

## License

MIT
