package builder

import "testing"

// Pins builder.QueryMarker.IsQueryOp (builder.go:277) and
// builder.MutationMarker.IsMutationOp (builder.go:285).
//
// These are zero-behaviour marker methods, so a test cannot assert on
// what they DO — what matters is that they exist with these exact names
// and receivers. pkg/batch accepts a builder only if it satisfies an
// interface carrying one of these methods, and that check happens in the
// GENERATED SDK, not here. Renaming or dropping a marker therefore leaves
// this module compiling and green while every generated batch call site
// stops type-checking. The compile-time assertions below move that
// breakage into this package, where it is caught before release.
type queryOp interface{ IsQueryOp() }

type mutationOp interface{ IsMutationOp() }

var (
	_ queryOp    = QueryMarker{}
	_ mutationOp = MutationMarker{}
)

func TestQueryMarkerSatisfiesQueryOp(t *testing.T) {
	var op queryOp = QueryMarker{}
	op.IsQueryOp() // no-op by design; the call is what records coverage

	// An embedded marker must carry the interface too — that is how real
	// generated builders pick it up.
	type generatedQueryBuilder struct{ QueryMarker }
	var embedded queryOp = generatedQueryBuilder{}
	embedded.IsQueryOp()
}

func TestMutationMarkerSatisfiesMutationOp(t *testing.T) {
	var op mutationOp = MutationMarker{}
	op.IsMutationOp()

	type generatedMutationBuilder struct{ MutationMarker }
	var embedded mutationOp = generatedMutationBuilder{}
	embedded.IsMutationOp()
}

// The two markers must stay DISTINCT types. If MutationMarker were ever
// aliased to QueryMarker, a mutation would satisfy the query interface and
// batch would happily send it down the query path — the exact mix-up the
// marker pattern exists to prevent.
func TestMarkersAreNotInterchangeable(t *testing.T) {
	if _, ok := interface{}(QueryMarker{}).(mutationOp); ok {
		t.Fatal("QueryMarker satisfies mutationOp — markers have collapsed into one type")
	}
	if _, ok := interface{}(MutationMarker{}).(queryOp); ok {
		t.Fatal("MutationMarker satisfies queryOp — markers have collapsed into one type")
	}
}
