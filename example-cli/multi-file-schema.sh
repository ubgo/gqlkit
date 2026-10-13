#!/bin/sh
# Generate a Go SDK from a schema split across several SDL files, using a
# glob for --schema. Needs no server.
#
# Writes OUT_DIR/multi-file/ (go.mod, schema/*.graphql, sdk/).
set -eu

. "$(dirname -- "$0")/lib.sh"

build_clis

# See generate-go.sh: only needed because this module sits inside the
# gqlkit repo's go.work tree.
GOWORK=off
export GOWORK

APP=$OUT_DIR/multi-file
rm -rf "$APP"
mkdir -p "$APP/schema"
cd "$APP"

go mod init example.com/multifile

# 1. One file per area of the schema. A type defined in one file can be
#    used in another, and `extend type` adds fields to a type from elsewhere.
cat >schema/user.graphql <<'EOF'
type User {
  id: ID!
  name: String!
}

type Query {
  user(id: ID!): User
}
EOF

cat >schema/post.graphql <<'EOF'
type Post {
  id: ID!
  title: String!
  author: User!
}

extend type Query {
  posts: [Post!]!
}
EOF

# 2. Quote the glob so gqlkit expands it, not the shell. `**` also works,
#    e.g. './schema/**/*.graphql'.
gqlkit generate -s './schema/*.graphql' -o ./sdk -m "$(go list -m)/sdk"

# 3. Both files' operations are in the SDK.
ls sdk/queries
go build ./...
