package clientgents

// Rule: no bare strings for any value with a closed set of choices. Every
// generated directory, import path, name suffix and fallback type the
// generator emits is named here once, so the Go side and the TS output it
// writes cannot drift apart through a typo in one call site.

// Generated SDK layout. Each directory holds one module of the TypeScript
// SDK; operation files import their siblings through the relative paths
// below, so a rename must change both the directory and its import path.
const (
	dirBuilder   = "builder"
	dirScalars   = "scalars"
	dirEnums     = "enums"
	dirTypes     = "types"
	dirInputs    = "inputs"
	dirFields    = "fields"
	dirQueries   = "queries"
	dirMutations = "mutations"

	// tsExt is the extension of every generated file.
	tsExt = ".ts"
	// indexFile is the barrel module each directory exposes.
	indexFile = "index" + tsExt
	// rootFile holds the QueryRoot / MutationRoot entry class of an
	// operation directory.
	rootFile = "root" + tsExt
)

// jsExt is written on every relative import. Node's ESM resolver (and TS
// under moduleResolution node16/nodenext) needs the full file name: no
// extension guessing and no directory index lookup (TS2835). TS maps a ".js"
// specifier to the ".ts" source under every resolution mode, so bundler and
// commonjs/node10 users are unaffected.
const jsExt = ".js"

// dirImport returns the import path of a sibling SDK directory's barrel, as
// seen from a file one level below the SDK root: "enums" → "../enums/index.js".
func dirImport(dir string) string {
	return "../" + dir + "/index" + jsExt
}

// siblingImport returns the import path of a file in the same directory:
// "user" → "./user.js".
func siblingImport(file string) string {
	return "./" + file + jsExt
}

// generatedDirs are the SDK's own subdirectories, the only directories Prune
// may remove (and only when it has left them empty).
var generatedDirs = []string{dirBuilder, dirScalars, dirEnums, dirTypes, dirInputs, dirFields, dirQueries, dirMutations}

// Import paths used from inside an operation file (queries/*.ts or
// mutations/*.ts), which sits one level below the SDK root.
var (
	importBuilder = dirImport(dirBuilder)
	importScalars = dirImport(dirScalars)
	importEnums   = dirImport(dirEnums)
	importInputs  = dirImport(dirInputs)
	importFields  = dirImport(dirFields)
)

// Names exported by builder/index.ts (re-exported from the gqlkit-ts runtime)
// that every operation file imports, plus the generated SelectionRequired
// marker that object-returning builders start with.
const (
	tsBaseBuilder       = "BaseBuilder"
	tsGraphQLClient     = "GraphQLClient"
	tsSelectionRequired = "SelectionRequired"
	tsRequestOptions    = "RequestOptions"
)

// introspectionPrefix marks GraphQL introspection names (__schema, __Type,
// __typename). The spec reserves it, and the __* types are never generated,
// so anything carrying it is skipped.
const introspectionPrefix = "__"

// typenameField is GraphQL's meta-field naming an object's concrete type.
// Abstract selections always request it so results can be narrowed.
const typenameField = "__typename"

// variantMethodPrefix starts each inline-fragment method on an abstract
// selector: on<Type>() selects fields of one possible type.
const variantMethodPrefix = "on"

// selectorSuffix turns a GraphQL object type name into its field selector
// class name (User → UserFields).
const selectorSuffix = "Fields"

// selectorGeneric and selectGeneric are the type parameters the generated
// selector callbacks and select() methods narrow the result through.
const (
	selectorGeneric = "U"
	selectGeneric   = "T"
)

// tsAny is the fallback TS type for a GraphQL type the generator cannot
// resolve (a nil type, or a name missing from both the schema and the
// bindings). It keeps the SDK compiling instead of failing generation.
const tsAny = "any"

// tsArraySuffix is the TypeScript array type suffix; see tsListOf for when
// the element needs parentheses before it.
const tsArraySuffix = "[]"

// tsNullSuffix marks a GraphQL-nullable position: the server sends JSON
// null there, so the TS type must admit null (see tsOrNull).
const tsNullSuffix = " | null"

// gqlStringScalar is GraphQL's built-in String scalar. Its scalars/index.ts
// alias is renamed to gqlStringAlias because `export type String` would
// shadow the global String wrapper type inside every module importing it.
const (
	gqlStringScalar = "String"
	gqlStringAlias  = "GqlString"
)

// conventionalRootNames are the GraphQL default operation root type names;
// see isRootType for why roots are matched by these names.
var conventionalRootNames = []string{"Query", "Mutation", "Subscription"}

// opKind is the GraphQL operation kind an operation builder is generated for.
// The value is written verbatim into the generated BaseBuilder constructor
// call, so it must be the GraphQL keyword ("query" / "mutation").
type opKind string

const (
	opQuery    opKind = "query"
	opMutation opKind = "mutation"
)

// opKindSpec is everything that differs between the query and mutation
// halves of the operation output.
type opKindSpec struct {
	dir           string // output directory, e.g. "queries"
	rootClass     string // entry class in <dir>/root.ts, e.g. "QueryRoot"
	builderSuffix string // appended to the PascalCase field name
}

// opKindSpecs maps each opKind to its output conventions. Mutation builders
// carry a distinct suffix so a query and a mutation with the same field name
// never export the same class name from the SDK.
var opKindSpecs = map[opKind]opKindSpec{
	opQuery:    {dir: dirQueries, rootClass: "QueryRoot", builderSuffix: "Builder"},
	opMutation: {dir: dirMutations, rootClass: "MutationRoot", builderSuffix: "MutationBuilder"},
}
