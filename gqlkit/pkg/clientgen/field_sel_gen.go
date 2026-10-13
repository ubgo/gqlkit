package clientgen

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/ubgo/gqlkit/gqlkit/pkg/util"
	"github.com/ubgo/gqlkit/gqlkit/pkg/writer"

	"github.com/vektah/gqlparser/v2/ast"
)

// FieldSelectorData is the template data for generating a type-safe field
// selector struct. Each GraphQL object type gets its own selector that wraps
// builder.FieldSelection with typed methods for each field.
type FieldSelectorData struct {
	BuilderPkgImport string // full import path, e.g. "github.com/ubgo/gqlkit/gqlkit/pkg/builder"
	BuilderPkgName   string // package identifier in code, e.g. "builder"
	TypeName         string
	SelectorName     string
	Fields           []FieldSelField
	// IsAbstract marks an interface/union selector: its constructor always
	// selects __typename, which the <Name>Value decoder dispatches on.
	IsAbstract bool
	// Fragments are the On<Type> methods of an abstract selector, one per
	// possible type, each rendering `... on Type { ... }`.
	Fragments []FragmentSel
}

// FragmentSel is one On<Type> method on an abstract type's selector.
type FragmentSel struct {
	MethodName string // "On" + Go type name
	TypeName   string // GraphQL type condition
	Selector   string // the concrete type's selector, e.g. "UserFields"
}

// FieldSelField describes one field on a field selector. Scalar fields produce
// simple AddField calls; object fields produce nested selector callbacks.
type FieldSelField struct {
	FieldName      string
	MethodName     string
	IsObject       bool
	NestedSelector string
}

// getBaseTypeName returns the named type after unwrapping List and NonNull
func getBaseTypeName(t *ast.Type) string {
	if t == nil {
		return ""
	}
	if t.Elem != nil {
		return getBaseTypeName(t.Elem)
	}
	return t.NamedType
}

// isObjectType reports whether the (unwrapped) type needs a selection set:
// an object, interface or union. Unions used to be treated as leaves, which
// produced a field with no selection set that every server rejects.
func isObjectType(schema *ast.Schema, t *ast.Type) bool {
	name := getBaseTypeName(t)
	if name == "" {
		return false
	}
	def := schema.Types[name]
	if def == nil {
		return false
	}
	return def.Kind == ast.Object || isAbstractKind(def)
}

// getSortedObjectTypeNames returns schema object type names (excluding builtin and roots), sorted
func (g *Generator) getSortedObjectTypeNames() []string {
	var names []string
	for name, def := range g.schema.Types {
		// Exactly the types that get a data struct get a selector; see
		// isGeneratedDataType for why non-conventional roots are included.
		if !isGeneratedDataType(def) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// buildFieldSelectorData builds template data for one object type's field selector
func (g *Generator) buildFieldSelectorData(def *ast.Definition) FieldSelectorData {
	data := FieldSelectorData{
		TypeName:     def.Name,
		SelectorName: exportName(def.Name) + "Fields",
		Fields:       make([]FieldSelField, 0, len(def.Fields)),
	}

	// Validate guarantees a non-empty Package, so the import is always set.
	data.BuilderPkgImport = strings.TrimSuffix(g.config.Package, "/") + "/" + pkgBuilder
	data.BuilderPkgName = pkgBuilder

	// On<Type> fragment methods are generator-defined, so they are reserved
	// before field methods claim names (a field `onUser` becomes OnUser_).
	if isAbstractKind(def) {
		data.IsAbstract = true
		for _, pt := range g.possibleTypes(def) {
			data.Fragments = append(data.Fragments, FragmentSel{
				MethodName: "On" + pt.GoName,
				TypeName:   pt.TypeName,
				Selector:   pt.GoName + "Fields",
			})
		}
	}
	reserved := make([]string, 0, len(data.Fragments))
	for _, fr := range data.Fragments {
		reserved = append(reserved, fr.MethodName)
	}
	methods := fieldGoNames(def.Fields, reserved...)

	for _, field := range def.Fields {
		if skipGenField(field.Name) {
			continue
		}
		baseName := getBaseTypeName(field.Type)
		isObj := isObjectType(g.schema, field.Type)

		f := FieldSelField{
			FieldName:  field.Name,
			MethodName: methods[field.Name],
			IsObject:   isObj,
		}
		if isObj {
			f.NestedSelector = exportName(baseName) + "Fields"
		}
		data.Fields = append(data.Fields, f)
	}

	return data
}

// generateFieldSelectionFiles generates one Go file per object type in the
// fields/ directory. Each file contains a type-safe field selector struct that
// wraps builder.FieldSelection with methods for each field of that type.
func (g *Generator) generateFieldSelectionFiles() error {
	names := g.getSortedObjectTypeNames()
	for _, name := range names {
		def := g.schema.Types[name]
		if def == nil {
			continue
		}
		data := g.buildFieldSelectorData(def)
		var buf bytes.Buffer
		if err := g.templates.ExecuteTemplate(&buf, "field_selector", data); err != nil {
			return fmt.Errorf("field selector template for %s: %w", name, err)
		}
		filename := fmt.Sprintf("%s/field_%s.go", pkgFields, util.ToSnakeCase(name))
		content := writer.GeneratedHeader + "package " + pkgFields + "\n\nimport (\n\t\"" + data.BuilderPkgImport + "\"\n)\n\n" + buf.String()
		if err := g.writer.WriteFile(filename, content); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
	}
	return nil
}
