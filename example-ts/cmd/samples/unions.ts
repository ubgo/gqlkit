// Unions: select each member with on<Type>(), get a discriminated result
// type keyed on __typename, and narrow it with a switch.
//
// Guide: docs/typescript/unions.md
// Run:   npx tsx cmd/samples/unions.ts   (needs mockapi)

import assert from "node:assert/strict";
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "../../sdk/queries";
import { MOCKAPI_ENDPOINT, runSample } from "./mockapi";

/** Search term that matches both a seeded todo ("Try the ping query") and a user ("Alice"). */
const SEARCH_TERM = "i";

const qr = new QueryRoot(new GraphQLClient(MOCKAPI_ENDPOINT));

/** Step 1: one on<Type>() per member you want fields from. */
async function searchByMember(): Promise<void> {
  const hits = await qr
    .search()
    .term(SEARCH_TERM)
    .select((r) =>
      r
        .onTodo((t) => t.id().text().done())
        .onUser((u) => u.id().name()),
    )
    .execute();

  for (const hit of hits) {
    // __typename is always selected, so it narrows each hit to its member.
    switch (hit.__typename) {
      case "Todo":
        console.log(`todo ${hit.id}: ${hit.text} (done: ${hit.done})`);
        break;
      case "User":
        console.log(`user ${hit.id}: ${hit.name}`);
        break;
    }
  }
  assert.ok(hits.some((h) => h.__typename === "Todo") && hits.some((h) => h.__typename === "User"));
}

/** Step 2: members you do not select still come back, with only __typename. */
async function oneMemberOnly(): Promise<void> {
  const hits = await qr
    .search()
    .term(SEARCH_TERM)
    .select((r) => r.onUser((u) => u.name()))
    .execute();

  const todos = hits.filter((h) => h.__typename === "Todo");
  console.log("todo hits without a fragment:", JSON.stringify(todos));
}

/** Step 3: without select(), the builder still sends `{ __typename }`. */
async function typenameOnly(): Promise<void> {
  const kinds = await qr.search().term(SEARCH_TERM).execute();
  console.log("kinds:", kinds.map((h) => h.__typename).join(", "));
}

export async function run(): Promise<void> {
  console.log("== Unions ==");
  await searchByMember();
  await oneMemberOnly();
  await typenameOnly();
}

if (require.main === module) runSample("unions", run);
