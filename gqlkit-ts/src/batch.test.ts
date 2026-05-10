import { test } from "node:test";
import { strict as assert } from "node:assert";
import { BaseBuilder } from "./builder";
import { GraphQLClient } from "./graphqlclient";
import { batch } from "./batch";

interface FetchCapture {
  url: string;
  body: { query: string; variables?: Record<string, unknown> };
}

/**
 * Build a `GraphQLClient` whose every request is captured on `captured` and
 * whose response is the supplied JSON `data` payload. Lets tests assert on
 * the exact query string and variables the batch builder produced.
 */
function makeMockClient(
  captured: FetchCapture[],
  data: Record<string, unknown>,
): GraphQLClient {
  const fakeFetch = (async (url: string, init?: RequestInit) => {
    const body = JSON.parse(String(init?.body ?? "{}"));
    captured.push({ url, body });
    return {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      json: async () => ({ data }) as any,
    };
  }) as unknown as typeof fetch;
  return new GraphQLClient("http://test/graphql", { fetch: fakeFetch });
}

/**
 * Minimal stand-in for a generated query builder: takes an optional `status`
 * argument and selects `id` + `name` from the `tasks` root field.
 */
class TasksBuilder extends BaseBuilder {
  constructor(client: GraphQLClient) {
    super(client, "query", "Tasks", "tasks");
    this.getSelection().addField("id");
    this.getSelection().addField("name");
  }

  status(value: string): this {
    this.setArg("status", value, "Status");
    return this;
  }

  async execute(): Promise<Array<{ id: string; name: string }>> {
    const res = await this.executeRaw();
    return res.tasks as Array<{ id: string; name: string }>;
  }
}

/** Mutation stand-in used to verify that `batch` rejects mixed op types. */
class CreateTaskBuilder extends BaseBuilder {
  constructor(client: GraphQLClient) {
    super(client, "mutation", "CreateTask", "createTask");
    this.getSelection().addField("id");
  }

  async execute(): Promise<{ id: string }> {
    const res = await this.executeRaw();
    return res.createTask as { id: string };
  }
}

test("batch merges multiple builders into one operation with aliases", async () => {
  const captured: FetchCapture[] = [];
  const client = makeMockClient(captured, {
    open: [{ id: "1", name: "A" }],
    completed: [{ id: "2", name: "B" }],
  });

  const result = await batch(client, {
    open: new TasksBuilder(client),
    completed: new TasksBuilder(client).status("completed"),
  });

  assert.equal(captured.length, 1, "exactly one HTTP request");
  const { query, variables } = captured[0]!.body;

  assert.match(query, /^query Batch\(\$completed_status: Status\) \{/);
  assert.match(query, /open: tasks \{/);
  assert.match(query, /completed: tasks\(status: \$completed_status\) \{/);
  assert.deepEqual(variables, { completed_status: "completed" });

  assert.deepEqual(result, {
    open: [{ id: "1", name: "A" }],
    completed: [{ id: "2", name: "B" }],
  });
});

test("batch namespaces same-named arguments across builders", async () => {
  const captured: FetchCapture[] = [];
  const client = makeMockClient(captured, { a: [], b: [] });

  await batch(client, {
    a: new TasksBuilder(client).status("open"),
    b: new TasksBuilder(client).status("completed"),
  });

  const { query, variables } = captured[0]!.body;
  assert.match(query, /\$a_status: Status/);
  assert.match(query, /\$b_status: Status/);
  assert.match(query, /a: tasks\(status: \$a_status\)/);
  assert.match(query, /b: tasks\(status: \$b_status\)/);
  assert.deepEqual(variables, { a_status: "open", b_status: "completed" });
});

test("batch with no arguments emits no variable declaration", async () => {
  const captured: FetchCapture[] = [];
  const client = makeMockClient(captured, { x: [], y: [] });

  await batch(client, {
    x: new TasksBuilder(client),
    y: new TasksBuilder(client),
  });

  const { query, variables } = captured[0]!.body;
  assert.match(query, /^query Batch \{/);
  assert.deepEqual(variables, {});
});

test("batch honors a custom operation name", async () => {
  const captured: FetchCapture[] = [];
  const client = makeMockClient(captured, { x: [] });

  await batch(
    client,
    { x: new TasksBuilder(client) },
    { opName: "DashboardLoad" },
  );

  assert.match(captured[0]!.body.query, /^query DashboardLoad \{/);
});

test("batch rejects mixed query and mutation builders", async () => {
  const client = makeMockClient([], {});

  await assert.rejects(
    () =>
      batch(client, {
        a: new TasksBuilder(client),
        b: new CreateTaskBuilder(client),
      }),
    /cannot mix operation types/,
  );
});

test("batch rejects an empty builder map", async () => {
  const client = makeMockClient([], {});
  await assert.rejects(() => batch(client, {}), /at least one builder/);
});
