# errors — Errors

Produces each kind of failure and classifies it: a resolver error as `GraphQLErrors` with `extensions.code`, partial data from a batch with `*batch.Error`, a missing required argument (HTTP 422, also `GraphQLErrors`), a non-GraphQL 502 from a stand-in gateway as `*graphqlclient.HTTPError`, a connection refused, and a 200 response that is not JSON.

Guide: [docs/go/errors.md](../../../docs/go/errors.md), which quotes this program and its output.

## Run

Start mockapi from the repo root (`task mockapi:run`), then from `example-go-mockapi/`:

```bash
go run ./cmd/errors
go test -count=1 ./cmd/errors
```

The test runs the program against mockapi and checks the lines the guide shows. It fails with a message naming the address if mockapi is not reachable; set `MOCKAPI_URL` to use another address.
