# example-go-chat — Go SDK Example (Production Schema)

Demonstrates generating a Go SDK from a production-scale chatbot GraphQL schema with complex types, Relay-style cursor pagination, and many operations.

## What you can and cannot run

The schema here comes from a private chatbot API. **The sample program (`cmd/samples`) calls that private server at `http://localhost:2310/api/sa/query` with a workspace header and a token read from the environment, so it will not run for anyone outside that project.** Nobody else has the server, and nothing in this repository starts it.

If you do have access to that server, the sample program takes the connection from three environment variables and the samples to run from its arguments:

| Variable | Sent as | Default |
| --- | --- | --- |
| `CHAT_API_URL` | endpoint | `http://localhost:2310/api/sa/query` |
| `CHAT_WORKSPACE_ID` | `x-workspace-id` header | not sent |
| `CHAT_TOKEN` | `token` header | not sent |

```bash
go run ./cmd/samples                      # ping and chatbots (the default)
go run ./cmd/samples users folders-nested # any names, run in the order given
go run ./cmd/samples all                  # every sample
```

The sample names are `ping`, `chatbots`, `credentials`, `users`, `folders`, `chatbots-nested`, `folders-nested`, `channels-nested` and `html-to-markdown` (a mutation). An unknown name exits 1 and lists the valid ones.

What does work without it:

- `go run ./cmd/generate` regenerates the SDK from the committed `cmd/generate/schema.graphql`. It reads only local files.
- `go build ./...` compiles the SDK and the samples, which is what this example is for: proof that a large real-world schema (84 types, 44 queries, 54 mutations) generates a compiling SDK.
- Reading `cmd/samples/main.go` shows `WithHeaders`, cursor pagination with ordering and a mutation with an input object.

To run generated code end to end against a server you can start yourself, use [example-go-mockapi](../example-go-mockapi/) (Go SDK from Go code), [example-ts](../example-ts/) (TypeScript), or [example-cli](../example-cli/README.md) (the `gqlkit` and `gqlkit-sdl` CLIs), all of which call the repo's `mockapi` server.

## Structure

```
cmd/
  generate/
    main.go              SDK generator entry point
    schema.graphql       Production chatbot GraphQL schema
    config.jsonc         Scalar bindings (Cursor, Time, Password, JSON, etc.)
  samples/
    main.go              Sample queries, picked by name on the command line
sdk/                     Generated SDK (do not edit)
  builder/               FieldSelection + BaseBuilder runtime
  scalars/               Custom scalars (Cursor, Password, JSON, Time, etc.)
  enums/                 Order field enums (ChatbotOrderField, etc.)
  types/                 84 Go structs (Chatbot, Channel, User, etc.)
  inputs/                Input structs for ordering, filtering, etc.
  fields/                85 field selector files (complex nested types)
  queries/               44 query builders + QueryRoot
  mutations/             54 mutation builders + MutationRoot
```

## Generate

```bash
go run ./cmd/generate
```

## SDK Usage

```go
client := graphqlclient.NewClient("http://localhost:2310/api/sa/query",
    graphqlclient.WithHeaders(map[string]string{
        "x-workspace-id": os.Getenv("CHAT_WORKSPACE_ID"),
        "token":          os.Getenv("CHAT_TOKEN"),
    }),
)

qr := queries.NewQueryRoot(client)

// Relay cursor pagination with ordering
chatbots, _ := qr.Chatbots().
    First(intPtr(10)).
    OrderBy([]inputs.ChatbotOrder{{
        Field: enums.ChatbotOrderFieldCreatedAt,
    }}).
    Select(func(conn *fields.ChatbotConnectionFields) {
        conn.TotalCount().
            Edges(func(e *fields.ChatbotEdgeFields) {
                e.Cursor().Node(func(c *fields.ChatbotFields) {
                    c.ID().Name().CreatedAt().UpdatedAt()
                })
            }).
            PageInfo(func(p *fields.PageInfoFields) {
                p.HasNextPage().HasPreviousPage().StartCursor().EndCursor()
            })
    }).
    Execute(ctx)
```

## Custom Scalars

Scalar → Go type bindings in the generated `sdk/scalars/` (`Time` is set in `cmd/generate/config.jsonc`; the rest use generator defaults):

| GraphQL Scalar | Go Type |
|---------------|---------|
| `Cursor` | `any` |
| `Time` | `time.Time` |
| `Password` | `any` |
| `JSON` | `encoding/json.RawMessage` |
| `Uint64` | `uint64` |
