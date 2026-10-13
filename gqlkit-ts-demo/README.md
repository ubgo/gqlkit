# gqlkit-ts demo

A smoke test for the **published** [gqlkit-ts](https://www.npmjs.com/package/gqlkit-ts) npm package: it installs `gqlkit-ts` from the npm registry (not from this repo), checks that the exports load, and runs one raw query against a public GraphQL API. Use it after a release to confirm that what npm serves actually works.

It is not a tutorial. To learn the TypeScript SDK, start with [Getting started — TypeScript](../docs/getting-started-typescript.md) and the runnable samples in [example-ts](../example-ts/README.md), which use the local runtime and the local mockapi server.

## Before you start

- Node.js 18 or later (the demo uses the global `fetch`).
- Internet access: the query goes to `https://countries.trevorblades.com/graphql`, a free public API run by a third party.

## Run

```bash
npm install
npm start
```

## What it does

- Imports `GraphQLClient`, `GraphQLErrors`, `FieldSelection` and `BaseBuilder` from `gqlkit-ts` and prints the type of each, so a broken build of the package fails here first.
- Sends `query { countries { name code } }` with `client.execute<T>()` and prints the first five countries.
- On a GraphQL error, prints `err.errors`; any other error is re-thrown, which exits non-zero.

## Expected output

```
GraphQLClient: function
GraphQLErrors: function
FieldSelection: function
BaseBuilder: function

Fetched 250 countries. First 5:
  AD - Andorra
  AE - United Arab Emirates
  AF - Afghanistan
  AG - Antigua and Barbuda
  AI - Anguilla

gqlkit-ts is working!
```

## Pitfalls

- **It tests the version range in `package.json`, not the latest release.** `npm install` resolves the `gqlkit-ts` range written there; check `npm ls gqlkit-ts` to see which version actually ran, and bump the range to test a new major or minor release.
- **It depends on a service outside this project.** If `countries.trevorblades.com` is down, slow or changes its schema, the demo fails even though `gqlkit-ts` is fine. Check the URL in a browser before suspecting the package.
- **It does not use a generated SDK.** Builders, `.select()` narrowing and `batch()` are exercised by [example-ts](../example-ts/README.md), not here.

## Related

- [gqlkit-ts README](../gqlkit-ts/README.md) — the runtime API.
- [Low-level API](../docs/typescript/low-level.md) — `client.execute<T>()`, used by this demo.
- [Errors](../docs/typescript/errors.md)
