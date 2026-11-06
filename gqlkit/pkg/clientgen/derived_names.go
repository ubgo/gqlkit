package clientgen

import "sort"

// Go names derived from a schema type name by appending a suffix.
//
// Two generated packages declare names built as <TypeName><Suffix>:
//
//   - types/: the <Name>Value decoding wrapper of every interface and union
//     shares the package with every object, interface and union type. The
//     Shopify Admin schema has union SubscriptionDiscount AND object
//     SubscriptionDiscountValue, so the plain rule would declare
//     SubscriptionDiscountValue twice.
//   - fields/: every selector type <Name>Fields comes with a constructor
//     New<Name>Fields, so an object NewTodo (selector NewTodoFields) and an
//     object Todo (constructor NewTodoFields) would clash.
//
// Both follow the naming.go rule: names taken straight from the schema keep
// their spelling, and a derived name that is already taken gets "_" appended
// until it is free (SubscriptionDiscountValue_). Types claim in GraphQL-name
// order, so the result is deterministic, and a schema with no clash
// generates exactly what it did before.

// selectorSuffix names the field selector of a generated data type.
const selectorSuffix = "Fields"

// selectorCtorPrefix names a selector's constructor: New<Selector>.
const selectorCtorPrefix = "New"

// derivedNames holds the claimed Go names, keyed by GraphQL type name.
type derivedNames struct {
	// valueNames is the types/ wrapper name of each interface and union.
	valueNames map[string]string
	// selectorNames is the fields/ selector name of each generated data type.
	selectorNames map[string]string
}

// derived returns the claimed names, computing them on first use. The schema
// is fixed after New, so one computation serves the whole run.
func (g *Generator) derived() *derivedNames {
	if g.derivedNames != nil {
		return g.derivedNames
	}

	var names []string
	for name, def := range g.schema.Types {
		if isGeneratedDataType(def) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	d := &derivedNames{valueNames: map[string]string{}, selectorNames: map[string]string{}}

	// types/: every schema type's own name is reserved first.
	typeNames := make([]string, 0, len(names))
	for _, n := range names {
		typeNames = append(typeNames, exportName(n))
	}
	types := newNameClaimer(typeNames...)
	for _, n := range names {
		if isAbstractKind(g.schema.Types[n]) {
			d.valueNames[n] = types.claim(exportName(n) + abstractValueSuffix)
		}
	}

	// fields/: a selector name is free only when its constructor name is too.
	taken := map[string]bool{}
	for _, n := range names {
		sel := exportName(n) + selectorSuffix
		for taken[sel] || taken[selectorCtorPrefix+sel] {
			sel += collisionSuffix
		}
		taken[sel] = true
		taken[selectorCtorPrefix+sel] = true
		d.selectorNames[n] = sel
	}

	g.derivedNames = d
	return d
}

// abstractValueName returns the Go name of the decoding wrapper for an
// abstract GraphQL type name (normally <Name>Value).
func (g *Generator) abstractValueName(name string) string {
	if v, ok := g.derived().valueNames[name]; ok {
		return v
	}
	return exportName(name) + abstractValueSuffix
}

// selectorName returns the Go name of the field selector for a GraphQL type
// name (normally <Name>Fields); its constructor is New<selectorName>.
func (g *Generator) selectorName(name string) string {
	if v, ok := g.derived().selectorNames[name]; ok {
		return v
	}
	return exportName(name) + selectorSuffix
}
