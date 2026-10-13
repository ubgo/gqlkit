// Package batch merges multiple builders into a single GraphQL operation with
// aliased root fields, so callers can fetch several queries (or run several
// mutations) in one HTTP round trip.
//
// Operation type is locked at compile time via two entry points:
//   - RunQueries   accepts only query builders   (embed builder.QueryMarker)
//   - RunMutations accepts only mutation builders (embed builder.MutationMarker)
//
// Mixing query + mutation builders in a single batch is therefore a compile
// error, not a runtime error.
//
// Variable names from each builder are namespaced with the alias used in the
// Items map (e.g. {"open": qr.Todos()....} produces $open_filter, not
// $filter), so two builders sharing an argument name coexist without
// colliding.
//
// Batches are partial-tolerant: when the server returns both data and errors
// in the same response, the caller's destination struct is populated for
// every alias the server resolved successfully, and the per-alias errors are
// surfaced via *batch.Error. Callers always check err and decide whether the
// partial data is sufficient.
package batch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ubgo/gqlkit/gqlkit/pkg/builder"
	"github.com/ubgo/gqlkit/gqlkit/pkg/graphqlclient"
)

// QueryBatchable is satisfied by every generated query builder. The
// IsQueryOp marker method (provided via embedded builder.QueryMarker) prevents
// passing a mutation builder where a query is expected.
type QueryBatchable interface {
	GetOpFragment(alias string) builder.OpFragment
	IsQueryOp()
}

// MutationBatchable is the dual of QueryBatchable for mutation operations.
type MutationBatchable interface {
	GetOpFragment(alias string) builder.OpFragment
	IsMutationOp()
}

// QueryItems maps alias → query builder. Aliases become GraphQL response keys
// in the merged operation and must match the JSON tags on the destination
// struct passed to RunQueries.
type QueryItems map[string]QueryBatchable

// MutationItems is the mutation-side counterpart to QueryItems: alias →
// mutation builder. Mutations in one document run serially in alias order.
type MutationItems map[string]MutationBatchable

// Error wraps the GraphQL errors returned alongside (possibly populated)
// data. Callers can inspect Errors for per-alias diagnostics. The Path on
// each underlying GraphQLError starts with the alias name, so a failed
// "users" alias surfaces as Path: ["users", ...].
type Error struct {
	Errors graphqlclient.GraphQLErrors
}

// Error implements the error interface; delegates to the underlying GraphQL
// errors slice for the message.
func (e *Error) Error() string {
	return e.Errors.Error()
}

// Unwrap exposes the underlying GraphQLErrors so existing
// errors.As(err, &graphqlclient.GraphQLErrors{}) sites continue to work
// against batch errors — important for callers migrating from single-query
// Execute to RunQueries without rewriting their error-handling.
func (e *Error) Unwrap() error {
	return e.Errors
}

// partialExecutor is the subset of *graphqlclient.Client used by batch when
// the underlying client supports partial-data responses.
//
// It's an *optional* interface: builder.GraphQLClient (the interface every
// generated builder accepts) only requires Execute, which short-circuits on
// GraphQL errors. Batch type-asserts the client up to this richer interface
// at runtime — when it succeeds we get partial-success semantics, when it
// fails we fall back to strict Execute. Defining the contract as a local
// interface (instead of importing *graphqlclient.Client directly) keeps the
// dependency one-way and lets tests inject a fake.
type partialExecutor interface {
	ExecuteWithPartialData(ctx context.Context, query string, variables map[string]interface{}) (json.RawMessage, graphqlclient.GraphQLErrors, error)
}

// fragmentSource is satisfied by both QueryBatchable and MutationBatchable —
// the bit of their surface that batch internals actually consume. The op-kind
// separation only matters at the public API; once we're inside run() the
// merging logic doesn't care which marker the builder embeds.
type fragmentSource interface {
	GetOpFragment(alias string) builder.OpFragment
}

// clientHolder is the slice of *builder.BaseBuilder that exposes the GraphQL
// client. Generated builders compose (rather than extend) *BaseBuilder, so
// they satisfy this via method promotion — we can't reach the client by
// downcasting to a concrete *BaseBuilder. Type-asserting against this
// interface keeps batch decoupled from any specific builder layout.
type clientHolder interface {
	GetClient() builder.GraphQLClient
}

// batchOpName is the GraphQL operation name of every merged document
// (`query Batch(...) { ... }`). Server logs and persisted-query tooling key on
// it, so it is fixed rather than derived from the aliases.
const batchOpName = "Batch"

// Sentinel errors for batch inputs that cannot form a valid merged document.
// They are returned before any request is sent, so the caller sees the real
// cause instead of a server-side parse or validation error.
var (
	// ErrNilBuilder is returned when an Items map holds a nil builder.
	ErrNilBuilder = errors.New("batch: nil builder")
	// ErrInvalidAlias is returned when an alias is not a GraphQL Name
	// (/[_A-Za-z][_0-9A-Za-z]*/). The alias is spliced verbatim into the
	// document as the response key and as the variable prefix.
	ErrInvalidAlias = errors.New("batch: alias is not a valid GraphQL name")
	// ErrVariableCollision is returned when two aliases produce the same
	// namespaced variable. Namespacing is "<alias>_<arg>", and both parts
	// may contain "_", so alias "a" with arg "b_c" and alias "a_b" with arg
	// "c" both yield $a_b_c — merging them would silently send one value
	// for both. Renaming either alias resolves it.
	ErrVariableCollision = errors.New("batch: namespaced variable collision")
)

// isGraphQLName reports whether s matches the GraphQL Name production
// /[_A-Za-z][_0-9A-Za-z]*/ (ASCII only, by spec).
func isGraphQLName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// RunQueries merges the supplied query builders into a single GraphQL query
// with aliased root fields, executes it once, and decodes the response into
// dest (a pointer to a struct with json tags matching the alias keys).
//
// On partial success the dest struct is still populated for the aliases the
// server resolved; the GraphQL errors are returned wrapped in *Error.
func RunQueries(ctx context.Context, dest any, items QueryItems) error {
	if len(items) == 0 {
		return errors.New("batch.RunQueries: requires at least one builder")
	}
	srcs := make(map[string]fragmentSource, len(items))
	for alias, b := range items {
		srcs[alias] = b
	}
	return run(ctx, dest, srcs, batchOpName)
}

// RunMutations is the mutation-side counterpart of RunQueries. Aliases are
// sorted, so the server executes the mutations in alias order.
func RunMutations(ctx context.Context, dest any, items MutationItems) error {
	if len(items) == 0 {
		return errors.New("batch.RunMutations: requires at least one builder")
	}
	srcs := make(map[string]fragmentSource, len(items))
	for alias, b := range items {
		srcs[alias] = b
	}
	return run(ctx, dest, srcs, batchOpName)
}

// run is the shared implementation for RunQueries and RunMutations. The
// public functions only differ in the type of their items map (compile-time
// op-kind lock); once erased to fragmentSource, the merging logic is the
// same.
func run(ctx context.Context, dest any, items map[string]fragmentSource, opName string) error {
	// Sort aliases so the same input map always produces a byte-identical
	// query string. Matters for: HTTP cache keys, response snapshots, log
	// readability, and any tooling that fingerprints queries by hash.
	aliases := make([]string, 0, len(items))
	for alias := range items {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	var (
		opType    string
		varDecls  []string
		varValues = map[string]interface{}{}
		fields    []string
	)

	// varOwner maps each namespaced variable to the alias that produced it,
	// to detect ErrVariableCollision.
	varOwner := map[string]string{}

	for i, alias := range aliases {
		b := items[alias]
		if b == nil {
			return fmt.Errorf("%w: alias %q", ErrNilBuilder, alias)
		}
		if !isGraphQLName(alias) {
			return fmt.Errorf("%w: %q", ErrInvalidAlias, alias)
		}
		frag := b.GetOpFragment(alias)
		if i == 0 {
			opType = frag.OpType
		} else if frag.OpType != opType {
			// Should not happen given compile-time op-kind separation, but
			// defend against custom builder impls regardless.
			return fmt.Errorf("batch: cannot mix operation types — saw %q and %q", opType, frag.OpType)
		}
		varDecls = append(varDecls, frag.VarDecls...)
		for k, v := range frag.VarValues {
			if owner, taken := varOwner[k]; taken {
				return fmt.Errorf("%w: $%s is produced by both alias %q and alias %q — rename one alias", ErrVariableCollision, k, owner, alias)
			}
			varOwner[k] = alias
			varValues[k] = v
		}
		fields = append(fields, frag.AliasedField)
	}

	// Assemble document
	var sb strings.Builder
	sb.WriteString(opType)
	sb.WriteString(" ")
	sb.WriteString(opName)
	if len(varDecls) > 0 {
		sb.WriteString("(")
		sb.WriteString(strings.Join(varDecls, ", "))
		sb.WriteString(")")
	}
	sb.WriteString(" {\n  ")
	sb.WriteString(strings.Join(fields, "\n  "))
	sb.WriteString("\n}")
	query := sb.String()

	// Locate the client by reaching into the first builder. Generated
	// builders embed *builder.BaseBuilder which exposes GetClient, so this
	// covers every standard SDK; tests inject a fake via the same method.
	holder, ok := items[aliases[0]].(clientHolder)
	if !ok {
		return fmt.Errorf("batch: builder %T does not expose GetClient — cannot derive transport", items[aliases[0]])
	}
	client := holder.GetClient()
	if client == nil {
		return errors.New("batch: builder returned a nil GraphQLClient")
	}

	// Prefer partial-data path when available so partial successes survive.
	if pe, ok := client.(partialExecutor); ok {
		data, gqlErrs, transportErr := pe.ExecuteWithPartialData(ctx, query, varValues)
		if transportErr != nil {
			return transportErr
		}
		// Decode opportunistically: if the server returned `errors` and
		// `data: null`, len(data) is 0 and json.Unmarshal would no-op
		// anyway — we skip explicitly so dest stays its zero value rather
		// than getting "null" decoded into it.
		if dest != nil && len(data) > 0 {
			if err := json.Unmarshal(data, dest); err != nil {
				return fmt.Errorf("batch: failed to unmarshal data: %w", err)
			}
		}
		if len(gqlErrs) > 0 {
			return &Error{Errors: gqlErrs}
		}
		return nil
	}

	// Fallback: clients without partial-data support. Loses partial-success
	// data when the server returns errors alongside data, but the standard
	// "no errors" path remains correct.
	if err := client.Execute(ctx, query, varValues, dest); err != nil {
		var gqlErrs graphqlclient.GraphQLErrors
		if errors.As(err, &gqlErrs) {
			return &Error{Errors: gqlErrs}
		}
		return err
	}
	return nil
}
