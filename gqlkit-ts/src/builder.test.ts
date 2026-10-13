/**
 * Contract tests for FieldSelection and BaseBuilder.
 *
 * Written as input-space matrices rather than one-off examples: a bug that
 * needs two dimensions at once (an inline fragment inside a list field, a
 * nullable list arg on a mutation) only shows up when the dimensions are
 * crossed, which is how issue #5 escaped line coverage.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import {
  BaseBuilder,
  FieldSelection,
  OPERATION_TYPES,
  OperationType,
  ROOT_SELECTION_INDENT,
} from "./builder";
import { GraphQLClient } from "./graphqlclient";
import { clientReturningData, onlyRequest, TEST_ENDPOINT } from "./testutil/fake-fetch";

/** A client that must never be called (pure build tests). */
const idleClient = new GraphQLClient(TEST_ENDPOINT, {
  fetch: async () => {
    throw new Error("unexpected request in a build-only test");
  },
});

/** Build a selection with leaves only. */
function leaves(...names: string[]): FieldSelection {
  const sel = new FieldSelection();
  for (const n of names) sel.addField(n);
  return sel;
}

// ---------------------------------------------------------------------------
// FieldSelection: selection shapes
// ---------------------------------------------------------------------------

interface SelectionCase {
  name: string;
  make: () => FieldSelection;
  /** Expected `build()` output at the default indent. */
  expected: string;
}

const SELECTION_CASES: SelectionCase[] = [
  { name: "empty", make: () => new FieldSelection(), expected: "" },
  { name: "one leaf", make: () => leaves("id"), expected: "  id" },
  { name: "leaves keep insertion order", make: () => leaves("name", "id"), expected: "  name\n  id" },
  { name: "duplicate leaf emitted twice", make: () => leaves("id", "id"), expected: "  id\n  id" },
  {
    name: "one child",
    make: () => {
      const s = leaves("id");
      s.addChild("user", leaves("name"));
      return s;
    },
    expected: "  id\n  user {\n    name\n  }",
  },
  {
    name: "child added before leaf still renders after leaves",
    make: () => {
      const s = new FieldSelection();
      s.addChild("user", leaves("name"));
      s.addField("id");
      return s;
    },
    expected: "  id\n  user {\n    name\n  }",
  },
  {
    name: "three-deep nesting",
    make: () => {
      const c = leaves("city");
      const b = new FieldSelection();
      b.addChild("address", c);
      const a = new FieldSelection();
      a.addChild("user", b);
      return a;
    },
    expected: "  user {\n    address {\n      city\n    }\n  }",
  },
  {
    name: "empty child is omitted",
    make: () => {
      const s = leaves("id");
      s.addChild("user", new FieldSelection());
      return s;
    },
    expected: "  id",
  },
  {
    name: "child whose only children are empty is omitted",
    make: () => {
      const inner = new FieldSelection();
      inner.addChild("address", new FieldSelection());
      const s = leaves("id");
      s.addChild("user", inner);
      return s;
    },
    expected: "  id",
  },
  {
    name: "only an empty child → whole selection empty",
    make: () => {
      const s = new FieldSelection();
      s.addChild("user", new FieldSelection());
      return s;
    },
    expected: "",
  },
  {
    name: "child filled after addChild (held by reference)",
    make: () => {
      const child = new FieldSelection();
      const s = new FieldSelection();
      s.addChild("user", child);
      child.addField("name");
      return s;
    },
    expected: "  user {\n    name\n  }",
  },
  {
    name: "re-added child replaces selection, keeps position",
    make: () => {
      const s = new FieldSelection();
      s.addChild("a", leaves("x"));
      s.addChild("b", leaves("y"));
      s.addChild("a", leaves("z"));
      return s;
    },
    expected: "  a {\n    z\n  }\n  b {\n    y\n  }",
  },
  {
    name: "union binding: __typename + inline fragments",
    make: () => {
      const s = leaves("__typename");
      s.addChild("... on Todo", leaves("id", "text"));
      s.addChild("... on User", leaves("name"));
      return s;
    },
    expected: "  __typename\n  ... on Todo {\n    id\n    text\n  }\n  ... on User {\n    name\n  }",
  },
  {
    name: "union binding inside a list field inside an object",
    make: () => {
      const results = leaves("__typename");
      results.addChild("... on Todo", leaves("id"));
      results.addChild("... on User", leaves("name"));
      const page = leaves("total");
      page.addChild("results", results);
      return page;
    },
    expected:
      "  total\n  results {\n    __typename\n    ... on Todo {\n      id\n    }\n    ... on User {\n      name\n    }\n  }",
  },
  {
    name: "inline fragment with empty selection is omitted",
    make: () => {
      const s = leaves("__typename");
      s.addChild("... on Todo", leaves("id"));
      s.addChild("... on User", new FieldSelection());
      return s;
    },
    expected: "  __typename\n  ... on Todo {\n    id\n  }",
  },
];

for (const c of SELECTION_CASES) {
  test(`FieldSelection.build: ${c.name}`, () => {
    assert.equal(c.make().build(), c.expected);
  });

  test(`FieldSelection.isEmpty agrees with build() === "": ${c.name}`, () => {
    const sel = c.make();
    assert.equal(sel.isEmpty(), sel.build() === "");
  });
}

test("FieldSelection.build honors a custom indent at every depth", () => {
  const s = leaves("id");
  s.addChild("user", leaves("name"));
  assert.equal(s.build(0), "id\nuser {\n  name\n}");
  assert.equal(s.build(ROOT_SELECTION_INDENT), "    id\n    user {\n      name\n    }");
});

test("FieldSelection.build is idempotent", () => {
  const s = leaves("id");
  s.addChild("user", leaves("name"));
  assert.equal(s.build(), s.build());
});

// ---------------------------------------------------------------------------
// BaseBuilder: op kind × arguments × selection
// ---------------------------------------------------------------------------

interface ArgCase {
  name: string;
  apply: (b: BaseBuilder) => void;
  /** Expected `($a: T, ...)` declaration list, or "" for none. */
  decls: string;
  /** Expected `(a: $a, ...)` pass-through list, or "" for none. */
  passes: string;
  /** Expected `getVariables()` result. */
  vars: Record<string, unknown>;
}

const ARG_CASES: ArgCase[] = [
  { name: "no args", apply: () => {}, decls: "", passes: "", vars: {} },
  {
    name: "one non-null scalar",
    apply: (b) => b.setArg("id", "1", "ID!"),
    decls: "($id: ID!)",
    passes: "(id: $id)",
    vars: { id: "1" },
  },
  {
    name: "two args keep set order",
    apply: (b) => {
      b.setArg("limit", 10, "Int");
      b.setArg("filter", { done: true }, "TodoFilter");
    },
    decls: "($limit: Int, $filter: TodoFilter)",
    passes: "(limit: $limit, filter: $filter)",
    vars: { limit: 10, filter: { done: true } },
  },
  {
    name: "non-null list of non-null",
    apply: (b) => b.setArg("ids", ["1", "2"], "[ID!]!"),
    decls: "($ids: [ID!]!)",
    passes: "(ids: $ids)",
    vars: { ids: ["1", "2"] },
  },
  {
    name: "nullable list holding null",
    apply: (b) => b.setArg("tags", null, "[String]"),
    decls: "($tags: [String])",
    passes: "(tags: $tags)",
    vars: { tags: null },
  },
  {
    name: "nested list type",
    apply: (b) => b.setArg("grid", [[1], [2, 3]], "[[Int!]!]"),
    decls: "($grid: [[Int!]!])",
    passes: "(grid: $grid)",
    vars: { grid: [[1], [2, 3]] },
  },
  {
    name: "undefined value still declared",
    apply: (b) => b.setArg("after", undefined, "String"),
    decls: "($after: String)",
    passes: "(after: $after)",
    vars: { after: undefined },
  },
  {
    name: "re-set arg keeps first position, last value and type",
    apply: (b) => {
      b.setArg("a", 1, "Int");
      b.setArg("b", 2, "Int");
      b.setArg("a", "x", "String!");
    },
    decls: "($a: String!, $b: Int)",
    passes: "(a: $a, b: $b)",
    vars: { a: "x", b: 2 },
  },
];

interface RootSelectionCase {
  name: string;
  apply: (sel: FieldSelection) => void;
  /** Expected text after `field(args)`, including the leading space and braces. */
  rendered: string;
}

const ROOT_SELECTION_CASES: RootSelectionCase[] = [
  { name: "scalar root (no selection)", apply: () => {}, rendered: "" },
  {
    name: "leaves",
    apply: (s) => {
      s.addField("id");
      s.addField("text");
    },
    rendered: " {\n    id\n    text\n  }",
  },
  {
    name: "nested object",
    apply: (s) => {
      s.addField("id");
      s.addChild("user", leaves("name"));
    },
    rendered: " {\n    id\n    user {\n      name\n    }\n  }",
  },
  {
    name: "list of union",
    apply: (s) => {
      s.addField("__typename");
      s.addChild("... on Todo", leaves("id"));
      s.addChild("... on User", leaves("name"));
    },
    rendered: " {\n    __typename\n    ... on Todo {\n      id\n    }\n    ... on User {\n      name\n    }\n  }",
  },
  {
    name: "only empty children (treated as scalar root)",
    apply: (s) => s.addChild("user", new FieldSelection()),
    rendered: "",
  },
];

/** Rename `$x` to `$<alias>_x` in an expected pass-through/decl list. */
function prefixVars(text: string, alias: string): string {
  return text.replace(/\$(\w+)/g, (_m, name: string) => `$${alias}_${name}`);
}

function makeBuilder(op: OperationType, args: ArgCase, sel: RootSelectionCase): BaseBuilder {
  const b = new BaseBuilder(idleClient, op, "Op", "field");
  args.apply(b);
  sel.apply(b.getSelection());
  return b;
}

for (const op of OPERATION_TYPES) {
  for (const args of ARG_CASES) {
    for (const sel of ROOT_SELECTION_CASES) {
      const label = `${op} × ${args.name} × ${sel.name}`;

      test(`BaseBuilder.buildQuery: ${label}`, () => {
        const b = makeBuilder(op, args, sel);
        assert.equal(
          b.buildQuery(),
          `${op} Op${args.decls} {\n  field${args.passes}${sel.rendered}\n}`,
        );
        assert.deepEqual(b.getVariables(), args.vars);
      });

      test(`BaseBuilder.getOpFragment: ${label}`, () => {
        const b = makeBuilder(op, args, sel);
        const before = b.buildQuery();
        const frag = b.getOpFragment("al");

        assert.equal(frag.opType, op);
        assert.equal(frag.aliasedField, `al: field${prefixVars(args.passes, "al")}${sel.rendered}`);
        const expectedDecls = args.decls === "" ? [] : prefixVars(args.decls.slice(1, -1), "al").split(", ");
        assert.deepEqual(frag.varDecls, expectedDecls);
        const expectedValues: Record<string, unknown> = {};
        for (const [k, v] of Object.entries(args.vars)) expectedValues[`al_${k}`] = v;
        assert.deepEqual(frag.varValues, expectedValues);

        // Producing a fragment must not change the standalone operation.
        assert.equal(b.buildQuery(), before);
      });
    }
  }
}

test("BaseBuilder.getOpFragment: two aliases from one builder are independent", () => {
  const b = new BaseBuilder(idleClient, OperationType.Query, "Op", "tasks");
  b.setArg("status", "open", "Status");
  const x = b.getOpFragment("x");
  const y = b.getOpFragment("y");
  assert.deepEqual(x.varValues, { x_status: "open" });
  assert.deepEqual(y.varValues, { y_status: "open" });
  assert.equal(y.aliasedField, "y: tasks(status: $y_status)");
});

test("BaseBuilder.getVariables returns a fresh object each call", () => {
  const b = new BaseBuilder(idleClient, OperationType.Query, "Op", "f");
  b.setArg("a", 1, "Int");
  const vars = b.getVariables();
  vars.a = 2;
  vars.injected = true;
  assert.deepEqual(b.getVariables(), { a: 1 });
});

test("BaseBuilder.getSelection is the live selection (later adds appear)", () => {
  const b = new BaseBuilder(idleClient, OperationType.Query, "Op", "f");
  assert.equal(b.buildQuery(), "query Op {\n  f\n}");
  b.getSelection().addField("id");
  assert.equal(b.buildQuery(), "query Op {\n  f {\n    id\n  }\n}");
});

test("BaseBuilder.getClient returns the constructor client", () => {
  const b = new BaseBuilder(idleClient, OperationType.Query, "Op", "f");
  assert.equal(b.getClient(), idleClient);
});

test("BaseBuilder never inlines argument values into the document", () => {
  const b = new BaseBuilder(idleClient, OperationType.Mutation, "Op", "f");
  const hostile = '") { __schema { types { name } } } #';
  b.setArg("text", hostile, "String!");
  assert.ok(!b.buildQuery().includes(hostile));
  assert.deepEqual(b.getVariables(), { text: hostile });
});

// ---------------------------------------------------------------------------
// BaseBuilder.executeRaw: request wiring
// ---------------------------------------------------------------------------

test("BaseBuilder.executeRaw sends buildQuery + variables and returns data", async () => {
  const { client, requests } = clientReturningData({ todo: { id: "1" } });
  const b = new BaseBuilder(client, OperationType.Query, "Todo", "todo");
  b.setArg("id", "1", "ID!");
  b.getSelection().addField("id");

  const data = await b.executeRaw();

  const req = onlyRequest(requests);
  assert.equal(req.query, b.buildQuery());
  assert.deepEqual(req.variables, { id: "1" });
  assert.deepEqual(data, { todo: { id: "1" } });
});

test("BaseBuilder.executeRaw drops undefined variables from the wire body", async () => {
  const { client, requests } = clientReturningData({ f: null });
  const b = new BaseBuilder(client, OperationType.Query, "Op", "f");
  b.setArg("after", undefined, "String");
  b.setArg("first", null, "Int");
  await b.executeRaw();
  assert.deepEqual(onlyRequest(requests).variables, { first: null });
});
