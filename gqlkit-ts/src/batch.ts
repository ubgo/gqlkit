/**
 * @module batch
 *
 * Merge multiple builders into a single GraphQL operation with aliases, so
 * callers can fetch several root fields in one round trip.
 *
 * @example
 * ```ts
 * const { open, completed } = await batch(client, {
 *   open: sdk.tasks(),
 *   completed: sdk.tasks().status(Status.Completed),
 * });
 * ```
 */

import { GraphQLClient } from "./graphqlclient";

/**
 * Shape of an operation slice produced by a builder so it can be merged into
 * a batched, multi-root document.
 */
export interface OpFragment {
  opType: string;
  varDecls: string[];
  varValues: Record<string, unknown>;
  aliasedField: string;
}

/**
 * Structural type for any builder that can participate in a {@link batch}
 * call. Generated builders compose (rather than extend) `BaseBuilder`, so this
 * interface only requires the two methods batch needs — `execute()` to derive
 * the result type and `getOpFragment(alias)` to merge into the operation.
 */
export interface BatchableBuilder<TResult = unknown> {
  execute(): Promise<TResult>;
  getOpFragment(alias: string): OpFragment;
}

/**
 * Map an alias-keyed builder map to its result shape, where each alias key
 * resolves to that builder's `execute()` return type.
 */
export type BatchResult<
  T extends Record<string, BatchableBuilder<unknown>>,
> = {
  [K in keyof T]: T[K] extends BatchableBuilder<infer R> ? R : never;
};

/**
 * Options for {@link batch}.
 */
export interface BatchOptions {
  /** GraphQL operation name. Default: `"Batch"`. */
  opName?: string;
}

/**
 * Execute multiple builders as a single GraphQL operation.
 *
 * - All builders must share the same operation type (all `query` or all
 *   `mutation`); mixing produces an error.
 * - Each builder's arguments are namespaced with the alias to avoid variable
 *   name collisions in the merged document.
 * - Returns an object keyed by alias, each value typed from that builder's
 *   `execute()` return type.
 *
 * @param client   - Shared client used to send the merged operation.
 * @param builders - Alias → builder map. Aliases become GraphQL response keys.
 * @param options  - Optional operation name override.
 */
export async function batch<
  T extends Record<string, BatchableBuilder<unknown>>,
>(
  client: GraphQLClient,
  builders: T,
  options: BatchOptions = {},
): Promise<BatchResult<T>> {
  const entries = Object.entries(builders);
  if (entries.length === 0) {
    throw new Error("batch: requires at least one builder");
  }

  const fragments = entries.map(([alias, b]) => b.getOpFragment(alias));

  const opType = fragments[0]!.opType;
  for (const f of fragments) {
    if (f.opType !== opType) {
      throw new Error(
        `batch: cannot mix operation types — saw "${opType}" and "${f.opType}"`,
      );
    }
  }

  const allDecls = fragments.flatMap((f) => f.varDecls);
  const varStr = allDecls.length > 0 ? `(${allDecls.join(", ")})` : "";

  const fields = fragments.map((f) => f.aliasedField).join("\n  ");

  const variables: Record<string, unknown> = {};
  for (const f of fragments) {
    Object.assign(variables, f.varValues);
  }

  const opName = options.opName ?? "Batch";
  const query = `${opType} ${opName}${varStr} {\n  ${fields}\n}`;

  return (await client.execute<Record<string, unknown>>(
    query,
    variables,
  )) as BatchResult<T>;
}
