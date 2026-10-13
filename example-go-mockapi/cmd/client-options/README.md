# client-options — Client options

Configures the generated `graphqlclient`: a bearer token, extra headers, a custom `*http.Client` with a timeout and a recording transport, and a per-call deadline through `context.Context`. It prints the headers as sent and the `Authorization` header as mockapi received it.

Guide: [docs/go/client.md](../../../docs/go/client.md), which quotes this program and its output.

## Run

Start mockapi from the repo root (`task mockapi:run`), then from `example-go-mockapi/`:

```bash
go run ./cmd/client-options
go test -count=1 ./cmd/client-options
```

The test runs the program against mockapi and checks the lines the guide shows. It fails with a message naming the address if mockapi is not reachable; set `MOCKAPI_URL` to use another address.
