package clientgen

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ubgo/gqlkit/gqlkit/pkg/util"

	"github.com/vektah/gqlparser/v2/ast"
)

// exportName upper-cases the first letter of a GraphQL type name so the
// generated Go type identifier is EXPORTED and therefore reachable from the
// other generated packages (queries, inputs, fields, …). GraphQL type names are
// not required to be capitalized — Hasura emits scalars like `timestamptz` /
// `uuid` and enums like `order_by` — and an unexported alias (`type timestamptz
// = any`) referenced cross-package as `scalars.timestamptz` fails to compile
// with "undefined". Only the first rune is touched, so already-exported names
// (JSON, DateTime, ID, URL) are returned unchanged and existing SDKs keep their
// identifiers; only lowercase-led names are lifted. Every definition site and
// every reference site must route through this, or the two drift apart.
func exportName(name string) string {
	if name == "" {
		return name
	}
	// Drop leading underscores so Apollo Federation types (_Service, _Entity,
	// _Any, _FieldSet) and any other underscore-led name become exported rather
	// than staying unexported (unreachable cross-package). ToUpper('_') == '_',
	// so a bare first-letter uppercase wouldn't fix these.
	trimmed := strings.TrimLeft(name, "_")
	if trimmed == "" {
		// All underscores — nothing to export; leave as-is (such fields/types
		// are dropped by the __/placeholder skips before this is rendered).
		return name
	}
	r := []rune(trimmed)
	if unicode.IsDigit(r[0]) {
		// Can't start a Go identifier with a digit; prefix deterministically.
		return "X" + trimmed
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// skipGenField reports whether a field should be omitted from generation. Two
// cases: introspection meta-fields (__schema / __type / __typename, injected by
// gqlparser) whose __* types we don't generate, and placeholder fields whose
// name produces no valid Go identifier — GraphQL allows `_: Int` (a federation
// "empty type" placeholder), and ToPascalCase("_") == "", which would emit a
// method/field with no name (`func (q *QueryRoot) () *Builder`).
func skipGenField(name string) bool {
	return strings.HasPrefix(name, "__") || util.ToPascalCase(name) == ""
}

// TypeDef holds the metadata for a single Go declaration generated from a
// GraphQL object, interface or input object type. It is the template data
// element of both types.tmpl and inputs.tmpl.
//
// Shapes by IsInterface: true → an abstract type (GraphQL interface or, with
// IsUnion, union): a Go interface with an Is<Name>() marker, plus the
// <Name>Value decoding wrapper over PossibleTypes; no Fields. false → a
// struct with Fields, plus an Is<X>() method per entry of Implements. See
// abstract.go for the full abstract-type contract.
type TypeDef struct {
	Name        string
	Description string
	Fields      []FieldDef
	IsInterface bool
	// IsUnion distinguishes a union from an interface (only the generated
	// comment differs).
	IsUnion bool
	// ValueName is the decoding wrapper's Go name (abstract types only).
	ValueName string
	// PossibleTypes are the concrete members (abstract types only).
	PossibleTypes []PossibleType
	// Implements lists the abstract Go types this object belongs to.
	Implements []string
	// NeedsWireMarshal marks an input type with at least one field whose
	// WireTag differs from its JSONTag, so it gets a go1.24 MarshalJSON.
	NeedsWireMarshal bool
}

// FieldDef holds the metadata for a single field within a generated Go struct.
// JSONTag is the complete backquoted struct tag; OmitEmpty reports whether it
// carries an omit option (omitempty or omitzero, see buildFieldDefs).
type FieldDef struct {
	Name        string
	Description string
	GoType      string
	JSONTag     string
	OmitEmpty   bool
	// WireTag is the tag used by the go1.24 MarshalJSON of input types (see
	// inputs_json.tmpl); it differs from JSONTag only for pointer-mode slice
	// fields (omitzero there, omitempty in JSONTag).
	WireTag string
}

// graphQLToGoType converts a GraphQL AST type to its Go equivalent string for
// struct fields in the types/ and inputs/ packages. It handles lists ([]T),
// non-null vs nullable (*T for pointers), and delegates named type resolution
// to namedTypeToGo.
func (g *Generator) graphQLToGoType(t *ast.Type) string {
	if t == nil {
		return goEmptyInterface
	}

	goType := g.resolveType(t)

	// Slice element pointering is handled in the recursive graphQLToGoType call
	// on t.Elem, so a []T is returned as-is here. A binding whose Go type is
	// itself a slice ([]byte) or pointer (*big.Int) is also left as-is: nil
	// already models null, and **T would be useless. Go type syntax is prefix-only, so prepending "*" / "[]"
	// can never re-associate the way TS's `A | B[]` does.
	if isNilableGoType(goType) {
		return goType
	}

	// Object / input-object fields are ALWAYS pointers, regardless of
	// nullability. Go forbids value-type cycles (a struct that contains itself
	// directly or transitively), which GraphQL object graphs routinely have —
	// e.g. Shopify's ProductVariant -> ...ContextualPricing -> QuantityRule ->
	// ProductVariant. A pointer breaks the cycle and also naturally models a
	// nullable object. Slices are already reference types, so []T cycles are
	// legal and left by value above.
	if g.isStructKind(t.NamedType) {
		return "*" + goType
	}

	// Nullable non-object fields become pointers to distinguish null from zero.
	if !t.NonNull {
		goType = "*" + goType
	}

	return goType
}

// isNilableGoType reports whether rendered Go type text is already nil-able
// by construction (a slice or a pointer), so nullability needs no extra "*".
func isNilableGoType(goType string) bool {
	return strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "*")
}

// isStructKind reports whether the named type is generated as a Go struct
// (a GraphQL object or input object). Fields of such types must be emitted as
// pointers to break value-type cycles — see graphQLToGoType. Custom-bound types
// (scalars mapped to time.Time, json.RawMessage, etc.) and everything else are
// not generated structs.
func (g *Generator) isStructKind(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := g.clientConfig.Bindings[name]; ok {
		return false
	}
	def := g.schema.Types[name]
	if def == nil {
		return false
	}
	// Abstract types render as their <Name>Value wrapper struct.
	return def.Kind == ast.Object || def.Kind == ast.InputObject || isAbstractKind(def)
}

// resolveType resolves the base Go type from a GraphQL type, recursing into
// list element types.
func (g *Generator) resolveType(t *ast.Type) string {
	if t.Elem != nil {
		return "[]" + g.graphQLToGoType(t.Elem)
	}
	return g.namedTypeToGo(t.NamedType)
}

// namedTypeToGo converts a named GraphQL type to its Go representation as
// seen from the types/ and inputs/ packages.
//
// Resolution order: a binding (built-in or config.jsonc) wins; then the schema
// kind decides — scalars and enums are qualified with their generated package,
// objects and input objects are same-package names, and interfaces and unions
// are their <Name>Value decoding wrapper (see abstract.go); a name absent from the schema (only reachable by a caller bug, since
// gqlparser validates every reference) maps to interface{} too rather than
// panicking.
func (g *Generator) namedTypeToGo(name string) string {
	if entry, ok := g.clientConfig.Bindings[name]; ok {
		return entry.GoType
	}

	def := g.schema.Types[name]
	if def == nil {
		return goEmptyInterface
	}

	switch def.Kind {
	case ast.Scalar:
		return pkgScalars + "." + exportName(def.Name)
	case ast.Enum:
		return pkgEnums + "." + exportName(def.Name)
	case ast.Object, ast.InputObject:
		return exportName(def.Name)
	case ast.Interface, ast.Union:
		return abstractValueName(def.Name)
	}

	return goEmptyInterface
}

// isGeneratedDataType reports whether a schema definition gets a data struct
// (or interface marker) in types/: user-defined objects and interfaces other
// than the conventional root operation types and the __* introspection types.
//
// Why the roots are skipped: they are represented by the queries/ and
// mutations/ builder packages. A non-conventionally named root (Shopify's
// "QueryRoot") is NOT skipped, because other types can reference it by value
// (Job.query: QueryRoot); its injected __schema / __type meta-fields are
// dropped by skipGenField instead. generateTypes, collectTypeImports and
// getSortedObjectTypeNames all route through this so the rendered types and
// their imports can never disagree.
func isGeneratedDataType(def *ast.Definition) bool {
	if def.BuiltIn || strings.HasPrefix(def.Name, "__") {
		return false
	}
	if def.Kind != ast.Object && !isAbstractKind(def) {
		return false
	}
	return def.Kind != ast.Object || !isConventionalRootName(def.Name)
}

// JSON tag options for generated struct fields.
const (
	// tagOmitEmpty drops nil pointers — and, for slices, empty ones too.
	tagOmitEmpty = "omitempty"
	// tagOmitZero (encoding/json, Go 1.24+) drops only the zero value: a nil
	// slice or an unset Opt, but NOT a non-nil empty slice, so [] can be sent.
	tagOmitZero = "omitzero"
)

// buildFieldDefs converts a definition's fields into struct-field metadata
// for the types/ and inputs/ templates, skipping meta and placeholder fields
// (see skipGenField).
//
// Tag and type per field (the wire contract of the generated SDK):
//   - non-null: plain tag, always sent.
//   - nullable object field (types/): omitempty — output only, never marshalled
//     by the SDK itself.
//   - nullable input field, OptionalOpt: Opt[T] with omitzero — unset is
//     omitted, Null[T]() sends null, Some(v) sends v.
//   - nullable input field, OptionalPointer, slice-typed (a list, or a
//     binding such as []byte): omitempty in the struct, omitzero in WireTag.
//     The struct tag must stay omitempty: a toolchain older than Go 1.24
//     ignores omitzero, so a nil slice would be sent as null and CLEAR the
//     server value. The go1.24-only MarshalJSON (inputs_json.tmpl) applies
//     WireTag, so on 1.24+ an empty non-nil slice is sent as [] while nil is
//     still omitted; older toolchains keep the previous (lossless) behaviour.
//   - nullable input field, OptionalPointer, otherwise: *T with omitempty.
func (g *Generator) buildFieldDefs(def *ast.Definition) []FieldDef {
	isInput := def.Kind == ast.InputObject
	// Methods the generator puts on this struct; fields must not shadow them.
	var reserved []string
	if isInput {
		reserved = inputReservedNames
	} else {
		// Object structs carry an Is<X>() marker per abstract membership.
		for _, x := range g.abstractMemberships(def) {
			reserved = append(reserved, "Is"+x)
		}
	}
	goNames := fieldGoNames(def.Fields, reserved...)
	var defs []FieldDef
	for _, field := range def.Fields {
		if skipGenField(field.Name) {
			continue
		}
		nullable := !field.Type.NonNull
		goType := g.graphQLToGoType(field.Type)
		omit, wireOmit := "", ""
		if nullable {
			omit = tagOmitEmpty
			if isInput {
				switch g.optionalModeFor(def.Name, field.Name) {
				case OptionalOpt:
					goType = "Opt[" + g.optInnerGoType(field.Type) + "]"
					omit = tagOmitZero // opt.go is go1.24-only, so this is safe
				case OptionalPointer:
					if strings.HasPrefix(goType, "[]") {
						wireOmit = tagOmitZero
					}
				}
			}
		}
		if wireOmit == "" {
			wireOmit = omit
		}
		defs = append(defs, FieldDef{
			Name:        goNames[field.Name],
			Description: field.Description,
			GoType:      goType,
			JSONTag:     jsonTagFor(field.Name, omit),
			OmitEmpty:   omit != "",
			WireTag:     jsonTagFor(field.Name, wireOmit),
		})
	}
	return defs
}

// jsonTagFor renders a backquoted json struct tag; GraphQL field names are
// the wire keys verbatim.
func jsonTagFor(name, omit string) string {
	if omit == "" {
		return fmt.Sprintf("`json:\"%s\"`", name)
	}
	return fmt.Sprintf("`json:\"%s,%s\"`", name, omit)
}

// sortedTypeDefs flattens a name-keyed TypeDef map into a slice sorted by Go
// name, so generated files are byte-identical across runs.
func sortedTypeDefs(m map[string]TypeDef) []TypeDef {
	list := make([]TypeDef, 0, len(m))
	for _, td := range m {
		list = append(list, td)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

// sortedImports flattens an import set into a sorted slice.
func sortedImports(set map[string]bool) []string {
	list := make([]string, 0, len(set))
	for imp := range set {
		list = append(list, imp)
	}
	sort.Strings(list)
	return list
}

// generateTypes collects all non-built-in object and interface types from the
// schema (excluding the conventional roots, see isGeneratedDataType), converts
// them to TypeDef structs with Go field metadata, and writes types/types.go via
// the types template. Interfaces get only a marker (no fields).
func (g *Generator) generateTypes() error {
	if err := g.checkAbstractNameCollisions(); err != nil {
		return err
	}
	typeDefMap := make(map[string]TypeDef)
	hasAbstract := false

	for _, def := range g.schema.Types {
		if !isGeneratedDataType(def) {
			continue
		}
		typeDef := TypeDef{
			Name:        exportName(def.Name),
			Description: def.Description,
			IsInterface: isAbstractKind(def),
			IsUnion:     def.Kind == ast.Union,
		}
		if isAbstractKind(def) {
			typeDef.ValueName = abstractValueName(def.Name)
			typeDef.PossibleTypes = g.possibleTypes(def)
			hasAbstract = true
		} else {
			typeDef.Fields = g.buildFieldDefs(def)
			typeDef.Implements = g.abstractMemberships(def)
		}
		typeDefMap[def.Name] = typeDef
	}

	imports := g.collectTypeImports()
	// The <Name>Value decoders use encoding/json.
	if hasAbstract && !slices.Contains(imports, "encoding/json") {
		imports = append(imports, "encoding/json")
		sort.Strings(imports)
	}

	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "types", typesTemplateData{
		headerData: headerData{Package: pkgTypes},
		Types:      sortedTypeDefs(typeDefMap),
		Imports:    imports,
	})
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return g.writer.WriteFile(pkgTypes+"/types.go", b.String())
}

// collectTypeImports collects the import paths the types/ package needs. It
// walks exactly the fields generateTypes renders: object fields only.
// Interfaces render as field-less markers, so walking their fields would
// import a package nothing references whenever an interface has no
// implementing object ("imported and not used").
func (g *Generator) collectTypeImports() []string {
	imports := make(map[string]bool)
	for _, def := range g.schema.Types {
		if !isGeneratedDataType(def) || def.Kind != ast.Object {
			continue
		}
		for _, field := range def.Fields {
			if skipGenField(field.Name) {
				continue
			}
			g.checkTypeForImports(field.Type, imports)
		}
	}
	return sortedImports(imports)
}

// checkTypeForImports adds to imports the packages the Go rendering of t
// (as produced by namedTypeToGo) references: a binding's GoImport, or the
// generated scalars / enums package. Objects, inputs and unbound
// interfaces/unions need no import from the types/ point of view.
func (g *Generator) checkTypeForImports(t *ast.Type, imports map[string]bool) {
	if t.Elem != nil {
		g.checkTypeForImports(t.Elem, imports)
		return
	}

	if entry, ok := g.clientConfig.Bindings[t.NamedType]; ok {
		if entry.GoImport != "" {
			imports[entry.GoImport] = true
		}
		return
	}

	def := g.schema.Types[t.NamedType]
	if def == nil {
		return
	}
	switch def.Kind {
	case ast.Scalar:
		imports[g.config.Package+"/"+pkgScalars] = true
	case ast.Enum:
		imports[g.config.Package+"/"+pkgEnums] = true
	}
}
