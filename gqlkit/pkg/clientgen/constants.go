package clientgen

// Rule: NO BARE STRINGS for any value with a closed set of choices. Every
// comparison, switch and generated-path decision in this package picks from
// the constants below, so a typo is a compile error and a rename is one place.

// opKind names a GraphQL root operation kind the generator emits builders
// for. The string value is written verbatim into generated code (it is the
// first token of the GraphQL document, `query Foo { ... }`, and the opType
// argument of builder.NewBaseBuilder), so it must stay lowercase GraphQL
// keyword spelling.
type opKind string

const (
	// opKindQuery selects the queries/ package, the "<Field>Builder" naming
	// and the builder.QueryMarker embed.
	opKindQuery opKind = "query"
	// opKindMutation selects the mutations/ package, the
	// "<Field>MutationBuilder" naming and the builder.MutationMarker embed.
	opKindMutation opKind = "mutation"
)

// Conventional GraphQL root type names. Types with exactly these names are
// represented by the generated queries/ and mutations/ builder packages, not
// as data structs or field selectors. A schema whose root has a different name
// (Shopify's "QueryRoot") is deliberately NOT matched: other types can
// reference such a root by value, so its struct and selector must exist.
const (
	rootQueryName        = "Query"
	rootMutationName     = "Mutation"
	rootSubscriptionName = "Subscription"
)

// Generated SDK sub-package names. Each is both the directory under the
// output root and the Go package identifier written into the files, and the
// import path suffix appended to Config.Package.
const (
	pkgTypes         = "types"
	pkgInputs        = "inputs"
	pkgEnums         = "enums"
	pkgScalars       = "scalars"
	pkgFields        = "fields"
	pkgBuilder       = "builder"
	pkgGraphQLClient = "graphqlclient"
	pkgBatch         = "batch"
	pkgQueries       = "queries"
	pkgMutations     = "mutations"
)

// goEmptyInterface is the Go type text emitted for GraphQL types that have no
// concrete Go representation in the generated SDK (unions, interfaces used as
// struct fields, and a defensive nil-type fallback). Kept as the long spelling
// rather than `any` so regenerated SDKs stay byte-identical to earlier
// releases.
const goEmptyInterface = "interface{}"

// isConventionalRootName reports whether name is one of the conventional root
// operation type names, see rootQueryName.
func isConventionalRootName(name string) bool {
	switch name {
	case rootQueryName, rootMutationName, rootSubscriptionName:
		return true
	}
	return false
}
