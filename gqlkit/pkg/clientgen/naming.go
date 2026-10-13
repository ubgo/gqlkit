package clientgen

import (
	"github.com/ubgo/gqlkit/gqlkit/pkg/util"

	"github.com/vektah/gqlparser/v2/ast"
)

// Go identifier collision handling.
//
// Schema names are mapped to Go names lossily (PascalCase, export), so two
// schema names can land on the same Go name, or on a name the generated code
// already uses for itself (an argument `select` → setter Select, clashing
// with the builder's Select method). The rule, applied everywhere a schema
// name becomes a Go identifier in a shared scope:
//
//   - names the generator itself defines in that scope are reserved first and
//     never change, so the SDK's own API is stable;
//   - schema-derived names then claim names in schema declaration order; a
//     name already taken gets "_" appended until free (Select → Select_,
//     a second clash → Select__).
//
// Declaration order makes the result deterministic, and a schema with no
// clash generates exactly what it did before.

// collisionSuffix is appended to a schema-derived Go name that is taken.
const collisionSuffix = "_"

// builderReservedNames are the methods and embedded-field names every
// generated operation builder already has: BaseBuilder's promoted methods,
// the op-kind markers, the embedded types themselves, and the template's
// Select / Execute. TestBuilderReservedNamesComplete keeps this list in sync
// with pkg/builder and operation_builder.tmpl.
var builderReservedNames = []string{
	"BaseBuilder", "QueryMarker", "MutationMarker",
	"SetArg", "GetSelection", "GetClient", "BuildQuery", "GetVariables",
	"ExecuteRaw", "GetOpFragment", "IsQueryOp", "IsMutationOp",
	"Select", "Execute",
}

// inputReservedNames are methods generated on every input struct (the go1.24
// wire encoder), which a field must not shadow.
var inputReservedNames = []string{"MarshalJSON"}

// nameClaimer hands out unique Go identifiers within one scope.
type nameClaimer struct {
	taken map[string]bool
}

// newNameClaimer returns a claimer with reserved names already taken.
func newNameClaimer(reserved ...string) *nameClaimer {
	c := &nameClaimer{taken: map[string]bool{}}
	for _, r := range reserved {
		c.taken[r] = true
	}
	return c
}

// claim returns want, or want with collisionSuffix appended until unused,
// and marks the result taken.
func (c *nameClaimer) claim(want string) string {
	name := want
	for c.taken[name] {
		name += collisionSuffix
	}
	c.taken[name] = true
	return name
}

// fieldGoNames maps each generated field of def (skipping skipGenField) to
// its unique PascalCase Go name within one scope, in declaration order.
func fieldGoNames(fields ast.FieldList, reserved ...string) map[string]string {
	c := newNameClaimer(reserved...)
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		if skipGenField(f.Name) {
			continue
		}
		out[f.Name] = c.claim(util.ToPascalCase(f.Name))
	}
	return out
}

// argGoNames maps each argument of an operation field to its unique setter
// method name on the builder.
func argGoNames(args ast.ArgumentDefinitionList) map[string]string {
	c := newNameClaimer(builderReservedNames...)
	out := make(map[string]string, len(args))
	for _, a := range args {
		out[a.Name] = c.claim(util.ToPascalCase(a.Name))
	}
	return out
}

// rootNames are the Go names derived from one root operation field: the
// QueryRoot/MutationRoot factory method, the builder type, and the file stem.
type rootNames struct {
	Method   string
	FileStem string
}

// rootGoNames assigns unique factory-method names (one scope: the root
// struct, and therefore the <Method><Suffix> builder types in the package)
// and unique file stems (one scope: the package directory) to every
// generated field of a root operation type. Operation files are named
// "<kind>_<stem>.go", so a stem can never collide with root.go.
func rootGoNames(def *ast.Definition) map[string]rootNames {
	methods := newNameClaimer()
	files := newNameClaimer()
	out := make(map[string]rootNames, len(def.Fields))
	for _, f := range def.Fields {
		if skipGenField(f.Name) {
			continue
		}
		out[f.Name] = rootNames{
			Method:   methods.claim(util.ToPascalCase(f.Name)),
			FileStem: files.claim(util.ToSnakeCase(f.Name)),
		}
	}
	return out
}
