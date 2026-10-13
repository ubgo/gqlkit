// Package clientgen is the main orchestrator for generating a type-safe Go
// client SDK from a parsed GraphQL schema. It coordinates schema parsing,
// type mapping, template execution, and file writing to produce a complete
// Go package with types, enums, scalars, input types, field selectors, and
// per-operation query/mutation builders.
package clientgen

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ubgo/gqlkit/gqlkit/pkg/schemagql"
	"github.com/ubgo/gqlkit/gqlkit/pkg/templater"
	"github.com/ubgo/gqlkit/gqlkit/pkg/typegql"
	"github.com/ubgo/gqlkit/gqlkit/pkg/writer"

	"github.com/vektah/gqlparser/v2/ast"
)

// ClientConfig is the parsed config.jsonc: scalar-to-Go-type bindings (which
// override the built-in mappings) and the nullable-input representation.
// Unknown top-level keys are ignored.
type ClientConfig struct {
	Bindings typegql.TypeMap `json:"bindings"`
	// Inputs configures how nullable input fields are generated.
	Inputs InputsConfig `json:"inputs"`
}

// Generator orchestrates the entire Go SDK generation process. It holds the
// parsed schema, resolved type bindings, compiled templates, and the file
// writer. Call New() to create one, then Generate() to produce all output files.
type Generator struct {
	config       *Config             // CLI/caller-supplied generation settings
	schema       *ast.Schema         // parsed and validated GraphQL schema
	writer       *writer.Writer      // writes formatted Go files to the output directory
	clientConfig *ClientConfig       // user-specified scalar bindings from config.jsonc
	templates    *templater.Template // compiled Go code generation templates
	pruned       []string            // result of the last prune, see PrunedFiles
}

// New creates a new Generator by validating the config, loading the client
// config file, parsing the GraphQL schema, merging built-in and user type
// bindings, and compiling the embedded Go templates.
func New(config *Config) (*Generator, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	clientConfig, err := loadClientConfig(config.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client config: %w", err)
	}

	if clientConfig == nil {
		return nil, errors.New("client config is nil")
	}

	schema, err := schemagql.GetSchema(schemagql.StringList{config.SchemaPath})
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	if err := validateInputsConfig(clientConfig.Inputs, schema); err != nil {
		return nil, err
	}

	typeMapEntryMap := typegql.Merge(typegql.BuiltInTypes(), clientConfig.Bindings)
	typeMapEntryMap = typegql.Build(typeMapEntryMap)
	clientConfig.Bindings = typeMapEntryMap

	templates := templater.MustParse(templater.NewTemplate("templates").
		ParseFS(templater.TemplateDir(), "template/*.tmpl"))

	return &Generator{
		config:       config,
		schema:       schema,
		clientConfig: clientConfig,
		writer:       writer.NewWriter(config.OutputDir),
		templates:    templates,
	}, nil
}

// GetSchema returns the parsed, validated schema the generator renders from.
// The returned value is shared, not a copy: callers must not mutate it
// between New and Generate.
func (g *Generator) GetSchema() *ast.Schema {
	return g.schema
}

// Generate runs the full code generation pipeline: scalars, enums, types,
// input types, the builder base, field selectors, and query/mutation operation
// builders. Each step writes one or more files to the configured output directory.
func (g *Generator) Generate() error {
	fmt.Printf("Generating SDK from %s\n", g.config.SchemaPath)
	fmt.Printf("Output directory: %s\n", g.config.OutputDir)
	fmt.Printf("Package name: %s\n", g.config.PackageName)

	// Ensure output directory exists
	if err := g.writer.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate scalars
	if err := g.generateScalars(); err != nil {
		return fmt.Errorf("failed to generate scalars: %w", err)
	}
	fmt.Println("Generated: scalars.go")

	// Generate enums
	if err := g.generateEnums(); err != nil {
		return fmt.Errorf("failed to generate enums: %w", err)
	}
	fmt.Println("Generated: enums.go")

	// Generate types
	if err := g.generateTypes(); err != nil {
		return fmt.Errorf("failed to generate types: %w", err)
	}
	fmt.Println("Generated: types.go")

	// Generate inputs
	if err := g.generateInputTypes(); err != nil {
		return fmt.Errorf("failed to generate input types: %w", err)
	}
	fmt.Println("Generated: inputs.go")

	// Generate builder files
	if err := g.generateBuilderFiles(); err != nil {
		return fmt.Errorf("failed to generate builder files: %w", err)
	}
	fmt.Println("Generated: builder.go")

	// Generate graphqlclient package
	if err := g.generateGraphQLClientFiles(); err != nil {
		return fmt.Errorf("failed to generate graphqlclient files: %w", err)
	}
	fmt.Println("Generated: graphqlclient/graphqlclient.go")

	// Generate field selection files (one per type in fields/)
	if err := g.generateFieldSelectionFiles(); err != nil {
		return fmt.Errorf("failed to generate field selection files: %w", err)
	}
	fmt.Println("Generated: fields/field_*.go")

	// Generate query and mutation builders (queries/, mutations/)
	if err := g.generateOperationFiles(); err != nil {
		return fmt.Errorf("failed to generate operation files: %w", err)
	}
	fmt.Println("Generated: queries/ and mutations/")

	// Generate batch package
	if err := g.generateBatchFiles(); err != nil {
		return fmt.Errorf("failed to generate batch files: %w", err)
	}
	fmt.Println("Generated: batch/batch.go")

	if g.config.Prune {
		pruned, err := g.writer.Prune(g.config.PruneDryRun)
		if err != nil {
			return err
		}
		g.pruned = pruned
		verb := "Pruned"
		if g.config.PruneDryRun {
			verb = "Would prune"
		}
		for _, f := range pruned {
			fmt.Printf("%s: %s\n", verb, f)
		}
	}

	fmt.Printf("SDK generated successfully in %s\n", g.config.OutputDir)
	return nil
}

// PrunedFiles returns the slash-separated paths (relative to OutputDir) that
// the last Generate deleted — or, with Config.PruneDryRun, would have
// deleted. Empty unless Config.Prune is set.
func (g *Generator) PrunedFiles() []string {
	return g.pruned
}

// generateBuilderFiles renders the builder/builder.go file that re-exports the
// base builder types from the builder package into the generated SDK.
func (g *Generator) generateBuilderFiles() error {
	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "builder", packageTemplateData{PackageName: pkgBuilder})
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	content := b.String()
	if err := g.writer.WriteFile(pkgBuilder+"/builder.go", content); err != nil {
		return err
	}
	// Every operation and selector imports builder/, so its guard stops any
	// pre-1.24 build of the SDK; inputs/ gets its own for SDKs used only
	// for their input types.
	return g.writeGoVersionGuard(pkgBuilder)
}

// minGoVersionGuardFile is the per-package file holding the minimum-Go guard.
const minGoVersionGuardFile = "go_version_guard.go"

// writeGoVersionGuard emits <pkg>/go_version_guard.go: a `//go:build !go1.24`
// file that fails to compile on toolchains older than Go 1.24, so the SDK
// can never be built where `omitzero` is silently ignored.
func (g *Generator) writeGoVersionGuard(pkg string) error {
	var b bytes.Buffer
	if err := g.templates.ExecuteTemplate(&b, "go_guard", packageTemplateData{PackageName: pkg}); err != nil {
		return fmt.Errorf("failed to execute go_guard template: %w", err)
	}
	return g.writer.WriteFile(pkg+"/"+minGoVersionGuardFile, b.String())
}

// generateGraphQLClientFiles renders graphqlclient/graphqlclient.go into the
// generated SDK so users don't need an external dependency for the HTTP client.
func (g *Generator) generateGraphQLClientFiles() error {
	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "graphqlclient", packageTemplateData{PackageName: pkgGraphQLClient})
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	return g.writer.WriteFile(pkgGraphQLClient+"/graphqlclient.go", b.String())
}

// generateBatchFiles renders batch/batch.go into the generated SDK. The batch
// package merges multiple builders into a single GraphQL operation with
// aliased root fields, importing the SDK's local builder + graphqlclient
// packages so it's fully self-contained.
func (g *Generator) generateBatchFiles() error {
	rootPkg := strings.TrimSuffix(g.config.Package, "/")
	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "batch", batchTemplateData{RootPackage: rootPkg})
	if err != nil {
		return fmt.Errorf("failed to execute batch template: %w", err)
	}
	return g.writer.WriteFile(pkgBatch+"/batch.go", b.String())
}

// generateScalars builds scalar type aliases (e.g. type DateTime = time.Time)
// and writes them to scalars/scalars.go.
func (g *Generator) generateScalars() error {
	scalarData := buildSchemaScalarMap(g.schema, g.clientConfig.Bindings)
	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "scalar", scalarsTemplateData{
		headerData: headerData{Package: pkgScalars},
		Scalars:    scalarData.SchemaScalarMap,
		Imports:    scalarData.Imports,
	})
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	content := b.String()

	return g.writer.WriteFile(pkgScalars+"/scalars.go", content)
}

// EnumDef represents a single GraphQL enum type with its values, ready for
// template rendering.
type EnumDef struct {
	Name        string
	Description string
	EnumValues  []EnumValueDef
}

// EnumValueDef represents a single value within a GraphQL enum. Name is the
// GraphQL value verbatim (the wire string); enums.tmpl derives the Go constant
// name as <EnumName><PascalCase(Name)>.
type EnumValueDef struct {
	Name        string
	Description string
	// GoName is the unique Go constant name. Constants of every enum share
	// the enums package scope with the enum type names, so values differing
	// only in case (ACTIVE / active) or colliding with a type name get "_"
	// appended in declaration order (see naming.go).
	GoName string
}

// EnumDefMap maps enum names to their definitions.
type EnumDefMap map[string]EnumDef

// generateEnums collects all non-built-in enum types from the schema,
// sorts them alphabetically, and writes enums/enums.go via the enums template.
func (g *Generator) generateEnums() error {
	enumDefMap := make(EnumDefMap)
	for _, def := range g.schema.Types {
		if def.BuiltIn || strings.HasPrefix(def.Name, "__") {
			continue
		}

		if def.Kind == ast.Enum {
			enumDef := EnumDef{
				// Exported so cross-package refs (enums.OrderBy) resolve —
				// GraphQL enum names may be lowercase (Hasura: order_by).
				Name:        exportName(def.Name),
				Description: def.Description,
			}
			for _, val := range def.EnumValues {
				enumDef.EnumValues = append(enumDef.EnumValues, EnumValueDef{
					Name:        val.Name,
					Description: val.Description,
				})
			}
			enumDefMap[def.Name] = enumDef
		}
	}

	// Convert map to sorted slice for deterministic output
	enumList := make([]EnumDef, 0, len(enumDefMap))
	for _, enumDef := range enumDefMap {
		enumList = append(enumList, enumDef)
	}
	sort.Slice(enumList, func(i, j int) bool {
		return enumList[i].Name < enumList[j].Name
	})

	// One identifier scope for the whole enums package: type names first
	// (they never change), then each enum's values in declaration order.
	names := newNameClaimer()
	for _, e := range enumList {
		names.claim(e.Name)
	}
	for i := range enumList {
		for j := range enumList[i].EnumValues {
			v := &enumList[i].EnumValues[j]
			v.GoName = names.claim(enumList[i].Name + templater.PascalCase(v.Name))
		}
	}

	b := bytes.NewBuffer(nil)
	err := g.templates.ExecuteTemplate(b, "enums", enumsTemplateData{
		headerData: headerData{Package: pkgEnums},
		Enums:      enumList,
	})
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	content := b.String()

	return g.writer.WriteFile(pkgEnums+"/enums.go", content)
}
