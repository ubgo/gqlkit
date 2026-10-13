package main

import (
	"fmt"
	"os"

	"github.com/ubgo/gqlkit/gqlkit/pkg/clientgents"
)

func main() {
	fmt.Println("Generating TypeScript SDK...")

	config := &clientgents.Config{
		SchemaPath: "cmd/generate/schema.graphql",
		OutputDir:  "./sdk",
		ConfigPath: "cmd/generate/config.jsonc",
	}

	gen, err := clientgents.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create generator: %v\n", err)
		os.Exit(1)
	}

	if err := gen.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate SDK: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("TypeScript SDK generation completed.")
}
