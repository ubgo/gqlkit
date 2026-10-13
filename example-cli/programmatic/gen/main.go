// Command gen regenerates the Go and TypeScript SDKs from schema.graphql.
// Run it from the module root: go run ./gen
package main

import (
	"log"

	"github.com/ubgo/gqlkit/gqlkit/pkg/clientgen"
	"github.com/ubgo/gqlkit/gqlkit/pkg/clientgents"
)

func main() {
	goGen, err := clientgen.New(&clientgen.Config{
		SchemaPath: "schema.graphql",
		OutputDir:  "./sdk",
		ModulePath: "example.com/prog/sdk",
		ConfigPath: "go.jsonc",
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := goGen.Generate(); err != nil {
		log.Fatal(err)
	}

	tsGen, err := clientgents.New(&clientgents.Config{
		SchemaPath: "schema.graphql",
		OutputDir:  "./web/src/sdk",
		ConfigPath: "ts.jsonc",
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := tsGen.Generate(); err != nil {
		log.Fatal(err)
	}
}
