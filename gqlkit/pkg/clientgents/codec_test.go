package clientgents

import (
	"strings"
	"testing"
)

// codecSDL exercises every codec position: decoded leaves at list depth 0
// and 2, a decoded scalar return at depth 0 and 1, a union whose member
// reaches a decoder, an object that does not, encoded scalar arguments, a
// recursive input with encoded fields (needs a plan thunk), an input with
// nothing to encode, and a scalar with a type-only binding (no codec).
const codecSDL = `
scalar DateTime
scalar JSON
input Filter { after: DateTime range: [DateTime!] nested: Filter plain: String }
input Plain { q: String }
type Todo { id: ID! at: DateTime! ats: [[DateTime]] json: JSON parent: Todo }
type Other { n: Int }
union R = Todo | Other
type Query {
  todos(filter: Filter, when: DateTime, plain: Plain): [Todo!]!
  now: DateTime
  nows: [DateTime!]!
  r: [R!]!
  other: Other
}
type Mutation { touch(at: DateTime!): Boolean }
`

const codecConfig = `{ "bindings": {
  "DateTime": { "type": "Date", "decode": "parseDate", "encode": "formatDate", "codecImport": "../codecs.js" },
  "JSON": "Record<string, unknown>"
} }`

// codecModule is the user's codec module the binding imports.
const codecModule = `export const parseDate = (iso: string): Date => new Date(iso);
export const formatDate = (d: Date): string => d.toISOString();
`

// TestGenerateScalarCodec pins the generated codec wiring, compiles it under
// CommonJS and Node ESM, and runs it: wire strings in a response come back as
// Date objects at every list depth and through a union fragment, and Date
// arguments and input fields are sent as ISO strings.
func TestGenerateScalarCodec(t *testing.T) {
	sdk := generateSDK(t, codecSDL, codecConfig)

	mustContain(t, read(t, sdk, "scalars/index.ts"), "codec re-export",
		`export { parseDate as decodeDateTime, formatDate as encodeDateTime } from "../codecs.js";`)
	mustContain(t, read(t, sdk, "builder/index.ts"), "runtime codec types",
		`export type { ArgEncoder, InputEncodePlan } from "gqlkit-ts";`)

	inputs := read(t, sdk, "inputs/index.ts")
	mustContain(t, inputs, "encode plan",
		`import type { InputEncodePlan } from "../builder/index.js";`,
		`import { encodeDateTime } from "../scalars/index.js";`,
		"export const FilterEncodePlan: InputEncodePlan = {\n  after: { scalar: encodeDateTime, listDepth: 0 },\n  range: { scalar: encodeDateTime, listDepth: 1 },\n  nested: { input: () => FilterEncodePlan },\n};",
	)
	assertAbsent(t, inputs, "PlainEncodePlan")

	mustContain(t, read(t, sdk, "fields/todo.ts"), "decoded leaves",
		`import { decodeDateTime } from "../scalars/index.js";`,
		`this.#selection.addField("at", decodeDateTime, 0);`,
		`this.#selection.addField("ats", decodeDateTime, 2);`,
		`this.#selection.addField("json");`,
	)

	todos := read(t, sdk, "queries/todos.ts")
	mustContain(t, todos, "encoded args, decoded result",
		`import { encodeDateTime } from "../scalars/index.js";`,
		`import { FilterEncodePlan } from "../inputs/index.js";`,
		`this.#builder.setArg("filter", v, "Filter", { input: () => FilterEncodePlan });`,
		`this.#builder.setArg("when", v, "DateTime", { scalar: encodeDateTime, listDepth: 0 });`,
		`this.#builder.setArg("plain", v, "Plain");`,
		"return this.#builder.decodeResult(response.todos);",
	)
	mustContain(t, read(t, sdk, "queries/now.ts"), "scalar return", "this.#builder.setResultDecoder(decodeDateTime, 0);", "decodeResult(response.now)")
	mustContain(t, read(t, sdk, "queries/nows.ts"), "scalar list return", "this.#builder.setResultDecoder(decodeDateTime, 1);")
	mustContain(t, read(t, sdk, "queries/r.ts"), "union reaching a decoder", "decodeResult(response.r)")
	other := read(t, sdk, "queries/other.ts")
	mustContain(t, other, "no decoder reachable", "return response.other;")
	assertAbsent(t, other, "decodeResult")

	extra := map[string]string{"sdk/codecs.ts": codecModule}
	for _, mode := range []tsModuleMode{tsModeCommonJS, tsModuleModes[2]} {
		if out, err := runTSCMode(t, sdk, extra, mode); err != nil {
			t.Fatalf("tsc (%s) failed: %v\n%s", mode.name, err, out)
		}
	}
	// Back to CommonJS for the tsx run below.
	if out, err := runTSC(t, sdk, extra); err != nil {
		t.Fatalf("tsc failed: %v\n%s", err, out)
	}

	out := runSDKScript(t, sdk, "run.ts", `
import { createServer } from "node:http";
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";

const ISO = "2026-10-13T10:00:00.000Z";
const data = {
  todos: [{ at: ISO, ats: [[ISO, null], null] }],
  now: ISO,
  r: [{ __typename: "Todo", at: ISO }, { __typename: "Other", n: 1 }],
};
let lastBody = "";
const server = createServer((req, res) => {
  let body = "";
  req.on("data", (c: Buffer) => { body += c.toString(); });
  req.on("end", () => {
    lastBody = body;
    res.setHeader("content-type", "application/json");
    res.end(JSON.stringify({ data }));
  });
});

async function main(): Promise<void> {
  await new Promise<void>((done) => server.listen(0, "127.0.0.1", done));
  const addr = server.address();
  if (addr === null || typeof addr === "string") throw new Error("server has no TCP address");
  const root = new QueryRoot(new GraphQLClient("http://127.0.0.1:" + addr.port));
  const when = new Date(ISO);
  const todos = await root.todos()
    .when(when)
    .filter({ after: when, range: [when], nested: { after: when }, plain: "x" })
    .select((t) => t.at().ats())
    .execute();
  console.log("body:", lastBody);
  console.log("at is Date:", todos[0].at instanceof Date);
  console.log("ats[0][0] is Date:", todos[0].ats?.[0]?.[0] instanceof Date, "null kept:", todos[0].ats?.[0]?.[1] === null && todos[0].ats?.[1] === null);
  const now = await root.now().execute();
  console.log("now is Date:", now instanceof Date);
  const r = await root.r().select((s) => s.onTodo((t) => t.at())).execute();
  const first = r[0];
  console.log("union member decoded:", first.__typename === "Todo" && first.at instanceof Date);
  server.close();
}
main().catch((e: unknown) => { console.error(e); server.close(); process.exitCode = 1; });
`)
	mustContain(t, out, "runtime codec",
		`"when":"2026-10-13T10:00:00.000Z"`,
		`"filter":{"after":"2026-10-13T10:00:00.000Z","range":["2026-10-13T10:00:00.000Z"],"nested":{"after":"2026-10-13T10:00:00.000Z"},"plain":"x"}`,
		"at is Date: true",
		"ats[0][0] is Date: true null kept: true",
		"now is Date: true",
		"union member decoded: true",
	)
}

// assertAbsent fails if s contains unwanted.
func assertAbsent(t *testing.T, s, unwanted string) {
	t.Helper()
	if strings.Contains(s, unwanted) {
		t.Errorf("unexpected %q in:\n%s", unwanted, s)
	}
}

// TestGenerateScalarCodecPartialData pins decoding of partial data on a
// failed single operation: the server's GraphQLErrors are still thrown, but
// err.data arrives decoded; when the partial data itself fails to decode,
// err.data is cleared and err.decodeError carries the ScalarCodecError.
func TestGenerateScalarCodecPartialData(t *testing.T) {
	sdk := generateSDK(t, codecSDL, codecConfig)
	mustContain(t, read(t, sdk, "queries/todos.ts"), "partial decode",
		`import { BaseBuilder, GraphQLClient, GraphQLErrors } from "../builder/index.js";`,
		"if (e instanceof GraphQLErrors) this.#builder.decodePartial(e);",
	)
	assertAbsent(t, read(t, sdk, "queries/other.ts"), "GraphQLErrors")

	strict := `export const parseDate = (iso: string): Date => {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) throw new Error("not a date: " + iso);
  return d;
};
export const formatDate = (d: Date): string => d.toISOString();
`
	if out, err := runTSC(t, sdk, map[string]string{"sdk/codecs.ts": strict}); err != nil {
		t.Fatalf("tsc failed: %v\n%s", err, out)
	}
	out := runSDKScript(t, sdk, "partial.ts", `
import { createServer } from "node:http";
import { GraphQLClient, GraphQLErrors, ScalarCodecError } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";

const replies = [
  { data: { todos: [{ at: "2026-10-13T10:00:00.000Z", ats: null }] }, errors: [{ message: "ats failed", path: ["todos", 0, "ats"] }] },
  { data: { todos: [{ at: "garbage", ats: null }] }, errors: [{ message: "ats failed" }] },
];
let n = 0;
const server = createServer((_req, res) => {
  res.setHeader("content-type", "application/json");
  res.end(JSON.stringify(replies[n++]));
});

async function failure(root: QueryRoot): Promise<GraphQLErrors> {
  try {
    await root.todos().select((t) => t.at().ats()).execute();
  } catch (e: unknown) {
    if (e instanceof GraphQLErrors) return e;
    throw e;
  }
  throw new Error("expected GraphQLErrors");
}

async function main(): Promise<void> {
  await new Promise<void>((done) => server.listen(0, "127.0.0.1", done));
  const addr = server.address();
  if (addr === null || typeof addr === "string") throw new Error("server has no TCP address");
  const root = new QueryRoot(new GraphQLClient("http://127.0.0.1:" + addr.port));

  const ok = await failure(root);
  const todos = ok.data?.todos;
  const first = Array.isArray(todos) ? todos[0] : undefined;
  const at = typeof first === "object" && first !== null && "at" in first ? first.at : undefined;
  console.log("errors kept:", ok.message, "| partial at is Date:", at instanceof Date, "| decodeError:", ok.decodeError === undefined ? "none" : "set");

  const bad = await failure(root);
  console.log("bad partial: data", bad.data === undefined ? "cleared" : "kept", "| decodeError is ScalarCodecError:", bad.decodeError instanceof ScalarCodecError);
  server.close();
}
main().catch((e: unknown) => { console.error(e); server.close(); process.exitCode = 1; });
`)
	mustContain(t, out, "partial data",
		"errors kept: ats failed | partial at is Date: true | decodeError: none",
		"bad partial: data cleared | decodeError is ScalarCodecError: true",
	)
}
