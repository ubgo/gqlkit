/**
 * @module gqlkit-ts
 *
 * Public API surface for the gqlkit-ts runtime library.
 * Generated GraphQL SDKs depend on these exports to build and execute queries,
 * so everything re-exported here is a compatibility contract.
 *
 * Re-exports:
 * - {@link FieldSelection}   - Tracks which fields are selected in a GraphQL query tree.
 * - {@link BaseBuilder}      - Operation builder that generated SDKs compose (query/mutation).
 * - {@link OperationType}    - The closed set of operation keywords.
 * - {@link GraphQLClient}    - HTTP client that sends GraphQL operations to a server.
 * - {@link GraphQLErrors}    - Thrown when the server returns GraphQL errors (carries partial data).
 * - {@link GraphQLHTTPError} - Thrown when the HTTP response is not a GraphQL response.
 * - {@link batch}            - Merge several builders into one aliased operation.
 * - {@link ScalarCodecError} - Thrown when a scalar codec fails, naming the path.
 */

export {
  FieldSelection,
  BaseBuilder,
  OperationType,
  OPERATION_TYPES,
} from "./builder";
export { GraphQLClient, GraphQLErrors, GraphQLHTTPError } from "./graphqlclient";
export type { ClientOptions, GraphQLError, RequestOptions } from "./graphqlclient";
export { batch, DEFAULT_BATCH_OP_NAME, GRAPHQL_NAME_PATTERN } from "./batch";
export { ScalarCodecError, CodecDirection } from "./codec";
export type {
  ScalarDecoder,
  ScalarEncoder,
  ArgEncoder,
  InputEncodePlan,
} from "./codec";
export type {
  BatchableBuilder,
  BatchResult,
  BatchOptions,
  OpFragment,
} from "./batch";
