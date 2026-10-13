// Run every guide sample in this folder against mockapi, in order, and exit
// non-zero if any of them fails. This is what proves the snippets quoted in
// docs/typescript/*.md still run.
//
// Run: npx tsx cmd/samples/run-all.ts   (needs mockapi: `task mockapi:run`)
//
// main.ts (the original tour) and type-test.ts (compile-time only) are not
// listed: main.ts runs itself on import, and type-test.ts is checked by tsc.

import { run as batchOptions } from "./batch-options";
import { run as clientOptions } from "./client-options";
import { run as enums } from "./enums";
import { run as errors } from "./errors";
import { run as lowLevel } from "./low-level";
import { requireMockapi } from "./mockapi";
import { run as scalars } from "./scalars";
import { run as selecting } from "./selecting";
import { run as unions } from "./unions";

/** Sample name → its run function. Order is the order the guides are read in. */
const SAMPLES: ReadonlyArray<readonly [string, () => Promise<void>]> = [
  ["client-options", clientOptions],
  ["selecting", selecting],
  ["enums", enums],
  ["scalars", scalars],
  ["errors", errors],
  ["batch-options", batchOptions],
  ["unions", unions],
  ["low-level", lowLevel],
];

async function main(): Promise<void> {
  await requireMockapi();
  const failed: string[] = [];
  for (const [name, run] of SAMPLES) {
    try {
      await run();
      console.log(`${name}: ok\n`);
    } catch (err) {
      console.error(`${name}: FAILED`, err, "\n");
      failed.push(name);
    }
  }
  if (failed.length > 0) {
    throw new Error(`${failed.length} of ${SAMPLES.length} samples failed: ${failed.join(", ")}`);
  }
  console.log(`all ${SAMPLES.length} samples passed`);
}

main().catch((err: unknown) => {
  console.error(err instanceof Error ? err.message : err);
  process.exitCode = 1;
});
