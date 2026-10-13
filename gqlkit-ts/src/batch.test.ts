/**
 * Contract tests for batch(). Mirrors gqlkit/pkg/batch/batch_test.go where the
 * two runtimes share behavior (merging, namespacing, no-args, ordering,
 * partial data, empty input, mutation merging, fragment shape), then crosses
 * builder count × argument shape × op kind as a matrix.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { BaseBuilder, FieldSelection, OPERATION_TYPES, OperationType } from "./builder";
import { GraphQLClient, GraphQLErrors } from "./graphqlclient";
import {
  batch,
  DEFAULT_BATCH_OP_NAME,
  type BatchableBuilder,
  type OpFragment,
} from "./batch";
import {
  clientReplying,
  clientReturningData,
  onlyRequest,
  type CapturedRequest,
} from "./testutil/fake-fetch";

interface Task {
  id: string;
  name: string;
}

/**
 * Minimal stand-in for a generated query builder, in the generated shape:
 * it composes a BaseBuilder and forwards to it.
 */
class TasksBuilder {
  private builder: BaseBuilder;

  constructor(client: GraphQLClient) {
    this.builder = new BaseBuilder(client, OperationType.Query, "Tasks", "tasks");
    this.builder.getSelection().addField("id");
    this.builder.getSelection().addField("name");
  }

  status(value: string): this {
    this.builder.setArg("status", value, "Status");
    return this;
  }

  async execute(): Promise<Task[]> {
    const res = await this.builder.getClient().execute<{ tasks: Task[] }>(
      this.builder.buildQuery(),
      this.builder.getVariables(),
    );
    return res.tasks;
  }

  getOpFragment(alias: string): OpFragment {
    return this.builder.getOpFragment(alias);
  }
}

/** Mutation stand-in used to verify op-kind gating. */
class CreateTaskBuilder {
  private builder: BaseBuilder;

  constructor(client: GraphQLClient) {
    this.builder = new BaseBuilder(client, OperationType.Mutation, "CreateTask", "createTask");
    this.builder.getSelection().addField("id");
  }

  name(value: string): this {
    this.builder.setArg("name", value, "String!");
    return this;
  }

  async execute(): Promise<{ id: string }> {
    const res = await this.builder.getClient().execute<{ createTask: { id: string } }>(
      this.builder.buildQuery(),
      this.builder.getVariables(),
    );
    return res.createTask;
  }

  getOpFragment(alias: string): OpFragment {
    return this.builder.getOpFragment(alias);
  }
}

/** A builder of any op kind with an arbitrary arg list; `execute` is a spy that must stay unused. */
class GenericBuilder implements BatchableBuilder<unknown> {
  executeCalls = 0;
  private builder: BaseBuilder;

  constructor(client: GraphQLClient, op: OperationType, field: string, args: [string, unknown, string][]) {
    this.builder = new BaseBuilder(client, op, "Generic", field);
    for (const [name, value, type] of args) this.builder.setArg(name, value, type);
  }

  selection(): FieldSelection {
    return this.builder.getSelection();
  }

  async execute(): Promise<unknown> {
    this.executeCalls++;
    return undefined;
  }

  getOpFragment(alias: string): OpFragment {
    return this.builder.getOpFragment(alias);
  }
}

// ---------------------------------------------------------------------------
// Go contract mirrors
// ---------------------------------------------------------------------------

test("batch merges multiple builders into one operation with aliases (Go: MergesIntoSingleOperation)", async () => {
  const { client, requests } = clientReturningData({
    open: [{ id: "1", name: "A" }],
    completed: [{ id: "2", name: "B" }],
  });

  const result = await batch(client, {
    open: new TasksBuilder(client),
    completed: new TasksBuilder(client).status("completed"),
  });

  const { query, variables } = onlyRequest(requests);
  assert.equal(
    query,
    "query Batch($completed_status: Status) {\n" +
      "  open: tasks {\n    id\n    name\n  }\n" +
      "  completed: tasks(status: $completed_status) {\n    id\n    name\n  }\n" +
      "}",
  );
  assert.deepEqual(variables, { completed_status: "completed" });

  // Compile-time: each alias is typed from its builder's execute().
  const open: Task[] = result.open;
  assert.deepEqual(open, [{ id: "1", name: "A" }]);
  assert.deepEqual(result.completed, [{ id: "2", name: "B" }]);
});

test("batch namespaces same-named arguments across builders (Go: NamespacesSameNamedArgs)", async () => {
  const { client, requests } = clientReturningData({ a: [], b: [] });
  await batch(client, {
    a: new TasksBuilder(client).status("open"),
    b: new TasksBuilder(client).status("completed"),
  });
  const { query, variables } = onlyRequest(requests);
  assert.match(query, /^query Batch\(\$a_status: Status, \$b_status: Status\) \{/);
  assert.match(query, /a: tasks\(status: \$a_status\)/);
  assert.match(query, /b: tasks\(status: \$b_status\)/);
  assert.deepEqual(variables, { a_status: "open", b_status: "completed" });
});

test("batch with no arguments emits no variable declaration (Go: NoArgs_OmitsVarBlock)", async () => {
  const { client, requests } = clientReturningData({ x: [], y: [] });
  await batch(client, { x: new TasksBuilder(client), y: new TasksBuilder(client) });
  const { query, variables } = onlyRequest(requests);
  assert.match(query, /^query Batch \{/);
  assert.deepEqual(variables, {});
});

test("batch emits aliases in insertion order (Go sorts; TS keeps map order)", async () => {
  const { client, requests } = clientReturningData({ zeta: [], alpha: [], mid: [] });
  await batch(client, {
    zeta: new TasksBuilder(client),
    alpha: new TasksBuilder(client),
    mid: new TasksBuilder(client),
  });
  const aliases = [...onlyRequest(requests).query.matchAll(/^ {2}(\w+): tasks/gm)].map((m) => m[1]);
  assert.deepEqual(aliases, ["zeta", "alpha", "mid"]);
});

test("batch is deterministic across repeated runs", async () => {
  const { client, requests } = clientReturningData({ a: [], b: [] });
  const items = { a: new TasksBuilder(client).status("x"), b: new TasksBuilder(client) };
  await batch(client, items);
  await batch(client, items);
  assert.equal(requests.length, 2);
  assert.equal(requests[0]?.query, requests[1]?.query);
  assert.deepEqual(requests[0]?.variables, requests[1]?.variables);
});

test("batch surfaces partial data on GraphQLErrors (Go: PartialData_PopulatesDestAndReturnsError)", async () => {
  const { client } = clientReplying({
    body: {
      data: { ok: [{ id: "1", name: "A" }], bad: null },
      errors: [{ message: "bad alias failed", path: ["bad"] }],
    },
  });
  await assert.rejects(
    batch(client, { ok: new TasksBuilder(client), bad: new TasksBuilder(client) }),
    (err: unknown) => {
      assert.ok(err instanceof GraphQLErrors);
      assert.equal(err.message, "bad alias failed");
      assert.deepEqual(err.errors[0]?.path, ["bad"]);
      assert.deepEqual(err.data, { ok: [{ id: "1", name: "A" }], bad: null });
      return true;
    },
  );
});

test("batch rejects an empty builder map (Go: EmptyItems)", async () => {
  const { client, requests } = clientReturningData({});
  await assert.rejects(() => batch(client, {}), /at least one builder/);
  assert.equal(requests.length, 0);
});

test("batch merges mutations into one mutation operation (Go: RunMutations_MergesIntoMutation)", async () => {
  const { client, requests } = clientReturningData({ first: { id: "1" }, second: { id: "2" } });
  const result = await batch(client, {
    first: new CreateTaskBuilder(client).name("A"),
    second: new CreateTaskBuilder(client).name("B"),
  });
  const { query, variables } = onlyRequest(requests);
  assert.equal(
    query,
    "mutation Batch($first_name: String!, $second_name: String!) {\n" +
      "  first: createTask(name: $first_name) {\n    id\n  }\n" +
      "  second: createTask(name: $second_name) {\n    id\n  }\n" +
      "}",
  );
  assert.deepEqual(variables, { first_name: "A", second_name: "B" });
  const second: { id: string } = result.second;
  assert.deepEqual(second, { id: "2" });
});

test("OpFragment direct shape (Go: OpFragment_DirectShape)", () => {
  const { client } = clientReturningData({});
  const frag = new TasksBuilder(client).status("open").getOpFragment("open");
  assert.deepEqual(frag, {
    opType: OperationType.Query,
    varDecls: ["$open_status: Status"],
    varValues: { open_status: "open" },
    aliasedField: "open: tasks(status: $open_status) {\n    id\n    name\n  }",
  });
});

// ---------------------------------------------------------------------------
// Matrix: builder count × argument shape × op kind
// ---------------------------------------------------------------------------

interface ArgShape {
  name: string;
  /** Args given to builder number `i` (0-based). */
  argsFor: (i: number) => [string, unknown, string][];
}

const ARG_SHAPES: ArgShape[] = [
  { name: "no args", argsFor: () => [] },
  { name: "same arg name in every builder", argsFor: (i) => [["status", `s${i}`, "Status"]] },
  { name: "distinct arg names", argsFor: (i) => [[`arg${i}`, i, "Int!"]] },
  {
    name: "list + nullable + input object",
    argsFor: (i) => [
      ["ids", [`${i}`], "[ID!]!"],
      ["after", null, "String"],
      ["filter", { done: i % 2 === 0 }, "TodoFilter"],
    ],
  },
  { name: "only some builders have args", argsFor: (i) => (i === 0 ? [] : [["limit", i, "Int"]]) },
];

const BUILDER_COUNTS = [1, 2, 3];

for (const op of OPERATION_TYPES) {
  for (const shape of ARG_SHAPES) {
    for (const count of BUILDER_COUNTS) {
      test(`batch matrix: ${op} × ${shape.name} × ${count} builder(s)`, async () => {
        const aliases = Array.from({ length: count }, (_, i) => `b${i}`);
        const data: Record<string, unknown> = {};
        for (const a of aliases) data[a] = { id: a };
        const { client, requests } = clientReturningData(data);

        const builders: Record<string, GenericBuilder> = {};
        const expectedDecls: string[] = [];
        const expectedFields: string[] = [];
        const expectedVars: Record<string, unknown> = {};
        aliases.forEach((alias, i) => {
          const args = shape.argsFor(i);
          const b = new GenericBuilder(client, op, "node", args);
          b.selection().addField("id");
          builders[alias] = b;
          const passes = args.map(([name]) => `${name}: $${alias}_${name}`);
          for (const [name, value, type] of args) {
            expectedDecls.push(`$${alias}_${name}: ${type}`);
            expectedVars[`${alias}_${name}`] = value;
          }
          const argStr = passes.length > 0 ? `(${passes.join(", ")})` : "";
          expectedFields.push(`${alias}: node${argStr} {\n    id\n  }`);
        });

        const result = await batch(client, builders);

        const req = onlyRequest(requests);
        const varStr = expectedDecls.length > 0 ? `(${expectedDecls.join(", ")})` : "";
        assert.equal(req.query, `${op} ${DEFAULT_BATCH_OP_NAME}${varStr} {\n  ${expectedFields.join("\n  ")}\n}`);
        assert.deepEqual(req.variables, expectedVars);
        assert.deepEqual(result, data);
        for (const b of Object.values(builders)) assert.equal(b.executeCalls, 0, "batch must not call execute()");
      });
    }
  }
}

// ---------------------------------------------------------------------------
// Op-kind gating
// ---------------------------------------------------------------------------

const GATING_CASES: { name: string; ops: OperationType[]; ok: boolean }[] = [
  { name: "query + query", ops: [OperationType.Query, OperationType.Query], ok: true },
  { name: "mutation + mutation", ops: [OperationType.Mutation, OperationType.Mutation], ok: true },
  { name: "query + mutation", ops: [OperationType.Query, OperationType.Mutation], ok: false },
  { name: "mutation + query", ops: [OperationType.Mutation, OperationType.Query], ok: false },
  {
    name: "two queries then a mutation",
    ops: [OperationType.Query, OperationType.Query, OperationType.Mutation],
    ok: false,
  },
  {
    name: "mutation between queries",
    ops: [OperationType.Query, OperationType.Mutation, OperationType.Query],
    ok: false,
  },
];

for (const c of GATING_CASES) {
  test(`batch op-kind gating: ${c.name}`, async () => {
    const data: Record<string, unknown> = {};
    c.ops.forEach((_, i) => (data[`b${i}`] = 1));
    const { client, requests } = clientReturningData(data);
    const builders: Record<string, GenericBuilder> = {};
    c.ops.forEach((op, i) => (builders[`b${i}`] = new GenericBuilder(client, op, "f", [])));

    if (c.ok) {
      await batch(client, builders);
      assert.equal(requests.length, 1);
    } else {
      await assert.rejects(batch(client, builders), /cannot mix operation types/);
      assert.equal(requests.length, 0, "no request may be sent for a rejected batch");
    }
  });
}

// ---------------------------------------------------------------------------
// Names and collisions
// ---------------------------------------------------------------------------

const ALIAS_CASES: { alias: string; valid: boolean }[] = [
  { alias: "a", valid: true },
  { alias: "_", valid: true },
  { alias: "__typenameish", valid: true },
  { alias: "camelCase2", valid: true },
  { alias: "", valid: false },
  { alias: "1st", valid: false },
  { alias: "0", valid: false },
  { alias: "my-alias", valid: false },
  { alias: "a b", valid: false },
  { alias: "a:b", valid: false },
  { alias: "a{", valid: false },
  { alias: "ünï", valid: false },
];

for (const c of ALIAS_CASES) {
  test(`batch alias "${c.alias}" is ${c.valid ? "accepted" : "rejected"}`, async () => {
    const { client, requests } = clientReturningData({ [c.alias]: [] });
    const run = batch(client, { [c.alias]: new TasksBuilder(client) });
    if (c.valid) {
      await run;
      assert.equal(requests.length, 1);
    } else {
      await assert.rejects(run, /invalid alias/);
      assert.equal(requests.length, 0);
    }
  });
}

test("batch honors a custom operation name", async () => {
  const { client, requests } = clientReturningData({ x: [] });
  await batch(client, { x: new TasksBuilder(client) }, { opName: "DashboardLoad" });
  assert.match(onlyRequest(requests).query, /^query DashboardLoad \{/);
});

for (const opName of ["", "Has Space", "9Lives", "Dash-Board"]) {
  test(`batch rejects invalid operation name "${opName}"`, async () => {
    const { client, requests } = clientReturningData({ x: [] });
    await assert.rejects(batch(client, { x: new TasksBuilder(client) }, { opName }), /invalid operation name/);
    assert.equal(requests.length, 0);
  });
}

test("batch rejects a variable-name collision namespacing cannot prevent", async () => {
  const { client, requests } = clientReturningData({});
  await assert.rejects(
    batch(client, {
      a: new GenericBuilder(client, OperationType.Query, "f", [["b_c", 1, "Int"]]),
      a_b: new GenericBuilder(client, OperationType.Query, "f", [["c", 2, "Int"]]),
    }),
    /variable "\$a_b_c" is produced by two aliases/,
  );
  assert.equal(requests.length, 0);
});

test("batch: aliases sharing a prefix but no variable name do not collide", async () => {
  const { client, requests } = clientReturningData({ a: 1, a_b: 2 });
  await batch(client, {
    a: new GenericBuilder(client, OperationType.Query, "f", [["x", 1, "Int"]]),
    a_b: new GenericBuilder(client, OperationType.Query, "f", [["x", 2, "Int"]]),
  });
  assert.deepEqual(onlyRequest(requests).variables, { a_x: 1, a_b_x: 2 });
});

// ---------------------------------------------------------------------------
// Selections and values crossing the batch boundary
// ---------------------------------------------------------------------------

test("batch carries a union binding inside a list field intact", async () => {
  const { client, requests } = clientReturningData({ hits: [] });
  const b = new GenericBuilder(client, OperationType.Query, "search", [["term", "x", "String!"]]);
  const sel = b.selection();
  sel.addField("total");
  const results = new FieldSelection();
  results.addField("__typename");
  const todo = new FieldSelection();
  todo.addField("id");
  results.addChild("... on Todo", todo);
  const user = new FieldSelection();
  user.addField("name");
  results.addChild("... on User", user);
  sel.addChild("results", results);

  await batch(client, { hits: b });

  assert.equal(
    onlyRequest(requests).query,
    "query Batch($hits_term: String!) {\n" +
      "  hits: search(term: $hits_term) {\n" +
      "    total\n" +
      "    results {\n" +
      "      __typename\n" +
      "      ... on Todo {\n        id\n      }\n" +
      "      ... on User {\n        name\n      }\n" +
      "    }\n" +
      "  }\n" +
      "}",
  );
});

test("batch: scalar root fields render without braces", async () => {
  const { client, requests } = clientReturningData({ p: "pong", q: "pong" });
  await batch(client, {
    p: new GenericBuilder(client, OperationType.Query, "ping", []),
    q: new GenericBuilder(client, OperationType.Query, "ping", []),
  });
  assert.equal(onlyRequest(requests).query, "query Batch {\n  p: ping\n  q: ping\n}");
});

test("batch drops undefined variable values but keeps null", async () => {
  const { client, requests } = clientReturningData({ a: 1 });
  await batch(client, {
    a: new GenericBuilder(client, OperationType.Query, "f", [
      ["x", undefined, "Int"],
      ["y", null, "Int"],
    ]),
  });
  const req: CapturedRequest = onlyRequest(requests);
  assert.match(req.query, /\$a_x: Int, \$a_y: Int/);
  assert.deepEqual(req.variables, { a_y: null });
});

test("batch returns the raw response, including aliases the server omitted", async () => {
  const { client } = clientReturningData({ a: [] });
  const result = await batch(client, { a: new TasksBuilder(client), b: new TasksBuilder(client) });
  assert.deepEqual(result.a, []);
  assert.equal(result.b, undefined);
});

test("batch accepts a hand-written BatchableBuilder alongside generated-style ones", async () => {
  const custom: BatchableBuilder<number> = {
    execute: async () => 0,
    getOpFragment: (alias) => ({
      opType: OperationType.Query,
      varDecls: [],
      varValues: {},
      aliasedField: `${alias}: count`,
    }),
  };
  const { client, requests } = clientReturningData({ n: 7, t: [] });
  const result = await batch(client, { n: custom, t: new TasksBuilder(client) });
  const n: number = result.n;
  assert.equal(n, 7);
  assert.match(onlyRequest(requests).query, /^ {2}n: count$/m);
});
