# Shared setup for the example-cli scripts. Sourced, never run directly.
#
# Every script builds gqlkit and gqlkit-sdl from this repository's source
# (never a downloaded release), puts them first on PATH, and writes all of its
# output under OUT_DIR so the example leaves nothing behind in the repo.

EXAMPLE_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "$EXAMPLE_DIR/.." && pwd)

# Where every generated file goes. Ignored by example-cli/.gitignore.
OUT_DIR=${OUT_DIR:-$EXAMPLE_DIR/out}

# The GraphQL endpoint to introspect. Defaults to the repo's mockapi server.
GRAPHQL_URL=${GRAPHQL_URL:-http://localhost:8081/query}
export GRAPHQL_URL

BIN_DIR=$OUT_DIR/bin

# build_clis compiles both CLIs into OUT_DIR/bin and puts them first on PATH,
# so the rest of each script calls `gqlkit` and `gqlkit-sdl` exactly as a user
# with the released binaries would.
build_clis() {
	mkdir -p "$BIN_DIR"
	(cd "$REPO_ROOT" && go build -o "$BIN_DIR/gqlkit" ./gqlkit/cmd/gqlkit)
	(cd "$REPO_ROOT" && go build -o "$BIN_DIR/gqlkit-sdl" ./gqlkit-sdl)
	PATH=$BIN_DIR:$PATH
	export PATH
}

# require_endpoint fails loudly when nothing answers GraphQL at GRAPHQL_URL,
# instead of letting a later step fail with a confusing connection error.
require_endpoint() {
	if ! curl -sf -m 5 -X POST -H 'Content-Type: application/json' \
		--data '{"query":"{__typename}"}' "$GRAPHQL_URL" >/dev/null; then
		echo "error: no GraphQL server answered at $GRAPHQL_URL" >&2
		echo "       start the repo's test server first: task mockapi:run" >&2
		exit 1
	fi
}
