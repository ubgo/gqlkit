package clientgents

// TypeScript class members that generated classes define themselves. A
// schema-derived method with one of these names would redeclare the member
// (or, for "constructor", turn into a second constructor), so it gets
// suffixed instead. Runtime state lives in `#private` fields, which can never
// clash with a method name, so only the public members are listed here.
var (
	// selectorMembers are defined by every fields/<type>.ts selector class.
	selectorMembers = []string{"constructor"}
	// builderMembers are defined by every queries|mutations/<op>.ts builder.
	// "select" is reserved even on builders without select(), so a setter's
	// name does not change when a field's return type becomes an object.
	builderMembers = []string{"constructor", "select", "execute", "executeRaw", "getOpFragment"}
	// rootMembers are defined by the QueryRoot / MutationRoot classes.
	rootMembers = []string{"constructor"}
)

// memberSuffix is appended to a schema name that clashes with a generated
// class member ("select" → "select_"). A trailing underscore keeps the name
// recognisable and is never produced by the generator for anything else.
const memberSuffix = "_"

// memberNamer hands out collision-free TS method names for one generated
// class. Why: GraphQL field and argument names are arbitrary identifiers, and
// the generated classes also carry their own API (constructor, select,
// execute, …). Names that do not clash are returned unchanged, so the public
// API of every non-clashing schema is unaffected.
//
// Invariant: the result is unique within the class. A suffixed name that is
// itself taken by a later schema name (`select` and `select_` both present)
// keeps growing the suffix, in schema order, so output stays deterministic.
type memberNamer struct {
	taken map[string]bool
}

// newMemberNamer returns a namer for a class that already defines reserved.
func newMemberNamer(reserved []string) *memberNamer {
	n := &memberNamer{taken: make(map[string]bool, len(reserved))}
	for _, r := range reserved {
		n.taken[r] = true
	}
	return n
}

// name returns the method name to use for the schema name want, and records
// it as taken.
func (n *memberNamer) name(want string) string {
	got := want
	for n.taken[got] {
		got += memberSuffix
	}
	n.taken[got] = true
	return got
}
