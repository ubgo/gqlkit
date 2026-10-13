/**
 * Contract tests for scalar codecs: decoding responses through FieldSelection /
 * BaseBuilder / batch, and encoding variables through setArg.
 *
 * The core matrix crosses LEAF shapes (null, lists of nullable, nested lists,
 * array-valued scalars, list depth given or not) with CONTAINER shapes (root,
 * alias, args, nested object, lists of objects, union and interface fragments)
 * — the two-dimension combinations a single-example test never reaches.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { BaseBuilder, FieldSelection, OperationType } from "./builder";
import { GraphQLClient, GraphQLErrors } from "./graphqlclient";
import { batch } from "./batch";
import {
  CodecDirection,
  ScalarCodecError,
  type ArgEncoder,
  type InputEncodePlan,
} from "./codec";
import { clientReplying, clientReturningData, onlyRequest } from "./testutil/fake-fetch";

/** App-side value a decoder produces; distinguishable from any wire value. */
class Stamp {
  constructor(readonly iso: string) {}
}

/** Counts calls so double decoding is caught, and rejects non-strings like a real parser. */
function makeDecoder(): { decode: (raw: string) => Stamp; calls: () => number } {
  let calls = 0;
  return {
    decode: (raw: string): Stamp => {
      calls++;
      if (typeof raw !== "string") throw new TypeError(`expected string, got ${JSON.stringify(raw)}`);
      if (raw === "bad") throw new RangeError("invalid timestamp");
      return new Stamp(raw);
    },
    calls: () => calls,
  };
}

/** Encoder for Stamp → wire string; throws on anything else so wrong routing is loud. */
function encodeStamp(value: Stamp): string {
  if (!(value instanceof Stamp)) throw new TypeError(`expected Stamp, got ${JSON.stringify(value)}`);
  if (value.iso === "bad") throw new RangeError("cannot encode");
  return value.iso;
}

const STAMP_ENCODER: ArgEncoder = { scalar: encodeStamp };

/** Render Stamps as `S:<iso>` so deepEqual can compare decoded trees. */
function show(value: unknown): unknown {
  if (value instanceof Stamp) return `S:${value.iso}`;
  if (Array.isArray(value)) return value.map(show);
  if (typeof value === "object" && value !== null) {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) out[k] = show(v);
    return out;
  }
  return value;
}

/** Read a property of an unknown object (test helper; fails loudly). */
function prop(value: unknown, key: string | number): unknown {
  if (typeof value !== "object" || value === null) throw new Error(`no ${String(key)} on ${JSON.stringify(value)}`);
  return Reflect.get(value, key);
}

// ---------------------------------------------------------------------------
// Decode matrix: leaf shape × container shape
// ---------------------------------------------------------------------------

interface LeafCase {
  name: string;
  wire: unknown;
  listDepth: number | undefined;
  /** Expected value after decoding, through {@link show}. */
  expected: unknown;
  /** Expected decoder calls per leaf position. */
  calls: number;
}

const LEAF_CASES: LeafCase[] = [
  { name: "scalar", wire: "a", listDepth: 0, expected: "S:a", calls: 1 },
  { name: "scalar, depth omitted", wire: "a", listDepth: undefined, expected: "S:a", calls: 1 },
  { name: "null", wire: null, listDepth: 0, expected: null, calls: 0 },
  { name: "list", wire: ["a", "b"], listDepth: 1, expected: ["S:a", "S:b"], calls: 2 },
  { name: "list, depth omitted", wire: ["a", "b"], listDepth: undefined, expected: ["S:a", "S:b"], calls: 2 },
  { name: "empty list", wire: [], listDepth: 1, expected: [], calls: 0 },
  { name: "list of nullable", wire: [null, "a", null], listDepth: 1, expected: [null, "S:a", null], calls: 1 },
  { name: "null list", wire: null, listDepth: 1, expected: null, calls: 0 },
  {
    name: "nested list with null rows",
    wire: [["a"], [null, "b"], null, []],
    listDepth: 2,
    expected: [["S:a"], [null, "S:b"], null, []],
    calls: 2,
  },
  {
    name: "nested list, depth omitted",
    wire: [["a"], ["b"]],
    listDepth: undefined,
    expected: [["S:a"], ["S:b"]],
    calls: 2,
  },
];

interface Container {
  name: string;
  /** Build a selection whose leaf is added by `addLeaf` at the container's position. */
  build: (addLeaf: (sel: FieldSelection) => void) => FieldSelection;
  /** Response with `leaf` at every leaf position (each position gets its own clone). */
  wrap: (leaf: () => unknown) => Record<string, unknown>;
  /** Leaf values at every leaf position, in order. */
  extract: (decoded: Record<string, unknown>) => unknown[];
  /** Number of leaf positions. */
  positions: number;
}

function leafSel(addLeaf: (sel: FieldSelection) => void): FieldSelection {
  const s = new FieldSelection();
  s.addField("id");
  addLeaf(s);
  return s;
}

const CONTAINERS: Container[] = [
  {
    name: "root object",
    build: (add) => leafSel(add),
    wrap: (leaf) => ({ id: "1", at: leaf() }),
    extract: (d) => [d.at],
    positions: 1,
  },
  {
    name: "nested object",
    build: (add) => {
      const s = new FieldSelection();
      s.addChild("user", leafSel(add));
      return s;
    },
    wrap: (leaf) => ({ user: { id: "u", at: leaf() } }),
    extract: (d) => [prop(d.user, "at")],
    positions: 1,
  },
  {
    name: "aliased object child",
    build: (add) => {
      const s = new FieldSelection();
      s.addChild("owner: user", leafSel(add));
      return s;
    },
    wrap: (leaf) => ({ owner: { at: leaf() } }),
    extract: (d) => [prop(d.owner, "at")],
    positions: 1,
  },
  {
    name: "object child with args",
    build: (add) => {
      const s = new FieldSelection();
      s.addChild('user(id: "1")', leafSel(add));
      return s;
    },
    wrap: (leaf) => ({ user: { at: leaf() } }),
    extract: (d) => [prop(d.user, "at")],
    positions: 1,
  },
  {
    name: "list of objects",
    build: (add) => {
      const s = new FieldSelection();
      s.addChild("items", leafSel(add));
      return s;
    },
    wrap: (leaf) => ({ items: [{ at: leaf() }, null, { at: leaf() }] }),
    extract: (d) => [prop(prop(d.items, 0), "at"), prop(prop(d.items, 2), "at")],
    positions: 2,
  },
  {
    name: "nested list of objects",
    build: (add) => {
      const s = new FieldSelection();
      s.addChild("grid", leafSel(add));
      return s;
    },
    wrap: (leaf) => ({ grid: [[{ at: leaf() }], [], null, [{ at: leaf() }]] }),
    extract: (d) => [prop(prop(prop(d.grid, 0), 0), "at"), prop(prop(prop(d.grid, 3), 0), "at")],
    positions: 2,
  },
  {
    name: "union fragment (matched)",
    build: (add) => {
      const s = new FieldSelection();
      s.addField("__typename");
      s.addChild("... on Todo", leafSel(add));
      return s;
    },
    wrap: (leaf) => ({ __typename: "Todo", at: leaf() }),
    extract: (d) => [d.at],
    positions: 1,
  },
  {
    name: "list of union inside an object",
    build: (add) => {
      const results = new FieldSelection();
      results.addField("__typename");
      results.addChild("... on Todo", leafSel(add));
      results.addChild("... on User", leafSel(() => {}));
      const s = new FieldSelection();
      s.addChild("results", results);
      return s;
    },
    wrap: (leaf) => ({
      results: [
        { __typename: "Todo", at: leaf() },
        { __typename: "User", id: "u" },
        { __typename: "Todo", at: leaf() },
      ],
    }),
    extract: (d) => [prop(prop(d.results, 0), "at"), prop(prop(d.results, 2), "at")],
    positions: 2,
  },
  {
    name: "interface fragment with possibleTypes",
    build: (add) => {
      const s = new FieldSelection();
      s.addField("__typename");
      s.addChild("... on Node", leafSel(add), ["Todo", "User"]);
      return s;
    },
    wrap: (leaf) => ({ __typename: "User", at: leaf() }),
    extract: (d) => [d.at],
    positions: 1,
  },
  {
    name: "fragment without type condition",
    build: (add) => {
      const s = new FieldSelection();
      s.addChild("... @include(if: true)", leafSel(add));
      return s;
    },
    wrap: (leaf) => ({ at: leaf() }),
    extract: (d) => [d.at],
    positions: 1,
  },
  {
    name: "fragment nested in a fragment's object",
    build: (add) => {
      const inner = new FieldSelection();
      inner.addField("__typename");
      inner.addChild("... on Todo", leafSel(add));
      const todo = new FieldSelection();
      todo.addChild("parent", inner);
      const s = new FieldSelection();
      s.addField("__typename");
      s.addChild("... on Todo", todo);
      return s;
    },
    wrap: (leaf) => ({ __typename: "Todo", parent: { __typename: "Todo", at: leaf() } }),
    extract: (d) => [prop(d.parent, "at")],
    positions: 1,
  },
];

for (const container of CONTAINERS) {
  for (const leaf of LEAF_CASES) {
    for (const aliased of [false, true]) {
      // Aliased: the response key is the alias "at", not the field name.
      const entry = aliased ? "at: createdAt" : "at";
      test(`decode: ${container.name} × ${leaf.name}${aliased ? " × leaf alias" : ""}`, () => {
        const dec = makeDecoder();
        const sel = container.build((s) => s.addField(entry, dec.decode, leaf.listDepth));
        const value = container.wrap(() => structuredClone(leaf.wire));
        const returned = sel.decode(value);
        assert.equal(returned, value, "decode works in place and returns the same object");
        const got = container.extract(value).map(show);
        assert.deepEqual(got, Array.from({ length: container.positions }, () => leaf.expected));
        assert.equal(dec.calls(), leaf.calls * container.positions);
      });
    }
  }
}

// ---------------------------------------------------------------------------
// Decode: routing details
// ---------------------------------------------------------------------------

test("decode: array-valued scalar with listDepth 0 reaches the decoder whole", () => {
  const seen: unknown[] = [];
  const sel = new FieldSelection();
  sel.addField("meta", (raw: unknown) => {
    seen.push(raw);
    return "decoded";
  }, 0);
  const value = { meta: [1, 2] };
  sel.decode(value);
  assert.deepEqual(seen, [[1, 2]]);
  assert.equal(value.meta, "decoded");
});

test("decode: listDepth 1 hands each inner array to the decoder whole", () => {
  const seen: unknown[] = [];
  const sel = new FieldSelection();
  sel.addField("metas", (raw: unknown) => {
    seen.push(raw);
    return seen.length;
  }, 1);
  const value = { metas: [[1], [2, 3]] };
  sel.decode(value);
  assert.deepEqual(seen, [[1], [2, 3]]);
  assert.deepEqual(value.metas, [1, 2]);
});

test("decode: unmatched union member is left untouched", () => {
  const dec = makeDecoder();
  const sel = new FieldSelection();
  sel.addField("__typename");
  sel.addChild("... on Todo", leafSel((s) => s.addField("at", dec.decode)));
  const value = { __typename: "User", at: "raw" };
  sel.decode(value);
  assert.equal(value.at, "raw");
  assert.equal(dec.calls(), 0);
});

test("decode: interface fragment without possibleTypes matches only the literal name", () => {
  const dec = makeDecoder();
  const sel = new FieldSelection();
  sel.addChild("... on Node", leafSel((s) => s.addField("at", dec.decode)));
  const value = { __typename: "Todo", at: "raw" };
  sel.decode(value);
  assert.equal(value.at, "raw");
});

test("decode: re-adding a fragment child without possibleTypes resets them", () => {
  const dec = makeDecoder();
  const sel = new FieldSelection();
  const child = leafSel((s) => s.addField("at", dec.decode));
  sel.addChild("... on Node", child, ["Todo"]);
  sel.addChild("... on Node", child);
  const value = { __typename: "Todo", at: "raw" };
  sel.decode(value);
  assert.equal(value.at, "raw");
});

test("decode: fragment with decoders but no __typename throws a path-naming error", () => {
  const sel = new FieldSelection();
  sel.addChild("... on Todo", leafSel((s) => s.addField("at", makeDecoder().decode)));
  const outer = new FieldSelection();
  outer.addChild("results", sel);
  assert.throws(
    () => outer.decode({ results: [{ __typename: "Todo", at: "a" }, { at: "b" }] }, "search"),
    (err: unknown) => {
      assert.ok(err instanceof ScalarCodecError);
      assert.equal(err.direction, CodecDirection.Decode);
      assert.deepEqual(err.path, ["search", "results", 1]);
      assert.match(err.message, /^gqlkit: failed to decode "search\.results\.1": .*__typename/);
      return true;
    },
  );
});

test("decode: a field selected directly and in a matching fragment is decoded once", () => {
  const dec = makeDecoder();
  const sel = new FieldSelection();
  sel.addField("__typename");
  sel.addField("at", dec.decode);
  sel.addChild("... on Todo", leafSel((s) => s.addField("at", dec.decode)));
  const owner = new FieldSelection();
  owner.addField("at", dec.decode);
  sel.addChild("owner", owner);
  const fragOwner = new FieldSelection();
  fragOwner.addField("at", dec.decode);
  const frag = new FieldSelection();
  frag.addChild("owner", fragOwner);
  sel.addChild("... on Todo ", frag);

  const value = { __typename: "Todo", at: "a", owner: { at: "b" } };
  sel.decode(value);
  assert.deepEqual(show(value), { __typename: "Todo", at: "S:a", owner: { at: "S:b" } });
  assert.equal(dec.calls(), 2);
});

test("decode: missing keys are not added and non-objects are skipped", () => {
  const dec = makeDecoder();
  const sel = new FieldSelection();
  sel.addField("at", dec.decode);
  const child = new FieldSelection();
  child.addField("at", dec.decode);
  sel.addChild("user", child);
  const value: Record<string, unknown> = { user: "not-an-object" };
  sel.decode(value);
  assert.deepEqual(value, { user: "not-an-object" });
  assert.equal(dec.calls(), 0);
});

test("decode: null and primitive roots pass through", () => {
  const sel = leafSel((s) => s.addField("at", makeDecoder().decode));
  assert.equal(sel.decode(null), null);
  assert.equal(sel.decode(undefined), undefined);
  assert.equal(sel.decode(5), 5);
});

test("decode: a decoder error names the full path and keeps the cause", () => {
  const items = leafSel((s) => s.addField("at", makeDecoder().decode));
  const sel = new FieldSelection();
  sel.addChild("items", items);
  assert.throws(
    () => sel.decode({ items: [{ at: "a" }, { at: ["x", "bad"] }] }, "todos"),
    (err: unknown) => {
      assert.ok(err instanceof ScalarCodecError);
      assert.deepEqual(err.path, ["todos", "items", 1, "at", 1]);
      assert.ok(err.cause instanceof RangeError);
      assert.equal(err.message, 'gqlkit: failed to decode "todos.items.1.at.1": invalid timestamp');
      assert.equal(err.name, "ScalarCodecError");
      return true;
    },
  );
});

test("decode: a non-Error throw is still wrapped with its text", () => {
  const sel = new FieldSelection();
  sel.addField("at", () => {
    throw "nope";
  });
  assert.throws(() => sel.decode({ at: 1 }), /failed to decode "at": nope/);
});

test("decode: without decoders the value is returned untouched", () => {
  const sel = new FieldSelection();
  sel.addField("id");
  sel.addChild("user", leafSel(() => {}));
  const value = { id: "1", user: { id: "2", at: "a" } };
  const before = structuredClone(value);
  assert.equal(sel.decode(value), value);
  assert.deepEqual(value, before);
  assert.equal(sel.hasDecoders(), false);
});

test("hasDecoders tracks decoders added to a child after addChild", () => {
  const child = new FieldSelection();
  const sel = new FieldSelection();
  sel.addChild("user", child);
  assert.equal(sel.hasDecoders(), false);
  child.addField("at", makeDecoder().decode);
  assert.equal(sel.hasDecoders(), true);
});

test("decoders do not change the built selection text", () => {
  const plain = new FieldSelection();
  plain.addField("recent: createdAt");
  const coded = new FieldSelection();
  coded.addField("recent: createdAt", makeDecoder().decode, 0);
  assert.equal(coded.build(), plain.build());
});

// ---------------------------------------------------------------------------
// BaseBuilder.decodeResult
// ---------------------------------------------------------------------------

const RESULT_CASES: { name: string; wire: unknown; listDepth: number | undefined; expected: unknown }[] = [
  { name: "scalar", wire: "a", listDepth: 0, expected: "S:a" },
  { name: "null", wire: null, listDepth: 0, expected: null },
  { name: "list of nullable", wire: ["a", null], listDepth: 1, expected: ["S:a", null] },
  { name: "nested list", wire: [["a"], null], listDepth: 2, expected: [["S:a"], null] },
  { name: "depth omitted", wire: [["a"]], listDepth: undefined, expected: [["S:a"]] },
];

for (const c of RESULT_CASES) {
  test(`decodeResult with a result decoder: ${c.name}`, () => {
    const { client } = clientReturningData({});
    const b = new BaseBuilder(client, OperationType.Query, "Now", "now");
    b.setResultDecoder(makeDecoder().decode, c.listDepth);
    assert.deepEqual(show(b.decodeResult(structuredClone(c.wire))), c.expected);
  });
}

test("decodeResult error path starts at the root field name", () => {
  const { client } = clientReturningData({});
  const b = new BaseBuilder(client, OperationType.Query, "Now", "now");
  b.setResultDecoder(makeDecoder().decode, 1);
  assert.throws(() => b.decodeResult(["a", "bad"]), (err: unknown) => {
    assert.ok(err instanceof ScalarCodecError);
    assert.deepEqual(err.path, ["now", 1]);
    return true;
  });
});

test("decodeResult falls back to the selection's decoders", () => {
  const { client } = clientReturningData({});
  const b = new BaseBuilder(client, OperationType.Query, "Todos", "todos");
  b.getSelection().addField("at", makeDecoder().decode);
  assert.deepEqual(show(b.decodeResult([{ at: "a" }, null])), [{ at: "S:a" }, null]);
});

test("executeRaw stays raw: no decoding of the response", async () => {
  const { client } = clientReturningData({ todo: { at: "a" } });
  const b = new BaseBuilder(client, OperationType.Query, "Todo", "todo");
  b.getSelection().addField("at", makeDecoder().decode);
  assert.deepEqual(await b.executeRaw(), { todo: { at: "a" } });
});

// ---------------------------------------------------------------------------
// Encoding: setArg → getVariables / getOpFragment
// ---------------------------------------------------------------------------

const FILTER_PLAN: InputEncodePlan = {
  createdAfter: STAMP_ENCODER,
  between: { scalar: encodeStamp, listDepth: 1 },
  and: { input: () => FILTER_PLAN },
  owner: { input: () => ({ joinedAt: STAMP_ENCODER }) },
};
const FILTER_ENCODER: ArgEncoder = { input: () => FILTER_PLAN };

interface EncodeCase {
  name: string;
  type: string;
  encoder: ArgEncoder;
  value: unknown;
  expected: unknown;
}

const s = (iso: string): Stamp => new Stamp(iso);

const ENCODE_CASES: EncodeCase[] = [
  { name: "scalar", type: "DateTime!", encoder: STAMP_ENCODER, value: s("a"), expected: "a" },
  { name: "null scalar", type: "DateTime", encoder: STAMP_ENCODER, value: null, expected: null },
  { name: "list", type: "[DateTime!]!", encoder: STAMP_ENCODER, value: [s("a"), s("b")], expected: ["a", "b"] },
  { name: "list of nullable", type: "[DateTime]", encoder: STAMP_ENCODER, value: [null, s("a")], expected: [null, "a"] },
  {
    name: "nested list",
    type: "[[DateTime!]]",
    encoder: STAMP_ENCODER,
    value: [[s("a")], null, []],
    expected: [["a"], null, []],
  },
  {
    name: "array-valued scalar at depth 0",
    type: "JSON",
    encoder: { scalar: (v: number[]) => v.join(",") },
    value: [1, 2],
    expected: "1,2",
  },
  {
    name: "input object: codec field + untouched fields",
    type: "TodoFilter",
    encoder: FILTER_ENCODER,
    value: { done: true, createdAfter: s("a"), text: "x" },
    expected: { done: true, createdAfter: "a", text: "x" },
  },
  {
    name: "input object: absent and null codec fields",
    type: "TodoFilter",
    encoder: FILTER_ENCODER,
    value: { createdAfter: null },
    expected: { createdAfter: null },
  },
  {
    name: "input object: list field with listDepth",
    type: "TodoFilter",
    encoder: FILTER_ENCODER,
    value: { between: [s("a"), null] },
    expected: { between: ["a", null] },
  },
  {
    name: "input object: nested input",
    type: "TodoFilter",
    encoder: FILTER_ENCODER,
    value: { owner: { joinedAt: s("j"), name: "n" } },
    expected: { owner: { joinedAt: "j", name: "n" } },
  },
  {
    name: "self-referential input (and: [Filter!])",
    type: "TodoFilter",
    encoder: FILTER_ENCODER,
    value: { and: [{ createdAfter: s("a") }, { and: [{ createdAfter: s("b") }] }] },
    expected: { and: [{ createdAfter: "a" }, { and: [{ createdAfter: "b" }] }] },
  },
  {
    name: "list of input objects",
    type: "[TodoFilter!]!",
    encoder: FILTER_ENCODER,
    value: [{ createdAfter: s("a") }, { createdAfter: s("b") }],
    expected: [{ createdAfter: "a" }, { createdAfter: "b" }],
  },
  { name: "null input object", type: "TodoFilter", encoder: FILTER_ENCODER, value: null, expected: null },
];

for (const c of ENCODE_CASES) {
  test(`encode: ${c.name} (getVariables + getOpFragment)`, () => {
    const { client } = clientReturningData({});
    const b = new BaseBuilder(client, OperationType.Query, "Op", "f");
    const original = c.value;
    const snapshot = show(original);
    b.setArg("v", original, c.type, c.encoder);

    assert.deepEqual(b.getVariables(), { v: c.expected });
    assert.deepEqual(b.getVariables(), { v: c.expected }, "encoding is not cumulative");
    assert.deepEqual(b.getOpFragment("al").varValues, { al_v: c.expected });
    assert.deepEqual(show(original), snapshot, "the caller's value is not mutated");
  });
}

test("encode: undefined argument stays undefined and is dropped on the wire", async () => {
  const { client, requests } = clientReturningData({ f: 1 });
  const b = new BaseBuilder(client, OperationType.Query, "Op", "f");
  b.setArg("v", undefined, "DateTime", STAMP_ENCODER);
  b.setArg("w", s("a"), "DateTime", STAMP_ENCODER);
  await b.executeRaw();
  assert.deepEqual(onlyRequest(requests).variables, { w: "a" });
});

test("encode: an encoder error names the variable path; nothing is sent", async () => {
  const { client, requests } = clientReturningData({ f: 1 });
  const b = new BaseBuilder(client, OperationType.Query, "Op", "f");
  b.setArg("filter", { and: [{ createdAfter: s("ok") }, { createdAfter: s("bad") }] }, "TodoFilter", FILTER_ENCODER);
  await assert.rejects(b.executeRaw(), (err: unknown) => {
    assert.ok(err instanceof ScalarCodecError);
    assert.equal(err.direction, CodecDirection.Encode);
    assert.deepEqual(err.path, ["$filter", "and", 1, "createdAfter"]);
    assert.equal(err.message, 'gqlkit: failed to encode "$filter.and.1.createdAfter": cannot encode');
    return true;
  });
  assert.equal(requests.length, 0);
});

test("encode: getOpFragment error path uses the namespaced variable", () => {
  const { client } = clientReturningData({});
  const b = new BaseBuilder(client, OperationType.Query, "Op", "f");
  b.setArg("at", s("bad"), "DateTime!", STAMP_ENCODER);
  assert.throws(() => b.getOpFragment("x"), (err: unknown) => {
    assert.ok(err instanceof ScalarCodecError);
    assert.deepEqual(err.path, ["$x_at"]);
    return true;
  });
});

test("encode: re-setting an arg without an encoder drops the old encoder", () => {
  const { client } = clientReturningData({});
  const b = new BaseBuilder(client, OperationType.Query, "Op", "f");
  b.setArg("at", s("a"), "DateTime!", STAMP_ENCODER);
  b.setArg("at", "2026-01-01", "DateTime!");
  assert.deepEqual(b.getVariables(), { at: "2026-01-01" });
});

// ---------------------------------------------------------------------------
// getOpFragment.decode + batch
// ---------------------------------------------------------------------------

function todoBuilder(client: GraphQLClient, withCodec: boolean): BaseBuilder {
  const b = new BaseBuilder(client, OperationType.Query, "Todos", "todos");
  b.getSelection().addField("id");
  if (withCodec) b.getSelection().addField("at", makeDecoder().decode);
  return b;
}

test("getOpFragment has no decode without decoders, and one with them", () => {
  const { client } = clientReturningData({});
  assert.equal("decode" in todoBuilder(client, false).getOpFragment("a"), false);
  assert.equal(typeof todoBuilder(client, true).getOpFragment("a").decode, "function");

  const scalarRoot = new BaseBuilder(client, OperationType.Query, "Now", "now");
  scalarRoot.setResultDecoder(makeDecoder().decode);
  assert.equal(typeof scalarRoot.getOpFragment("n").decode, "function");
});

test("batch decodes each alias with its own builder's codecs", async () => {
  const { client, requests } = clientReturningData({
    coded: [{ id: "1", at: "a" }],
    plain: [{ id: "2", at: "b" }],
    now: "n",
  });
  const now = new BaseBuilder(client, OperationType.Query, "Now", "now");
  now.setResultDecoder(makeDecoder().decode);
  now.setArg("tz", s("utc"), "DateTime", STAMP_ENCODER);

  const result = await batch(client, {
    coded: { execute: async () => undefined, getOpFragment: (a: string) => todoBuilder(client, true).getOpFragment(a) },
    plain: { execute: async () => undefined, getOpFragment: (a: string) => todoBuilder(client, false).getOpFragment(a) },
    now: { execute: async () => undefined, getOpFragment: (a: string) => now.getOpFragment(a) },
  });

  assert.deepEqual(show(result), {
    coded: [{ id: "1", at: "S:a" }],
    plain: [{ id: "2", at: "b" }],
    now: "S:n",
  });
  assert.deepEqual(onlyRequest(requests).variables, { now_tz: "utc" });
});

test("batch decode error names the alias", async () => {
  const { client } = clientReturningData({ x: [{ at: "bad" }] });
  await assert.rejects(
    batch(client, { x: { execute: async () => undefined, getOpFragment: (a: string) => todoBuilder(client, true).getOpFragment(a) } }),
    (err: unknown) => {
      assert.ok(err instanceof ScalarCodecError);
      assert.deepEqual(err.path, ["x", 0, "at"]);
      return true;
    },
  );
});

test("batch decodes partial data on GraphQLErrors", async () => {
  const { client } = clientReplying({
    body: { data: { ok: [{ id: "1", at: "a" }], bad: null }, errors: [{ message: "bad failed", path: ["bad"] }] },
  });
  const item = { execute: async () => undefined, getOpFragment: (a: string) => todoBuilder(client, true).getOpFragment(a) };
  await assert.rejects(batch(client, { ok: item, bad: item }), (err: unknown) => {
    assert.ok(err instanceof GraphQLErrors);
    assert.deepEqual(show(err.data), { ok: [{ id: "1", at: "S:a" }], bad: null });
    return true;
  });
});

test("batch: partial data that fails to decode is withheld, server errors still thrown", async () => {
  const { client } = clientReplying({
    body: { data: { ok: [{ id: "1", at: "bad" }] }, errors: [{ message: "other failed" }] },
  });
  const item = { execute: async () => undefined, getOpFragment: (a: string) => todoBuilder(client, true).getOpFragment(a) };
  await assert.rejects(batch(client, { ok: item }), (err: unknown) => {
    assert.ok(err instanceof GraphQLErrors);
    assert.equal(err.message, "other failed");
    assert.equal(err.data, undefined);
    assert.ok(err.decodeError instanceof ScalarCodecError);
    assert.deepEqual(err.decodeError.path, ["ok", 0, "at"]);
    return true;
  });
});

test("batch encode error: nothing is sent", async () => {
  const { client, requests } = clientReturningData({});
  const b = new BaseBuilder(client, OperationType.Query, "Now", "now");
  b.setArg("at", s("bad"), "DateTime!", STAMP_ENCODER);
  await assert.rejects(
    batch(client, { n: { execute: async () => undefined, getOpFragment: (a: string) => b.getOpFragment(a) } }),
    ScalarCodecError,
  );
  assert.equal(requests.length, 0);
});

// ---------------------------------------------------------------------------
// BaseBuilder.decodePartial (single, non-batch operations)
// ---------------------------------------------------------------------------

interface PartialCase {
  name: string;
  setup: (b: BaseBuilder) => void;
  data: Record<string, unknown> | undefined;
  /** Expected err.data through {@link show}. */
  expected: unknown;
  /** Expected decodeError path, or undefined for none. */
  errorPath: (string | number)[] | undefined;
}

const PARTIAL_CASES: PartialCase[] = [
  {
    name: "selection decoder, partial list",
    setup: (b) => b.getSelection().addField("at", makeDecoder().decode),
    data: { todos: [{ at: "a" }, null], other: "x" },
    expected: { todos: [{ at: "S:a" }, null], other: "x" },
    errorPath: undefined,
  },
  {
    name: "result decoder",
    setup: (b) => b.setResultDecoder(makeDecoder().decode, 1),
    data: { todos: ["a", null] },
    expected: { todos: ["S:a", null] },
    errorPath: undefined,
  },
  {
    name: "root field null",
    setup: (b) => b.getSelection().addField("at", makeDecoder().decode),
    data: { todos: null },
    expected: { todos: null },
    errorPath: undefined,
  },
  {
    name: "root field absent",
    setup: (b) => b.getSelection().addField("at", makeDecoder().decode),
    data: { other: { at: "a" } },
    expected: { other: { at: "a" } },
    errorPath: undefined,
  },
  {
    name: "no partial data",
    setup: (b) => b.getSelection().addField("at", makeDecoder().decode),
    data: undefined,
    expected: undefined,
    errorPath: undefined,
  },
  {
    name: "no decoders",
    setup: (b) => b.getSelection().addField("at"),
    data: { todos: [{ at: "a" }] },
    expected: { todos: [{ at: "a" }] },
    errorPath: undefined,
  },
  {
    name: "decoder fails",
    setup: (b) => b.getSelection().addField("at", makeDecoder().decode),
    data: { todos: [{ at: "a" }, { at: "bad" }] },
    expected: undefined,
    errorPath: ["todos", 1, "at"],
  },
  {
    name: "fragment without __typename fails",
    setup: (b) => {
      b.getSelection().addChild("... on Todo", leafSel((sel) => sel.addField("at", makeDecoder().decode)));
    },
    data: { todos: [{ at: "a" }] },
    expected: undefined,
    errorPath: ["todos", 0],
  },
];

for (const c of PARTIAL_CASES) {
  test(`decodePartial: ${c.name}`, () => {
    const { client } = clientReturningData({});
    const b = new BaseBuilder(client, OperationType.Query, "Todos", "todos");
    c.setup(b);
    const err = new GraphQLErrors([{ message: "boom" }], c.data);
    assert.doesNotThrow(() => b.decodePartial(err));
    assert.deepEqual(show(err.data), c.expected);
    if (c.errorPath === undefined) {
      assert.equal(err.decodeError, undefined);
    } else {
      assert.ok(err.decodeError instanceof ScalarCodecError);
      assert.deepEqual(err.decodeError.path, c.errorPath);
    }
    assert.equal(err.message, "boom", "the server errors are untouched");
  });
}

test("decodePartial in the generated execute() shape: decoded partial data on the rethrown error", async () => {
  const { client } = clientReplying({
    body: { data: { todos: [{ at: "a" }, null] }, errors: [{ message: "item 1 failed", path: ["todos", 1] }] },
  });
  const b = new BaseBuilder(client, OperationType.Query, "Todos", "todos");
  b.getSelection().addField("at", makeDecoder().decode);

  const execute = async (): Promise<unknown> => {
    try {
      const response = await client.execute<{ todos: unknown }>(b.buildQuery(), b.getVariables());
      return b.decodeResult(response.todos);
    } catch (e) {
      if (e instanceof GraphQLErrors) b.decodePartial(e);
      throw e;
    }
  };

  await assert.rejects(execute(), (err: unknown) => {
    assert.ok(err instanceof GraphQLErrors);
    assert.deepEqual(show(err.data), { todos: [{ at: "S:a" }, null] });
    assert.equal(err.decodeError, undefined);
    return true;
  });
});
