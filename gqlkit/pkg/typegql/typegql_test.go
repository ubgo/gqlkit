package typegql

import (
	"go/types"
	"testing"
)

// TestAnyType is the fallback binding for an unbound scalar.
func TestAnyType(t *testing.T) {
	got := AnyType()
	if got.Model != "any" || got.GoType != "any" {
		t.Errorf("AnyType() = %+v, want Model/GoType = any", got)
	}
}

// TestBuiltInTypes sanity-checks a few of the default scalar mappings.
func TestBuiltInTypes(t *testing.T) {
	bt := BuiltInTypes()
	cases := map[string]string{
		"String": "string", "Int": "int", "Boolean": "bool",
		"ID": "string", "Time": "time.Time", "JSON": "encoding/json.RawMessage",
	}
	for name, wantModel := range cases {
		if bt[name].Model != wantModel {
			t.Errorf("BuiltInTypes()[%q].Model = %q, want %q", name, bt[name].Model, wantModel)
		}
	}
}

// TestMerge overlays map2 onto map1 (map2 wins).
func TestMerge(t *testing.T) {
	m1 := TypeMap{"A": {Model: "int"}, "B": {Model: "string"}}
	m2 := TypeMap{"B": {Model: "bool"}, "C": {Model: "float64"}}
	got := Merge(m1, m2)
	if got["A"].Model != "int" || got["B"].Model != "bool" || got["C"].Model != "float64" {
		t.Errorf("Merge = %+v, want B overridden to bool and C added", got)
	}
}

// TestBuildResolvesTypes covers the three Build() outcomes: package-qualified
// named types (with import), Go builtins, and the JSON stdlib type.
func TestBuildResolvesTypes(t *testing.T) {
	in := TypeMap{
		"UUID":   {Model: "github.com/google/uuid.UUID"},
		"Time":   {Model: "time.Time"},
		"String": {Model: "string"},
		"JSON":   {Model: "encoding/json.RawMessage"},
	}
	got := Build(in)

	if e := got["UUID"]; e.GoType != "uuid.UUID" || e.PkgName != "uuid" || e.GoImport != "github.com/google/uuid" || e.TypeName != "UUID" {
		t.Errorf("UUID resolved wrong: %+v", e)
	}
	if e := got["Time"]; e.GoType != "time.Time" || e.GoImport != "time" {
		t.Errorf("Time resolved wrong: %+v", e)
	}
	if e := got["String"]; e.GoType != "string" || e.GoImport != "" {
		t.Errorf("String resolved wrong: %+v", e)
	}
	if e := got["JSON"]; e.GoType != "json.RawMessage" || e.GoImport != "encoding/json" {
		t.Errorf("JSON resolved wrong: %+v", e)
	}
}

// TestBuildEmptyModelDoesNotPanic is the regression guard for the nil-deref
// crash: a binding with an empty Model used to panic (types.Universe.Lookup("")
// returns nil). It must degrade to `any` instead.
func TestBuildEmptyModelDoesNotPanic(t *testing.T) {
	got := Build(TypeMap{"Bad": {Model: ""}})
	if got["Bad"].GoType != "any" {
		t.Errorf("empty-model binding = %+v, want GoType any", got["Bad"])
	}
}

// TestBuildBareLocalTypeName covers an unqualified, non-builtin type name: it
// should resolve to a same-package type (no import), not panic on a nil lookup.
func TestBuildBareLocalTypeName(t *testing.T) {
	got := Build(TypeMap{"Custom": {Model: "MyLocalType"}})
	e := got["Custom"]
	if e.GoType != "MyLocalType" || e.GoImport != "" || e.TypeName != "MyLocalType" {
		t.Errorf("bare local type = %+v, want GoType MyLocalType, no import", e)
	}
}

// TestBuildNamedType exercises the internal resolver directly.
func TestBuildNamedType(t *testing.T) {
	// Go builtin.
	if bt, ok := buildNamedType("int").(*types.Basic); !ok || bt.Name() != "int" {
		t.Errorf("buildNamedType(int) = %T, want *types.Basic int", buildNamedType("int"))
	}
	// Package-qualified.
	n, ok := buildNamedType("time.Time").(*types.Named)
	if !ok || n.Obj().Name() != "Time" || n.Obj().Pkg().Path() != "time" {
		t.Errorf("buildNamedType(time.Time) resolved wrong: %v", buildNamedType("time.Time"))
	}
	// Empty and unknown bare names -> Named with nil package, no panic.
	for _, in := range []string{"", "Unknown"} {
		nn, ok := buildNamedType(in).(*types.Named)
		if !ok || nn.Obj().Pkg() != nil {
			t.Errorf("buildNamedType(%q) = %v, want *types.Named with nil pkg", in, buildNamedType(in))
		}
	}
}

// TestBuiltInTypesReturnsIndependentCopies pins the fix for a process-wide
// leak: Merge and Build mutate the map they receive, and BuiltInTypes used to
// hand out the package-level map, so one generator's bindings (DateTime →
// time.Time) silently applied to every later generator in the process.
func TestBuiltInTypesReturnsIndependentCopies(t *testing.T) {
	first := Build(Merge(BuiltInTypes(), TypeMap{"DateTime": {Model: "time.Time"}, "String": {Model: "int"}}))
	if first["DateTime"].GoType != "time.Time" || first["String"].GoType != "int" {
		t.Fatalf("first map not merged: %+v", first)
	}

	second := BuiltInTypes()
	if _, leaked := second["DateTime"]; leaked {
		t.Error("DateTime binding from an earlier Merge leaked into BuiltInTypes")
	}
	if second["String"].Model != "string" {
		t.Errorf("String overridden by an earlier Merge: %+v", second["String"])
	}
	if second["String"].GoType != "" {
		t.Errorf("String already Build-resolved by an earlier call: %+v", second["String"])
	}
}

// TestBuildPointerAndSliceBindings is the matrix for prefixed bindings:
// prefixes stay on GoType and never leak into the import path.
func TestBuildPointerAndSliceBindings(t *testing.T) {
	cases := []struct {
		model, goType, goImport string
	}{
		{"*math/big.Int", "*big.Int", "math/big"},
		{"[]*math/big.Int", "[]*big.Int", "math/big"},
		{"*[]github.com/google/uuid.UUID", "*[]uuid.UUID", "github.com/google/uuid"},
		{"[]byte", "[]byte", ""},
		{"*string", "*string", ""},
		{"[][]int", "[][]int", ""},
		{"*MyType", "*MyType", ""},
		{"time.Time", "time.Time", "time"},
		{" *time.Time ", "*time.Time", "time"},
	}
	for _, c := range cases {
		got := Build(TypeMap{"X": {Model: c.model}})["X"]
		if got.GoType != c.goType || got.GoImport != c.goImport {
			t.Errorf("Build(%q) = GoType %q import %q, want %q / %q", c.model, got.GoType, got.GoImport, c.goType, c.goImport)
		}
	}
	for _, m := range []string{"*", "[]", "*[]"} {
		if got := Build(TypeMap{"X": {Model: m}})["X"]; got.GoType != "any" {
			t.Errorf("Build(%q) = %+v, want the any fallback", m, got)
		}
	}
}
