# batch-mutations — Batch mutations

Shows a batch rejecting an invalid alias and a nil builder before sending anything, creates two todos with one `batch.RunMutations` call, prints the merged document (aliases sorted, variables named `<alias>_<arg>`), then deletes both in a second batch. `testdata/mixed` is a program that passes a mutation builder to `RunQueries`; the test compiles it and requires the compiler to reject it.

Guide: [docs/go/batching.md](../../../docs/go/batching.md), which quotes this program and its output.

## Run

Start mockapi from the repo root (`task mockapi:run`), then from `example-go-mockapi/`:

```bash
go run ./cmd/batch-mutations
go test -count=1 ./cmd/batch-mutations
```

The test runs the program against mockapi and checks the lines the guide shows. It fails with a message naming the address if mockapi is not reachable; set `MOCKAPI_URL` to use another address.
