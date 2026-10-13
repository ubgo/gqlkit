package clientgents

import (
	"fmt"

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
// object type: TodoFilter → TodoFilterEncodePlan.
const encodePlanSuffix = "EncodePlan"

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
		return fmt.Sprintf("{ input: () => %s }", name+encodePlanSuffix)
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
			plans[name+encodePlanSuffix] = true
		}
	}
}
