// Package builder provides the runtime foundation for generated GraphQL operation
// builders. It contains FieldSelection (a recursive tree that assembles the
// requested fields of a GraphQL selection set) and BaseBuilder (the shared logic
// every generated query/mutation builder delegates to for argument tracking,
// query string construction, and execution).
package builder

import (
	"context"
	"sort"
	"strings"
)

// GraphQLClient is the interface that the generated SDK expects consumers to
// implement (or satisfy via graphqlclient.Client). It sends a single GraphQL
// request and unmarshals the JSON response into the supplied target.
type GraphQLClient interface {
	Execute(ctx context.Context, query string, variables map[string]interface{}, response any) error
}

// FieldSelection tracks selected fields for a GraphQL query.
// Scalar fields are stored in `fields`; nested object selections are stored in
// `children`, forming a recursive tree that mirrors the GraphQL selection set.
type FieldSelection struct {
	fields   []string                    // leaf/scalar field names
	children map[string]*FieldSelection  // nested object selections keyed by field name
}

// NewFieldSelection creates a new FieldSelection
func NewFieldSelection() *FieldSelection {
	return &FieldSelection{
		fields:   make([]string, 0),
		children: make(map[string]*FieldSelection),
	}
}

// AddField adds a scalar field to the selection
func (fs *FieldSelection) AddField(name string) {
	fs.fields = append(fs.fields, name)
}

// AddChild adds a nested field selection
func (fs *FieldSelection) AddChild(name string, child *FieldSelection) {
	fs.children[name] = child
}

// Build recursively renders the field selection into a GraphQL selection set
// string. Each level is indented by `indent` levels of two spaces. Child keys
// are sorted alphabetically to ensure deterministic output across runs.
func (fs *FieldSelection) Build(indent int) string {
	if len(fs.fields) == 0 && len(fs.children) == 0 {
		return ""
	}

	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)

	for _, field := range fs.fields {
		sb.WriteString(prefix + field + "\n")
	}

	// Sort children keys for deterministic output
	childKeys := make([]string, 0, len(fs.children))
	for k := range fs.children {
		childKeys = append(childKeys, k)
	}
	sort.Strings(childKeys)

	for _, name := range childKeys {
		child := fs.children[name]
		childStr := child.Build(indent + 1)
		if childStr != "" {
			sb.WriteString(prefix + name + " {\n")
			sb.WriteString(childStr)
			sb.WriteString(prefix + "}\n")
		}
	}

	return sb.String()
}

// BaseBuilder provides the shared state and logic for every generated operation
// builder. It tracks the GraphQL client, the operation type ("query" or
// "mutation"), the operation and field names, arguments with their GraphQL type
// annotations, and the field selection tree. Generated per-operation builders
// embed *BaseBuilder and add typed setter methods.
type BaseBuilder struct {
	client    GraphQLClient              // underlying HTTP transport
	opType    string                     // "query" or "mutation"
	opName    string                     // PascalCase operation name used in the query string
	fieldName string                     // the root field name inside the operation
	args      map[string]interface{}     // argument values keyed by GraphQL arg name
	argTypes  map[string]string          // GraphQL type strings keyed by arg name (e.g. "Int!", "[ID!]")
	selection *FieldSelection            // selected fields for the response
}

// NewBaseBuilder creates a new BaseBuilder
func NewBaseBuilder(client GraphQLClient, opType, opName, fieldName string) *BaseBuilder {
	return &BaseBuilder{
		client:    client,
		opType:    opType,
		opName:    opName,
		fieldName: fieldName,
		args:      make(map[string]interface{}),
		argTypes:  make(map[string]string),
		selection: NewFieldSelection(),
	}
}

// SetArg records an argument value and its corresponding GraphQL type string.
// Generated builders call this from their typed setter methods (e.g. SetLimit).
func (b *BaseBuilder) SetArg(name string, value interface{}, graphqlType string) {
	b.args[name] = value
	b.argTypes[name] = graphqlType
}

// GetSelection returns the field selection
func (b *BaseBuilder) GetSelection() *FieldSelection {
	return b.selection
}

// GetClient returns the client
func (b *BaseBuilder) GetClient() GraphQLClient {
	return b.client
}

// BuildQuery assembles the full GraphQL query/mutation string from the
// operation metadata, registered arguments, and field selection tree.
// Variable declarations are sorted alphabetically for deterministic output.
func (b *BaseBuilder) BuildQuery() string {
	var sb strings.Builder

	sb.WriteString(b.opType + " " + b.opName)

	if len(b.args) > 0 {
		sb.WriteString("(")
		vars := make([]string, 0, len(b.args))
		for name, gqlType := range b.argTypes {
			vars = append(vars, "$"+name+": "+gqlType)
		}
		sort.Strings(vars)
		sb.WriteString(strings.Join(vars, ", "))
		sb.WriteString(")")
	}

	sb.WriteString(" {\n")
	sb.WriteString("  " + b.fieldName)

	if len(b.args) > 0 {
		sb.WriteString("(")
		args := make([]string, 0, len(b.args))
		for name := range b.args {
			args = append(args, name+": $"+name)
		}
		sort.Strings(args)
		sb.WriteString(strings.Join(args, ", "))
		sb.WriteString(")")
	}

	selectionStr := b.selection.Build(2)
	if selectionStr != "" {
		sb.WriteString(" {\n")
		sb.WriteString(selectionStr)
		sb.WriteString("  }")
	}

	sb.WriteString("\n}")
	return sb.String()
}

// GetVariables returns the variables map
func (b *BaseBuilder) GetVariables() map[string]interface{} {
	return b.args
}

// ExecuteRaw builds the query, sends it via the GraphQL client, and returns the
// top-level response data as an untyped map. This is used internally by generated
// builders before unmarshalling into the concrete response struct.
func (b *BaseBuilder) ExecuteRaw(ctx context.Context) (map[string]interface{}, error) {
	query := b.BuildQuery()
	variables := b.GetVariables()

	var response map[string]interface{}
	if err := b.client.Execute(ctx, query, variables, &response); err != nil {
		return nil, err
	}

	return response, nil
}

// OpFragment is the slice of an operation produced by a single builder when it
// is being merged into a batched, multi-root document. The TypeScript runtime
// has the same shape — see gqlkit-ts/src/batch.ts.
//
// VarDecls and VarValues use the alias-prefixed variable names (e.g.
// "$open_filter" instead of "$filter") so two builders sharing an argument
// name can coexist in the merged operation.
type OpFragment struct {
	OpType       string
	VarDecls     []string
	VarValues    map[string]interface{}
	AliasedField string
}

// GetOpFragment renders this builder as a fragment that batch.RunQueries /
// batch.RunMutations can splice into a single GraphQL document. The returned
// AliasedField looks like:
//
//	open: todos(filter: $open_filter) {
//	    id
//	    text
//	  }
//
// Argument names are prefixed with the supplied alias to prevent variable
// collisions when merging multiple builders.
func (b *BaseBuilder) GetOpFragment(alias string) OpFragment {
	frag := OpFragment{
		OpType:    b.opType,
		VarValues: make(map[string]interface{}, len(b.args)),
	}

	// Sort argument names for deterministic output across builds
	argNames := make([]string, 0, len(b.args))
	for name := range b.args {
		argNames = append(argNames, name)
	}
	sort.Strings(argNames)

	argPasses := make([]string, 0, len(b.args))
	for _, name := range argNames {
		prefixed := alias + "_" + name
		gqlType := b.argTypes[name]
		frag.VarDecls = append(frag.VarDecls, "$"+prefixed+": "+gqlType)
		argPasses = append(argPasses, name+": $"+prefixed)
		frag.VarValues[prefixed] = b.args[name]
	}

	var sb strings.Builder
	sb.WriteString(alias + ": " + b.fieldName)
	if len(argPasses) > 0 {
		sb.WriteString("(")
		sb.WriteString(strings.Join(argPasses, ", "))
		sb.WriteString(")")
	}

	selectionStr := b.selection.Build(2)
	if selectionStr != "" {
		sb.WriteString(" {\n")
		sb.WriteString(selectionStr)
		sb.WriteString("  }")
	}

	frag.AliasedField = sb.String()
	return frag
}

// QueryMarker is a zero-size embed that tags a generated builder as a
// "query" operation. Embedded into every generated query builder via the
// operation_builder.tmpl template; mutation builders embed MutationMarker.
//
// The marker exists to give the type system something to discriminate on:
// pkg/batch.QueryBatchable requires both GetOpFragment and IsQueryOp, and
// MutationBatchable requires GetOpFragment and IsMutationOp. A query
// builder satisfies the first interface but not the second, so passing a
// mutation builder to batch.RunQueries fails at compile time rather than
// being caught by a runtime op-type check.
//
// We can't gate the interface on op-type alone (every builder embeds the
// same *BaseBuilder), and we can't use generics for this because the input
// is a heterogeneous map. The marker pattern is the cleanest way to lift
// "this is a query" into Go's type system at zero runtime cost.
type QueryMarker struct{}

// IsQueryOp is the marker method that identifies a builder as a query
// operation. It exists solely to satisfy the QueryBatchable interface in
// pkg/batch and has no runtime behaviour.
func (QueryMarker) IsQueryOp() {}

// MutationMarker is the dual of QueryMarker for mutation operations. See
// QueryMarker for the full rationale of the marker pattern.
type MutationMarker struct{}

// IsMutationOp is the marker method that identifies a builder as a mutation
// operation, dual of QueryMarker.IsQueryOp.
func (MutationMarker) IsMutationOp() {}
