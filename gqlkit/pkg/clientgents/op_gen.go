package clientgents

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/ubgo/gqlkit/gqlkit/pkg/util"
	"github.com/vektah/gqlparser/v2/ast"
)

// skipGenField reports whether a field should be omitted from TypeScript
// generation: introspection meta-fields (__schema / __type / __typename) whose
// __* types are not generated, and placeholder fields whose name yields no
// usable identifier. GraphQL allows `_: Int` (a federation "empty type"
// placeholder), and ToPascalCase("_") == "", which would otherwise produce an
// empty operation name and a generic `Builder` class that collides across
// multiple such fields. Mirrors clientgen.skipGenField on the Go side.
func skipGenField(name string) bool {
	return strings.HasPrefix(name, introspectionPrefix) || util.ToPascalCase(name) == ""
}

// TSArgumentData describes a single argument on a TypeScript operation builder,
// including the original GraphQL name, the TS method name, and the TS/GraphQL
// type strings.
type TSArgumentData struct {
	ArgName     string // original GraphQL arg name
	MethodName  string // setter name: the arg name, suffixed only if it clashes with a builder member
	TSType      string // TypeScript type for the arg
	GraphQLType string // e.g., "Cursor", "Int", "[ChatbotOrder!]"
	// EncodeArg is appended to the setArg call when the arg's values need
	// encoding: ", { scalar: encodeDateTime, listDepth: 0 }". Empty otherwise.
	EncodeArg string
}

// TSOperationBuilderData holds all the template data for generating a single
// TypeScript query or mutation builder class. Each builder wraps BaseBuilder
// and adds typed argument setters, an optional select method, and an execute method.
type TSOperationBuilderData struct {
	BuilderName string
	OpType      opKind // written verbatim into the BaseBuilder call
	FieldName   string // original GraphQL field name
	MethodName  string // PascalCase operation name
	Arguments   []TSArgumentData
	HasSelect   bool // true for composite returns (object, interface, union), which need a selection set
	IsAbstract  bool // interface/union return: the builder requests __typename up front
	// NeedsSelect marks an object (not abstract) return: TResult starts as
	// SelectionRequired and execute/executeRaw/getOpFragment are typed to be
	// uncallable until select(), with a runtime guard behind it for batch()
	// and plain-JS callers. An abstract return is valid unselected ({ __typename }).
	NeedsSelect bool
	// Decodes routes execute()'s result through decodeResult: a select()
	// return that can reach a decoder, or a scalar return with one.
	Decodes bool
	// ResultDecoder is "decodeDateTime, 0" for a scalar return whose scalar
	// has a decoder (passed to setResultDecoder); empty otherwise.
	ResultDecoder  string
	SelectorName   string // e.g., "ChatbotConnectionFields"; empty when HasSelect is false
	ReturnType     string // TypeScript return type of execute() when HasSelect is false
	DefaultGeneric string // TResult before select(): the full type, e.g. "Todo | null", "(Todo | null)[]"
	SelectWrapper  string // TResult after select(): the same structure around T, e.g. "T[] | null"
}

// TSOperationImport describes a single import statement needed in an operation
// builder file, with optional type-only flag for TS "import type" syntax.
type TSOperationImport struct {
	Names    []string
	Path     string
	TypeOnly bool
}

// TSOperationTemplateData wraps the builder data and its imports for passing
// to the ts_operation.tmpl template.
type TSOperationTemplateData struct {
	Data    TSOperationBuilderData
	Imports []TSOperationImport
}

// TSOperationRootBuilder holds the data for one builder factory method in a
// root file (QueryRoot or MutationRoot).
type TSOperationRootBuilder struct {
	BuilderName string
	FieldName   string // GraphQL field name
	MethodName  string // factory method: the field name, suffixed only on a root-member clash
	FileName    string
}

// TSOperationRootData is the template data for generating queries/root.ts or
// mutations/root.ts, which contain the root class with factory methods.
type TSOperationRootData struct {
	ClassName string
	OpType    opKind
	Builders  []TSOperationRootBuilder
}

// TSOperationIndexData is the template data for generating the barrel
// index.ts file that re-exports the root class and all individual builders.
type TSOperationIndexData struct {
	RootClassName string
	Builders      []TSOperationRootBuilder
}

// buildOperationBuilderData builds template data for a single query/mutation field.
func (g *Generator) buildOperationBuilderData(kind opKind, root *ast.Definition, field *ast.FieldDefinition) TSOperationBuilderData {
	isObjReturn := isCompositeType(g.schema, field.Type)
	methodName := util.ToPascalCase(field.Name)

	outerNull := g.outputNullSuffix(root.Name, field.Name)
	returnType := g.fieldTypeFor(root, field)
	defaultGeneric, selectWrapper := "", ""
	isAbstract := false
	if isObjReturn {
		// Before select(), an object return is typed as the full object and
		// an abstract one as its { __typename } variants — the only field an
		// unselected abstract operation fetches.
		leaf := getBaseTypeName(field.Type)
		if def := g.schema.Types[leaf]; isAbstractKind(def.Kind) {
			isAbstract = true
			leaf = g.variantsDefault(def)
		}
		defaultGeneric = outputTSType(field.Type, leaf, outerNull)
		if !isAbstract {
			defaultGeneric = tsSelectionRequired
		}
		selectWrapper = outputTSType(field.Type, selectGeneric, outerNull)
	}

	data := TSOperationBuilderData{
		BuilderName:    methodName + opKindSpecs[kind].builderSuffix,
		OpType:         kind,
		FieldName:      field.Name,
		MethodName:     methodName,
		Arguments:      make([]TSArgumentData, 0, len(field.Arguments)),
		HasSelect:      isObjReturn,
		IsAbstract:     isAbstract,
		NeedsSelect:    isObjReturn && !isAbstract,
		ReturnType:     returnType,
		DefaultGeneric: defaultGeneric,
		SelectWrapper:  selectWrapper,
	}

	if isObjReturn {
		data.SelectorName = getBaseTypeName(field.Type) + selectorSuffix
	}

	setters := newMemberNamer(builderMembers)
	base := getBaseTypeName(field.Type)
	if isObjReturn {
		data.Decodes = g.reachesDecoder(base)
	} else if dec := g.decoderName(base); dec != "" {
		data.Decodes = true
		data.ResultDecoder = fmt.Sprintf("%s, %d", dec, listDepth(field.Type))
	}

	for _, arg := range field.Arguments {
		a := TSArgumentData{
			ArgName:     arg.Name,
			MethodName:  setters.name(arg.Name),
			TSType:      g.fieldTSType(arg.Type),
			GraphQLType: formatGraphQLType(arg.Type),
		}
		if expr := g.argEncoderExpr(arg.Type); expr != "" {
			a.EncodeArg = ", " + expr
		}
		data.Arguments = append(data.Arguments, a)
	}

	return data
}

// generateOperationFiles generates per-field builders plus the root and
// barrel files for every operation root the schema declares. A schema with
// no mutation root produces no mutations/ directory at all.
func (g *Generator) generateOperationFiles() error {
	roots := []struct {
		kind opKind
		def  *ast.Definition
	}{
		{opQuery, g.schema.Query},
		{opMutation, g.schema.Mutation},
	}
	for _, r := range roots {
		if r.def == nil {
			continue
		}
		if err := g.generateOperationKind(r.kind, r.def); err != nil {
			return err
		}
	}
	return nil
}

// generateOperationKind writes <dir>/<field>.ts for every field of one
// operation root, then <dir>/root.ts and the <dir>/index.ts barrel.
func (g *Generator) generateOperationKind(kind opKind, def *ast.Definition) error {
	spec := opKindSpecs[kind]

	for _, field := range def.Fields {
		if skipGenField(field.Name) {
			continue
		}

		data := g.buildOperationBuilderData(kind, def, field)
		tmplData := TSOperationTemplateData{
			Data:    data,
			Imports: g.collectOperationImports(data, field),
		}

		var buf bytes.Buffer
		if err := g.templates.ExecuteTemplate(&buf, "ts_operation.tmpl", tmplData); err != nil {
			return fmt.Errorf("failed to execute operation template for %s: %w", field.Name, err)
		}

		rel := spec.dir + "/" + toKebabCase(field.Name) + tsExt
		if err := g.writer.WriteFile(rel, buf.String()); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}

	builders := g.buildOperationRootBuilders(kind, def)

	var root bytes.Buffer
	rootData := TSOperationRootData{ClassName: spec.rootClass, OpType: kind, Builders: builders}
	if err := g.templates.ExecuteTemplate(&root, "ts_operation_root.tmpl", rootData); err != nil {
		return fmt.Errorf("failed to execute %s root template: %w", kind, err)
	}
	if err := g.writer.WriteFile(spec.dir+"/"+rootFile, root.String()); err != nil {
		return err
	}

	var index bytes.Buffer
	indexData := TSOperationIndexData{RootClassName: spec.rootClass, Builders: builders}
	if err := g.templates.ExecuteTemplate(&index, "ts_operation_index.tmpl", indexData); err != nil {
		return fmt.Errorf("failed to execute %s index template: %w", kind, err)
	}
	return g.writer.WriteFile(spec.dir+"/"+indexFile, index.String())
}

// collectOperationImports gathers all TypeScript import statements needed for
// a single operation builder file, including builder base, field selectors,
// scalars, enums, and input types.
func (g *Generator) collectOperationImports(data TSOperationBuilderData, field *ast.FieldDefinition) []TSOperationImport {
	builderValues := []string{tsBaseBuilder, tsGraphQLClient}
	if data.Decodes {
		builderValues = append(builderValues, tsGraphQLErrors)
	}
	imports := []TSOperationImport{{
		Names: builderValues,
		Path:  importBuilder,
	}}

	builderTypes := []string{tsRequestOptions}
	if data.NeedsSelect {
		builderTypes = append(builderTypes, tsSelectionRequired)
	}
	imports = append(imports, TSOperationImport{
		Names:    builderTypes,
		Path:     importBuilder,
		TypeOnly: true,
	})

	if data.HasSelect {
		imports = append(imports, TSOperationImport{
			Names: []string{data.SelectorName},
			Path:  importFields,
		})
	}

	scalarImports := make(map[string]bool)
	enumImports := make(map[string]bool)
	inputImports := make(map[string]bool)

	g.classifyTypeImport(field.Type, scalarImports, enumImports, inputImports)
	for _, arg := range field.Arguments {
		g.classifyTypeImport(arg.Type, scalarImports, enumImports, inputImports)
	}

	// Codec values (decoders/encoders from scalars, plans from inputs) are
	// real imports, after the type-only ones; none exist without a codec.
	codecScalars, codecPlans := make(map[string]bool), make(map[string]bool)
	if data.ResultDecoder != "" {
		codecScalars[g.decoderName(getBaseTypeName(field.Type))] = true
	}
	argTypes := make([]*ast.Type, 0, len(field.Arguments))
	for _, arg := range field.Arguments {
		argTypes = append(argTypes, arg.Type)
	}
	g.codecImportsFor(argTypes, codecScalars, codecPlans)

	// Order is part of the generated output: scalars, enums, inputs.
	for _, bucket := range []struct {
		names map[string]bool
		path  string
	}{
		{scalarImports, importScalars},
		{enumImports, importEnums},
		{inputImports, importInputs},
	} {
		if len(bucket.names) > 0 {
			imports = append(imports, TSOperationImport{
				Names:    sortedKeys(bucket.names),
				Path:     bucket.path,
				TypeOnly: true,
			})
		}
	}

	for _, bucket := range []struct {
		names map[string]bool
		path  string
	}{
		{codecScalars, importScalars},
		{codecPlans, importInputs},
	} {
		if len(bucket.names) > 0 {
			imports = append(imports, TSOperationImport{Names: sortedKeys(bucket.names), Path: bucket.path})
		}
	}

	return imports
}

// classifyTypeImport recursively walks a GraphQL type and sorts each referenced
// named type into the correct import bucket (scalars, enums, or inputs).
//
// Invariant: a name lands in a bucket exactly when fieldTSType renders it as
// a bare reference to that module's export — otherwise the operation file
// gets an unused import (a hard error under noUnusedLocals) or a missing one.
func (g *Generator) classifyTypeImport(t *ast.Type, scalars, enums, inputs map[string]bool) {
	if t == nil {
		return
	}
	if t.Elem != nil {
		g.classifyTypeImport(t.Elem, scalars, enums, inputs)
		return
	}

	name := t.NamedType
	def := g.schema.Types[name]
	if def == nil {
		return
	}

	switch def.Kind {
	case ast.Object, ast.Interface, ast.Union:
		// Never imported: a composite type only appears as an operation's
		// return, and builders type that through SelectionRequired, the
		// { __typename } variants and select()'s generic — never by name.
	case ast.Scalar:
		if tsType, ok := g.tsTypeMap[name]; ok && isSimpleTSType(tsType) {
			return
		}
		scalars[scalarAliasName(name)] = true
	case ast.Enum:
		enums[name] = true
	case ast.InputObject:
		inputs[name] = true
	}
}

// buildOperationRootBuilders builds the list of builder entries for the root
// and index files, one per generated field on the operation root.
func (g *Generator) buildOperationRootBuilders(kind opKind, def *ast.Definition) []TSOperationRootBuilder {
	var builders []TSOperationRootBuilder
	suffix := opKindSpecs[kind].builderSuffix
	methods := newMemberNamer(rootMembers)

	for _, field := range def.Fields {
		if skipGenField(field.Name) {
			continue
		}
		builders = append(builders, TSOperationRootBuilder{
			BuilderName: util.ToPascalCase(field.Name) + suffix,
			FieldName:   field.Name,
			MethodName:  methods.name(field.Name),
			FileName:    toKebabCase(field.Name),
		})
	}
	return builders
}

// sortedKeys returns the keys of a map[string]bool in alphabetical order.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
