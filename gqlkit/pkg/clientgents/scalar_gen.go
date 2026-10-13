package clientgents

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// TSScalar holds the metadata for a single TypeScript type alias generated from
// a GraphQL scalar (e.g. "export type DateTime = string;").
type TSScalar struct {
	Name       string
	TSType     string
	Import     string // npm package to import from (empty = inline alias)
	ImportType string // type name to import from the package
	// Codec re-export (all empty without a codec): the user's functions from
	// CodecImport, renamed to the generated decode<Name> / encode<Name>.
	CodecImport string
	Decode      string // user's decode export
	DecoderName string // e.g. "decodeDateTime"
	Encode      string // user's encode export
	EncoderName string // e.g. "encodeDateTime"
}

// generateScalars writes scalars/index.ts with one alias per scalar in the
// schema — built-ins included, so `ID`, `Int` etc. are importable by name.
// A binding with an `import` becomes a type re-export instead of an alias.
func (g *Generator) generateScalars() error {
	var scalars []TSScalar

	for _, def := range g.schema.Types {
		if def.Kind != ast.Scalar {
			continue
		}

		scalar := TSScalar{
			Name:   scalarAliasName(def.Name),
			TSType: g.tsTypeMap.Get(def.Name),
		}
		if binding, ok := g.clientConfig.Bindings[def.Name]; ok && binding.Import != "" {
			scalar.Import = binding.Import
			scalar.ImportType = binding.Type
		}
		if binding, ok := g.scalarCodec(def.Name); ok {
			scalar.CodecImport = binding.CodecImport
			scalar.Decode, scalar.DecoderName = binding.Decode, g.decoderName(def.Name)
			scalar.Encode, scalar.EncoderName = binding.Encode, g.encoderName(def.Name)
		}

		scalars = append(scalars, scalar)
	}

	sort.Slice(scalars, func(i, j int) bool {
		return scalars[i].Name < scalars[j].Name
	})

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "ts_scalar.tmpl", scalars); err != nil {
		return fmt.Errorf("failed to execute scalar template: %w", err)
	}

	return g.writer.WriteFile(dirScalars+"/"+indexFile, buf.String())
}

// scalarAliasName returns the name a scalar is exported under from
// scalars/index.ts. Only String is renamed (see gqlStringAlias); every other
// scalar keeps its GraphQL name.
func scalarAliasName(name string) string {
	if name == gqlStringScalar {
		return gqlStringAlias
	}
	return name
}

// collectScalarImportNames returns the scalar aliases referenced by the
// emitted fields of defs, i.e. the names a module must import from
// "../scalars". Sorted for deterministic output.
func (g *Generator) collectScalarImportNames(defs []*ast.Definition) []string {
	seen := make(map[string]bool)
	for _, def := range defs {
		for _, field := range emittedFields(def) {
			g.collectScalarRefs(field.Type, seen)
		}
	}
	return sortedKeys(seen)
}

// collectScalarRefs recursively walks a GraphQL type and records each scalar
// that fieldTSType renders as an alias reference (rather than inline TS text).
func (g *Generator) collectScalarRefs(t *ast.Type, seen map[string]bool) {
	if t == nil {
		return
	}
	if t.Elem != nil {
		g.collectScalarRefs(t.Elem, seen)
		return
	}
	def := g.schema.Types[t.NamedType]
	if def == nil || def.Kind != ast.Scalar {
		return
	}
	if tsType, ok := g.tsTypeMap[t.NamedType]; ok && isSimpleTSType(tsType) {
		return
	}
	seen[scalarAliasName(t.NamedType)] = true
}

// isSimpleTSType returns true for TS types that don't need imports: the
// primitive keywords, plus any structurally-inline type expression (e.g.
// "Record<string, unknown>", "string[]", "{ [k: string]: unknown }").
// Mapped scalars render their tsTypeMap value VERBATIM at every use site,
// so a mapping that isn't a bare identifier can never require an import —
// without this check, a scalar like Map (→ Record<string, unknown>) was
// still added to the "../scalars" import list and produced an unused
// type-import in every generated operation that carried a Map argument.
func isSimpleTSType(t string) bool {
	switch t {
	case "string", "number", "boolean", "any", "unknown", "null", "undefined":
		return true
	}
	return strings.ContainsAny(t, "<>{}[]| ")
}

// collectEnumImportNames returns enum type names referenced by the emitted
// fields of defs, needed for import statements from "../enums".
func (g *Generator) collectEnumImportNames(defs []*ast.Definition) []string {
	seen := make(map[string]bool)
	for _, def := range defs {
		for _, field := range emittedFields(def) {
			g.collectEnumRefs(field.Type, seen)
		}
	}
	return sortedKeys(seen)
}

// collectEnumRefs recursively walks a GraphQL type and records any enum
// references into the seen map.
func (g *Generator) collectEnumRefs(t *ast.Type, seen map[string]bool) {
	if t == nil {
		return
	}
	if t.Elem != nil {
		g.collectEnumRefs(t.Elem, seen)
		return
	}
	name := t.NamedType
	def := g.schema.Types[name]
	if def != nil && def.Kind == ast.Enum {
		seen[name] = true
	}
}

// fieldTSType returns the TypeScript type text for a GraphQL type, with its
// nullability: every nullable position admits null, because the server sends
// JSON null there. It is the single resolver for every position the SDK
// renders a type in: interface and input fields, selector leaves, operation
// arguments and non-select operation returns.
//
// Why one resolver: operation files used to run a second resolver that
// rendered unbound custom scalars as `any` and imported the alias anyway,
// leaving an unused import; the import collectors (collectScalarRefs,
// classifyTypeImport) mirror namedTSType, so they must stay in step with it.
func (g *Generator) fieldTSType(t *ast.Type) string {
	if t == nil {
		return tsAny
	}
	return tsTypeWithLeaf(t, g.namedTSType(getBaseTypeName(t)))
}

// tsTypeWithLeaf renders the list and null structure of t around leaf, the
// TS text for t's named type: `[Int]!` with leaf "number" becomes
// "(number | null)[]". Callers pass a different leaf to reuse the structure
// for generics — "U" in selector callbacks, "T" in select() results.
func tsTypeWithLeaf(t *ast.Type, leaf string) string {
	text := leaf
	if t.Elem != nil {
		text = tsListOf(tsTypeWithLeaf(t.Elem, leaf))
	}
	if !t.NonNull {
		text = tsOrNull(text)
	}
	return text
}

// namedTSType returns the TS text for one GraphQL named type: a scalar whose
// binding is simple TS text (see isSimpleTSType) is inlined, any other scalar
// references its scalars/index.ts alias; enums, objects, interfaces and
// inputs reference their own name; a name missing from the schema falls back
// through the bindings to tsAny. A union references its types/index.ts alias.
func (g *Generator) namedTSType(name string) string {
	def := g.schema.Types[name]
	if def == nil {
		return g.tsTypeMap.Get(name)
	}

	switch def.Kind {
	case ast.Scalar:
		if tsType, ok := g.tsTypeMap[name]; ok && isSimpleTSType(tsType) {
			return tsType
		}
		return scalarAliasName(name)
	case ast.Enum, ast.Object, ast.Interface, ast.InputObject, ast.Union:
		return name
	}

	return tsAny
}

// collectFieldSelectorTypeImports collects enum and custom scalar import names
// needed by a field selector file. Scalar fields reference their TS type in
// generic return types, so their imports must be present.
func (g *Generator) collectFieldSelectorTypeImports(def *ast.Definition) (enumImports []string, scalarImports []string) {
	enumSeen := make(map[string]bool)
	scalarSeen := make(map[string]bool)

	for _, field := range emittedFields(def) {
		// Object fields use the generic U, not a concrete type name.
		if isCompositeType(g.schema, field.Type) {
			continue
		}
		g.collectEnumRefs(field.Type, enumSeen)
		g.collectScalarRefs(field.Type, scalarSeen)
	}

	return sortedKeys(enumSeen), sortedKeys(scalarSeen)
}
