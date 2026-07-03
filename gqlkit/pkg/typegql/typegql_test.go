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

// TestBuildType covers the pointer / slice / named / empty branches of buildType.
func TestBuildType(t *testing.T) {
	if _, ok := buildType("*time.Time").(*types.Pointer); !ok {
		t.Errorf("buildType(*time.Time) not a pointer: %T", buildType("*time.Time"))
	}
	if _, ok := buildType("[]string").(*types.Slice); !ok {
		t.Errorf("buildType([]string) not a slice: %T", buildType("[]string"))
	}
	if bt, ok := buildType("string").(*types.Basic); !ok || bt.Name() != "string" {
		t.Errorf("buildType(string) = %T", buildType("string"))
	}
	// Empty must not panic (index-out-of-range guard).
	if n, ok := buildType("").(*types.Named); !ok || n.Obj().Pkg() != nil {
		t.Errorf("buildType(\"\") = %v, want named with nil pkg", buildType(""))
	}
}
