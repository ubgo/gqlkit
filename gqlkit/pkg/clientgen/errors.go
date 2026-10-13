package clientgen

import "errors"

// Sentinel errors returned by Config.Validate and other initialization paths.
var (
	ErrSchemaPathRequired = errors.New("schema path is required")
	// ErrInvalidInputsConfig: the config.jsonc `inputs` section names an
	// unknown mode, type or field (see InputsConfig).
	ErrInvalidInputsConfig = errors.New("invalid inputs config")
	// ErrGoNameCollision: two schema names would generate the same Go
	// identifier.
	ErrGoNameCollision = errors.New("generated Go name collision")
	// ErrDryRunWithoutPrune: PruneDryRun only describes a prune.
	ErrDryRunWithoutPrune = errors.New("--dry-run requires --prune")
	// ErrPackageUndetermined: no --module, no import-path --package and no
	// enclosing go.mod, so the SDK's own import path is unknown.
	ErrPackageUndetermined = errors.New("cannot determine the generated SDK's import path: pass --module (-m), or run inside a Go module that contains the output directory")
)
