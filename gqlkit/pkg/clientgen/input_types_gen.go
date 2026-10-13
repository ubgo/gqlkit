package clientgen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// isGeneratedInputType reports whether a schema definition gets a struct in
// inputs/: every user-defined input object.
func isGeneratedInputType(def *ast.Definition) bool {
	return def.Kind == ast.InputObject && !def.BuiltIn && !strings.HasPrefix(def.Name, "__")
}

// generateInputTypes collects all GraphQL InputObject types from the schema,
// converts them to Go struct definitions, and writes inputs/inputs.go. Input
// fields share the struct-field mapping of object fields (graphQLToGoType),
// including always-pointer nested inputs, so self-referencing inputs
// (`input Filter { and: Filter }`) are legal Go.
func (g *Generator) generateInputTypes() error {
	typeDefMap := make(map[string]TypeDef)

	for _, def := range g.schema.Types {
		if !isGeneratedInputType(def) {
			continue
		}
		td := TypeDef{
			Name:        exportName(def.Name),
			Description: def.Description,
			Fields:      g.buildFieldDefs(def),
		}
		for _, f := range td.Fields {
			if f.WireTag != f.JSONTag {
				td.NeedsWireMarshal = true
			}
		}
		typeDefMap[def.Name] = td
	}
	types := sortedTypeDefs(typeDefMap)

	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "inputs", typesTemplateData{
		headerData: headerData{Package: pkgInputs},
		Types:      types,
		Imports:    g.collectInputTypeImports(),
	})
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := g.writer.WriteFile(pkgInputs+"/inputs.go", b.String()); err != nil {
		return err
	}

	if err := g.generateInputWireMarshal(types); err != nil {
		return err
	}
	if err := g.writeGoVersionGuard(pkgInputs); err != nil {
		return err
	}

	// opt.go is always emitted (not only when some field uses
	// OptionalOpt) so Opt/Some/Null are a stable part of every SDK's API and
	// switching a field's mode never adds or removes a file.
	var opt bytes.Buffer
	if err := g.templates.ExecuteTemplate(&opt, "inputs_opt", packageTemplateData{PackageName: pkgInputs}); err != nil {
		return fmt.Errorf("failed to execute opt template: %w", err)
	}
	return g.writer.WriteFile(pkgInputs+"/opt.go", opt.String())
}

// collectInputTypeImports collects the Go import paths required by input type
// fields (e.g. scalar packages, enum packages). It skips exactly the fields
// buildFieldDefs skips, so a placeholder field (`_: DateTime`) cannot leave
// behind an unused import.
func (g *Generator) collectInputTypeImports() []string {
	imports := make(map[string]bool)
	for _, def := range g.schema.Types {
		if !isGeneratedInputType(def) {
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

// generateInputWireMarshal writes inputs/inputs_json.go: a go1.24-only
// MarshalJSON for every input type with NeedsWireMarshal, encoding through a
// tag-only copy of the struct (WireTag) so a non-nil empty slice is sent as
// []. The file is always written (empty of methods when no type needs one)
// so a stale copy can never linger with methods for fields that changed.
func (g *Generator) generateInputWireMarshal(types []TypeDef) error {
	var needed []TypeDef
	importSet := map[string]bool{}
	for _, td := range types {
		if !td.NeedsWireMarshal {
			continue
		}
		needed = append(needed, td)
	}
	if len(needed) > 0 {
		importSet["encoding/json"] = true
		for _, def := range g.schema.Types {
			if !isGeneratedInputType(def) {
				continue
			}
			for _, td := range needed {
				if td.Name != exportName(def.Name) {
					continue
				}
				for _, field := range def.Fields {
					if !skipGenField(field.Name) {
						g.checkTypeForImports(field.Type, importSet)
					}
				}
			}
		}
	}
	var b bytes.Buffer
	err := g.templates.ExecuteTemplate(&b, "inputs_json", typesTemplateData{
		headerData: headerData{Package: pkgInputs},
		Types:      needed,
		Imports:    sortedImports(importSet),
	})
	if err != nil {
		return fmt.Errorf("failed to execute inputs_json template: %w", err)
	}
	return g.writer.WriteFile(pkgInputs+"/inputs_json.go", b.String())
}
