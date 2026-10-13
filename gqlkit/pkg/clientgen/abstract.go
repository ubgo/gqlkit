package clientgen

import (
	"fmt"
	"sort"

	"github.com/vektah/gqlparser/v2/ast"
)

// GraphQL abstract types (interfaces and unions) in the generated Go SDK.
//
// Shape, per abstract type X:
//   - `type X interface{ IsX() }` — a marker every possible concrete type
//     implements via a value-receiver `func (T) IsX() {}`, so both T and *T
//     satisfy it.
//   - `type XValue struct{ Typename string; Value X; Raw json.RawMessage }`
//     with an UnmarshalJSON that reads `__typename` and decodes into the
//     matching concrete *T. An unknown or missing __typename does NOT fail:
//     Value stays nil and Typename + Raw keep what the server sent, so a
//     server that adds a new member type never breaks an older SDK.
//   - Every reference to X (struct fields, operation results, list
//     elements) uses XValue, following the struct rules (pointer as a struct
//     field, by value as a non-null result).
//   - The XFields selector always selects `__typename` (needed for the
//     dispatch) and has On<T>(func(*TFields)) per possible type, rendered as
//     an inline fragment `... on T { ... }`.

// abstractValueSuffix names the decoding wrapper of an abstract type.
const abstractValueSuffix = "Value"

// PossibleType is one concrete member of an abstract type, as rendered into
// the XValue decoder switch and the On<T> selector methods.
type PossibleType struct {
	// TypeName is the GraphQL name — the `__typename` value and the inline
	// fragment's type condition.
	TypeName string
	// GoName is the generated Go struct name (exportName(TypeName)).
	GoName string
}

// isAbstractKind reports whether a definition is an interface or a union.
func isAbstractKind(def *ast.Definition) bool {
	return def != nil && (def.Kind == ast.Interface || def.Kind == ast.Union)
}

// abstractValueName returns the Go name of the decoding wrapper for an
// abstract GraphQL type name.
func abstractValueName(name string) string {
	return exportName(name) + abstractValueSuffix
}

// possibleTypes returns the concrete members of an abstract type sorted by
// GraphQL name, so the generated switch and selector methods are stable.
func (g *Generator) possibleTypes(def *ast.Definition) []PossibleType {
	var out []PossibleType
	for _, pt := range g.schema.GetPossibleTypes(def) {
		if pt.Kind != ast.Object {
			continue
		}
		out = append(out, PossibleType{TypeName: pt.Name, GoName: exportName(pt.Name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TypeName < out[j].TypeName })
	return out
}

// abstractMemberships returns the sorted Go names of the abstract types an
// object belongs to, i.e. the Is<X>() marker methods it must implement.
func (g *Generator) abstractMemberships(def *ast.Definition) []string {
	var out []string
	for _, a := range g.schema.GetImplements(def) {
		if isAbstractKind(a) && isGeneratedDataType(a) {
			out = append(out, exportName(a.Name))
		}
	}
	sort.Strings(out)
	return out
}

// checkAbstractNameCollisions fails generation when a schema type already
// uses the Go name a wrapper would take (interface Node + type NodeValue), so
// the conflict is reported up front instead of as a "redeclared" compile
// error in the generated SDK.
func (g *Generator) checkAbstractNameCollisions() error {
	goNames := map[string]string{}
	for _, def := range g.schema.Types {
		if isGeneratedDataType(def) {
			goNames[exportName(def.Name)] = def.Name
		}
	}
	for _, def := range g.schema.Types {
		if !isAbstractKind(def) || !isGeneratedDataType(def) {
			continue
		}
		if other, taken := goNames[abstractValueName(def.Name)]; taken {
			return fmt.Errorf("%w: wrapper %s for %s %q collides with schema type %q",
				ErrGoNameCollision, abstractValueName(def.Name), def.Kind, def.Name, other)
		}
	}
	return nil
}
