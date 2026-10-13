#!/bin/sh
# Fetch a schema by introspection with gqlkit-sdl, then trim it with the
# filter flags. Needs a GraphQL server at GRAPHQL_URL (mockapi by default).
#
# Writes into OUT_DIR/schema/:
#   full.graphql      the whole schema as SDL
#   full.json         the raw introspection result
#   trimmed.graphql   two queries and one mutation, unused types pruned
#   no-todos.graphql  everything except the todo operations (regex excludes)
#   curl.txt          the request --debug prints instead of sending it
set -eu

. "$(dirname -- "$0")/lib.sh"

build_clis
require_endpoint

mkdir -p "$OUT_DIR/schema"
cd "$OUT_DIR"

# 1. The whole schema as SDL, the format gqlkit generate reads.
gqlkit-sdl fetch --url "$GRAPHQL_URL" -o schema/full.graphql

# 2. The raw introspection response, for tools that want JSON.
#    gqlkit itself cannot read this file; it only reads SDL.
gqlkit-sdl fetch --url "$GRAPHQL_URL" -f json -o schema/full.json

# 3. Keep only what the app calls, then drop every type nothing reaches.
#    Plain names are exact matches.
gqlkit-sdl fetch --url "$GRAPHQL_URL" \
	--only-queries "todo,todos" \
	--only-mutations "createTodo" \
	--remove-unused \
	-o schema/trimmed.graphql

# 4. The inverse: drop operations by regex. A value with a regex
#    metacharacter is a regex, anchored at both ends, so "todo.*" removes
#    todo, todos and todosConnection but not a field named "myTodo".
gqlkit-sdl fetch --url "$GRAPHQL_URL" \
	--exclude-queries "todo.*" \
	--exclude-mutations ".*Todo.*" \
	--remove-unused \
	-o schema/no-todos.graphql

# 5. Headers, and --debug to see the exact request. --debug prints a curl
#    command and stops: nothing is sent and no schema file is written.
gqlkit-sdl fetch --url "$GRAPHQL_URL" \
	-H "Authorization: Bearer example-token" \
	-H "X-Request-Source: example-cli" \
	--debug >schema/curl.txt

echo
echo "Schema files:"
wc -l schema/full.graphql schema/trimmed.graphql schema/no-todos.graphql
