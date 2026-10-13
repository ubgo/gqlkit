package clientgen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// OptionalMode selects how a NULLABLE input-object field is represented in the
// generated inputs/ package. It only affects input objects: operation
// arguments already have three states (setter not called = omitted, called
// with nil = null, called with a value = value).
type OptionalMode string

const (
	// OptionalPointer renders a nullable field as *T (or []T for lists).
	// nil is omitted from the request, so there is no way to send an
	// explicit null. Non-nil empty slices ARE sent as [] (omitzero). The
	// default, because it is what every earlier gqlkit release generated.
	OptionalPointer OptionalMode = "pointer"
	// OptionalOpt renders a nullable field as inputs.Opt[T], which has
	// three states: unset (omitted), null (sent as null) and a value. Build
	// values with inputs.Some(v) / inputs.Null[T](); the zero value is
	// unset. Recommended for new SDKs: it is the only mode that can clear a
	// field in an update mutation.
	OptionalOpt OptionalMode = "opt"
)

// OptionalModeValues is the canonical list of valid modes, used for config
// validation and error messages.
var OptionalModeValues = []OptionalMode{OptionalPointer, OptionalOpt}

// defaultOptionalMode applies when config.jsonc sets nothing.
const defaultOptionalMode = OptionalPointer

// InputsConfig is the `inputs` section of config.jsonc: how nullable input
// fields are generated, configurable at three levels. Resolution for a field
// T.f: Overrides["T.f"], then Overrides["T"], then Optional, then
// defaultOptionalMode.
//
//	"inputs": {
//	  "optional": "opt",                         // every nullable input field
//	  "overrides": {
//	    "LegacyFilter": "pointer",                // every nullable field of one input type
//	    "UpdateTodoInput.tags": "opt"             // one field
//	  }
//	}
//
// Keys use GraphQL names, not generated Go names. New rejects an unknown mode,
// an override naming a type or field the schema does not have, a type key
// that is not an input object, and a field key on a NON-NULL field (it has no
// optional state to configure) — so a typo fails loudly instead of being
// ignored.
type InputsConfig struct {
	Optional  OptionalMode            `json:"optional,omitempty"`
	Overrides map[string]OptionalMode `json:"overrides,omitempty"`
}

// validOptionalMode reports whether m is a known mode.
func validOptionalMode(m OptionalMode) bool {
	return slices.Contains(OptionalModeValues, m)
}

// validateInputsConfig checks the inputs section against the schema; see
// InputsConfig for the rules.
func validateInputsConfig(cfg InputsConfig, schema *ast.Schema) error {
	if cfg.Optional != "" && !validOptionalMode(cfg.Optional) {
		return fmt.Errorf("%w: inputs.optional %q (valid: %v)", ErrInvalidInputsConfig, cfg.Optional, OptionalModeValues)
	}
	for key, mode := range cfg.Overrides {
		if !validOptionalMode(mode) {
			return fmt.Errorf("%w: inputs.overrides[%q] = %q (valid: %v)", ErrInvalidInputsConfig, key, mode, OptionalModeValues)
		}
		typeName, fieldName, isField := strings.Cut(key, ".")
		def := schema.Types[typeName]
		if def == nil || def.Kind != ast.InputObject {
			return fmt.Errorf("%w: inputs.overrides[%q]: %q is not an input type in the schema", ErrInvalidInputsConfig, key, typeName)
		}
		if !isField {
			continue
		}
		field := def.Fields.ForName(fieldName)
		if field == nil {
			return fmt.Errorf("%w: inputs.overrides[%q]: input %s has no field %q", ErrInvalidInputsConfig, key, typeName, fieldName)
		}
		if field.Type.NonNull {
			return fmt.Errorf("%w: inputs.overrides[%q]: field is non-null, so it has no optional representation", ErrInvalidInputsConfig, key)
		}
	}
	return nil
}

// optionalModeFor resolves the mode of input field typeName.fieldName.
func (g *Generator) optionalModeFor(typeName, fieldName string) OptionalMode {
	cfg := g.clientConfig.Inputs
	if m, ok := cfg.Overrides[typeName+"."+fieldName]; ok {
		return m
	}
	if m, ok := cfg.Overrides[typeName]; ok {
		return m
	}
	if cfg.Optional != "" {
		return cfg.Optional
	}
	return defaultOptionalMode
}

// optInnerGoType is the T of Opt[T] for a nullable input field: the Go type
// of the same field were it non-null, with generated structs by value
// (Opt stores a pointer internally, so a self-referencing input stays legal).
func (g *Generator) optInnerGoType(t *ast.Type) string {
	nonNull := *t
	nonNull.NonNull = true
	goType := g.graphQLToGoType(&nonNull)
	if t.Elem == nil && g.isStructKind(t.NamedType) {
		goType = strings.TrimPrefix(goType, "*")
	}
	return goType
}
