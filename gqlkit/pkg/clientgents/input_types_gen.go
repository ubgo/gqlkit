package clientgents

import (
	"bytes"
	"fmt"

	"github.com/vektah/gqlparser/v2/ast"
)

// TSInputsData is the template data passed to ts_inputs.tmpl to generate
// inputs/index.ts. It includes enum and scalar import lists needed by the
// input type interfaces.
type TSInputsData struct {
	EnumImports   []string
	ScalarImports []string
	Types         []TSTypeDef
	// Codec only (empty without one): encoders imported from scalars/ and
	// one InputEncodePlan per input type that reaches an encoder.
	EncoderImports []string
	Plans          []TSEncodePlan
	// PlanType is the local name of the runtime InputEncodePlan type:
	// "InputEncodePlan", or an alias when an input type has that name.
	PlanType string
	// PlanImport is the import specifier for it: the bare name, or
	// "InputEncodePlan as <PlanType>" when aliased.
	PlanImport string
}

// TSEncodePlan is the generated InputEncodePlan constant of one input type:
// only fields whose values need encoding are listed (the runtime copies the
// rest as-is).
type TSEncodePlan struct {
	Name   string // e.g. "TodoFilterEncodePlan"
	Input  string // the input type it encodes, e.g. "TodoFilter"
	Fields []TSEncodePlanField
}

// TSEncodePlanField is one entry of an encode plan.
type TSEncodePlanField struct {
	Field string // input field name
	Expr  string // ArgEncoder expression (see argEncoderExpr)
}

// generateInputTypes writes inputs/index.ts: one interface per GraphQL input
// object, with the enum and scalar imports its fields need.
func (g *Generator) generateInputTypes() error {
	defs := g.generatedDefs(ast.InputObject)
	data := TSInputsData{
		EnumImports:   g.collectEnumImportNames(defs),
		ScalarImports: g.collectScalarImportNames(defs),
		Types:         g.buildTypeDefs(defs),
	}
	encoders, plans := map[string]bool{}, map[string]bool{}
	for _, def := range defs {
		if !g.reachesEncoder(def.Name) {
			continue
		}
		plan := TSEncodePlan{Name: g.encodePlanName(def.Name), Input: def.Name}
		for _, f := range emittedFields(def) {
			if expr := g.argEncoderExpr(f.Type); expr != "" {
				plan.Fields = append(plan.Fields, TSEncodePlanField{Field: f.Name, Expr: expr})
				g.codecImportsFor([]*ast.Type{f.Type}, encoders, plans)
			}
		}
		data.Plans = append(data.Plans, plan)
	}
	data.EncoderImports = sortedKeys(encoders)
	data.PlanType = g.encodePlanNames().planType
	data.PlanImport = tsInputEncodePlan
	if data.PlanType != tsInputEncodePlan {
		data.PlanImport += " as " + data.PlanType
	}

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "ts_inputs.tmpl", data); err != nil {
		return fmt.Errorf("failed to execute inputs template: %w", err)
	}

	return g.writer.WriteFile(dirInputs+"/"+indexFile, buf.String())
}
