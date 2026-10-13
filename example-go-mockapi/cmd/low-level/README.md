# low-level — Low-level builder

Prints a builder's document and variables with `BuildQuery` and `GetVariables`, adds a field by hand through `GetSelection`, reads a result with `ExecuteRaw`, and sends hand-written documents with `Client.Execute`, `Client.RawQuery` and `Client.ExecuteWithPartialData`.

Guide: [docs/go/low-level-builder.md](../../../docs/go/low-level-builder.md), which quotes this program and its output.

## Run

Start mockapi from the repo root (`task mockapi:run`), then from `example-go-mockapi/`:

```bash
go run ./cmd/low-level
go test -count=1 ./cmd/low-level
```

The test runs the program against mockapi and checks the lines the guide shows. It fails with a message naming the address if mockapi is not reachable; set `MOCKAPI_URL` to use another address.
