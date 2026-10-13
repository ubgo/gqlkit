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

import { GraphQLClient, GraphQLErrors } from "./graphqlclient";
import { toCodecError } from "./codec";
import type { OperationType } from "./builder";

/**
 * Shape of an operation slice produced by a builder so it can be merged into
 * a batched, multi-root document. Produced by `BaseBuilder.getOpFragment`.
 */
export interface OpFragment {
  /** Operation keyword; every fragment in a batch must agree. */
  opType: OperationType;
  /** Variable declarations, already alias-namespaced, e.g. `$open_status: Status`. */
  varDecls: string[];
  /** Variable values keyed by the same namespaced names as {@link varDecls}. */
  varValues: Record<string, unknown>;
  /** `alias: field(args) { selection }`, ready to splice into the operation body. */
  aliasedField: string;
  /**
   * Decodes this alias's value in the response (scalar codecs). Present only
   * when the builder has a reachable decoder, so batches without codecs skip
   * the step entirely. Receives `data[alias]`; returns the replacement.
   */
  decode?: (value: unknown) => unknown;
}

/**
 * Structural type for any builder that can participate in a {@link batch}
 * call. Generated builders compose (rather than extend) `BaseBuilder`, so this
 * interface only requires the two methods batch needs — `execute()` to derive
 * the result type and `getOpFragment(alias)` to merge into the operation.
 *
 * `execute()` is never called by `batch`; it exists only so the result type
 * can be inferred. Scalar decoding reaches batched results through
 * `OpFragment.decode` instead; any other post-processing a builder's
 * `execute()` does is NOT applied.
 */
export interface BatchableBuilder<TResult = unknown> {
  execute(): Promise<TResult>;
  getOpFragment(alias: string): OpFragment;
}

/**
 * Map an alias-keyed builder map to its result shape, where each alias key
 * resolves to that builder's `execute()` return type.
 *
 * This is the key win over the Go side: TS mapped types let us *infer* a
 * different result type per alias from a heterogeneous input map, so callers
 * get fully-typed `result.open`, `result.completed`, etc. without declaring
 * a separate result struct. The Go batch package can't replicate this and
 * uses a caller-supplied struct instead.
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
  /** GraphQL operation name. Default: {@link DEFAULT_BATCH_OP_NAME}. Must be a valid GraphQL name. */
  opName?: string;
  /** Cancels the merged request; same semantics as `RequestOptions.signal` on `GraphQLClient.execute`. */
  signal?: AbortSignal;
}

/** Operation name used when {@link BatchOptions.opName} is not given. Matches the Go runtime. */
export const DEFAULT_BATCH_OP_NAME = "Batch";

/**
 * A GraphQL `Name` token (spec §2.1.9): letter or underscore, then letters,
 * digits or underscores.
 *
 * Aliases and the operation name are spliced into the document verbatim, so
 * anything else (`my-alias`, `1st`, `a b`) would produce a document the server
 * rejects with an opaque syntax error — or, worse, change its meaning.
 * Validating up front turns that into a clear client-side error. It also rules
 * out integer-like keys, which JS objects would silently reorder.
 */
export const GRAPHQL_NAME_PATTERN = /^[_A-Za-z][_0-9A-Za-z]*$/;

/**
 * Execute multiple builders as a single GraphQL operation.
 *
 * - All builders must share the same operation type (all `query` or all
 *   `mutation`); mixing produces an error.
 * - Each builder's arguments are namespaced as `<alias>_<arg>` to avoid
 *   variable name collisions; the rare collision namespacing cannot prevent
 *   (alias `a` + arg `b_c` vs alias `a_b` + arg `c`) is rejected.
 * - Root fields are emitted in the map's insertion order. For mutations that
 *   is the execution order: GraphQL runs root mutation fields serially.
 * - Returns an object keyed by alias, each value typed from that builder's
 *   `execute()` return type. The values are the response values, decoded by
 *   each builder's scalar codecs when it has any (see `OpFragment.decode`).
 * - If any alias fails, the whole call rejects with `GraphQLErrors`; the
 *   aliases that resolved are on its `data`, and each error's `path[0]` names
 *   the failing alias.
 *
 * @param client   - Shared client used to send the merged operation.
 * @param builders - Alias → builder map. Aliases become GraphQL response keys.
 * @param options  - Optional operation name override.
 * @throws {Error} On an empty map, an invalid alias or operation name, mixed
 *                 operation types, or a variable-name collision — all before
 *                 any request is sent.
 */
export async function batch<
  T extends Record<string, BatchableBuilder<unknown>>,
>(
  client: GraphQLClient,
  builders: T,
  options: BatchOptions = {},
): Promise<BatchResult<T>> {
  const opName = options.opName ?? DEFAULT_BATCH_OP_NAME;
  if (!GRAPHQL_NAME_PATTERN.test(opName)) {
    throw new Error(`batch: invalid operation name "${opName}"`);
  }

  const fragments: OpFragment[] = [];
  const decoders: [alias: string, decode: (value: unknown) => unknown][] = [];
  for (const [alias, builder] of Object.entries(builders)) {
    if (!GRAPHQL_NAME_PATTERN.test(alias)) {
      throw new Error(`batch: invalid alias "${alias}" — must match ${GRAPHQL_NAME_PATTERN}`);
    }
    const fragment = builder.getOpFragment(alias);
    fragments.push(fragment);
    if (fragment.decode) decoders.push([alias, fragment.decode]);
  }

  const [first] = fragments;
  if (first === undefined) {
    throw new Error("batch: requires at least one builder");
  }

  // Op-type validation is runtime-only on the TS side: every builder
  // structurally satisfies BatchableBuilder regardless of whether it
  // produces a query or mutation, so the type system can't reject mixing.
  // The Go batch package gets compile-time safety here via marker types —
  // see gqlkit/pkg/batch for the equivalent guarantee on the Go side.
  const opType = first.opType;
  const variables: Record<string, unknown> = {};
  for (const f of fragments) {
    if (f.opType !== opType) {
      throw new Error(
        `batch: cannot mix operation types — saw "${opType}" and "${f.opType}"`,
      );
    }
    for (const [name, value] of Object.entries(f.varValues)) {
      if (Object.prototype.hasOwnProperty.call(variables, name)) {
        throw new Error(
          `batch: variable "$${name}" is produced by two aliases — rename one alias so "<alias>_<arg>" names stay unique`,
        );
      }
      variables[name] = value;
    }
  }

  const allDecls = fragments.flatMap((f) => f.varDecls);
  const varStr = allDecls.length > 0 ? `(${allDecls.join(", ")})` : "";
  const fields = fragments.map((f) => f.aliasedField).join("\n  ");
  const query = `${opType} ${opName}${varStr} {\n  ${fields}\n}`;

  let data: Record<string, unknown>;
  try {
    data = await client.execute<Record<string, unknown>>(query, variables, { signal: options.signal });
  } catch (err) {
    if (err instanceof GraphQLErrors && err.data !== undefined && decoders.length > 0) {
      // Partial data is exposed only once it decodes cleanly: a mix of decoded
      // and wire values would be worse than none. The server's errors stay the
      // thrown error either way — they are the primary failure.
      try {
        applyDecoders(err.data, decoders);
      } catch (decodeErr) {
        err.data = undefined;
        err.decodeError = toCodecError(decodeErr);
      }
    }
    throw err;
  }
  applyDecoders(data, decoders);

  // Boundary cast: the response is keyed by the aliases we sent, and each
  // value's type is the builder's declared `execute()` type — the same
  // unverified claim a single builder's `execute()` makes.
  return data as BatchResult<T>;
}

/** Decode each alias that has a decoder and is present in `data`, in place. */
function applyDecoders(
  data: Record<string, unknown>,
  decoders: [alias: string, decode: (value: unknown) => unknown][],
): void {
  for (const [alias, decode] of decoders) {
    if (alias in data) data[alias] = decode(data[alias]);
  }
}
