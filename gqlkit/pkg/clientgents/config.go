package clientgents

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tidwall/jsonc"
)

// defaultOutputDir is where the SDK is written when Config.OutputDir is empty.
const defaultOutputDir = "./sdk"

// Config holds all settings needed to run the TypeScript SDK generator,
// typically populated from CLI flags.
type Config struct {
	// SchemaPath is the path to the GraphQL SDL file
	SchemaPath string
	// OutputDir is the directory where the generated SDK will be written
	OutputDir string
	// ConfigPath is the path to config.jsonc
	ConfigPath string
	// Prune, after a successful generation, deletes files under OutputDir
	// whose first line is the gqlkit banner but that this run did not write
	// (leftovers of removed types or operations), then removes generated
	// subdirectories left empty. Files without the banner are never touched.
	// Off by default. Same rule as the Go generator (clientgen.Config.Prune).
	Prune bool
	// PruneDryRun makes Prune only report what it would delete. Validate
	// rejects it without Prune, so a dry run is never silently a no-op.
	PruneDryRun bool
}

// ClientConfig is loaded from config.jsonc and contains user-specified
// scalar-to-TypeScript type overrides.
type ClientConfig struct {
	Bindings ConfigTSBindings `json:"bindings"`
	// Nullable is the global NullMode for nullable output positions; empty
	// means NullModeNull.
	Nullable NullMode `json:"nullable,omitempty"`
	// NullableOverrides sets a NullMode per output type ("Todo") or field
	// ("Todo.priority", "Query.now"); a field key beats a type key, which
	// beats Nullable. Keys are checked against the schema in New.
	NullableOverrides map[string]NullMode `json:"nullableOverrides,omitempty"`
}

// Validate checks that required fields are set and applies defaults.
func (c *Config) Validate() error {
	if c.SchemaPath == "" {
		return ErrSchemaPathRequired
	}
	if c.PruneDryRun && !c.Prune {
		return ErrDryRunWithoutPrune
	}
	if c.OutputDir == "" {
		c.OutputDir = defaultOutputDir
	}
	return nil
}

// loadClientConfig reads and parses a JSONC config file into a ClientConfig.
// If path is empty or the file does not exist, returns an empty config.
func loadClientConfig(path string) (*ClientConfig, error) {
	if path == "" {
		return &ClientConfig{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ClientConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var config ClientConfig
	err = json.Unmarshal(jsonc.ToJSON(content), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Bindings.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	if err := config.validateNullModes(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &config, nil
}
