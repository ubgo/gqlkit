package clientgents

import "testing"

// TestGenerateRequestSignal pins that generated execute() and executeRaw()
// accept RequestOptions and hand them to the client: an abort cancels an
// in-flight request, and an already-aborted signal never reaches the server.
func TestGenerateRequestSignal(t *testing.T) {
	sdk := generateSDK(t, `
type User { id: ID! }
type Query { me: User! ping: String }
`, "")
	mustContain(t, read(t, sdk, "queries/ping.ts"), "signal passthrough",
		"async execute(request?: RequestOptions): Promise<string | null> {",
		".execute<{ ping: string | null }>(query, variables, request);",
		".execute<Record<string, unknown>>(this.#builder.buildQuery(), this.#builder.getVariables(), request);",
	)
	typecheckSDK(t, sdk, nil)

	out := runSDKScript(t, sdk, "signal.ts", `
import { createServer } from "node:http";
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";

let hits = 0;
const server = createServer((_req, res) => {
  hits++;
  setTimeout(() => {
    res.setHeader("content-type", "application/json");
    res.end(JSON.stringify({ data: { ping: "pong", me: { id: "1" } } }));
  }, 2000);
});

async function outcome(p: Promise<unknown>): Promise<string> {
  try { await p; return "resolved"; } catch (e: unknown) { return e instanceof Error ? e.name : String(e); }
}

async function main(): Promise<void> {
  await new Promise<void>((done) => server.listen(0, "127.0.0.1", done));
  const addr = server.address();
  if (addr === null || typeof addr === "string") throw new Error("server has no TCP address");
  const root = new QueryRoot(new GraphQLClient("http://127.0.0.1:" + addr.port));

  const pre = new AbortController();
  pre.abort();
  console.log("pre-aborted:", await outcome(root.ping().execute({ signal: pre.signal })), "hits=" + hits);

  const mid = new AbortController();
  setTimeout(() => mid.abort(), 50);
  console.log("mid-flight:", await outcome(root.ping().execute({ signal: mid.signal })));

  const raw = new AbortController();
  setTimeout(() => raw.abort(), 50);
  console.log("raw mid-flight:", await outcome(root.me().select((u) => u.id()).executeRaw({ signal: raw.signal })));
  server.closeAllConnections();
  server.close();
}
main().catch((e: unknown) => { console.error(e); server.close(); process.exitCode = 1; });
`)
	mustContain(t, out, "abort behaviour", "pre-aborted: AbortError hits=0", "mid-flight: AbortError", "raw mid-flight: AbortError")
}
