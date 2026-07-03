// Package typegql maps GraphQL scalar type names to Go types. It resolves
// user-specified bindings (e.g. "github.com/google/uuid.UUID") into their
// constituent package name, type name, and import path using the go/types
// package. The resolved TypeMap is consumed by the Go code generator to emit
// correct type references and import statements.
package typegql

import (
	"go/types"
	"strings"
)

// TypeMapEntry describes how a single GraphQL scalar maps to a Go type.
// The Model field holds the raw binding string (e.g. "time.Time" or
// "github.com/google/uuid.UUID"). After Build() processes the entry, the
// remaining fields are populated with resolved package metadata.
type TypeMapEntry struct {
	Model    string `json:"model"`               // github.com/99designs/gqlgen/graphql.UUID
	PkgName  string `json:"goPkgName,omitempty"` // graphql
	TypeName string `json:"typeName,omitempty"`  // UUID
	GoType   string `json:"goType,omitempty"`    // graphql.UUID (PkgName + "." + TypeName)
	GoImport string `json:"goImport,omitempty"`  // github.com/99designs/gqlgen/graphql
}

// TypeMap is a mapping from GraphQL type name to its Go type metadata.
type TypeMap map[string]TypeMapEntry

// AnyType returns a TypeMapEntry that maps to Go's `any` type. Used as the
// fallback when no explicit binding exists for a custom scalar.
func AnyType() TypeMapEntry {
	return TypeMapEntry{
		Model:  "any",
		GoType: "any",
	}
}

// Build builds a type map from a type map entry map
// It converts the model string to a types.Type and sets the GoType, GoPackage, and GoImport fields
func Build(typeMap TypeMap) TypeMap {
	for k, v := range typeMap {
		// An empty Model would panic buildNamedType (nil Universe lookup); a
		// binding with no Go type maps to `any`, same as an unbound scalar.
		if v.Model == "" {
			typeMap[k] = AnyType()
			continue
		}
		t := buildNamedType(v.Model)
		switch t := t.(type) {
		case *types.Named:
			// A bare, non-builtin identifier (e.g. a local type name with no
			// import path) resolves to a Named with no package. Treat it as a
			// same-package type: GoType is the bare name, no import — rather
			// than dereferencing a nil Pkg().
			if t.Obj().Pkg() == nil {
				typeMap[k] = TypeMapEntry{
					Model:    t.Obj().Name(),
					TypeName: t.Obj().Name(),
					GoType:   t.Obj().Name(),
				}
				continue
			}
			typeMap[k] = TypeMapEntry{
				Model:    t.String(),
				PkgName:  t.Obj().Pkg().Name(),
				TypeName: t.Obj().Name(),
				GoType:   t.Obj().Pkg().Name() + "." + t.Obj().Name(),
				GoImport: t.Obj().Pkg().Path(),
			}
		default:
			typeMap[k] = TypeMapEntry{
				Model:    t.String(),
				GoType:   t.String(),
				GoImport: "",
			}
		}
	}
	return typeMap
}

// Merge merges two type maps like user specified bindings and built-in types
func Merge(map1, map2 TypeMap) TypeMap {
	for k, v := range map2 {
		map1[k] = v
	}
	return map1
}

// builtInTypes defines the default mappings from GraphQL built-in scalars and
// common extended scalars (Int64, Float32, Time, JSON, etc.) to Go primitives.
var builtInTypes = TypeMap{
	"String": {
		Model: "string",
	},
	"Int": {
		Model: "int",
	},
	"Int64": {
		Model: "int64",
	},
	"Int32": {
		Model: "int32",
	},
	"Float": {
		Model: "float64",
	},
	"Float64": {
		Model: "float64",
	},
	"Float32": {
		Model: "float32",
	},
	"Boolean": {
		Model: "bool",
	},
	"Uint": {
		Model: "uint",
	},
	"Uint64": {
		Model: "uint64",
	},
	"Uint32": {
		Model: "uint32",
	},
	"ID": {
		Model: "string",
	},
	"Time": {
		Model: "time.Time",
	},
	"JSON": {
		Model: "encoding/json.RawMessage",
		// GoPackage: "encoding",
		// GoImport:  "encoding/json",
	},
}

// BuiltInTypes returns a copy-safe reference to the default scalar type map.
func BuiltInTypes() TypeMap {
	return builtInTypes
}

// buildType constructs a types.Type for the given string (using the syntax
// from the extra field config Type field).
func buildType(typeString string) types.Type {
	switch {
	case typeString == "":
		return buildNamedType("")
	case typeString[0] == '*':
		return types.NewPointer(buildType(typeString[1:]))
	case strings.HasPrefix(typeString, "[]"):
		return types.NewSlice(buildType(typeString[2:]))
	default:
		return buildNamedType(typeString)
	}
}

// buildNamedType returns the specified named or builtin type.
//
// Note that we don't look up the full types.Type object from the appropriate
// package -- gqlgen doesn't give us the package-map we'd need to do so.
// Instead we construct a placeholder type that has all the fields gqlgen
// wants. This is roughly what gqlgen itself does, anyway:
// https://github.com/99designs/gqlgen/blob/master/plugin/modelgen/models.go#L119
func buildNamedType(fullName string) types.Type {
	dotIndex := strings.LastIndex(fullName, ".")
	if dotIndex == -1 { // builtin (int, string, …) or a bare local type name
		if obj := types.Universe.Lookup(fullName); obj != nil {
			return obj.Type()
		}
		// Not a Go universe builtin (empty string, or an unqualified local type
		// name like "MyType"): return a Named with no package rather than
		// dereferencing a nil Universe lookup. Build() detects the nil package
		// and emits it as a same-package type with no import.
		return types.NewNamed(types.NewTypeName(0, nil, fullName, nil), nil, nil)
	}

	// type is pkg.Name
	pkgPath := fullName[:dotIndex]
	typeName := fullName[dotIndex+1:]

	pkgName := pkgPath
	slashIndex := strings.LastIndex(pkgPath, "/")
	if slashIndex != -1 {
		pkgName = pkgPath[slashIndex+1:]
	}

	pkg := types.NewPackage(pkgPath, pkgName)
	// gqlgen doesn't use some of the fields, so we leave them 0/nil
	return types.NewNamed(types.NewTypeName(0, pkg, typeName, nil), nil, nil)
}
