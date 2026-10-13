/**
 * @module builder
 *
 * Provides the query-building primitives that generated SDK code composes.
 * - {@link FieldSelection} models the recursive field tree in a GraphQL selection set.
 * - {@link BaseBuilder} assembles a complete operation string (with variables) and executes it.
 *
 * Generated SDKs hold a `BaseBuilder` instance (composition, not inheritance)
 * and forward to it, so every public method here is part of the contract the
 * `gqlkit generate-ts` templates rely on. Renaming or re-typing any of them is
 * a breaking change for every generated SDK in the wild.
 */

import { GraphQLClient, GraphQLErrors } from "./graphqlclient";
import type { OpFragment } from "./batch";
import {
  childPath,
  Decoder,
  encodeValue,
  listDepthOf,
  type ArgEncoder,
  type DecodePlan,
  type FragmentPlan,
  type LeafDecoder,
  type ScalarDecoder,
  toCodecError,
} from "./codec";

/**
 * GraphQL operation kinds a builder can emit.
 *
 * Why it exists: the op kind is a closed set, and `batch()` compares it to
 * refuse mixing queries with mutations, so it must come from one place rather
 * than from string literals scattered across call sites. Subscriptions are not
 * listed because the runtime has no streaming transport.
 */
export const OperationType = {
  Query: "query",
  Mutation: "mutation",
} as const;

/** Union of the {@link OperationType} values (`"query" | "mutation"`). */
export type OperationType = (typeof OperationType)[keyof typeof OperationType];

/** Every {@link OperationType} value, for iteration and validation. */
export const OPERATION_TYPES: readonly OperationType[] = Object.values(OperationType);

/**
 * Spaces added per nesting level when serializing a selection set.
 *
 * Purely cosmetic (GraphQL ignores whitespace), but tests and humans reading
 * logged queries depend on stable output, so it is fixed here.
 */
export const SELECTION_INDENT_STEP = 2;

/**
 * Indent of the first selection level under a root field.
 *
 * The operation body is laid out as `op Name {` (column 0) → root field
 * (column 2) → its selection (column 4), so the root selection starts at two
 * indent steps.
 */
export const ROOT_SELECTION_INDENT = SELECTION_INDENT_STEP * 2;

/** Leading token of a GraphQL Name; the first one in a field entry is its response key. */
const FIRST_NAME_PATTERN = /^\s*([_A-Za-z][_0-9A-Za-z]*)/;

/** Prefix of an inline-fragment head (`... on User`), as passed to `addChild`. */
const FRAGMENT_SPREAD = "...";

/** Type-condition keyword inside an inline-fragment head. */
const TYPE_CONDITION_KEYWORD = "on";

/**
 * Response key of a selection entry: the alias if there is one, else the field
 * name. Entries may carry args or directives (`todos(first: 2)`,
 * `recent: todos(first: 2)`, `id @include(if: $x)`), and in every form the
 * first Name token is the key — an alias precedes `:`, otherwise the name
 * leads.
 */
function responseKeyOf(entry: string): string {
  const match = FIRST_NAME_PATTERN.exec(entry);
  return match?.[1] ?? entry.trim();
}

/**
 * Parse an `addChild` key that is an inline fragment.
 *
 * @returns `undefined` when the key is a plain field; `{ typeName: undefined }`
 *          for a fragment without a type condition (`... @include(if: $x)`);
 *          otherwise the type condition's name.
 */
function parseFragmentHead(key: string): { typeName: string | undefined } | undefined {
  const trimmed = key.trim();
  if (!trimmed.startsWith(FRAGMENT_SPREAD)) return undefined;
  const rest = trimmed.slice(FRAGMENT_SPREAD.length).trim();
  const tokens = rest.split(/\s+/);
  if (tokens[0] === TYPE_CONDITION_KEYWORD && tokens[1] !== undefined) {
    return { typeName: tokens[1] };
  }
  return { typeName: undefined };
}

/**
 * Tracks which fields are selected in a GraphQL query.
 *
 * Maintains two lists:
 * - `fields`   : Leaf entries emitted verbatim, like `id`, `name`, or `__typename`.
 * - `children` : Nested selections, each with its own {@link FieldSelection}.
 *   The key is emitted verbatim before the braces, so it may be a field name
 *   (`address`) or an inline fragment head (`... on User`).
 *
 * Optionally it also carries scalar decoders (see `codec.ts`): a decoder on a
 * leaf, plus the `__typename` values each inline fragment applies to. They do
 * not affect {@link build}; {@link decode} uses them to turn wire values into
 * app values.
 *
 * Invariants:
 * - Leaves are emitted before children, each group in insertion order.
 *   Duplicate leaves are emitted as-is (GraphQL merges identical fields).
 * - Re-adding a child under the same key replaces its sub-selection but keeps
 *   its original position.
 * - A child whose sub-selection is empty is omitted entirely, because
 *   `field {}` is not valid GraphQL. This mirrors the Go runtime's
 *   `FieldSelection.Build`.
 *
 * @example
 * ```ts
 * const sel = new FieldSelection();
 * sel.addField("id");
 * sel.addField("name");
 * const addressSel = new FieldSelection();
 * addressSel.addField("city");
 * sel.addChild("address", addressSel);
 * console.log(sel.build());
 * // =>
 * //   id
 * //   name
 * //   address {
 * //     city
 * //   }
 * ```
 */
export class FieldSelection {
  /** Leaf entries at this level of the selection, in insertion order. */
  private fields: string[] = [];

  /** Nested selections keyed by field name or inline-fragment head. */
  private children: Map<string, FieldSelection> = new Map();

  /** Leaf decoders keyed by response key. Empty unless a scalar codec is configured. */
  private decoders: Map<string, LeafDecoder> = new Map();

  /** Explicit `__typename` sets for inline-fragment children, keyed like {@link children}. */
  private possibleTypes: Map<string, readonly string[]> = new Map();

  /**
   * Add a scalar (leaf) field to this selection level.
   *
   * @param name      - The field entry to include (e.g., "id", "email", or
   *                    "recent: createdAt"); emitted verbatim.
   * @param decode    - Optional scalar decoder for this field's value. Keyed by
   *                    the response key (the alias, if any).
   * @param listDepth - List wrappers in the field's GraphQL type
   *                    (`[DateTime]` → 1). Omit to map arrays at any depth;
   *                    pass it when the scalar's own wire value can be an array.
   */
  addField(name: string, decode?: ScalarDecoder, listDepth?: number): void {
    this.fields.push(name);
    if (decode) this.decoders.set(responseKeyOf(name), { decode, listDepth });
  }

  /**
   * Add a nested object field (or inline fragment) with its own sub-selection.
   *
   * The child is held by reference, so fields added to it after this call
   * still appear in the output.
   *
   * @param name          - The field name that returns an object type, or an
   *                        inline fragment head such as `... on User`.
   * @param child         - The {@link FieldSelection} describing which sub-fields to request.
   * @param possibleTypes - For a fragment head only: the `__typename` values
   *                        its decoders apply to. Defaults to the type
   *                        condition (`["User"]`); pass the concrete types for
   *                        an interface condition (`... on Node`), because the
   *                        response carries the concrete `__typename`.
   */
  addChild(name: string, child: FieldSelection, possibleTypes?: readonly string[]): void {
    this.children.set(name, child);
    if (possibleTypes) {
      this.possibleTypes.set(name, possibleTypes);
    } else {
      this.possibleTypes.delete(name);
    }
  }

  /**
   * Whether any field at any depth carries a decoder.
   *
   * Computed from the current tree on each call (children are held by
   * reference and may gain decoders later), so it is never stale.
   */
  hasDecoders(): boolean {
    return this.compileDecodePlan() !== undefined;
  }

  /**
   * Decode a response value selected by this selection, in place.
   *
   * Arrays are walked at any depth, `null` / `undefined` pass through, object
   * children recurse, and `... on X` children apply only to objects whose
   * `__typename` matches. With no decoders this returns immediately.
   *
   * @param value   - The value of the field this selection belongs to (an
   *                  object, a list of objects, or null).
   * @param rootKey - First segment of error paths (the root field or alias).
   * @returns `value` itself, with decoded leaves written into it.
   * @throws {ScalarCodecError} If a decoder throws, or a fragment needs
   *         `__typename` and the response has none.
   */
  decode<T>(value: T, rootKey?: string): T {
    const plan = this.compileDecodePlan();
    if (plan) {
      const root = rootKey === undefined ? undefined : childPath(undefined, rootKey);
      new Decoder().decodeObjects(value, plan, root);
    }
    return value;
  }

  /**
   * Reduce this selection to its decoding-relevant branches.
   *
   * @returns `undefined` when nothing under this selection decodes — the
   *          signal every caller uses to skip the response walk entirely.
   */
  private compileDecodePlan(): DecodePlan | undefined {
    const fields = new Map<string, DecodePlan>();
    const fragments: FragmentPlan[] = [];
    for (const [key, child] of this.children) {
      const plan = child.compileDecodePlan();
      if (!plan) continue;
      const head = parseFragmentHead(key);
      if (head === undefined) {
        fields.set(responseKeyOf(key), plan);
      } else {
        const explicit = this.possibleTypes.get(key);
        const typeNames = explicit ?? (head.typeName === undefined ? undefined : [head.typeName]);
        fragments.push({ typeNames, plan });
      }
    }
    if (this.decoders.size === 0 && fields.size === 0 && fragments.length === 0) {
      return undefined;
    }
    return { leaves: new Map(this.decoders), fields, fragments };
  }

  /**
   * Serialize this selection into a GraphQL selection-set string.
   * Recursively builds nested selections with increasing indentation.
   *
   * @param indent - Number of leading spaces for the current depth (default: one step).
   * @returns A multi-line string representing the GraphQL field selection, or
   *          `""` when {@link isEmpty} is true.
   */
  build(indent: number = SELECTION_INDENT_STEP): string {
    const pad = " ".repeat(indent);
    const parts: string[] = [];

    for (const field of this.fields) {
      parts.push(`${pad}${field}`);
    }

    for (const [name, child] of this.children) {
      // An empty sub-selection would render `name {}`, which servers reject.
      if (child.isEmpty()) continue;
      const nested = child.build(indent + SELECTION_INDENT_STEP);
      parts.push(`${pad}${name} {\n${nested}\n${pad}}`);
    }

    return parts.join("\n");
  }

  /**
   * Check whether this selection would serialize to nothing.
   *
   * A selection is empty when it has no leaves and every child is itself
   * empty — so callers can rely on `isEmpty() === (build() === "")`.
   *
   * @returns `true` if no field at any depth was selected.
   */
  isEmpty(): boolean {
    if (this.fields.length > 0) return false;
    for (const child of this.children.values()) {
      if (!child.isEmpty()) return false;
    }
    return true;
  }
}

/** A recorded operation argument: its runtime value and declared GraphQL type. */
interface ArgEntry {
  /** Value sent in `variables`. `undefined` is dropped by JSON serialization. */
  value: unknown;
  /** GraphQL type annotation used in the variable declaration, e.g. `[ID!]!`. */
  graphqlType: string;
  /** Optional encoder applied when variables are built; the stored value stays the app value. */
  encode: ArgEncoder | undefined;
}

/**
 * Base class for generated GraphQL operation builders (queries and mutations).
 *
 * Generated SDK classes hold a BaseBuilder to provide typed setter methods for
 * each argument and field-selection helpers. BaseBuilder handles:
 * - Storing operation arguments with their GraphQL type annotations.
 * - Accumulating the field selection via {@link FieldSelection}.
 * - Serializing everything into a valid GraphQL operation string.
 * - Executing the operation through the provided {@link GraphQLClient}.
 *
 * Invariants:
 * - Every argument becomes a variable of the same name (`id: $id`); values are
 *   never inlined into the document, so user input cannot alter the query.
 * - Setting the same argument twice keeps the last value and type.
 * - Arguments are declared in first-set order.
 *
 * @example
 * ```
 * // Generated code (simplified):
 * class GetUserBuilder {
 *   private builder = new BaseBuilder(client, "query", "GetUser", "user");
 *   id(value: string) { this.builder.setArg("id", value, "ID!"); return this; }
 *   async execute() { return (await this.builder.executeRaw()).user as User; }
 * }
 * ```
 */
export class BaseBuilder {
  /** The GraphQL client used to execute the built operation. */
  private client: GraphQLClient;

  /** The operation type keyword, emitted verbatim at the start of the document. */
  private opType: OperationType;

  /** The operation name used in the GraphQL document (e.g., "GetUser"). */
  private opName: string;

  /** The root field name inside the operation (e.g., "user", "createUser"). */
  private fieldName: string;

  /**
   * Map of argument name to its runtime value and GraphQL type string.
   * For example: "id" -> { value: "123", graphqlType: "ID!" }
   */
  private args: Map<string, ArgEntry> = new Map();

  /** The field selection tree for the operation's return type. */
  private selection: FieldSelection = new FieldSelection();

  /**
   * Decoder for a root field that returns a codec scalar directly (no
   * selection), e.g. `now: DateTime`. When set it replaces selection decoding.
   */
  private resultDecoder: LeafDecoder | undefined;

  /**
   * @param client    - The GraphQL client to execute operations with.
   * @param opType    - Operation kind, one of {@link OperationType}. Typed as
   *                    the closed union so a typo is a compile error rather
   *                    than a document the server rejects.
   * @param opName    - The named operation identifier (e.g., "GetUser").
   * @param fieldName - The root field to query/mutate (e.g., "user").
   */
  constructor(
    client: GraphQLClient,
    opType: OperationType,
    opName: string,
    fieldName: string,
  ) {
    this.client = client;
    this.opType = opType;
    this.opName = opName;
    this.fieldName = fieldName;
  }

  /**
   * Register an argument for the operation.
   * Called by generated setter methods to record each argument's value and type.
   *
   * @param name        - The argument name (doubles as the variable name).
   * @param value       - The runtime value to send as a variable.
   * @param graphqlType - The GraphQL type annotation (e.g., "String!", "Int", "[ID!]!").
   * @param encode      - Optional encoder for custom scalars (or input objects
   *                      containing them). Applied when variables are built,
   *                      so the stored value stays the app value. List levels
   *                      come from `graphqlType`.
   */
  setArg(name: string, value: unknown, graphqlType: string, encode?: ArgEncoder): void {
    this.args.set(name, { value, graphqlType, encode });
  }

  /**
   * Decode the root field's value with a scalar decoder instead of the
   * selection. For operations whose root field is a codec scalar (`now: DateTime`).
   *
   * @param decode    - The scalar decoder.
   * @param listDepth - List wrappers in the root field's type; omit for any depth.
   */
  setResultDecoder(decode: ScalarDecoder, listDepth?: number): void {
    this.resultDecoder = { decode, listDepth };
  }

  /**
   * Decode the root field's value: with the result decoder if one is set,
   * otherwise with the selection's decoders. Mutates objects and arrays in place.
   *
   * Generated `execute()` calls this only when a codec is reachable, so SDKs
   * without codecs never enter it.
   *
   * @param value   - The root field's value from the response.
   * @param rootKey - First segment of error paths; defaults to the root field name.
   * @returns The decoded value.
   * @throws {ScalarCodecError} If a decoder throws.
   */
  decodeResult<T>(value: T, rootKey: string = this.fieldName): T {
    if (this.resultDecoder === undefined) {
      return this.selection.decode(value, rootKey);
    }
    const decoded = new Decoder().decodeLeaf(
      value,
      this.resultDecoder,
      this.resultDecoder.listDepth,
      childPath(undefined, rootKey),
    );
    // Boundary cast: a scalar decoder replaces the value, so its type changes;
    // T is the generated code's declaration of the decoded type, the same
    // unchecked claim execute<T> makes about the response.
    return decoded as T;
  }

  /**
   * Decode the partial data on a {@link GraphQLErrors} from this operation.
   *
   * Why it exists: the client throws on server errors before generated
   * `execute()` can call {@link decodeResult}, so without this the partial
   * `err.data[fieldName]` would stay in wire form. Generated code calls it in
   * its catch (only when a codec is reachable) and rethrows; `batch()` does the
   * same for every alias.
   *
   * Never throws. On a codec failure `err.data` becomes `undefined` and
   * `err.decodeError` holds the {@link ScalarCodecError}. A no-op when there is
   * no partial data, the root field is absent, or nothing decodes.
   *
   * @param err - The error thrown by `GraphQLClient.execute` for this operation.
   */
  decodePartial(err: GraphQLErrors): void {
    const data = err.data;
    if (data === undefined || !(this.fieldName in data) || !this.hasResultDecoding()) return;
    try {
      data[this.fieldName] = this.decodeResult(data[this.fieldName]);
    } catch (decodeErr) {
      err.data = undefined;
      err.decodeError = toCodecError(decodeErr, [this.fieldName]);
    }
  }

  /** Whether {@link decodeResult} would change anything. */
  private hasResultDecoding(): boolean {
    return this.resultDecoder !== undefined || this.selection.hasDecoders();
  }

  /** The wire value of one argument: encoded when it has an encoder. */
  private wireValue(entry: ArgEntry, variable: string): unknown {
    if (entry.encode === undefined) return entry.value;
    return encodeValue(
      entry.value,
      entry.encode,
      listDepthOf(entry.graphqlType),
      childPath(undefined, `$${variable}`),
    );
  }

  /**
   * Access the field selection tree so callers can add fields and nested selections.
   * @returns The root {@link FieldSelection} for this operation.
   */
  getSelection(): FieldSelection {
    return this.selection;
  }

  /**
   * Access the underlying GraphQL client (used by generated code if needed).
   * @returns The {@link GraphQLClient} instance.
   */
  getClient(): GraphQLClient {
    return this.client;
  }

  /**
   * Extract the runtime variable values as a plain object for the request payload.
   * Maps each registered argument name to its value, discarding the type
   * info and applying any encoder.
   *
   * @returns A fresh `{ [argName]: value }` record suitable for the `variables`
   *          field; mutating it does not affect the builder.
   * @throws {ScalarCodecError} If an encoder throws.
   */
  getVariables(): Record<string, unknown> {
    const vars: Record<string, unknown> = {};
    for (const [name, entry] of this.args) {
      vars[name] = this.wireValue(entry, name);
    }
    return vars;
  }

  /**
   * Build the complete GraphQL operation string.
   *
   * Produces output like:
   * ```graphql
   * query GetUser($id: ID!) {
   *   user(id: $id) {
   *     id
   *     name
   *   }
   * }
   * ```
   *
   * When nothing is selected (scalar-returning root fields such as `ping`),
   * the root field is emitted without braces.
   *
   * @returns The full GraphQL operation document as a string.
   */
  buildQuery(): string {
    const varDecls: string[] = [];
    for (const [name, { graphqlType }] of this.args) {
      varDecls.push(`$${name}: ${graphqlType}`);
    }
    const varStr = varDecls.length > 0 ? `(${varDecls.join(", ")})` : "";
    const rootField = this.renderRootField((name) => name);
    return `${this.opType} ${this.opName}${varStr} {\n  ${rootField}\n}`;
  }

  /**
   * Build and execute the operation, returning the raw `data` object.
   *
   * Generated subclasses typically wrap this method to extract and cast
   * the specific root field from the response (e.g., `(await executeRaw()).user`).
   *
   * @returns The full `data` object from the GraphQL response.
   * @throws {GraphQLErrors} If the server returns errors.
   * @throws {GraphQLHTTPError} If the transport response is not a GraphQL response.
   */
  async executeRaw(): Promise<Record<string, unknown>> {
    return await this.client.execute<Record<string, unknown>>(
      this.buildQuery(),
      this.getVariables(),
    );
  }

  /**
   * Produce the pieces needed to merge this operation into a batched, multi-root
   * operation under a caller-chosen alias. Variable names are namespaced as
   * `<alias>_<arg>` so multiple builders sharing argument names (e.g., two
   * `tasks` calls each taking `$status`) do not collide in the merged document.
   *
   * Invariant: the alias is not validated here; {@link batch} validates it
   * and rejects variable-name collisions the namespacing cannot prevent
   * (alias `a` + arg `b_c` vs alias `a_b` + arg `c`).
   *
   * @param alias - GraphQL response key for this operation's root field.
   * @returns Operation kind, prefixed variable declarations + (encoded)
   *          values, the aliased root-field string ready to be concatenated
   *          into a batch, and — only when a decoder is reachable — `decode`.
   * @throws {ScalarCodecError} If an encoder throws.
   */
  getOpFragment(alias: string): OpFragment {
    const varDecls: string[] = [];
    const varValues: Record<string, unknown> = {};
    const varName = (arg: string): string => `${alias}_${arg}`;

    for (const [name, entry] of this.args) {
      varDecls.push(`$${varName(name)}: ${entry.graphqlType}`);
      varValues[varName(name)] = this.wireValue(entry, varName(name));
    }

    const aliasedField = `${alias}: ${this.renderRootField(varName)}`;
    const fragment: OpFragment = { opType: this.opType, varDecls, varValues, aliasedField };
    if (this.hasResultDecoding()) {
      fragment.decode = (value: unknown): unknown => this.decodeResult(value, alias);
    }
    return fragment;
  }

  /**
   * Render `field(arg: $var, ...) { selection }` for the root field.
   *
   * Shared by {@link buildQuery} and {@link getOpFragment} so a standalone
   * operation and its batched form can never drift apart.
   *
   * @param varName - Maps an argument name to the variable that carries it.
   */
  private renderRootField(varName: (arg: string) => string): string {
    const argPasses: string[] = [];
    for (const name of this.args.keys()) {
      argPasses.push(`${name}: $${varName(name)}`);
    }
    const argStr = argPasses.length > 0 ? `(${argPasses.join(", ")})` : "";
    const selStr = this.selection.isEmpty()
      ? ""
      : ` {\n${this.selection.build(ROOT_SELECTION_INDENT)}\n  }`;
    return `${this.fieldName}${argStr}${selStr}`;
  }
}
