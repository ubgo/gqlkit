package clientgents

import "errors"

// Sentinel errors returned by Config.Validate and binding validation.
var (
	ErrSchemaPathRequired = errors.New("schema path is required")
	// ErrDryRunWithoutPrune: PruneDryRun only describes a prune.
	ErrDryRunWithoutPrune = errors.New("--dry-run requires --prune")

	// ErrNullModeInvalid: "nullable" or a "nullableOverrides" value is not a NullMode.
	ErrNullModeInvalid = errors.New("unknown nullable mode")
	// ErrNullOverrideUnknown: a "nullableOverrides" key names no output type or field.
	ErrNullOverrideUnknown = errors.New("unknown type or field")

	// ErrBindingTypeRequired: a binding has no (or an empty) TS "type".
	ErrBindingTypeRequired = errors.New(`missing TypeScript "type"`)

	// ErrCodecImportRequired: a binding names decode/encode but no codecImport.
	ErrCodecImportRequired = errors.New("decode/encode need codecImport, the module that exports them")
	// ErrCodecWithoutFunctions: a binding names codecImport but no decode/encode.
	ErrCodecWithoutFunctions = errors.New("codecImport is set but neither decode nor encode is")
	// ErrCodecNameInvalid: decode/encode is not a plain identifier.
	ErrCodecNameInvalid = errors.New("decode/encode must be a plain identifier exported by codecImport")
)
