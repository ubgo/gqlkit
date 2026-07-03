---
name: Bug report
about: Something isn't working as expected
title: "[bug] "
labels: bug
---

**What happened**
A clear description of the bug.

**Minimal reproduction**
The smallest GraphQL schema + exact command that reproduces it. For a generated-SDK compile error, include the failing `go build` / `tsc` output.

```graphql
# schema.graphql
```

```sh
gqlkit generate -s schema.graphql -o out -p sdk -m example.com/sdk
```

**Expected behavior**
What you expected to happen instead.

**Environment**
- Artifact + version: <!-- e.g. gqlkit 0.9.0 (`gqlkit --version`), gqlkit-sdl, or gqlkit-ts -->
- Go / Node version:
- OS / arch:
- Install method: <!-- install.sh / go install / binary / npm -->

**Logs / output**
Paste any relevant output (redact secrets/tokens).
