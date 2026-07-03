package clientgen

import (
	"bytes"
	"fmt"
	"github.com/khanakia/gqlkit/gqlkit/pkg/util"
	"sort"
	"strings"
	"unicode"

	"github.com/vektah/gqlparser/v2/ast"
)

// exportName upper-cases the first letter of a GraphQL type name so the
// generated Go type identifier is EXPORTED and therefore reachable from the
// other generated packages (queries, inputs, fields, …). GraphQL type names are
// not required to be capitalized — Hasura emits scalars like `timestamptz` /
// `uuid` and enums like `order_by` — and an unexported alias (`type timestamptz
// = any`) referenced cross-package as `scalars.timestamptz` fails to compile
// with "undefined". Only the first rune is touched, so already-exported names
// (JSON, DateTime, ID, URL) are returned unchanged and existing SDKs keep their
// identifiers; only lowercase-led names are lifted. Every definition site and
// every reference site must route through this, or the two drift apart.
func exportName(name string) string {
	if name == "" {
		return name
	}
	// Drop leading underscores so Apollo Federation types (_Service, _Entity,
	// _Any, _FieldSet) and any other underscore-led name become exported rather
	// than staying unexported (unreachable cross-package). ToUpper('_') == '_',
	// so a bare first-letter uppercase wouldn't fix these.
	trimmed := strings.TrimLeft(name, "_")
	if trimmed == "" {
		// All underscores — nothing to export; leave as-is (such fields/types
		// are dropped by the __/placeholder skips before this is rendered).
		return name
	}
	r := []rune(trimmed)
	if unicode.IsDigit(r[0]) {
		// Can't start a Go identifier with a digit; prefix deterministically.
		return "X" + trimmed
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// skipGenField reports whether a field should be omitted from generation. Two
// cases: introspection meta-fields (__schema / __type / __typename, injected by
// gqlparser) whose __* types we don't generate, and placeholder fields whose
// name produces no valid Go identifier — GraphQL allows `_: Int` (a federation
// "empty type" placeholder), and ToPascalCase("_") == "", which would emit a
// method/field with no name (`func (q *QueryRoot) () *Builder`).
func skipGenField(name string) bool {
	return strings.HasPrefix(name, "__") || util.ToPascalCase(name) == ""
}

// TypeDef holds the metadata for a single Go struct generated from a GraphQL
// object or interface type. Interfaces produce marker types with an Is<Name>()
// method; objects produce full structs with fields.
type TypeDef struct {
	Name        string
	Description string
	Fields      []FieldDef
	IsInterface bool
}

// FieldDef holds the metadata for a single field within a generated Go struct.
type FieldDef struct {
	Name        string
	Description string
	GoType      string
	JSONTag     string
	OmitEmpty   bool
}

// TypeDefList is a sortable slice of TypeDef used for deterministic output.
type TypeDefList []TypeDef

// graphQLToGoType converts a GraphQL AST type to its Go equivalent string.
// It handles lists ([]T), non-null vs nullable (*T for pointers), and
// delegates named type resolution to namedTypeToGo.
func (g *Generator) graphQLToGoType(t *ast.Type) string {
	if t == nil {
		return "interface{}"
	}

	goType := g.resolveType(t)

	// Slice element pointering is handled in the recursive graphQLToGoType call
	// on t.Elem, so a []T is returned as-is here.
	if strings.HasPrefix(goType, "[]") {
		return goType
	}

	// Object / input-object fields are ALWAYS pointers, regardless of
	// nullability. Go forbids value-type cycles (a struct that contains itself
	// directly or transitively), which GraphQL object graphs routinely have —
	// e.g. Shopify's ProductVariant -> ...ContextualPricing -> QuantityRule ->
	// ProductVariant. A pointer breaks the cycle and also naturally models a
	// nullable object. Slices are already reference types, so []T cycles are
	// legal and left by value above.
	if g.isStructKind(t.NamedType) {
		return "*" + goType
	}

	// Nullable non-object fields become pointers to distinguish null from zero.
	if !t.NonNull {
		goType = "*" + goType
	}

	return goType
}

// isStructKind reports whether the named type is generated as a Go struct
// (a GraphQL object or input object). Fields of such types must be emitted as
// pointers to break value-type cycles — see graphQLToGoType. Custom-bound types
// (scalars mapped to time.Time, json.RawMessage, etc.) and everything else are
// not generated structs.
func (g *Generator) isStructKind(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := g.clientConfig.Bindings[name]; ok {
		return false
	}
	def := g.schema.Types[name]
	if def == nil {
		return false
	}
	return def.Kind == ast.Object || def.Kind == ast.InputObject
}

// resolveType resolves the base Go type from a GraphQL type
func (g *Generator) resolveType(t *ast.Type) string {
	if t.Elem != nil {
		// It's a list type
		elemType := g.graphQLToGoType(t.Elem)
		return "[]" + elemType
	}

	// Named type
	return g.namedTypeToGo(t.NamedType)
}

// namedTypeToGo converts a named GraphQL type to its Go representation.
// It checks custom bindings first, then falls back to schema kind-based
// resolution (scalars. prefix, enums. prefix, or plain type name).
func (g *Generator) namedTypeToGo(name string) string {

	// Check if there's a custom binding
	if entry, ok := g.clientConfig.Bindings[name]; ok {
		return entry.GoType
		// // If GoPackage is set by typegql.Build, construct type as package.TypeName
		// if entry.PkgName != "" {
		// 	// Extract type name from Model (which is set to t.Obj().Name() by typegql.Build)
		// 	typeName := entry.Model
		// 	if typeName == "" {
		// 		typeName = name
		// 	}
		// 	// return entry.GoPackage + "." + typeName
		// 	return entry.GoType
		// }
		// // Use GoType if available (for built-in types processed by typegql)
		// if entry.GoType != "" {
		// 	return entry.GoType
		// }
		// // Fallback to Model
		// return entry.Model
	}

	def := g.schema.Types[name]

	switch def.Kind {
	case ast.Scalar:
		return "scalars." + exportName(def.Name)
	case ast.Enum:
		return "enums." + exportName(def.Name)
	case ast.Object:
		return exportName(def.Name)
	case ast.InputObject:
		return exportName(def.Name)
		// case ast.Interface:
		// 	return def.Name
		// // TODO: Handle union types
		// case ast.Union:
		// 	return def.Name
	}

	return "interface{}"

	// Built-in scalars
	// switch name {
	// case "String":
	// 	return "string"
	// case "Int":
	// 	return "int"
	// case "Float":
	// 	return "float64"
	// case "Boolean":
	// 	return "bool"
	// case "ID":
	// 	return "string"
	// }

	// // User-defined types (keep the name)
	// return name
}

// generateTypes collects all non-built-in object and interface types from the
// schema (excluding Query, Mutation, Subscription), converts them to TypeDef
// structs with Go field metadata, and writes types/types.go via the types template.
func (g *Generator) generateTypes() error {
	typeDefMap := make(map[string]TypeDef)

	for _, def := range g.schema.Types {
		if def.BuiltIn || strings.HasPrefix(def.Name, "__") {
			continue
		}

		// Skip the conventional root operation types — they are represented by
		// the queries/ and mutations/ builder packages, not as data structs.
		// A schema whose root is named non-conventionally (e.g. Shopify's
		// "QueryRoot") is NOT skipped: other types can reference it by value
		// (Job.query: QueryRoot), so its struct + field selector must exist.
		// Either way, the injected __schema / __type meta-fields are dropped in
		// the field loop below, so no reference to the ungenerated __Schema /
		// __Type builtin types leaks out.
		if def.Kind == ast.Object && (def.Name == "Query" || def.Name == "Mutation" || def.Name == "Subscription") {
			continue
		}

		if def.Kind == ast.Object || def.Kind == ast.Interface {
			typeDef := TypeDef{
				Name:        exportName(def.Name),
				Description: def.Description,
				IsInterface: def.Kind == ast.Interface,
			}

			// Interfaces only need the Is<Name>() method, not fields
			if def.Kind == ast.Interface {
				typeDefMap[def.Name] = typeDef
				continue
			}

			for _, field := range def.Fields {
				// Skip injected introspection meta-fields and placeholder fields
				// with no valid Go identifier (see skipGenField).
				if skipGenField(field.Name) {
					continue
				}
				goType := g.graphQLToGoType(field.Type)
				omitempty := !field.Type.NonNull
				jsonName := field.Name // GraphQL field names are already camelCase

				var jsonTag string
				if omitempty {
					jsonTag = fmt.Sprintf("`json:\"%s,omitempty\"`", jsonName)
				} else {
					jsonTag = fmt.Sprintf("`json:\"%s\"`", jsonName)
				}

				fieldDef := FieldDef{
					Name:        util.ToPascalCase(field.Name),
					Description: field.Description,
					GoType:      goType,
					JSONTag:     jsonTag,
					OmitEmpty:   omitempty,
				}

				typeDef.Fields = append(typeDef.Fields, fieldDef)
			}

			typeDefMap[def.Name] = typeDef
		}
	}

	// Convert map to sorted slice for deterministic output
	typeList := make([]TypeDef, 0, len(typeDefMap))
	for _, typeDef := range typeDefMap {
		typeList = append(typeList, typeDef)
	}
	sort.Slice(typeList, func(i, j int) bool {
		return typeList[i].Name < typeList[j].Name
	})

	// Collect imports
	imports := g.collectTypeImports()

	// goutil.PrintToJSON(imports)

	// goutil.PrintToJSON(g.clientConfig.Bindings)

	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "types", map[string]interface{}{
		"Config":  g.config,
		"Types":   typeList,
		"Imports": imports,
		"Package": "types",
	})
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	content := b.String()

	return g.writer.WriteFile("types/types.go", content)
}

// collectTypeImports collects necessary imports for types
func (g *Generator) collectTypeImports() []string {
	imports := make(map[string]bool)

	for _, def := range g.schema.Types {
		if def.BuiltIn || strings.HasPrefix(def.Name, "__") {
			continue
		}

		// Skip the conventional root types — mirrors generateTypes so import
		// collection matches what is actually rendered. Non-conventional roots
		// (QueryRoot) are rendered, so their imports are collected here too,
		// minus the injected __* meta-fields.
		if def.Kind == ast.Object && (def.Name == "Query" || def.Name == "Mutation" || def.Name == "Subscription") {
			continue
		}

		if def.Kind == ast.Object || def.Kind == ast.Interface {
			for _, field := range def.Fields {
				if skipGenField(field.Name) {
					continue
				}
				g.checkTypeForImports(field.Type, imports)
			}
		}
	}

	// Convert to sorted slice
	importList := make([]string, 0, len(imports))
	for imp := range imports {
		importList = append(importList, imp)
	}
	sort.Strings(importList)

	return importList
}

// checkTypeForImports checks a type and adds necessary imports
func (g *Generator) checkTypeForImports(t *ast.Type, imports map[string]bool) {
	if t.Elem != nil {
		g.checkTypeForImports(t.Elem, imports)
		return
	}

	// Check if the type has a custom binding with an import
	if entry, ok := g.clientConfig.Bindings[t.NamedType]; ok {
		// Use GoImport from typegql if available
		if entry.GoImport != "" {
			imports[entry.GoImport] = true
			return
		}
		// // Fallback: check Model for standard library types
		// if strings.Contains(entry.Model, "time.Time") {
		// 	imports["time"] = true
		// } else if strings.Contains(entry.Model, "json.RawMessage") {
		// 	imports["encoding/json"] = true
		// }
		return
	}

	if g.schema.Types[t.NamedType].Kind == ast.Scalar {
		imports[g.config.Package+"/scalars"] = true
		return
	}

	fmt.Println("Enum: ", t.NamedType)
	if g.schema.Types[t.NamedType].Kind == ast.Enum {
		imports[g.config.Package+"/enums"] = true
		return
	}

	// Check built-in types that need imports
	// switch t.NamedType {
	// case "Time", "DateTime", "Date":
	// 	imports["time"] = true
	// case "JSON":
	// 	imports["encoding/json"] = true
	// }
}
