package clientgents

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/vektah/gqlparser/v2/ast"
)

// TSFieldSelectorData is the template data for generating a single TypeScript
// field selector class. Each GraphQL composite type gets its own selector
// class wrapping a FieldSelection, with typed methods for each field and, for
// interfaces and unions, one on<Type>() inline-fragment method per possible type.
type TSFieldSelectorData struct {
	TypeName     string
	SelectorName string
	// IsAbstract marks an interface or union selector: its constructor adds
	// __typename and it carries Variants.
	IsAbstract bool
	// DefaultResult is the class's default T: "{}" for an object; for an
	// abstract type the discriminated { __typename } union (variantsDefault).
	DefaultResult string
	Variants      []TSFieldVariant
	Fields        []TSFieldSelField
	Imports       []TSFieldImport
	EnumImports   []string
	ScalarImports []string
	// DecoderImports are the generated decoders (values) this selector passes
	// to addField; empty without a codec.
	DecoderImports []string
}

// TSFieldSelField describes one field on a TypeScript field selector class.
// Scalar fields produce addField calls; object fields produce nested selector
// callbacks with the appropriate generic type parameter.
type TSFieldSelField struct {
	FieldName      string // original GraphQL field name (camelCase)
	MethodName     string // field name, suffixed only if it clashes with a selector member (see memberNamer)
	IsObject       bool
	NestedSelector string // e.g., "UserFields"
	// ResultType is the property type added to the selection result. A
	// selected field is always present, so it is never optional; nullable
	// positions admit null instead. Leaves carry their TS type ("string |
	// null", "(number | null)[]"); object fields carry the same structure
	// around the callback's generic ("U[] | null").
	ResultType string
	// DecodeArgs is appended to the addField call for a leaf whose scalar has
	// a decoder: ", decodeDateTime, 1" (decoder, list depth). Empty otherwise.
	DecodeArgs string
}

// TSFieldVariant is one on<Type>() method of an abstract selector: an inline
// fragment on a possible type, narrowing the result's matching member.
type TSFieldVariant struct {
	TypeName     string // possible type, e.g. "Todo" (also its __typename)
	MethodName   string // e.g. "onTodo"; suffixed if it clashes (memberNamer)
	SelectorName string // e.g. "TodoFields"
}

// TSFieldImport describes one import of another field selector class needed
// for nested object selection methods.
type TSFieldImport struct {
	ClassName string // e.g., "UserFields"
	FilePath  string // e.g., "./user.js" (see siblingImport)
}

// TSFieldIndexEntry holds one entry for the fields/index.ts barrel file that
// re-exports all field selector classes.
type TSFieldIndexEntry struct {
	SelectorName string
	FileName     string
}

// buildFieldSelectorData builds template data for one object type's field selector
func (g *Generator) buildFieldSelectorData(def *ast.Definition) TSFieldSelectorData {
	data := TSFieldSelectorData{
		TypeName:      def.Name,
		SelectorName:  def.Name + selectorSuffix,
		IsAbstract:    isAbstractKind(def.Kind),
		DefaultResult: "{}",
		Fields:        make([]TSFieldSelField, 0, len(def.Fields)),
	}

	importMap := make(map[string]TSFieldImport) // keyed by NestedSelector
	methods := newMemberNamer(selectorMembers)
	decoders := make(map[string]bool)

	// Variant methods are generator API, so they claim their names before
	// schema fields do: an interface field literally called onTodo is the
	// one that gets suffixed.
	if data.IsAbstract {
		data.DefaultResult = g.variantsDefault(def)
		for _, name := range g.possibleTypeNames(def) {
			v := TSFieldVariant{
				TypeName:     name,
				MethodName:   methods.name(variantMethodPrefix + upperFirst(name)),
				SelectorName: name + selectorSuffix,
			}
			data.Variants = append(data.Variants, v)
			importMap[v.SelectorName] = TSFieldImport{ClassName: v.SelectorName, FilePath: siblingImport(toKebabCase(name))}
		}
	}

	for _, field := range emittedFields(def) {
		baseName := getBaseTypeName(field.Type)
		isObj := isCompositeType(g.schema, field.Type)

		f := TSFieldSelField{
			FieldName:  field.Name,
			MethodName: methods.name(field.Name),
			IsObject:   isObj,
		}
		if isObj {
			f.ResultType = outputTSType(field.Type, selectorGeneric, g.outputNullSuffix(def.Name, field.Name))
			f.NestedSelector = baseName + selectorSuffix
			// Skip the import only when the field type *is* the enclosing
			// type (e.g. Item.parent: Item) — TS classes self-reference
			// fine, but `import { ItemFields } from "./item"` inside
			// item.ts is a self-import that breaks the build. The earlier
			// version of this code mistakenly skipped the entire object-
			// field treatment for self-refs, generating a scalar leaf
			// without a selector callback; that's the bug fixed in 871c081.
			if baseName != def.Name {
				filePath := siblingImport(toKebabCase(baseName))
				importMap[f.NestedSelector] = TSFieldImport{
					ClassName: f.NestedSelector,
					FilePath:  filePath,
				}
			}
		} else {
			f.ResultType = g.fieldTypeFor(def, field)
			if dec := g.decoderName(baseName); dec != "" {
				f.DecodeArgs = fmt.Sprintf(", %s, %d", dec, listDepth(field.Type))
				decoders[dec] = true
			}
		}
		data.Fields = append(data.Fields, f)
	}

	// Sort imports by class name for deterministic output
	for _, imp := range importMap {
		data.Imports = append(data.Imports, imp)
	}
	sort.Slice(data.Imports, func(i, j int) bool {
		return data.Imports[i].ClassName < data.Imports[j].ClassName
	})

	// Collect enum and custom scalar imports
	enumImports, scalarImports := g.collectFieldSelectorTypeImports(def)
	data.EnumImports = enumImports
	data.ScalarImports = scalarImports
	data.DecoderImports = sortedKeys(decoders)

	return data
}

// generateFieldSelectionFiles generates one TS file per object type in the
// fields/ directory, plus a barrel index.ts that re-exports all selectors.
func (g *Generator) generateFieldSelectionFiles() error {
	var entries []TSFieldIndexEntry

	for _, def := range g.selectorTypeDefs() {
		name := def.Name
		data := g.buildFieldSelectorData(def)

		var buf bytes.Buffer
		if err := g.templates.ExecuteTemplate(&buf, "ts_field_selector.tmpl", data); err != nil {
			return fmt.Errorf("failed to execute field selector template for %s: %w", name, err)
		}

		rel := dirFields + "/" + toKebabCase(name) + tsExt
		if err := g.writer.WriteFile(rel, buf.String()); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}

		entries = append(entries, TSFieldIndexEntry{
			SelectorName: data.SelectorName,
			FileName:     toKebabCase(name),
		})
	}

	// Generate fields/index.ts barrel file
	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "ts_field_selector_index.tmpl", entries); err != nil {
		return fmt.Errorf("failed to execute field selector index template: %w", err)
	}

	return g.writer.WriteFile(dirFields+"/"+indexFile, buf.String())
}

// upperFirst upper-cases the first ASCII letter of s, so on<Type>() reads as
// camelCase for lowercase (Hasura-style) type names: users → onUsers.
func upperFirst(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}
