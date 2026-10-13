#!/bin/sh
# Generate a Go SDK with `gqlkit generate` inside a fresh Go module, build it,
# and run a small program that calls the API through it.
# Needs a GraphQL server at GRAPHQL_URL (mockapi by default).
#
# Writes OUT_DIR/go-app/ (go.mod, schema.graphql, sdk/, main.go).
set -eu

. "$(dirname -- "$0")/lib.sh"

build_clis
require_endpoint

# The app module lives inside this repo's go.work tree. A real project is not
# inside someone else's workspace, so it does not need this line.
GOWORK=off
export GOWORK

APP=$OUT_DIR/go-app
rm -rf "$APP"
mkdir -p "$APP"
cd "$APP"

# 1. A module to generate into. The SDK has no runtime dependency on gqlkit,
#    so go.mod never gains a require line for it.
go mod init example.com/cliapp

# 2. The schema.
gqlkit-sdl fetch --url "$GRAPHQL_URL" -o schema.graphql

# 3. The SDK. -m is the import path of the SDK directory itself, which is
#    the module path plus the output directory, not the module path alone.
gqlkit generate \
	-s schema.graphql \
	-o ./sdk \
	-m "$(go list -m)/sdk" \
	-c "$EXAMPLE_DIR/go.config.jsonc"

# 4. A program that uses it.
cat >main.go <<'EOF'
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"example.com/cliapp/sdk/fields"
	"example.com/cliapp/sdk/graphqlclient"
	"example.com/cliapp/sdk/inputs"
	"example.com/cliapp/sdk/queries"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := graphqlclient.NewClient(os.Getenv("GRAPHQL_URL"))
	qr := queries.NewQueryRoot(client)

	pong, err := qr.Ping().Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ping:", pong)

	todos, err := qr.Todos().
		Pagination(&inputs.PaginationInput{Limit: 2}).
		Select(func(t *fields.TodoFields) {
			t.ID().Text().CreatedAt()
		}).
		Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, t := range todos {
		// CreatedAt is a time.Time because go.config.jsonc binds DateTime.
		fmt.Printf("todo %s: %q created %s\n", t.ID, t.Text, t.CreatedAt.Format(time.DateOnly))
	}
}
EOF

# 5. Prove it: vet, build, run.
go vet ./...
go build ./...
go run .
