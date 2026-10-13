package main

import (
	"fmt"
	"os"

	"github.com/ubgo/gqlkit/gqlkit/pkg/clientgen"
)

func main() {
	config := &clientgen.Config{
		SchemaPath:  "cmd/generate/schema.graphql",
		OutputDir:   "./sdk",
		PackageName: "sdk",
		ModulePath:  "github.com/example/sdk",
		ConfigPath:  "cmd/generate/config.jsonc",
		Package:     "example-go-chat/sdk",
	}

	gen, err := clientgen.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create generator: %v\n", err)
		os.Exit(1)
	}

	if err := gen.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate SDK: %v\n", err)
		os.Exit(1)
	}
}
