package clientgen

import (
	"bytes"
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/ubgo/gqlkit/gqlkit/pkg/writer"

	"github.com/vektah/gqlparser/v2/ast"
)

// ArgumentData describes a single argument on a generated operation builder,
// including its GraphQL name, Go method name, Go type, and the original
// GraphQL type annotation string used in SetArg calls.
//
// MethodName is unique on the builder: an argument whose PascalCase name
// equals a builder method (Select, Execute, BuildQuery, ...; see
// builderReservedNames) or another argument's is suffixed with "_"
// (Select_). ArgName stays the GraphQL name, so the wire is unaffected.
type ArgumentData struct {
	ArgName     string
	MethodName  string
	GoType      string
	GraphQLType string
}

// OperationBuilderData holds all the template data needed to generate a single
// query or mutation builder Go file. Each builder embeds BaseBuilder and adds
// typed argument setters, an optional Select method, and an Execute method.
//
// OpType is the opKind value as a plain string because operation_builder.tmpl
// compares it against the literal "query" to pick the op-kind marker embed.
type OperationBuilderData struct {
	BuilderName string
	// Description is the root field's GraphQL description, rendered as the
	// builder's doc comment; "" falls back to a generated sentence.
	Description  string
	OpType       string
	FieldName    string
	Arguments    []ArgumentData
	HasSelect    bool
	SelectorName string
	ReturnType   string
	ZeroValue    string
}

// opKindSpec carries every per-kind naming decision for one root operation
// kind, so the query and mutation code paths share one implementation and can
// never drift apart.
type opKindSpec struct {
	kind          opKind
	pkg           string // generated sub-package (and directory) name
	rootType      string // the factory struct in <pkg>/root.go
	receiver      string // receiver identifier used on rootType's methods
	builderSuffix string // appended to the PascalCase field name
}

// opKindSpecs is the canonical per-kind naming table. Query builders keep the
// historical bare "Builder" suffix; mutation builders add "Mutation" so a
// query and a mutation with the same field name do not collide when both are
// referenced from one file.
var opKindSpecs = map[opKind]opKindSpec{
	opKindQuery: {
		kind: opKindQuery, pkg: pkgQueries, rootType: "QueryRoot",
		receiver: "q", builderSuffix: "Builder",
	},
	opKindMutation: {
		kind: opKindMutation, pkg: pkgMutations, rootType: "MutationRoot",
		receiver: "m", builderSuffix: "MutationBuilder",
	},
}

// graphQLOpToGoType converts a GraphQL type to the Go type used in operation
// builder files. Unlike graphQLToGoType, this version prefixes object types
// with "types." and input objects with "inputs." since builders live in
// separate packages from the type definitions, and it does NOT force object
// types to pointers (a non-null object return is returned by value).
func (g *Generator) graphQLOpToGoType(t *ast.Type) string {
	if t == nil {
		return goEmptyInterface
	}

	goType := g.resolveOpType(t)

	// Nullable becomes a pointer, except for slices and pointer bindings:
	// nil already represents null. Deliberately decided on the rendered text, not on
	// t.Elem, so a binding whose Go type is itself a slice ([]byte) also
	// stays unpointered — changing that would alter the public type of
	// existing generated SDKs. Go type syntax is prefix-only, so prepending
	// "*" or "[]" can never re-associate (unlike TS's `A | B[]`).
	if !t.NonNull && !isNilableGoType(goType) {
		goType = "*" + goType
	}

	return goType
}

// resolveOpType resolves the base Go type for operation builders.
func (g *Generator) resolveOpType(t *ast.Type) string {
	if t.Elem != nil {
		return "[]" + g.graphQLOpToGoType(t.Elem)
	}

	name := t.NamedType
	if pkg := g.opLocalPackage(name); pkg != "" {
		return pkg + "." + g.namedTypeToGo(name)
	}
	return g.namedTypeToGo(name)
}

// opLocalPackage returns the generated sub-package (pkgTypes / pkgInputs) a
// named type is qualified with inside queries/ and mutations/ (objects and
// abstract-type wrappers → types, input objects → inputs), or "" when it needs
// no local qualifier (scalars, enums, bindings, unknown names).
//
// It is the single source of truth for both the rendered type text
// (resolveOpType) and the import set (opLocalImports), so an import is added
// exactly when a reference is emitted — never by substring-matching the type
// text, which mistook bindings such as "dbtypes.Money" for the types package.
func (g *Generator) opLocalPackage(name string) string {
	def := g.schema.Types[name]
	if def == nil {
		return ""
	}
	switch def.Kind {
	case ast.Object, ast.Interface, ast.Union:
		return pkgTypes
	case ast.InputObject:
		return pkgInputs
	}
	return ""
}

// opLocalImports adds to set the sub-package suffixes (pkgTypes, pkgInputs)
// referenced by the Go rendering of t.
func (g *Generator) opLocalImports(t *ast.Type, set map[string]bool) {
	if pkg := g.opLocalPackage(getBaseTypeName(t)); pkg != "" {
		set[pkg] = true
	}
}

// formatGraphQLType reconstructs the GraphQL type notation string from an AST
// type, e.g. "[AiModelOrder!]" or "Int!". This string is passed to SetArg so
// the builder can include proper variable type declarations in the query.
func formatGraphQLType(t *ast.Type) string {
	if t == nil {
		return ""
	}

	var result string
	if t.Elem != nil {
		result = "[" + formatGraphQLType(t.Elem) + "]"
	} else {
		result = t.NamedType
	}

	if t.NonNull {
		result += "!"
	}

	return result
}

// getZeroValue returns the Go zero-value expression for goType, used in the
// generated Execute method's error return.
//
// isStruct must be true only when goType is known to be a struct (a generated
// object or input object, by value): only then is the composite literal
// `T{}` valid. Every other non-basic named type — enums (`type X string`),
// interface types, `any` scalar aliases, and bindings whose kind is unknown
// (json.Number, int16, a named slice) — gets `*new(T)`, which is the zero
// value of any Go type and always compiles.
func getZeroValue(goType string, isStruct bool) string {
	if strings.HasPrefix(goType, "*") ||
		strings.HasPrefix(goType, "[]") ||
		strings.HasPrefix(goType, "map[") ||
		goType == goEmptyInterface ||
		goType == "any" {
		return "nil"
	}

	if obj := types.Universe.Lookup(goType); obj != nil {
		if basic, ok := obj.Type().(*types.Basic); ok {
			switch {
			case basic.Info()&types.IsString != 0:
				return `""`
			case basic.Info()&types.IsBoolean != 0:
				return "false"
			case basic.Info()&types.IsNumeric != 0:
				return "0"
			}
		}
	}

	if isStruct {
		return goType + "{}"
	}
	return "*new(" + goType + ")"
}

// buildOperationBuilderData builds template data for a single query/mutation field.
//
// methodName is the field's unique factory-method name (see rootGoNames);
// the builder type is methodName + the kind's suffix.
func (g *Generator) buildOperationBuilderData(spec opKindSpec, field *ast.FieldDefinition, methodName string) OperationBuilderData {
	returnTypeName := getBaseTypeName(field.Type)
	returnType := g.graphQLOpToGoType(field.Type)
	isObjReturn := isObjectType(g.schema, field.Type)
	// A by-value struct return only happens for a non-null, non-list
	// object; isStructKind already excludes bound and interface types.
	isStructReturn := field.Type.Elem == nil && g.isStructKind(returnTypeName)

	data := OperationBuilderData{
		BuilderName: methodName + spec.builderSuffix,
		Description: field.Description,
		OpType:      string(spec.kind),
		FieldName:   field.Name,
		Arguments:   make([]ArgumentData, 0, len(field.Arguments)),
		HasSelect:   isObjReturn,
		ReturnType:  returnType,
		ZeroValue:   getZeroValue(returnType, isStructReturn),
	}

	if isObjReturn {
		data.SelectorName = exportName(returnTypeName) + "Fields"
	}

	setters := argGoNames(field.Arguments)
	for _, arg := range field.Arguments {
		data.Arguments = append(data.Arguments, ArgumentData{
			ArgName:     arg.Name,
			MethodName:  setters[arg.Name],
			GoType:      g.graphQLOpToGoType(arg.Type),
			GraphQLType: formatGraphQLType(arg.Type),
		})
	}

	return data
}

// collectOperationImports returns the sorted, de-duplicated import paths one
// generated operation file needs: context, the SDK's builder / types /
// inputs / fields packages as referenced, and any scalar, enum or binding
// packages reached through the return and argument types.
func (g *Generator) collectOperationImports(field *ast.FieldDefinition, data OperationBuilderData) []string {
	importSet := map[string]bool{"context": true}

	rootPkg := strings.TrimSuffix(g.config.Package, "/")
	importSet[rootPkg+"/"+pkgBuilder] = true

	local := map[string]bool{}
	g.opLocalImports(field.Type, local)
	for _, arg := range field.Arguments {
		g.opLocalImports(arg.Type, local)
	}
	// The fields package is only referenced by the Select method.
	if data.HasSelect {
		local[pkgFields] = true
	}
	for pkg := range local {
		importSet[rootPkg+"/"+pkg] = true
	}

	g.checkTypeForImports(field.Type, importSet)
	for _, arg := range field.Arguments {
		g.checkTypeForImports(arg.Type, importSet)
	}

	imports := make([]string, 0, len(importSet))
	for imp := range importSet {
		// Defensive: an empty path would render `import ""`.
		if imp == "" {
			continue
		}
		imports = append(imports, imp)
	}
	sort.Strings(imports)
	return imports
}

// generateOperationFiles generates per-field builder files for all queries and
// mutations, plus the root.go entrypoint files that provide factory methods.
// Subscriptions are not generated.
func (g *Generator) generateOperationFiles() error {
	roots := []struct {
		def  *ast.Definition
		kind opKind
	}{
		{g.schema.Query, opKindQuery},
		{g.schema.Mutation, opKindMutation},
	}
	for _, r := range roots {
		if r.def == nil {
			continue
		}
		spec := opKindSpecs[r.kind]
		if err := g.generateOperationFilesForType(spec, r.def); err != nil {
			return err
		}
		if err := g.generateRootFile(spec, r.def); err != nil {
			return err
		}
	}
	return nil
}

// generateOperationFilesForType emits one Go builder file per field on the
// given root type (Query or Mutation). Each file contains a builder struct,
// argument setters, optional Select method, and an Execute method.
func (g *Generator) generateOperationFilesForType(spec opKindSpec, def *ast.Definition) error {
	if def == nil {
		return nil
	}

	names := rootGoNames(def)
	for _, field := range def.Fields {
		if skipGenField(field.Name) {
			continue
		}

		data := g.buildOperationBuilderData(spec, field, names[field.Name].Method)
		imports := g.collectOperationImports(field, data)

		var buf bytes.Buffer
		if err := g.templates.ExecuteTemplate(&buf, "operation_builder", data); err != nil {
			return fmt.Errorf("operation template for %s %s: %w", spec.kind, field.Name, err)
		}

		var file bytes.Buffer
		file.WriteString(writer.GeneratedHeader)
		fmt.Fprintf(&file, "package %s\n\n", spec.pkg)
		file.WriteString("import (\n")
		for _, imp := range imports {
			fmt.Fprintf(&file, "\t%q\n", imp)
		}
		file.WriteString(")\n\n")
		file.Write(buf.Bytes())

		filename := fmt.Sprintf("%s/%s_%s.go", spec.pkg, spec.kind, names[field.Name].FileStem)
		if err := g.writer.WriteFile(filename, file.String()); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
	}

	return nil
}

// generateRootFile generates <pkg>/root.go (queries/root.go or
// mutations/root.go): the QueryRoot / MutationRoot struct with one factory
// method per root field, each returning a fresh builder bound to the client.
func (g *Generator) generateRootFile(spec opKindSpec, def *ast.Definition) error {
	rootPkg := strings.TrimSuffix(g.config.Package, "/")

	var sb strings.Builder
	sb.WriteString(writer.GeneratedHeader)
	fmt.Fprintf(&sb, "package %s\n\n", spec.pkg)
	fmt.Fprintf(&sb, "import %q\n\n", rootPkg+"/"+pkgBuilder)

	fmt.Fprintf(&sb, "// %s is the entry point for %s\n", spec.rootType, spec.pkg)
	fmt.Fprintf(&sb, "type %s struct {\n", spec.rootType)
	sb.WriteString("\tclient builder.GraphQLClient\n")
	sb.WriteString("}\n\n")

	fmt.Fprintf(&sb, "// New%s creates a new %s\n", spec.rootType, spec.rootType)
	fmt.Fprintf(&sb, "func New%s(client builder.GraphQLClient) *%s {\n", spec.rootType, spec.rootType)
	fmt.Fprintf(&sb, "\treturn &%s{client: client}\n", spec.rootType)
	sb.WriteString("}\n")

	names := rootGoNames(def)
	for _, field := range def.Fields {
		if skipGenField(field.Name) {
			continue
		}

		methodName := names[field.Name].Method
		builderName := methodName + spec.builderSuffix

		sb.WriteString("\n")
		fmt.Fprintf(&sb, "// %s creates a new %s\n", methodName, builderName)
		fmt.Fprintf(&sb, "func (%s *%s) %s() *%s {\n", spec.receiver, spec.rootType, methodName, builderName)
		fmt.Fprintf(&sb, "\treturn &%s{\n", builderName)
		fmt.Fprintf(&sb, "\t\tBaseBuilder: builder.NewBaseBuilder(%s.client, %q, %q, %q),\n", spec.receiver, string(spec.kind), methodName, field.Name)
		sb.WriteString("\t}\n")
		sb.WriteString("}\n")
	}

	return g.writer.WriteFile(spec.pkg+"/root.go", sb.String())
}
