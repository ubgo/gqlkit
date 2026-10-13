package clientgents

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// TSTypeDef holds the metadata for a single TypeScript interface generated from
// a GraphQL object, interface or input object type.
type TSTypeDef struct {
	Name   string
	Fields []TSFieldDef
}

// TSFieldDef holds the metadata for a single field within a TS interface.
type TSFieldDef struct {
	Name     string // GraphQL field name, used verbatim as the TS property name
	Optional bool   // nullable in GraphQL → rendered as an optional `name?:` property
	TSType   string // TS type text from fieldTSType
}

// TSTypesData is the template data passed to ts_types.tmpl to generate types/index.ts.
type TSTypesData struct {
	EnumImports   []string
	ScalarImports []string
	Types         []TSTypeDef
	Unions        []TSUnionDef
}

// TSUnionDef is one GraphQL union, rendered as a TS union alias of its
// members' interfaces so fields typed with it are not `any`.
type TSUnionDef struct {
	Name    string
	Members string // member names joined with " | ", sorted
}

// buildTypeDefs converts definitions to interface data, one TSFieldDef per
// emitted field. Shared by types/index.ts and inputs/index.ts; output object
// and interface fields honour the NullMode config, input fields do not.
func (g *Generator) buildTypeDefs(defs []*ast.Definition) []TSTypeDef {
	typeDefs := make([]TSTypeDef, 0, len(defs))
	for _, def := range defs {
		td := TSTypeDef{Name: def.Name}
		for _, field := range emittedFields(def) {
			td.Fields = append(td.Fields, TSFieldDef{
				Name:     field.Name,
				Optional: !field.Type.NonNull,
				TSType:   g.fieldTypeFor(def, field),
			})
		}
		typeDefs = append(typeDefs, td)
	}
	return typeDefs
}

// generateTypes writes types/index.ts: one interface per object and
// interface type (operation roots excluded, see isRootType).
func (g *Generator) generateTypes() error {
	defs := g.objectTypeDefs()
	data := TSTypesData{
		EnumImports:   g.collectEnumImportNames(defs),
		ScalarImports: g.collectScalarImportNames(defs),
		Types:         g.buildTypeDefs(defs),
	}
	for _, u := range g.generatedDefs(ast.Union) {
		data.Unions = append(data.Unions, TSUnionDef{Name: u.Name, Members: strings.Join(g.possibleTypeNames(u), " | ")})
	}

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "ts_types.tmpl", data); err != nil {
		return fmt.Errorf("failed to execute types template: %w", err)
	}

	return g.writer.WriteFile(dirTypes+"/"+indexFile, buf.String())
}

// fieldTypeFor renders field of def: an output field (object/interface)
// through its NullMode, an input field with the default " | null".
func (g *Generator) fieldTypeFor(def *ast.Definition, field *ast.FieldDefinition) string {
	if def.Kind == ast.InputObject {
		return g.fieldTSType(field.Type)
	}
	return outputTSType(field.Type, g.namedTSType(getBaseTypeName(field.Type)), g.outputNullSuffix(def.Name, field.Name))
}
