// Shared plumbing for the runnable samples in this folder.
//
// Every sample talks to the local mockapi test server and nothing else, so a
// sample run can never touch a real account. Each sample file exports a
// `run()` function (so run-all.ts can chain them) and also runs itself when
// executed directly with `npx tsx cmd/samples/<name>.ts`.

import { GraphQLClient } from "gqlkit-ts";

/**
 * GraphQL endpoint of the mockapi test server. Start it with
 * `task mockapi:run` from the repo root; it listens on port 8081.
 */
export const MOCKAPI_ENDPOINT = "http://localhost:8081/query";

/** The fixed answer mockapi gives to `ping`; used to prove it is really mockapi. */
export const MOCKAPI_PING_RESPONSE = "pong from gqlgenapi";

/**
 * Fail loudly, with the fix in the message, when mockapi is not running.
 *
 * Why: without this, a stopped server surfaces as a bare `TypeError: fetch
 * failed` from deep inside the first sample, which reads like a gqlkit bug.
 */
export async function requireMockapi(): Promise<void> {
  const client = new GraphQLClient(MOCKAPI_ENDPOINT);
  let pong: unknown;
  try {
    pong = (await client.execute<{ ping: unknown }>("query Ping { ping }")).ping;
  } catch (err) {
    throw new Error(
      `mockapi is not reachable at ${MOCKAPI_ENDPOINT} (${String(err)}). Start it with \`task mockapi:run\` from the repo root.`,
    );
  }
  if (pong !== MOCKAPI_PING_RESPONSE) {
    throw new Error(
      `${MOCKAPI_ENDPOINT} answered ping with ${JSON.stringify(pong)}, not ${JSON.stringify(MOCKAPI_PING_RESPONSE)}: is something other than mockapi on port 8081?`,
    );
  }
}

/**
 * Run one sample as a standalone program: check mockapi, run, and turn any
 * failure into a non-zero exit code so `tsx` (and CI) report it.
 */
export function runSample(name: string, run: () => Promise<void>): void {
  requireMockapi()
    .then(run)
    .then(() => console.log(`\n${name}: ok`))
    .catch((err: unknown) => {
      console.error(`\n${name}: FAILED`);
      console.error(err);
      process.exitCode = 1;
    });
}
