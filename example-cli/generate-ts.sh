#!/bin/sh
# Generate a TypeScript SDK with `gqlkit generate-ts`, type-check it with a
# small program that uses it, and run that program.
# Needs a GraphQL server at GRAPHQL_URL (mockapi by default), and the npm
# packages in example-cli/package.json (npm install), which include the
# local gqlkit-ts build (task gqlkit-ts:setup).
#
# Writes OUT_DIR/ts-app/ (schema.graphql, sdk/, codecs.ts, main.ts) and compiles it to
# OUT_DIR/ts-dist/.
set -eu

. "$(dirname -- "$0")/lib.sh"

build_clis
require_endpoint

if [ ! -d "$EXAMPLE_DIR/node_modules/gqlkit-ts/dist" ]; then
	echo "error: npm packages missing; run: (cd example-cli && npm install)" >&2
	echo "       and build gqlkit-ts once: task gqlkit-ts:setup" >&2
	exit 1
fi

APP=$OUT_DIR/ts-app
rm -rf "$APP" "$OUT_DIR/ts-dist"
mkdir -p "$APP"
cd "$APP"

# 1. The schema.
gqlkit-sdl fetch --url "$GRAPHQL_URL" -o schema.graphql

# 2. The SDK. There is no -m or -p: TypeScript imports are relative, and the
#    runtime comes from the gqlkit-ts npm package.
gqlkit generate-ts \
	-s schema.graphql \
	-o ./sdk \
	-c "$EXAMPLE_DIR/ts.config.jsonc"

# 3. The codec module ts.config.jsonc points DateTime at.
cat >codecs.ts <<'EOF'
// Wire <-> app conversion for the DateTime scalar. The server sends
// ISO-8601 strings; app code works with Date objects.
export function decodeDate(raw: string): Date {
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) throw new RangeError(`DateTime: not a date: ${raw}`);
  return d;
}

export function encodeDate(value: Date): string {
  return value.toISOString();
}
EOF

# 4. A program that uses it. The .js suffix on the SDK import works in both
#    CommonJS and Node ESM (nodenext) projects.
cat >main.ts <<'EOF'
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries/index.js";

async function main() {
  const client = new GraphQLClient(process.env.GRAPHQL_URL ?? "");
  const qr = new QueryRoot(client);

  console.log("ping:", await qr.ping().execute());

  const todos = await qr
    .todos()
    .pagination({ limit: 2, offset: 0 })
    .select((t) => t.id().text().createdAt())
    .execute();

  for (const t of todos) {
    // createdAt is a Date: the DateTime codec decoded the wire string.
    // t.done would be a compile error here: it was not selected.
    console.log(`todo ${t.id}: ${JSON.stringify(t.text)} created ${t.createdAt.toISOString().slice(0, 10)}`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
EOF

# 5. Prove it: type-check, compile, run.
cd "$EXAMPLE_DIR"
npx tsc --noEmit -p .
npx tsc -p .
node "$OUT_DIR/ts-dist/main.js"
