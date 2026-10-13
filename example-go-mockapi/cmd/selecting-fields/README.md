# selecting-fields — Selecting fields

Selects a self-referencing field (`Todo.parent`) two levels deep, shows that a nested selector with no fields is dropped from the document without an error, reads a `Node` interface result and a `SearchResult` union with `OnTodo` / `OnUser` fragments and a type switch on the `<Name>Value` wrapper, and decodes a union value with an unknown `__typename` offline to show it is kept, not lost.

Guide: [docs/go/selecting-fields.md](../../../docs/go/selecting-fields.md), which quotes this program and its output.

## Run

Start mockapi from the repo root (`task mockapi:run`), then from `example-go-mockapi/`:

```bash
go run ./cmd/selecting-fields
go test -count=1 ./cmd/selecting-fields
```

The test runs the program against mockapi and checks the lines the guide shows. It fails with a message naming the address if mockapi is not reachable; set `MOCKAPI_URL` to use another address.
