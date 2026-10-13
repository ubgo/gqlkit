/**
 * Test-only fetch double. Excluded from the published build (see
 * `tsconfig.json` `exclude`) and type-checked by `tsconfig.test.json`.
 *
 * It returns real `Response` objects, so the client under test exercises the
 * same `text()` / `status` / `ok` paths it uses against a live server, and the
 * double satisfies `typeof fetch` without any cast.
 */

import { GraphQLClient, type ClientOptions } from "../graphqlclient";

/** Endpoint every fake client posts to. */
export const TEST_ENDPOINT = "http://test/graphql";

/** One request as the fake server saw it. */
export interface CapturedRequest {
  url: string;
  method: string | undefined;
  /** Header names exactly as sent, to detect duplicate spellings. */
  headerNames: string[];
  /** Headers keyed by lower-cased name. */
  headers: Record<string, string>;
  /** Raw body text. */
  rawBody: string;
  query: string;
  /** `undefined` when the body had no `variables` key. */
  variables: Record<string, unknown> | undefined;
  /** Whether the body JSON had a `variables` key at all. */
  hasVariablesKey: boolean;
  /** The abort signal passed in `init`, if any. */
  signal: AbortSignal | undefined;
  /** Whether `init` had a `signal` key at all. */
  hasSignalKey: boolean;
}

/** What the fake server answers with. */
export interface FakeReply {
  status?: number;
  /** Sent verbatim when a string, JSON-encoded otherwise. */
  body: unknown;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Flatten any `HeadersInit` into ordered `[name, value]` pairs without merging. */
function headerPairs(init: HeadersInit | undefined): [string, string][] {
  if (init === undefined) return [];
  if (init instanceof Headers) {
    // forEach rather than entries(): the build's lib has DOM but not DOM.Iterable.
    const pairs: [string, string][] = [];
    init.forEach((value, name) => pairs.push([name, value]));
    return pairs;
  }
  if (Array.isArray(init)) {
    return init.map((pair): [string, string] => [String(pair[0]), String(pair[1])]);
  }
  return Object.entries(init);
}

/** Parse a request into a {@link CapturedRequest}, failing the test on a malformed body. */
function capture(input: RequestInfo | URL, init: RequestInit | undefined): CapturedRequest {
  const rawBody = typeof init?.body === "string" ? init.body : "";
  const parsed: unknown = JSON.parse(rawBody);
  if (!isRecord(parsed) || typeof parsed.query !== "string") {
    throw new Error(`fake fetch: request body is not {query}: ${rawBody}`);
  }
  const vars = parsed.variables;
  if (vars !== undefined && !isRecord(vars)) {
    throw new Error(`fake fetch: variables is not an object: ${rawBody}`);
  }
  const pairs = headerPairs(init?.headers);
  const headers: Record<string, string> = {};
  for (const [name, value] of pairs) headers[name.toLowerCase()] = value;
  return {
    url: input instanceof Request ? input.url : String(input),
    method: init?.method,
    headerNames: pairs.map(([name]) => name),
    headers,
    rawBody,
    query: parsed.query,
    variables: vars,
    hasVariablesKey: Object.prototype.hasOwnProperty.call(parsed, "variables"),
    signal: init?.signal ?? undefined,
    hasSignalKey: init !== undefined && Object.prototype.hasOwnProperty.call(init, "signal"),
  };
}

/** Turn a {@link FakeReply} into a real `Response`. */
export function toResponse(reply: FakeReply): Response {
  const text = typeof reply.body === "string" ? reply.body : JSON.stringify(reply.body);
  return new Response(text, { status: reply.status ?? 200 });
}

/** A fake fetch plus everything it recorded. */
export interface FakeFetch {
  fetch: typeof fetch;
  requests: CapturedRequest[];
}

/**
 * Build a fetch double that records each request and answers with `reply`
 * (a fixed reply, or a function of the captured request).
 */
export function fakeFetch(
  reply: FakeReply | ((req: CapturedRequest) => FakeReply),
): FakeFetch {
  const requests: CapturedRequest[] = [];
  const fetchImpl: typeof fetch = async (input, init) => {
    const req = capture(input, init);
    requests.push(req);
    return toResponse(typeof reply === "function" ? reply(req) : reply);
  };
  return { fetch: fetchImpl, requests };
}

/**
 * A fetch double that records the request, then never answers on its own —
 * it settles only by rejecting with the signal's reason when aborted, like a
 * real fetch stuck on a slow server. For mid-flight abort tests.
 */
export function hangingFetch(): FakeFetch {
  const requests: CapturedRequest[] = [];
  const fetchImpl: typeof fetch = (input, init) => {
    requests.push(capture(input, init));
    return new Promise<Response>((_resolve, reject) => {
      const signal = init?.signal;
      if (!signal) return;
      signal.addEventListener("abort", () => {
        const reason: unknown = signal.reason;
        reject(reason);
      });
    });
  };
  return { fetch: fetchImpl, requests };
}

/** A client wired to a {@link fakeFetch}, plus the recorded requests. */
export interface FakeClient {
  client: GraphQLClient;
  requests: CapturedRequest[];
}

/** Shorthand: a client whose server answers `{ data }` with HTTP 200. */
export function clientReturningData(
  data: Record<string, unknown>,
  options: Omit<ClientOptions, "fetch"> = {},
): FakeClient {
  return clientReplying({ body: { data } }, options);
}

/** A client whose server answers with `reply`. */
export function clientReplying(
  reply: FakeReply | ((req: CapturedRequest) => FakeReply),
  options: Omit<ClientOptions, "fetch"> = {},
): FakeClient {
  const fake = fakeFetch(reply);
  return {
    client: new GraphQLClient(TEST_ENDPOINT, { ...options, fetch: fake.fetch }),
    requests: fake.requests,
  };
}

/** Return the only captured request, failing if there is not exactly one. */
export function onlyRequest(requests: CapturedRequest[]): CapturedRequest {
  const [first, ...rest] = requests;
  if (first === undefined || rest.length > 0) {
    throw new Error(`expected exactly one request, saw ${requests.length}`);
  }
  return first;
}
