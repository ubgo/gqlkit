// Command sdlapi fetches a schema, keeps two queries, prunes unused types,
// and writes both SDL and JSON, using the gqlkit-sdl Go API instead of the CLI.
package main

import (
	"log"
	"os"
	"time"

	"github.com/ubgo/gqlkit/gqlkit-sdl/schema"
)

func main() {
	introspection, err := schema.FetchSchema("http://localhost:8081/query", &schema.FetchOptions{
		Headers: map[string]string{"Authorization": "Bearer " + os.Getenv("API_TOKEN")},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	trimmed, err := schema.FilterSchema(introspection, &schema.FilterOptions{
		OnlyQueries:  []string{"todo", "todos"},
		RemoveUnused: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := schema.SaveToFile(schema.ConvertToSDL(trimmed), "schema.graphql"); err != nil {
		log.Fatal(err)
	}
	if err := schema.SaveAsJSON(trimmed, "schema.json"); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote schema.graphql and schema.json")
}
