/**
 * @module graphqlclient
 *
 * Provides the low-level HTTP transport for executing GraphQL operations.
 * Handles JSON serialization, header management, auth tokens, and error extraction.
 *
 * The response body is untrusted input: it is parsed as `unknown` and narrowed
 * field by field here, so nothing past this module ever sees an unchecked shape.
 */

import type { ScalarCodecError } from "./codec";

/**
 * Configuration options for {@link GraphQLClient}.
 *
 * The object is read on every request (not copied), so mutating `authToken`
 * or `headers` on the same object after construction affects later requests.
 *
 * @property headers   - Additional HTTP headers merged into every request.
 *                       Names are case-insensitive: a user `content-type`
 *                       replaces the default `Content-Type`.
 * @property authToken - Bearer token sent as `Authorization: Bearer <token>`.
 *                       Takes precedence over any `Authorization` entry in
 *                       `headers`, whatever its case. Empty string means none.
 * @property fetch     - Custom fetch implementation (SSR, testing, retries,
 *                       abort signals). Defaults to `globalThis.fetch`.
 */
export interface ClientOptions {
  headers?: Record<string, string>;
  authToken?: string;
  fetch?: typeof fetch;
}

/**
 * Per-request options for {@link GraphQLClient.execute} and friends.
 *
 * Separate from {@link ClientOptions} because these vary per call (one
 * `AbortController` per screen or per request), while client options are
 * shared by every request.
 *
 * @property signal - Cancels the request. Already aborted → rejects before
 *                    anything is sent; aborted mid-flight → the fetch rejects.
 *                    Either way the rejection is the signal's `reason`
 *                    (an `AbortError` `DOMException` by default), unwrapped.
 */
export interface RequestOptions {
  signal?: AbortSignal;
}

/**
 * Represents a single error entry from a GraphQL response's `errors` array.
 * Follows the GraphQL spec error format.
 *
 * @property message    - Human-readable error description. Always a string:
 *                        entries without one get {@link MISSING_ERROR_MESSAGE}.
 * @property locations  - Source locations in the query that caused the error.
 * @property path       - The response field path where the error occurred;
 *                        for a batch, the first segment is the alias.
 * @property extensions - Vendor-specific metadata (e.g., error codes).
 */
export interface GraphQLError {
  message: string;
  locations?: { line: number; column: number }[];
  path?: (string | number)[];
  extensions?: Record<string, unknown>;
}

/** HTTP header names the client sets itself. Compared case-insensitively. */
export const HEADER = {
  ContentType: "Content-Type",
  Authorization: "Authorization",
} as const;

/** Content type of every request body (GraphQL-over-HTTP JSON encoding). */
export const JSON_CONTENT_TYPE = "application/json";

/** HTTP method used for every operation, including queries. */
export const GRAPHQL_HTTP_METHOD = "POST";

/** Scheme prefix for {@link ClientOptions.authToken}. */
export const BEARER_PREFIX = "Bearer ";

/** Message used for an error entry that has no string `message`. */
export const MISSING_ERROR_MESSAGE = "Unknown GraphQL error";

/** Message thrown when a response has neither `data` nor `errors`. */
export const NO_DATA_MESSAGE = "No data returned from GraphQL query";

/**
 * Longest slice of a non-GraphQL response body quoted in an error message.
 * The full body stays available on {@link GraphQLHTTPError.body}; the cap only
 * keeps a multi-kilobyte HTML error page out of logs.
 */
export const ERROR_BODY_SNIPPET_LENGTH = 200;

/**
 * Thrown when the server reports one or more GraphQL errors.
 *
 * The `message` is a `"; "`-joined summary of every error message. The raw
 * entries are on `errors`, and any partial result the server returned next to
 * them is on `data` — GraphQL allows both at once (e.g. one alias of a batch
 * failed while the others resolved), so callers can still use what succeeded.
 */
export class GraphQLErrors extends Error {
  /** The narrowed GraphQL error objects from the server response. */
  public errors: GraphQLError[];

  /**
   * Partial `data` sent alongside the errors, or `undefined` when the server
   * sent none (or `null`). Untyped because errors can null out any field.
   */
  public data?: Record<string, unknown>;

  /**
   * Set only when the partial `data` failed to decode with the operation's
   * scalar codecs (see `BaseBuilder.decodePartial` and `batch`). `data` is
   * then `undefined`: a mix of decoded and wire values would be worse than
   * none. The server's errors stay the thrown error; this is secondary.
   */
  public decodeError?: ScalarCodecError;

  /**
   * @param errors - Array of GraphQL error objects from the response.
   * @param data   - Partial data returned alongside the errors, if any.
   */
  constructor(errors: GraphQLError[], data?: Record<string, unknown>) {
    super(errors.map((e) => e.message).join("; "));
    this.name = "GraphQLErrors";
    this.errors = errors;
    this.data = data;
  }
}

/**
 * Thrown when the HTTP response is not a GraphQL response at all: a body that
 * is not JSON, or JSON with neither `data` nor `errors` (an HTML 502 page, a
 * proxy's `{"message":"Unauthorized"}`).
 *
 * Why it exists: without it, a gateway error surfaces as a bare `SyntaxError`
 * from `JSON.parse` that hides the status code. A non-2xx response whose body
 * IS a GraphQL response (allowed by GraphQL-over-HTTP) raises
 * {@link GraphQLErrors} instead, because those errors are more specific.
 */
export class GraphQLHTTPError extends Error {
  /** HTTP status code of the response. */
  public status: number;

  /** Full response body text, untruncated. */
  public body: string;

  /**
   * @param status - HTTP status code.
   * @param body   - Raw response body text.
   * @param reason - What made the response unusable.
   */
  constructor(status: number, body: string, reason: string) {
    const snippet = body.slice(0, ERROR_BODY_SNIPPET_LENGTH);
    super(`GraphQL HTTP ${status}: ${reason}${snippet ? ` — ${snippet}` : ""}`);
    this.name = "GraphQLHTTPError";
    this.status = status;
    this.body = body;
  }
}

/** Narrow `unknown` to a plain (non-array, non-null) object. */
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Narrow a `locations` entry list, dropping entries that are not `{line, column}` numbers. */
function toLocations(value: unknown): GraphQLError["locations"] {
  if (!Array.isArray(value)) return undefined;
  const out: { line: number; column: number }[] = [];
  for (const loc of value) {
    if (isRecord(loc) && typeof loc.line === "number" && typeof loc.column === "number") {
      out.push({ line: loc.line, column: loc.column });
    }
  }
  return out;
}

/** Narrow a `path`, keeping it only if every segment is a string or number. */
function toPath(value: unknown): GraphQLError["path"] {
  if (!Array.isArray(value)) return undefined;
  const out: (string | number)[] = [];
  for (const seg of value) {
    if (typeof seg !== "string" && typeof seg !== "number") return undefined;
    out.push(seg);
  }
  return out;
}

/**
 * Narrow one untrusted `errors[]` entry into a {@link GraphQLError}.
 *
 * Never throws: a malformed entry still becomes an error (with
 * {@link MISSING_ERROR_MESSAGE} or the stringified value), because dropping it
 * would turn a failed request into an apparent success.
 */
function toGraphQLError(entry: unknown): GraphQLError {
  if (!isRecord(entry)) {
    return { message: typeof entry === "string" ? entry : MISSING_ERROR_MESSAGE };
  }
  const err: GraphQLError = {
    message: typeof entry.message === "string" ? entry.message : MISSING_ERROR_MESSAGE,
  };
  const locations = toLocations(entry.locations);
  if (locations) err.locations = locations;
  const path = toPath(entry.path);
  if (path) err.path = path;
  if (isRecord(entry.extensions)) err.extensions = entry.extensions;
  return err;
}

/**
 * Narrow the `errors` member of a response.
 *
 * Shapes accepted: absent / `null` / `[]` (no errors), an array of entries,
 * or a single non-array value, which a few non-conforming servers send and
 * which is treated as one entry rather than ignored.
 */
function toGraphQLErrors(value: unknown): GraphQLError[] {
  if (value === undefined || value === null) return [];
  if (Array.isArray(value)) return value.map(toGraphQLError);
  return [toGraphQLError(value)];
}

/**
 * Build request headers: the JSON content type, then user headers, then the
 * bearer token. A later entry replaces an earlier one whose name matches
 * case-insensitively, so the request never carries two spellings of the same
 * header (fetch would join them into an invalid `a, b` value).
 */
function buildHeaders(options: ClientOptions): Record<string, string> {
  const headers: Record<string, string> = {};
  const set = (name: string, value: string): void => {
    const lower = name.toLowerCase();
    for (const existing of Object.keys(headers)) {
      if (existing.toLowerCase() === lower) delete headers[existing];
    }
    headers[name] = value;
  };
  set(HEADER.ContentType, JSON_CONTENT_TYPE);
  for (const [name, value] of Object.entries(options.headers ?? {})) {
    set(name, value);
  }
  if (options.authToken) {
    set(HEADER.Authorization, `${BEARER_PREFIX}${options.authToken}`);
  }
  return headers;
}

/**
 * HTTP client for executing GraphQL queries and mutations against a single endpoint.
 *
 * Responsibilities:
 * - Sends POST requests with JSON-encoded `{ query, variables }` bodies.
 * - Merges custom headers and optional Bearer auth token into each request.
 * - Parses and narrows the JSON response, throwing {@link GraphQLErrors} when
 *   the server reports errors and {@link GraphQLHTTPError} when the response
 *   is not a GraphQL response.
 * - Supports a custom `fetch` function for environments without a global `fetch`.
 *
 * Network failures and aborts are not wrapped: the rejection from `fetch`
 * propagates unchanged, so `err.name === "AbortError"` checks keep working.
 *
 * @example
 * ```ts
 * const client = new GraphQLClient("https://api.example.com/graphql", {
 *   authToken: "my-token",
 * });
 * const data = await client.execute<{ user: User }>(query, variables);
 * ```
 */
export class GraphQLClient {
  /** The GraphQL endpoint URL (e.g., "https://api.example.com/graphql"). */
  private endpoint: string;

  /** Client configuration including headers, auth, and optional custom fetch. */
  private options: ClientOptions;

  /**
   * @param endpoint - The URL of the GraphQL server.
   * @param options  - Optional client configuration.
   */
  constructor(endpoint: string, options?: ClientOptions) {
    this.endpoint = endpoint;
    this.options = options ?? {};
  }

  /**
   * Execute a GraphQL operation and return the typed `data` payload.
   *
   * Outcome by response shape:
   * - `errors` non-empty → {@link GraphQLErrors} (with partial `data`, if any),
   *   regardless of HTTP status.
   * - `data` is an object, no errors → returned as `T`.
   * - JSON object with neither → `Error(NO_DATA_MESSAGE)` on 2xx,
   *   {@link GraphQLHTTPError} otherwise.
   * - Body not JSON, or JSON that is not an object → {@link GraphQLHTTPError}.
   *
   * `T` is the caller's claim about the shape of `data`; it is not validated
   * beyond "is an object", because the runtime has no schema to check against.
   *
   * @typeParam T - The expected shape of `response.data`.
   * @param query     - The GraphQL query or mutation string.
   * @param variables - Optional variables map; omitted from the body when undefined.
   * @param request   - Optional per-request options (an abort `signal`).
   * @returns The `data` field from the GraphQL response, typed as `T`.
   * @throws {GraphQLErrors} If the response contains a non-empty `errors` array.
   * @throws {GraphQLHTTPError} If the response is not a GraphQL response.
   * @throws {Error} If the response has no `data` and no `errors`, or no fetch exists.
   */
  async execute<T>(
    query: string,
    variables?: Record<string, unknown>,
    request: RequestOptions = {},
  ): Promise<T> {
    const { signal } = request;
    // Checked here, not left to fetch: a custom fetch may ignore `signal`, and
    // an already-cancelled request must never reach the server.
    if (signal?.aborted) {
      const reason: unknown = signal.reason;
      throw reason;
    }

    // Read globalThis.fetch per call (not at construction) so polyfills and
    // test stubs installed later still apply. Called unbound on purpose:
    // browsers throw "Illegal invocation" if fetch runs with a foreign `this`.
    const fetchFn = this.options.fetch ?? globalThis.fetch;
    if (typeof fetchFn !== "function") {
      throw new Error(
        "GraphQLClient: no fetch implementation — pass options.fetch or run on a platform with a global fetch",
      );
    }

    const init: RequestInit = {
      method: GRAPHQL_HTTP_METHOD,
      headers: buildHeaders(this.options),
      body: JSON.stringify({ query, variables }),
    };
    if (signal) init.signal = signal;
    const response = await fetchFn(this.endpoint, init);

    const text = await response.text();
    let parsed: unknown;
    try {
      parsed = JSON.parse(text);
    } catch {
      throw new GraphQLHTTPError(response.status, text, "response body is not JSON");
    }
    if (!isRecord(parsed)) {
      throw new GraphQLHTTPError(response.status, text, "response body is not a JSON object");
    }

    const errors = toGraphQLErrors(parsed.errors);
    const data = isRecord(parsed.data) ? parsed.data : undefined;

    if (errors.length > 0) {
      throw new GraphQLErrors(errors, data);
    }
    if (data === undefined) {
      if (!response.ok) {
        throw new GraphQLHTTPError(response.status, text, "response has no data or errors");
      }
      throw new Error(NO_DATA_MESSAGE);
    }

    // Boundary cast: `data` is a verified object; `T` is the caller's
    // declaration of its fields (see the doc comment above).
    return data as T;
  }

  /**
   * Execute a raw GraphQL query without type inference on the result.
   * Convenience wrapper around {@link execute} for ad-hoc / untyped queries.
   *
   * @param query     - The GraphQL query or mutation string.
   * @param variables - Optional variables map for the operation.
   * @param request   - Optional per-request options (an abort `signal`).
   * @returns The untyped `data` payload from the GraphQL response.
   */
  async rawQuery(
    query: string,
    variables?: Record<string, unknown>,
    request?: RequestOptions,
  ): Promise<unknown> {
    return this.execute<Record<string, unknown>>(query, variables, request);
  }
}
