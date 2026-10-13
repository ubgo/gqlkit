package clientgents

import (
	"bytes"
	"fmt"

	"github.com/vektah/gqlparser/v2/ast"
)

// TSEnumDef holds the metadata for a single TypeScript enum generated from a
// GraphQL enum type. Values are the raw enum value strings.
type TSEnumDef struct {
	Name   string
	Values []string
}

// generateEnums writes enums/index.ts: one string enum per user-declared
// GraphQL enum, member name and value both the GraphQL value verbatim.
func (g *Generator) generateEnums() error {
	var enums []TSEnumDef
	for _, def := range g.generatedDefs(ast.Enum) {
		enumDef := TSEnumDef{Name: def.Name}
		for _, val := range def.EnumValues {
			enumDef.Values = append(enumDef.Values, val.Name)
		}
		enums = append(enums, enumDef)
	}

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "ts_enums.tmpl", enums); err != nil {
		return fmt.Errorf("failed to execute enums template: %w", err)
	}

	return g.writer.WriteFile(dirEnums+"/"+indexFile, buf.String())
}
