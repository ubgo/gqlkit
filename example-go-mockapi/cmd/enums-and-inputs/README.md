# enums-and-inputs — Enums, inputs and scalar types

Passes an enum argument with and without the schema default, sends input structs with nil and non-nil optional fields, shows that unselected fields are zero values, clears a list by sending an empty slice, shows the three states of an `inputs.Opt[T]` field (unset, `Some`, `Null`), and prints the Go types of a bound scalar, the built-in `JSON` scalar and an unbound scalar. Creates a todo (deleted again) and two users named `Eve` per run.

Guide: [docs/go/arguments-and-inputs.md](../../../docs/go/arguments-and-inputs.md), which quotes this program and its output.

## Run

Start mockapi from the repo root (`task mockapi:run`), then from `example-go-mockapi/`:

```bash
go run ./cmd/enums-and-inputs
go test -count=1 ./cmd/enums-and-inputs
```

The test runs the program against mockapi and checks the lines the guide shows. It fails with a message naming the address if mockapi is not reachable; set `MOCKAPI_URL` to use another address.
