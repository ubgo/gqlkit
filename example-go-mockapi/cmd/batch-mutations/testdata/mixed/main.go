// This program must NOT compile. It passes a mutation builder to
// batch.RunQueries, which accepts only query builders. TestMixedKindsDoNotCompile
// builds it and asserts the compiler rejects it; the directory is under
// testdata/ so `go build ./...` and `go vet ./...` skip it.
package main

import (
	"context"

	"example-go-mockapi/sdk/batch"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/mutations"
	"example-go-mockapi/sdk/queries"
)

func main() {
	client := graphqlclient.NewClient("http://localhost:8081/query")
	qr := queries.NewQueryRoot(client)
	mr := mutations.NewMutationRoot(client)

	var dest struct{}
	_ = batch.RunQueries(context.Background(), &dest, batch.QueryItems{
		"ping":  qr.Ping(),
		"clear": mr.CompleteAllTodos(), // compile error: not a query builder
	})
}
