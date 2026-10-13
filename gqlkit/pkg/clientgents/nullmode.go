package clientgents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// NullMode selects how a nullable OUTPUT position is typed. The server sends
// JSON null there, so NullModeNull ("T | null") is exact and the default.
// NullModeNullUndefined ("T | null | undefined") is a looser opt-in for code
// written against older SDKs, which typed nullable fields as `field?: T` and
// compared them with `=== undefined`.
//
// Scope: only the outermost nullability of output positions — object and
// interface fields (types/, fields/ selector results) and operation returns.
// List elements stay "| null" (JSON arrays never hold undefined), and inputs
// and arguments are unaffected (callers may already omit them).
type NullMode string

const (
	NullModeNull          NullMode = "null"
	NullModeNullUndefined NullMode = "null-undefined"
)

// NullModeValues is the canonical list of accepted NullMode values.
var NullModeValues = []NullMode{NullModeNull, NullModeNullUndefined}

// nullModeSuffix is the TS text each mode appends to a nullable type.
var nullModeSuffix = map[NullMode]string{
	NullModeNull:          tsNullSuffix,
	NullModeNullUndefined: tsNullSuffix + " | undefined",
}

// fieldKeySep joins a type and field name in a NullableOverrides key.
const fieldKeySep = "."

// validate reports an error for a value outside NullModeValues. The empty
// value means "not set" and is valid everywhere.
func (m NullMode) validate(where string) error {
	if _, ok := nullModeSuffix[m]; ok || m == "" {
		return nil
	}
	return fmt.Errorf("%s: %q: %w (use %q or %q)", where, m, ErrNullModeInvalid, NullModeNull, NullModeNullUndefined)
}

// validateNullModes checks the global mode and every override value; called
// on config load, before the schema is known.
func (c *ClientConfig) validateNullModes() error {
	if err := c.Nullable.validate(`"nullable"`); err != nil {
		return err
	}
	for _, key := range sortedNullKeys(c.NullableOverrides) {
		if err := c.NullableOverrides[key].validate(fmt.Sprintf(`"nullableOverrides" %q`, key)); err != nil {
			return err
		}
	}
	return nil
}

// validateNullOverrideKeys checks every override key names an output type
// ("Todo") or an output field ("Todo.priority", "Query.now") of the schema,
// so a typo fails generation instead of silently doing nothing.
func validateNullOverrideKeys(schema *ast.Schema, overrides map[string]NullMode) error {
	for _, key := range sortedNullKeys(overrides) {
		typeName, fieldName, hasField := strings.Cut(key, fieldKeySep)
		def := schema.Types[typeName]
		if def == nil || (def.Kind != ast.Object && def.Kind != ast.Interface) {
			return fmt.Errorf(`"nullableOverrides" %q: %w: no object or interface type %q`, key, ErrNullOverrideUnknown, typeName)
		}
		if hasField && def.Fields.ForName(fieldName) == nil {
			return fmt.Errorf(`"nullableOverrides" %q: %w: %s has no field %q`, key, ErrNullOverrideUnknown, typeName, fieldName)
		}
	}
	return nil
}

// sortedNullKeys returns the override keys in order, for deterministic errors.
func sortedNullKeys(m map[string]NullMode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// outputNullSuffix returns the suffix for a nullable output field
// typeName.fieldName: the field override, else the type override, else the
// global mode, else NullModeNull.
func (g *Generator) outputNullSuffix(typeName, fieldName string) string {
	cfg := g.clientConfig
	for _, m := range []NullMode{cfg.NullableOverrides[typeName+fieldKeySep+fieldName], cfg.NullableOverrides[typeName], cfg.Nullable} {
		if m != "" {
			return nullModeSuffix[m]
		}
	}
	return tsNullSuffix
}

// outputTSType renders an output position: tsTypeWithLeaf with the outermost
// nullability spelled with outerNull (see NullMode).
func outputTSType(t *ast.Type, leaf, outerNull string) string {
	if t.NonNull {
		return tsTypeWithLeaf(t, leaf)
	}
	inner := *t
	inner.NonNull = true
	return tsOrNullWith(tsTypeWithLeaf(&inner, leaf), outerNull)
}
