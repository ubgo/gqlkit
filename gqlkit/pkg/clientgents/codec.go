package clientgents

import (
	"fmt"
	"sort"

	"github.com/vektah/gqlparser/v2/ast"
)

// Scalar codecs: a binding with decode/encode (see TSBinding) converts values
// at runtime. The generator only wires the user's functions into the gqlkit-ts
// runtime API — FieldSelection.addField(name, decode, listDepth),
// BaseBuilder.setArg(name, value, type, encoder), setResultDecoder and
// decodeResult — and emits nothing codec-related when no binding has a codec,
// so a codec-free schema generates exactly what it did before.

// Prefixes of the names scalars/index.ts re-exports codec functions under:
// decode<Alias> / encode<Alias>, e.g. decodeDateTime.
const (
	decoderPrefix = "decode"
	encoderPrefix = "encode"
)

// encodePlanSuffix names the generated InputEncodePlan constant of an input
// object type: TodoFilter → TodoFilterEncodePlan (see planNames for clashes).
const encodePlanSuffix = "EncodePlan"

// planNames holds the names inputs/index.ts declares or imports besides the
// input interfaces, keyed by input type name. Computed once per run.
//
// Why: an input type can already use a derived name. inputs/index.ts holds
// every input interface, every <Input>EncodePlan const and the
// InputEncodePlan type import, and operation files import interfaces and
// plans from it together, so input F (plan FEncodePlan) next to input
// FEncodePlan, or an input named InputEncodePlan, broke tsc. Input names keep
// their spelling; a derived name that is taken gets memberSuffix appended
// until free (FEncodePlan_), claimed in input-name order. A schema with no
// clash generates exactly what it did before.
type planNames struct {
	// plans maps an input type name to its plan constant name.
	plans map[string]string
	// planType is the local name of the runtime InputEncodePlan type in
	// inputs/index.ts (aliased on import when an input type takes it).
	planType string
}

// encodePlanNames returns the claimed plan names, computing them on first use.
func (g *Generator) encodePlanNames() *planNames {
	if g.planNames != nil {
		return g.planNames
	}
	defs := g.generatedDefs(ast.InputObject)
	inputs := make([]string, 0, len(defs))
	for _, def := range defs {
		inputs = append(inputs, def.Name)
	}
	sort.Strings(inputs)
	namer := newMemberNamer(inputs)
	pn := &planNames{plans: make(map[string]string, len(inputs)), planType: namer.name(tsInputEncodePlan)}
	for _, in := range inputs {
		pn.plans[in] = namer.name(in + encodePlanSuffix)
	}
	g.planNames = pn
	return pn
}

// encodePlanName returns the plan constant name of an input type.
func (g *Generator) encodePlanName(input string) string {
	if n, ok := g.encodePlanNames().plans[input]; ok {
		return n
	}
	return input + encodePlanSuffix
}

// Runtime codec types re-exported through builder/index.ts when a codec exists.
const (
	tsArgEncoder      = "ArgEncoder"
	tsInputEncodePlan = "InputEncodePlan"
	// tsGraphQLErrors is caught by a decoding execute() so the partial data
	// it carries is decoded before it reaches the caller.
	tsGraphQLErrors = "GraphQLErrors"
)

// scalarCodec returns the codec binding of scalar name, if it has one.
func (g *Generator) scalarCodec(name string) (TSBinding, bool) {
	def := g.schema.Types[name]
	if def == nil || def.Kind != ast.Scalar {
		return TSBinding{}, false
	}
	b, ok := g.clientConfig.Bindings[name]
	return b, ok && b.HasCodec()
}

// hasAnyCodec reports whether any scalar binding declares a codec; it gates
// every codec-only line of generated output.
func (g *Generator) hasAnyCodec() bool {
	for name := range g.clientConfig.Bindings {
		if _, ok := g.scalarCodec(name); ok {
			return true
		}
	}
	return false
}

// decoderName returns the generated decoder name for scalar name, or "" when
// it has no decode function.
func (g *Generator) decoderName(name string) string {
	if b, ok := g.scalarCodec(name); ok && b.Decode != "" {
		return decoderPrefix + scalarAliasName(name)
	}
	return ""
}

// encoderName returns the generated encoder name for scalar name, or "" when
// it has no encode function.
func (g *Generator) encoderName(name string) string {
	if b, ok := g.scalarCodec(name); ok && b.Encode != "" {
		return encoderPrefix + scalarAliasName(name)
	}
	return ""
}

// listDepth counts the list wrappers of t: DateTime! → 0, [[DateTime]] → 2.
// The runtime needs it for scalar leaves because a scalar's own wire value
// may itself be an array (a JSON scalar), which must not be mapped over.
func listDepth(t *ast.Type) int {
	depth := 0
	for ; t != nil && t.Elem != nil; t = t.Elem {
		depth++
	}
	return depth
}

// reachesDecoder reports whether a selection on the named composite type can
// contain a scalar with a decoder: directly, through nested composites, or
// through union members / interface implementations. It decides whether an
// operation's execute() routes the response through decodeResult.
func (g *Generator) reachesDecoder(name string) bool {
	return g.reaches(name, map[string]bool{}, func(t string) bool { return g.decoderName(t) != "" })
}

// reachesEncoder reports whether the named input type (or any input nested
// in it) has a field whose scalar has an encoder, i.e. whether it needs an
// InputEncodePlan.
func (g *Generator) reachesEncoder(name string) bool {
	return g.reaches(name, map[string]bool{}, func(t string) bool { return g.encoderName(t) != "" })
}

// reaches walks the type graph from name, depth-first with a visited set so
// recursive types terminate, and reports whether any reachable scalar
// satisfies hit.
func (g *Generator) reaches(name string, visited map[string]bool, hit func(scalar string) bool) bool {
	if visited[name] {
		return false
	}
	visited[name] = true
	def := g.schema.Types[name]
	if def == nil {
		return false
	}
	switch def.Kind {
	case ast.Scalar:
		return hit(name)
	case ast.Union, ast.Interface:
		for _, p := range g.possibleTypeNames(def) {
			if g.reaches(p, visited, hit) {
				return true
			}
		}
	}
	for _, f := range emittedFields(def) {
		if g.reaches(getBaseTypeName(f.Type), visited, hit) {
			return true
		}
	}
	return false
}

// argEncoderExpr returns the TS ArgEncoder expression for an argument or
// input field of type t, or "" when its values need no encoding. A scalar
// carries its list depth; an input object points at its plan through a thunk
// so self-referential inputs work.
func (g *Generator) argEncoderExpr(t *ast.Type) string {
	name := getBaseTypeName(t)
	if enc := g.encoderName(name); enc != "" {
		return fmt.Sprintf("{ scalar: %s, listDepth: %d }", enc, listDepth(t))
	}
	if def := g.schema.Types[name]; def != nil && def.Kind == ast.InputObject && g.reachesEncoder(name) {
		return fmt.Sprintf("{ input: () => %s }", g.encodePlanName(name))
	}
	return ""
}

// codecImportsFor collects, for the types in ts, the generated encoder names
// (from scalars/) and input plan names (from inputs/) their ArgEncoder
// expressions reference.
func (g *Generator) codecImportsFor(ts []*ast.Type, encoders, plans map[string]bool) {
	for _, t := range ts {
		name := getBaseTypeName(t)
		if enc := g.encoderName(name); enc != "" {
			encoders[enc] = true
		} else if def := g.schema.Types[name]; def != nil && def.Kind == ast.InputObject && g.reachesEncoder(name) {
			plans[g.encodePlanName(name)] = true
		}
	}
}
