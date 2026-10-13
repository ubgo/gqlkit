// Package clientgents is the main orchestrator for generating a type-safe
// TypeScript client SDK from a parsed GraphQL schema. It mirrors the structure
// of the Go clientgen package but targets TypeScript output: interfaces instead
// of structs, TS enums, type aliases for scalars, and class-based builders that
// extend the gqlkit-ts BaseBuilder.
package clientgents

import (
	"embed"
	"fmt"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/ubgo/gqlkit/gqlkit/pkg/schemagql"
	"github.com/vektah/gqlparser/v2/ast"
)

// templateDir embeds all .tmpl files under the template/ subdirectory. These
// are the TypeScript code generation templates.
//
//go:embed template/*
var templateDir embed.FS

// Generator orchestrates the entire TypeScript SDK generation process. It holds
// the parsed schema, TS type map, compiled templates, and the file writer.
type Generator struct {
	config       *Config
	schema       *ast.Schema
	writer       *TSWriter
	clientConfig *ClientConfig
	tsTypeMap    TSTypeMap
	templates    *template.Template
	pruned       []string // result of the last prune, see PrunedFiles
}

// PrunedFiles returns the files the last Generate pruned (or, with
// PruneDryRun, would prune), as slash-separated paths relative to the output
// directory. Empty when Prune is off.
func (g *Generator) PrunedFiles() []string {
	return g.pruned
}

// New creates a new Generator by validating config, loading the client config,
// parsing the schema, building the TS type map, and compiling templates.
func New(config *Config) (*Generator, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	clientConfig, err := loadClientConfig(config.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client config: %w", err)
	}

	schema, err := schemagql.GetSchema(schemagql.StringList{config.SchemaPath})
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}
	if err := validateNullOverrideKeys(schema, clientConfig.NullableOverrides); err != nil {
		return nil, fmt.Errorf("invalid client config: %w", err)
	}

	// Build TS type map: built-in types + config overrides
	tsTypeMap := BuiltInTSTypes()
	if clientConfig.Bindings != nil {
		tsTypeMap.Merge(clientConfig.Bindings)
	}

	funcMap := template.FuncMap{
		"joinComma":     func(s []string) string { return strings.Join(s, ", ") },
		"dirImport":     dirImport,
		"siblingImport": siblingImport,
	}

	templates, err := template.New("").Funcs(funcMap).ParseFS(templateDir, "template/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Generator{
		config:       config,
		schema:       schema,
		clientConfig: clientConfig,
		tsTypeMap:    tsTypeMap,
		writer:       NewTSWriter(config.OutputDir),
		templates:    templates,
	}, nil
}

// Generate runs the full TypeScript code generation pipeline: builder re-exports,
// scalars, enums, types, inputs, field selectors, and operation builders.
func (g *Generator) Generate() error {
	fmt.Printf("Generating TypeScript SDK from %s\n", g.config.SchemaPath)
	fmt.Printf("Output directory: %s\n", g.config.OutputDir)

	if err := g.writer.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 1. builder/index.ts
	if err := g.generateBuilderIndex(); err != nil {
		return fmt.Errorf("failed to generate builder/index.ts: %w", err)
	}
	fmt.Println("Generated: builder/index.ts")

	// 2. scalars/index.ts
	if err := g.generateScalars(); err != nil {
		return fmt.Errorf("failed to generate scalars: %w", err)
	}
	fmt.Println("Generated: scalars/index.ts")

	// 3. enums/index.ts
	if err := g.generateEnums(); err != nil {
		return fmt.Errorf("failed to generate enums: %w", err)
	}
	fmt.Println("Generated: enums/index.ts")

	// 4. types/index.ts
	if err := g.generateTypes(); err != nil {
		return fmt.Errorf("failed to generate types: %w", err)
	}
	fmt.Println("Generated: types/index.ts")

	// 5. inputs/index.ts
	if err := g.generateInputTypes(); err != nil {
		return fmt.Errorf("failed to generate inputs: %w", err)
	}
	fmt.Println("Generated: inputs/index.ts")

	// 6. fields/*.ts + fields/index.ts
	if err := g.generateFieldSelectionFiles(); err != nil {
		return fmt.Errorf("failed to generate field selectors: %w", err)
	}
	fmt.Println("Generated: fields/*.ts")

	// 7. queries/*.ts + queries/root.ts + queries/index.ts
	// 8. mutations/*.ts + mutations/root.ts + mutations/index.ts
	if err := g.generateOperationFiles(); err != nil {
		return fmt.Errorf("failed to generate operations: %w", err)
	}
	fmt.Println("Generated: queries/ and mutations/")

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

	fmt.Printf("TypeScript SDK generated successfully in %s\n", g.config.OutputDir)
	return nil
}

// generateBuilderIndex generates builder/index.ts which re-exports the
// FieldSelection, BaseBuilder, and GraphQLClient types from the gqlkit-ts npm
// package, and declares SelectionRequired (see TSOperationBuilderData.NeedsSelect).
func (g *Generator) generateBuilderIndex() error {
	content := tsBanner + `

// Re-export builder classes from gqlkit-ts
export { FieldSelection, BaseBuilder } from "gqlkit-ts";
export { GraphQLClient } from "gqlkit-ts";
export type { RequestOptions } from "gqlkit-ts";

/**
 * SelectionRequired is the result type of an operation builder whose object
 * result has no selection yet. A GraphQL server rejects an object field
 * without a selection set, so execute(), executeRaw() and getOpFragment() are
 * not callable until select() has narrowed the result.
 */
export interface ` + tsSelectionRequired + ` {
  readonly "select() is required before execute()": never;
}
`
	if g.hasAnyCodec() {
		content += "\nexport { " + tsGraphQLErrors + ` } from "gqlkit-ts";` + "\n"
		content += "export type { " + tsArgEncoder + ", " + tsInputEncodePlan + ` } from "gqlkit-ts";` + "\n"
	}
	return g.writer.WriteFile(dirBuilder+"/"+indexFile, content)
}

// isRootType reports whether def is excluded from types/ and fields/ as an
// operation root. The check is by the conventional names (Query, Mutation,
// Subscription), not by the schema's root pointers, on purpose and in parity
// with the Go generator: a root with a non-conventional name (Shopify's
// QueryRoot) is still generated as a normal type so other types can
// reference it (e.g. `Job.query: QueryRoot`). Known consequences, pinned by
// tests: a non-root type that happens to be called Query is skipped too, and
// a field typed as a conventionally-named root has no interface to point at.
func (g *Generator) isRootType(def *ast.Definition) bool {
	return def.Kind == ast.Object && slices.Contains(conventionalRootNames, def.Name)
}

// generatedDefs returns the user-declared definitions of the given kinds,
// sorted by name so every generated file is deterministic. Built-ins,
// introspection (__*) types and operation roots are excluded.
func (g *Generator) generatedDefs(kinds ...ast.DefinitionKind) []*ast.Definition {
	var defs []*ast.Definition
	for name, def := range g.schema.Types {
		if def.BuiltIn || strings.HasPrefix(name, introspectionPrefix) || g.isRootType(def) {
			continue
		}
		for _, k := range kinds {
			if def.Kind == k {
				defs = append(defs, def)
				break
			}
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

// objectTypeDefs returns the object and interface types that get a
// types/index.ts interface (and, with unions, a fields/ selector).
func (g *Generator) objectTypeDefs() []*ast.Definition {
	return g.generatedDefs(ast.Object, ast.Interface)
}

// selectorTypeDefs returns every composite type, each of which gets a
// fields/<type>.ts selector class.
func (g *Generator) selectorTypeDefs() []*ast.Definition {
	return g.generatedDefs(ast.Object, ast.Interface, ast.Union)
}

// possibleTypeNames returns the concrete object types an abstract type can
// resolve to (union members, interface implementations), sorted so the
// generated variant order is deterministic.
func (g *Generator) possibleTypeNames(def *ast.Definition) []string {
	var names []string
	for _, p := range g.schema.GetPossibleTypes(def) {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

// variantsDefault returns the TS result type of an abstract selection before
// any on<Type>() call: a discriminated union with one { __typename } member
// per possible type, since only __typename is fetched. An interface with no
// implementations falls back to an undiscriminated { __typename: string }.
func (g *Generator) variantsDefault(def *ast.Definition) string {
	names := g.possibleTypeNames(def)
	if len(names) == 0 {
		return tsTypenameObject("string")
	}
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = tsTypenameObject(fmt.Sprintf("%q", n))
	}
	return strings.Join(parts, " | ")
}

// tsTypenameObject renders the object type { __typename: <tsType> }.
func tsTypenameObject(tsType string) string {
	return "{ " + typenameField + ": " + tsType + " }"
}

// emittedFields returns the fields of def the generator renders. Object and
// interface fields drop introspection and placeholder names (skipGenField);
// input fields are kept as-is because every legal input field name is a
// valid TS property. Generators and their import collectors must both read
// fields through here, or a skipped field still drags in an unused import.
func emittedFields(def *ast.Definition) ast.FieldList {
	if def.Kind == ast.InputObject {
		return def.Fields
	}
	fields := make(ast.FieldList, 0, len(def.Fields))
	for _, f := range def.Fields {
		if !skipGenField(f.Name) {
			fields = append(fields, f)
		}
	}
	return fields
}

// getBaseTypeName returns the named type after unwrapping List and NonNull
func getBaseTypeName(t *ast.Type) string {
	if t == nil {
		return ""
	}
	if t.Elem != nil {
		return getBaseTypeName(t.Elem)
	}
	return t.NamedType
}

// isCompositeType reports whether t (after unwrapping lists) names a GraphQL
// composite type — object, interface or union. Those are the types a query
// must give a selection set, so they get a selector callback in fields/ and
// a select() on operations, never a scalar leaf.
func isCompositeType(schema *ast.Schema, t *ast.Type) bool {
	def := schema.Types[getBaseTypeName(t)]
	return def != nil && isCompositeKind(def.Kind)
}

// isCompositeKind reports whether kind is object, interface or union.
func isCompositeKind(kind ast.DefinitionKind) bool {
	return kind == ast.Object || kind == ast.Interface || isAbstractKind(kind)
}

// isAbstractKind reports whether kind is interface or union: a type whose
// runtime value is one of several concrete object types. Abstract selections
// always request __typename and expose on<Type>() inline fragments.
func isAbstractKind(kind ast.DefinitionKind) bool {
	return kind == ast.Interface || kind == ast.Union
}

// formatGraphQLType reconstructs the GraphQL type notation string from an AST
// type (e.g. "[ChatbotOrder!]" or "Int!"). Used in generated setArg calls.
func formatGraphQLType(t *ast.Type) string {
	if t == nil {
		return ""
	}

	var result string
	if t.Elem != nil {
		result = "[" + formatGraphQLType(t.Elem) + "]"
	} else {
		result = t.NamedType
	}

	if t.NonNull {
		result += "!"
	}

	return result
}
