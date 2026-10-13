/**
 * @module codec
 *
 * Opt-in scalar codecs: turn custom-scalar wire values into app values on the
 * way in (decode) and back on the way out (encode).
 *
 * Wiring (the generator emits all of it; nothing runs unless a binding in
 * `config.jsonc` sets `decode` / `encode`):
 * - `FieldSelection.addField(name, decode)` attaches a decoder to a leaf; the
 *   selection compiles its decoders into a {@link DecodePlan} and walks the
 *   response with it (lists, nested objects, aliases, inline fragments).
 * - `BaseBuilder.setArg(name, value, type, encoder)` attaches an
 *   {@link ArgEncoder}; values are encoded when variables are built.
 * - `BaseBuilder.getOpFragment` exposes the decode step so `batch()` applies it.
 *
 * Invariants shared by both directions:
 * - `null` / `undefined` never reach a codec; they pass through at every level.
 * - Without a `listDepth`, arrays are mapped at any depth. With one, exactly
 *   that many array levels are mapped and anything deeper is handed to the
 *   codec whole — needed for a scalar whose own wire value is an array.
 * - A throwing codec is wrapped in {@link ScalarCodecError} naming the path.
 */

/**
 * Turns one non-null wire value into an app value.
 *
 * The `never` parameter is deliberate: user codecs have concrete parameter
 * types (`(iso: string) => DateTime`), and `never` makes every one-argument
 * function assignable without a cast in generated code. The runtime is the
 * only caller and owns the single boundary cast (see {@link callCodec}).
 */
export type ScalarDecoder = (raw: never) => unknown;

/** Turns one non-null app value into its wire value. `never` for the same reason as {@link ScalarDecoder}. */
export type ScalarEncoder = (value: never) => unknown;

/**
 * Encoding plan for one argument or input-object field.
 *
 * - `{ scalar }` — encode the value (or each list element) with a scalar encoder.
 *   `listDepth` is the number of list wrappers in the GraphQL type
 *   (`DateTime!` → 0, `[[DateTime]]` → 2); omit it to map arrays at any depth.
 * - `{ input }` — the value is an input object; encode its fields with the
 *   returned plan. A thunk so self-referential inputs
 *   (`input Filter { and: [Filter!] }`) can reference their own plan.
 *   Input objects are never arrays, so lists around them map at any depth.
 */
export type ArgEncoder =
  | { readonly scalar: ScalarEncoder; readonly listDepth?: number }
  | { readonly input: () => InputEncodePlan };

/** Per-field encoders of an input object. Fields not listed are copied as-is. */
export type InputEncodePlan = { readonly [field: string]: ArgEncoder };

/** Which way a codec was running when it failed. */
export const CodecDirection = {
  Decode: "decode",
  Encode: "encode",
} as const;

/** Union of {@link CodecDirection} values. */
export type CodecDirection = (typeof CodecDirection)[keyof typeof CodecDirection];

/**
 * Thrown when a scalar codec throws, or when a value cannot be routed to its
 * codec (an inline fragment with decoders but no `__typename` in the response).
 *
 * Why it exists: a bare `Invalid DateTime` from deep inside a list of unions is
 * undebuggable; this names the exact response (or variable) path.
 *
 * Path shapes: decode paths start at the root field name or batch alias
 * (`["todos", 0, "createdAt"]`); encode paths start at `$<arg>`
 * (`["$filter", "createdAfter"]`).
 */
export class ScalarCodecError extends Error {
  /** Whether the failure happened while decoding a response or encoding variables. */
  public direction: CodecDirection;
  /** Location of the failing value. */
  public path: (string | number)[];
  /** What the codec threw (or the routing failure). */
  public cause: unknown;

  /**
   * @param direction - Decode or encode.
   * @param path      - Path of the failing value.
   * @param cause     - The underlying error.
   */
  constructor(direction: CodecDirection, path: (string | number)[], cause: unknown) {
    const reason = cause instanceof Error ? cause.message : String(cause);
    super(`gqlkit: failed to ${direction} "${path.join(".")}": ${reason}`);
    this.name = "ScalarCodecError";
    this.direction = direction;
    this.path = path;
    this.cause = cause;
  }
}

/**
 * Normalize anything thrown while decoding into a {@link ScalarCodecError}.
 *
 * Decoding already wraps codec failures, so this is the identity in practice;
 * the fallback keeps the `decodeError` slots honestly typed if an unexpected
 * error (a bug in the walker itself) ever escapes.
 *
 * @param err      - What was thrown.
 * @param rootPath - Path to report for a non-codec error.
 */
export function toCodecError(err: unknown, rootPath: (string | number)[] = []): ScalarCodecError {
  return err instanceof ScalarCodecError ? err : new ScalarCodecError(CodecDirection.Decode, rootPath, err);
}

/**
 * A path as a parent-linked list, so walking a large response allocates one
 * small node per step and only materializes an array when an error is thrown.
 */
export interface PathNode {
  readonly parent: PathNode | undefined;
  readonly key: string | number;
}

/** Extend a path by one segment. */
export function childPath(parent: PathNode | undefined, key: string | number): PathNode {
  return { parent, key };
}

/** Materialize a {@link PathNode} into root-first segments. */
export function pathToArray(node: PathNode | undefined): (string | number)[] {
  const out: (string | number)[] = [];
  for (let n = node; n !== undefined; n = n.parent) out.push(n.key);
  return out.reverse();
}

/**
 * Invoke a codec on one non-null value, wrapping failures with the path.
 *
 * Boundary cast: codecs are typed `(x: never) => unknown` so concrete user
 * functions are assignable (see {@link ScalarDecoder}). The generated code
 * guarantees the value at this position is what the codec expects — the same
 * unchecked claim `execute<T>` makes about the response.
 */
function callCodec(
  codec: ScalarDecoder | ScalarEncoder,
  value: unknown,
  direction: CodecDirection,
  path: PathNode | undefined,
): unknown {
  try {
    return codec(value as never);
  } catch (err) {
    if (err instanceof ScalarCodecError) throw err;
    throw new ScalarCodecError(direction, pathToArray(path), err);
  }
}

/** Narrow `unknown` to a plain (non-array, non-null) object. */
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Remaining list depth after descending one level; `undefined` stays "any depth". */
function deeper(listDepth: number | undefined): number | undefined {
  return listDepth === undefined ? undefined : listDepth - 1;
}

/** Whether an array at this point is a GraphQL list to map over, rather than a scalar's own value. */
function isListLevel(value: unknown, listDepth: number | undefined): value is unknown[] {
  return Array.isArray(value) && (listDepth === undefined || listDepth > 0);
}

/**
 * Count the list wrappers in a GraphQL type string (`[[ID!]!]` → 2).
 * Used to derive an argument's list depth from the type `setArg` already gets.
 */
export function listDepthOf(graphqlType: string): number {
  let depth = 0;
  for (const ch of graphqlType) if (ch === "[") depth++;
  return depth;
}

// ---------------------------------------------------------------------------
// Decoding
// ---------------------------------------------------------------------------

/** A decoder bound to a leaf field. */
export interface LeafDecoder {
  readonly decode: ScalarDecoder;
  /** List wrappers in the field type; `undefined` = map arrays at any depth. */
  readonly listDepth: number | undefined;
}

/** An inline fragment's decoders and the `__typename` values it applies to. */
export interface FragmentPlan {
  /** `undefined` for a fragment without a type condition (always applies). */
  readonly typeNames: readonly string[] | undefined;
  readonly plan: DecodePlan;
}

/**
 * The decoding-relevant part of a selection, keyed by response key. Branches
 * without any decoder are left out, so the walk only visits what it changes.
 */
export interface DecodePlan {
  readonly leaves: Map<string, LeafDecoder>;
  readonly fields: Map<string, DecodePlan>;
  readonly fragments: FragmentPlan[];
}

/** A plan with fragments resolved for one concrete `__typename`. */
interface FlatPlan {
  readonly leaves: Map<string, LeafDecoder>;
  readonly fields: Map<string, DecodePlan>;
}

/** Merge two plans for the same response object. First leaf decoder per key wins. */
function mergePlans(a: DecodePlan, b: DecodePlan): DecodePlan {
  const leaves = new Map(a.leaves);
  for (const [k, v] of b.leaves) if (!leaves.has(k)) leaves.set(k, v);
  const fields = new Map(a.fields);
  for (const [k, v] of b.fields) {
    const existing = fields.get(k);
    fields.set(k, existing ? mergePlans(existing, v) : v);
  }
  return { leaves, fields, fragments: [...a.fragments, ...b.fragments] };
}

/**
 * Fold the fragments that match `typeName` into one flat plan.
 *
 * Why: GraphQL merges a field selected both directly and inside a matching
 * fragment into ONE response key. Applying each plan separately would decode
 * that value twice (the second call would get a `DateTime`, not a string), so
 * every applicable decoder is merged first and each key is decoded once.
 */
function flatten(plan: DecodePlan, typeName: string | undefined, out: { leaves: Map<string, LeafDecoder>; fields: Map<string, DecodePlan> }): void {
  for (const [k, v] of plan.leaves) if (!out.leaves.has(k)) out.leaves.set(k, v);
  for (const [k, v] of plan.fields) {
    const existing = out.fields.get(k);
    out.fields.set(k, existing ? mergePlans(existing, v) : v);
  }
  for (const frag of plan.fragments) {
    if (frag.typeNames === undefined || (typeName !== undefined && frag.typeNames.includes(typeName))) {
      flatten(frag.plan, typeName, out);
    }
  }
}

/** Whether a plan (or any nested fragment) needs `__typename` to resolve. */
function needsTypeName(plan: DecodePlan): boolean {
  return plan.fragments.some((f) => f.typeNames !== undefined || needsTypeName(f.plan));
}

/** Response key read from the GraphQL meta-field that discriminates union/interface members. */
export const TYPENAME_KEY = "__typename";

/**
 * Walks a response with one {@link DecodePlan}, mutating it in place.
 *
 * In place on purpose: the response was just parsed by this runtime, nobody
 * else holds it, and copying every list would cost more than the decoding.
 * Flattened plans are cached per `(plan, __typename)` for the walk's lifetime,
 * so a 1000-element union list resolves its fragments once per member type.
 */
export class Decoder {
  private flatCache = new Map<DecodePlan, Map<string, FlatPlan>>();

  /** Decode the value of an object-typed field (object, list of objects, or null). */
  decodeObjects(value: unknown, plan: DecodePlan, path: PathNode | undefined): void {
    if (Array.isArray(value)) {
      value.forEach((item, i) => this.decodeObjects(item, plan, childPath(path, i)));
    } else if (isRecord(value)) {
      this.decodeRecord(value, plan, path);
    }
    // null / undefined / unexpected primitives: nothing to decode.
  }

  /** Decode a scalar-typed value (leaf or list of leaves), returning the replacement. */
  decodeLeaf(value: unknown, leaf: LeafDecoder, listDepth: number | undefined, path: PathNode | undefined): unknown {
    if (value === null || value === undefined) return value;
    if (isListLevel(value, listDepth)) {
      for (let i = 0; i < value.length; i++) {
        value[i] = this.decodeLeaf(value[i], leaf, deeper(listDepth), childPath(path, i));
      }
      return value;
    }
    return callCodec(leaf.decode, value, CodecDirection.Decode, path);
  }

  private decodeRecord(obj: Record<string, unknown>, plan: DecodePlan, path: PathNode | undefined): void {
    const flat = this.resolve(obj, plan, path);
    for (const [key, leaf] of flat.leaves) {
      if (key in obj) obj[key] = this.decodeLeaf(obj[key], leaf, leaf.listDepth, childPath(path, key));
    }
    for (const [key, child] of flat.fields) {
      if (key in obj) this.decodeObjects(obj[key], child, childPath(path, key));
    }
  }

  private resolve(obj: Record<string, unknown>, plan: DecodePlan, path: PathNode | undefined): FlatPlan {
    let typeName: string | undefined;
    if (needsTypeName(plan)) {
      const raw = obj[TYPENAME_KEY];
      if (typeof raw !== "string") {
        throw new ScalarCodecError(
          CodecDirection.Decode,
          pathToArray(path),
          new Error(`an inline fragment has decoders but the response has no ${TYPENAME_KEY}; select ${TYPENAME_KEY} next to the fragment`),
        );
      }
      typeName = raw;
    }
    const cacheKey = typeName ?? "";
    let byType = this.flatCache.get(plan);
    if (!byType) {
      byType = new Map();
      this.flatCache.set(plan, byType);
    }
    let flat = byType.get(cacheKey);
    if (!flat) {
      flat = { leaves: new Map(), fields: new Map() };
      flatten(plan, typeName, flat);
      byType.set(cacheKey, flat);
    }
    return flat;
  }
}

// ---------------------------------------------------------------------------
// Encoding
// ---------------------------------------------------------------------------

/**
 * Encode one argument or input-field value. Returns a new value; the caller's
 * objects and arrays are never mutated (they are app state, unlike responses).
 *
 * @param listDepth - List wrappers left to map; `undefined` = any depth.
 */
export function encodeValue(
  value: unknown,
  encoder: ArgEncoder,
  listDepth: number | undefined,
  path: PathNode | undefined,
): unknown {
  if (value === null || value === undefined) return value;
  if (isListLevel(value, listDepth)) {
    return value.map((item, i) => encodeValue(item, encoder, deeper(listDepth), childPath(path, i)));
  }
  if ("scalar" in encoder) {
    return callCodec(encoder.scalar, value, CodecDirection.Encode, path);
  }
  if (!isRecord(value)) return value;
  const plan = encoder.input();
  const out: Record<string, unknown> = { ...value };
  for (const [field, fieldEncoder] of Object.entries(plan)) {
    if (!(field in out)) continue;
    const depth = "scalar" in fieldEncoder ? fieldEncoder.listDepth : undefined;
    out[field] = encodeValue(out[field], fieldEncoder, depth, childPath(path, field));
  }
  return out;
}
