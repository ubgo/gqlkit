# TypeScript Code Generation — Technical Reference

This document is the full technical reference for the gqlkit TypeScript SDK code generator. It covers architecture, configuration, type mapping, every generation module, templates, the runtime library, CLI usage, and the generated output structure.

This page is for contributors changing the generator. To use a generated TypeScript SDK, read the [TypeScript guides](./README.md#typescript-guides) instead, starting with [selecting fields](./typescript/selecting-fields.md); to run the generator, see [Generate a TypeScript SDK](./cli/generate-ts.md).


## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Pipeline Stages](#pipeline-stages)
- [CLI Usage](#cli-usage)
- [Configuration](#configuration)
  - [Generator Config (Go flags)](#generator-config-go-flags)
  - [Client Config (config.jsonc)](#client-config-configjsonc)
  - [Scalar Bindings](#scalar-bindings)
  - [External Package Imports](#external-package-imports)
- [Type Mapping System](#type-mapping-system)
  - [TSTypeMap](#tstypemap)
  - [Built-in Type Mappings](#built-in-type-mappings)
  - [Type Resolution Functions](#type-resolution-functions)
  - [Import Classification](#import-classification)
- [Generation Modules](#generation-modules)
  - [Builder Index](#1-builder-index)
  - [Scalars](#2-scalars)
  - [Enums](#3-enums)
  - [Types (Interfaces)](#4-types-interfaces)
  - [Input Types](#5-input-types)
  - [Field Selectors](#6-field-selectors)
  - [Operation Builders](#7-operation-builders)
- [Templates](#templates)
- [Runtime Library (gqlkit-ts)](#runtime-library-gqlkit-ts)
  - [FieldSelection](#fieldselection)
  - [BaseBuilder](#basebuilder)
  - [GraphQLClient](#graphqlclient)
  - [GraphQLErrors](#graphqlerrors)
- [Generated Output Structure](#generated-output-structure)
- [Data Flow Walkthrough](#data-flow-walkthrough)
- [Design Patterns](#design-patterns)
- [Error Handling](#error-handling)


## Architecture Overview

```
                     ┌─────────────────────┐
                     │  GraphQL Schema SDL  │
                     └─────────┬───────────┘
                               │
                     ┌─────────▼───────────┐
                     │  gqlparser (AST)     │
                     └─────────┬───────────┘
                               │
  ┌──────────────┐   ┌────────▼─────────┐   ┌──────────────────┐
  │ config.jsonc │──▶│    Generator      │◀──│  9 Go Templates  │
  │ (bindings)   │   │  (clientgents)    │   │  (embed.FS)      │
  └──────────────┘   └────────┬─────────┘   └──────────────────┘
                               │
                     ┌─────────▼───────────┐
                     │   TSWriter           │
                     │   (file I/O)         │
                     └─────────┬───────────┘
                               │
              ┌────────────────▼────────────────┐
              │     Generated TypeScript SDK     │
              │                                  │
              │  sdk/                             │
              │  ├── builder/index.ts             │
              │  ├── scalars/index.ts             │
              │  ├── enums/index.ts               │
              │  ├── types/index.ts               │
              │  ├── inputs/index.ts              │
              │  ├── fields/*.ts + index.ts       │
              │  ├── queries/*.ts + root + index  │
              │  └── mutations/*.ts + root + index│
              └─────────────┬───────────────────┘
                            │
                  ┌─────────▼───────────┐
                  │  gqlkit-ts runtime   │
                  │  (npm package)       │
                  └─────────────────────┘
```

The system is implemented in the Go package `gqlkit/pkg/clientgents`. It reads a GraphQL schema SDL file and a JSONC configuration file, then generates a structured TypeScript SDK organized into functional modules. The generated code depends on the `gqlkit-ts` npm package as a runtime library.

### Source Files

| File | Purpose |
|------|---------|
| `clientgents.go` | Generator struct, `New()` constructor, `Generate()` orchestrator, `PrunedFiles()` |
| `config.go` | `Config` / `ClientConfig` structs, JSONC loading and validation |
| `ts_type_map.go` | `TSTypeMap`, `TSBinding` (with codec fields), list and `\| null` rendering helpers |
| `nullmode.go` | `NullMode` (`null`, `null-undefined`) for nullable output positions, and its overrides |
| `codec.go` | Scalar codec wiring: which fields and arguments reach a decoder or encoder |
| `member_names.go` | `memberNamer`: `_` suffix for schema names that clash with generated class members |
| `constants.go` | Import-path helpers (`dirImport`, `siblingImport`) that add the `.js` suffix |
| `scalar_gen.go` | Scalar type aliases, the shared type resolver (`fieldTSType`, `namedTSType`), import collectors |
| `enum_gen.go` | Enum generation |
| `types_gen.go` | Interface generation for Object/Interface types, aliases for unions |
| `input_types_gen.go` | Interface generation for InputObject types |
| `field_sel_gen.go` | Field selector class generation, including `on<Type>()` variants for unions and interfaces |
| `op_gen.go` | Operation builder (query/mutation) generation |
| `writer.go` | `TSWriter` file I/O, the generated-code banner, `Prune` |
| `errors.go` | Sentinel errors |
| `template/*.tmpl` | 9 Go text/template files |


## Pipeline Stages

The `Generate()` method runs these steps in order:

| Step | Method | Output |
|------|--------|--------|
| 1 | `generateBuilderIndex()` | `builder/index.ts` |
| 2 | `generateScalars()` | `scalars/index.ts` |
| 3 | `generateEnums()` | `enums/index.ts` |
| 4 | `generateTypes()` | `types/index.ts` |
| 5 | `generateInputTypes()` | `inputs/index.ts` |
| 6 | `generateFieldSelectionFiles()` | `fields/*.ts` + `fields/index.ts` |
| 7 | `generateOperationFiles()` | `queries/*.ts`, `queries/root.ts`, `queries/index.ts`, `mutations/*.ts`, `mutations/root.ts`, `mutations/index.ts` |
| 8 | `TSWriter.Prune()` (only with `Prune`) | deletes banner-carrying files this run did not write |

Each step walks the parsed GraphQL AST, builds template data structs, executes the appropriate template, and writes the result to disk via `TSWriter`, which prepends the `// Code generated by gqlkit. DO NOT EDIT.` banner to every file. Every relative import ends in `.js` (`../builder/index.js`, `./todo.js`), so the output compiles under Node ESM (`nodenext`), bundlers and CommonJS alike.


## CLI Usage

```bash
gqlkit generate-ts \
  --schema path/to/schema.graphql \
  --config path/to/config.jsonc \
  --output ./sdk
```

| Flag | Short | Required | Default | Description |
|------|-------|----------|---------|-------------|
| `--schema` | `-s` | Yes | — | Path to GraphQL SDL file, or a quoted glob |
| `--config` | `-c` | No | — | Path to config.jsonc file |
| `--output` | `-o` | No | `./sdk` | Output directory for generated SDK |
| `--prune` | — | No | off | Delete stale files that carry the gqlkit banner |
| `--dry-run` | — | No | off | With `--prune`: list what would be deleted |

### Programmatic Usage

```go
config := &clientgents.Config{
    SchemaPath: "schema.graphql",
    OutputDir:  "./sdk",
    ConfigPath: "config.jsonc",
}

gen, err := clientgents.New(config)
if err != nil {
    log.Fatal(err)
}

if err := gen.Generate(); err != nil {
    log.Fatal(err)
}
```


## Configuration

### Generator Config (Go flags)

```go
type Config struct {
    SchemaPath  string // Required: path to .graphql SDL file or glob
    OutputDir   string // Optional: defaults to "./sdk"
    ConfigPath  string // Optional: path to config.jsonc
    Prune       bool   // Delete stale banner-carrying files after generating
    PruneDryRun bool   // Report what Prune would delete; requires Prune
}
```

`Validate()` enforces the required schema path, applies the `./sdk` default and rejects `PruneDryRun` without `Prune`.

### Client Config (config.jsonc)

The config file uses JSONC format (JSON with comments), parsed via `tidwall/jsonc`.

```go
type ClientConfig struct {
    Bindings          ConfigTSBindings    `json:"bindings"`
    Nullable          NullMode            `json:"nullable,omitempty"`          // "null" (default) or "null-undefined"
    NullableOverrides map[string]NullMode `json:"nullableOverrides,omitempty"` // per type ("Todo") or field ("Todo.priority")
}
```

`nullable` controls how nullable output positions are typed: `T | null` by default, or `T | null | undefined` for code written against older SDKs. A field key in `nullableOverrides` beats a type key, which beats `nullable`; keys are checked against the schema in `New`. List elements stay `| null`, and inputs and arguments are unaffected.

### Scalar Bindings

Bindings map custom GraphQL scalars to TypeScript types. Each binding is either a plain string (inline type alias) or an object with `type` and optionally `import` and a codec.

```go
type TSBinding struct {
    Type        string `json:"type"`                  // TypeScript type name
    Import      string `json:"import,omitempty"`      // npm package or path to re-export Type from
    Decode      string `json:"decode,omitempty"`      // export of CodecImport: wire value → Type
    Encode      string `json:"encode,omitempty"`      // export of CodecImport: Type → wire value
    CodecImport string `json:"codecImport,omitempty"` // module exporting Decode / Encode
}

type ConfigTSBindings map[string]TSBinding
```

`TSBinding` implements a custom `UnmarshalJSON` that accepts both forms, and `Validate` rejects an object without `type` (such as a Go-style `{ "model": ... }` binding) and a codec without its import:

```jsonc
{
  "bindings": {
    // Plain string, produces: export type Cursor = string;
    "Cursor": "string",

    // Object with import, produces: export type { DateTime } from "luxon";
    // decode/encode add a codec: responses are converted to DateTime and
    // arguments back to the wire format.
    "DateTime": { "type": "DateTime", "import": "luxon", "decode": "decodeDateTime", "encode": "encodeDateTime", "codecImport": "../../codecs/datetime.js" },

    // Object with import + rename, produces: export type { JsonValue as JSON } from "type-fest";
    "JSON": { "type": "JsonValue", "import": "type-fest" }
  }
}
```

Scalars not listed in bindings fall back to `any`. Without a codec a binding changes only the type: the value at run time is whatever JSON the server sent.

### External Package Imports

When a binding has an `import` field:

1. **`scalars/index.ts`** emits a re-export from the npm package instead of a type alias (and, with a codec, re-exports its `decode<Scalar>` / `encode<Scalar>` functions)
2. **All other files** import the scalar by its GraphQL name from `../scalars/index.js`

The `Merge` function handles this:

```go
func (m TSTypeMap) Merge(bindings ConfigTSBindings) TSTypeMap {
    for k, v := range bindings {
        if v.Import != "" {
            m[k] = k  // use scalar name — re-exported from scalars/
        } else {
            m[k] = v.Type
        }
    }
    return m
}
```

When `Import` is set, the type map stores the scalar name itself as the value. This ensures:
- `isSimpleTSType()` returns `false`, so the type is not inlined
- `collectScalarRefs()` collects it, so the scalar is imported from `../scalars/index.js`
- `fieldTSType()` returns the scalar name, used in interface declarations

The real `scalars/index.ts` of `example-ts`, and how `types/index.ts` uses it, are under [Generation Modules](#generation-modules).


## Type Mapping System

### TSTypeMap

```go
type TSTypeMap map[string]string
```

Maps GraphQL scalar names to their TypeScript type string. Built from `BuiltInTSTypes()` merged with user config bindings.

### Built-in Type Mappings

```go
func BuiltInTSTypes() TSTypeMap {
    return TSTypeMap{
        "String":  "string",
        "Int":     "number",
        "Int64":   "number",
        "Int32":   "number",
        "Float":   "number",
        "Float64": "number",
        "Float32": "number",
        "Boolean": "boolean",
        "ID":      "string",
        "Uint":    "number",
        "Uint64":  "number",
        "Uint32":  "number",
    }
}
```

Config bindings can override any of these.

### Type Resolution Functions

One resolver renders every position (object and input fields, arguments and operation returns), so imports and types cannot disagree.

#### `fieldTSType(t *ast.Type) string`

Renders a full GraphQL type reference: list wrapping through `tsListOf`, nullability through `tsOrNull` (`T | null`), and the named leaf through `namedTSType`.

```
Int!       → number
String     → string | null
[Todo!]!   → Todo[]
[Todo]     → (Todo | null)[] | null
```

For nullable output positions, `outputTSType` and `outputNullSuffix` apply the configured `NullMode`, so `"null-undefined"` gives `string | null | undefined`.

#### `namedTSType(name string) string`

Maps one named GraphQL type to TypeScript:

| Kind | Result |
|------|--------|
| Scalar with a simple binding (`string`, `number`, `boolean`, `any`) | inlined |
| Other scalar | its alias from `scalars/index.ts` |
| Enum, Object, Interface, InputObject | its own name |
| Union | its alias from `types/index.ts` |
| Unknown name | the binding, else `any` |

#### `isSimpleTSType(t string) bool`

Returns `true` for `"string"`, `"number"`, `"boolean"`, `"any"`. Simple types are inlined and don't need imports from `../scalars/index.js`.

#### `toKebabCase(s string) string`

PascalCase to kebab-case for TypeScript file naming. Handles acronyms gracefully:

```
"ChatbotConnection" → "chatbot-connection"
"URLParser"         → "url-parser"
"TodoEdge"          → "todo-edge"
```

#### `memberNamer`

GraphQL field and argument names are arbitrary, but generated classes also define their own members (`constructor`, `select`, `execute`, `executeRaw`, `on<Type>`, …). A schema name that would clash gets a `_` suffix (`select` → `select_`, `constructor` → `constructor_`). Runtime state lives in `#private` fields, which can never clash with a method name.

### Import Classification

#### `classifyTypeImport(t *ast.Type, scalars, enums, inputs map[string]bool)`

Recursively walks a GraphQL type tree and sorts each named reference into the correct import bucket. Invariant: a name lands in a bucket exactly when `fieldTSType` renders it as a bare reference to that module's export, so no import is unused or missing.

| Schema Kind | Import Bucket | Import Path |
|-------------|---------------|-------------|
| Scalar (non-simple) | `scalars` | `../scalars/index.js` |
| Enum | `enums` | `../enums/index.js` |
| InputObject | `inputs` | `../inputs/index.js` |
| Scalar (simple) | — | (inlined, no import) |

#### `collectScalarRefs` / `collectEnumRefs`

Recursively collect custom scalar and enum references from a type.

#### `collectFieldSelectorTypeImports(def *ast.Definition) (enumImports, scalarImports []string)`

Collects enum and scalar imports needed by a specific field selector file.

#### `collectEnumImportNames(defs) / collectScalarImportNames(defs) []string`

Collect the enum and scalar names referenced by the emitted fields of a set of definitions, for `types/index.ts` and `inputs/index.ts`. Sorted for deterministic output.


## Generation Modules

### 1. Builder Index

**Method:** `generateBuilderIndex()`
**Output:** `builder/index.ts`

Static file that re-exports runtime types from the `gqlkit-ts` npm package:

```typescript
// Code generated by gqlkit. DO NOT EDIT.

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
export interface SelectionRequired {
  readonly "select() is required before execute()": never;
}

export { GraphQLErrors } from "gqlkit-ts";
export type { ArgEncoder, InputEncodePlan } from "gqlkit-ts";
```

This indirection lets generated code import everything runtime-related from `../builder/index.js`. `SelectionRequired` is the default result type of a builder whose object result has no selection yet; `execute()`, `executeRaw()` and `getOpFragment()` declare a `this` type that is `never` until `select()` narrows it, so calling them early is a compile error (TS2684), with a run-time guard for JavaScript callers and empty selections.

### 2. Scalars

**Method:** `generateScalars()`
**Output:** `scalars/index.ts`
**Template:** `ts_scalar.tmpl`

#### Data Struct

```go
// TSScalar holds the metadata for a single TypeScript type alias generated from
// a GraphQL scalar (e.g. "export type DateTime = string;").
type TSScalar struct {
	Name       string
	TSType     string
	Import     string // npm package to import from (empty = inline alias)
	ImportType string // type name to import from the package
	// Codec re-export (all empty without a codec): the user's functions from
	// CodecImport, renamed to the generated decode<Name> / encode<Name>.
	CodecImport string
	Decode      string // user's decode export
	DecoderName string // e.g. "decodeDateTime"
	Encode      string // user's encode export
	EncoderName string // e.g. "encodeDateTime"
}
```

#### Process

1. Iterate every scalar in the schema, built-ins included, so other files can reference `Int`, `ID` and friends through one module
2. Resolve the TS type via `g.tsTypeMap.Get(def.Name)`
3. Special case: the GraphQL `String` scalar is exported as `GqlString` (`scalarAliasName`) so it does not shadow the global `String`
4. If the binding has an external import, set `Import` and `ImportType`
5. If the binding has a codec, set `CodecImport`, `Decode` / `DecoderName` and `Encode` / `EncoderName`; the template re-exports them as `decode<Scalar>` / `encode<Scalar>`
6. Sort alphabetically by name
7. Execute template

#### Generated Output

```typescript
// Code generated by gqlkit. DO NOT EDIT.

export type Boolean = boolean;
export type Cursor = string;
export type { DateTime } from "luxon";
export { decodeDateTime as decodeDateTime, encodeDateTime as encodeDateTime } from "../../codecs/datetime.js";
export type Float = number;
export type GqlString = string;
export type ID = string;
export type Int = number;
export type { JsonValue as JSON } from "type-fest";
export type Metadata = any;
```

### 3. Enums

**Method:** `generateEnums()`
**Output:** `enums/index.ts`
**Template:** `ts_enums.tmpl`

#### Data Struct

```go
// TSEnumDef holds the metadata for a single TypeScript enum generated from a
// GraphQL enum type. Values are the raw enum value strings.
type TSEnumDef struct {
	Name   string
	Values []string
}
```

#### Process

1. Collect all non-built-in enum types
2. Extract enum values
3. Sort alphabetically
4. Execute template

#### Generated Output

```typescript
// Code generated by gqlkit. DO NOT EDIT.

export enum Role {
  ADMIN = "ADMIN",
  USER = "USER",
  GUEST = "GUEST",
}
```

TypeScript string enums are used (value = name) to match GraphQL enum semantics.

### 4. Types (Interfaces)

**Method:** `generateTypes()`
**Output:** `types/index.ts`
**Template:** `ts_types.tmpl`

#### Data Structs

```go
// TSTypeDef holds the metadata for a single TypeScript interface generated from
// a GraphQL object, interface or input object type.
type TSTypeDef struct {
	Name   string
	Fields []TSFieldDef
}

// TSFieldDef holds the metadata for a single field within a TS interface.
type TSFieldDef struct {
	Name     string // GraphQL field name, used verbatim as the TS property name
	Optional bool   // nullable in GraphQL → rendered as an optional `name?:` property
	TSType   string // TS type text from fieldTSType
}

// TSUnionDef is one GraphQL union, rendered as a TS union alias of its
// members' interfaces so fields typed with it are not `any`.
type TSUnionDef struct {
	Name    string
	Members string // member names joined with " | ", sorted
}

// TSTypesData is the template data passed to ts_types.tmpl to generate types/index.ts.
type TSTypesData struct {
	EnumImports   []string
	ScalarImports []string
	Types         []TSTypeDef
	Unions        []TSUnionDef
}
```

#### Process

1. Take the generated Object and Interface definitions (`objectTypeDefs()`: no built-ins, no `__` names, no root types)
2. For each field, resolve the TS type via `fieldTypeFor()`: nullable output fields become `T | null` (or `T | null | undefined` under the `null-undefined` NullMode); fields are never optional properties, because the server always sends a selected field
3. Collect enum and scalar imports via `collectEnumImportNames()` / `collectScalarImportNames()`
4. Emit one `export type X = A | B;` alias per union (`TSUnionDef`)
5. Execute template

#### Generated Output

```typescript
// Code generated by gqlkit. DO NOT EDIT.

import type { Role } from "../enums/index.js";

import type { DateTime, Metadata } from "../scalars/index.js";


export interface Node {
  id: string;
}

export interface PageInfo {
  hasNextPage: boolean;
  hasPreviousPage: boolean;
  startCursor?: string | null;
  endCursor?: string | null;
}

export interface Todo {
  id: string;
  text: string;
  done: boolean;
  priority?: number | null;
  tags: string[];
  user: User;
  createdAt: DateTime;
  metadata?: Metadata | null;
  parent?: Todo | null;
}

export interface TodoConnection {
  edges: TodoEdge[];
  pageInfo: PageInfo;
  totalCount: number;
}

export interface TodoEdge {
  cursor: string;
  node: Todo;
}

export interface User {
  id: string;
  name: string;
  email?: string | null;
  role: Role;
}

export type SearchResult = Todo | User;
```

### 5. Input Types

**Method:** `generateInputTypes()`
**Output:** `inputs/index.ts`
**Template:** `ts_inputs.tmpl`

Same approach as types, with its own template (`ts_inputs.tmpl`), for `ast.InputObject` definitions. Nullable input fields are optional and accept `null` (`field?: T | null`), so a caller can omit a field or clear it explicitly. When a codec has an encoder, the file also exports an encode plan per input type (`TSEncodePlan`) that the runtime walks to encode nested values.

#### Data Struct

```go
// TSInputsData is the template data passed to ts_inputs.tmpl to generate
// inputs/index.ts. It includes enum and scalar import lists needed by the
// input type interfaces.
type TSInputsData struct {
	EnumImports   []string
	ScalarImports []string
	Types         []TSTypeDef
	// Codec only (empty without one): encoders imported from scalars/ and
	// one InputEncodePlan per input type that reaches an encoder.
	EncoderImports []string
	Plans          []TSEncodePlan
}

// TSEncodePlan is the generated InputEncodePlan constant of one input type:
// only fields whose values need encoding are listed (the runtime copies the
// rest as-is).
type TSEncodePlan struct {
	Name   string // e.g. "TodoFilterEncodePlan"
	Fields []TSEncodePlanField
}

// TSEncodePlanField is one entry of an encode plan.
type TSEncodePlanField struct {
	Field string // input field name
	Expr  string // ArgEncoder expression (see argEncoderExpr)
}
```

#### Generated Output

```typescript
// Code generated by gqlkit. DO NOT EDIT.



export interface NewTodo {
  text: string;
  userId: string;
  priority?: number | null;
  tags?: string[] | null;
}

export interface PaginationInput {
  limit: number;
  offset: number;
}

export interface TodoFilter {
  textContains?: string | null;
  done?: boolean | null;
  minPriority?: number | null;
  hasTag?: string | null;
}

export interface UpdateTodoInput {
  text?: string | null;
  done?: boolean | null;
  priority?: number | null;
  tags?: string[] | null;
}
```

### 6. Field Selectors

**Method:** `generateFieldSelectionFiles()`
**Output:** `fields/{kebab-name}.ts` + `fields/index.ts`
**Templates:** `ts_field_selector.tmpl`, `ts_field_selector_index.tmpl`

Field selectors are the core type-safety mechanism. Each GraphQL object, interface and union type gets a selector class that uses TypeScript generics with intersection types to build up a fully-typed result. State lives in a `#private` field (`#selection`), and every method returns a new, correctly typed selector instead of casting `this`.

#### Data Structs

```go
// TSFieldSelectorData is the template data for generating a single TypeScript
// field selector class. Each GraphQL composite type gets its own selector
// class wrapping a FieldSelection, with typed methods for each field and, for
// interfaces and unions, one on<Type>() inline-fragment method per possible type.
type TSFieldSelectorData struct {
	TypeName     string
	SelectorName string
	// IsAbstract marks an interface or union selector: its constructor adds
	// __typename and it carries Variants.
	IsAbstract bool
	// DefaultResult is the class's default T: "{}" for an object; for an
	// abstract type the discriminated { __typename } union (variantsDefault).
	DefaultResult string
	Variants      []TSFieldVariant
	Fields        []TSFieldSelField
	Imports       []TSFieldImport
	EnumImports   []string
	ScalarImports []string
	// DecoderImports are the generated decoders (values) this selector passes
	// to addField; empty without a codec.
	DecoderImports []string
}

// TSFieldSelField describes one field on a TypeScript field selector class.
// Scalar fields produce addField calls; object fields produce nested selector
// callbacks with the appropriate generic type parameter.
type TSFieldSelField struct {
	FieldName      string // original GraphQL field name (camelCase)
	MethodName     string // field name, suffixed only if it clashes with a selector member (see memberNamer)
	IsObject       bool
	NestedSelector string // e.g., "UserFields"
	// ResultType is the property type added to the selection result. A
	// selected field is always present, so it is never optional; nullable
	// positions admit null instead. Leaves carry their TS type ("string |
	// null", "(number | null)[]"); object fields carry the same structure
	// around the callback's generic ("U[] | null").
	ResultType string
	// DecodeArgs is appended to the addField call for a leaf whose scalar has
	// a decoder: ", decodeDateTime, 1" (decoder, list depth). Empty otherwise.
	DecodeArgs string
}

// TSFieldVariant is one on<Type>() method of an abstract selector: an inline
// fragment on a possible type, narrowing the result's matching member.
type TSFieldVariant struct {
	TypeName     string // possible type, e.g. "Todo" (also its __typename)
	MethodName   string // e.g. "onTodo"; suffixed if it clashes (memberNamer)
	SelectorName string // e.g. "TodoFields"
}

// TSFieldImport describes one import of another field selector class needed
// for nested object selection methods.
type TSFieldImport struct {
	ClassName string // e.g., "UserFields"
	FilePath  string // e.g., "./user.js" (see siblingImport)
}

// TSFieldIndexEntry holds one entry for the fields/index.ts barrel file that
// re-exports all field selector classes.
type TSFieldIndexEntry struct {
	SelectorName string
	FileName     string
}
```

#### Process

1. Take the selector definitions (`selectorTypeDefs()`: objects, interfaces and unions, excluding built-ins, `__` names and root types)
2. For each type, call `buildFieldSelectorData(def)`:
   - Method names go through a `memberNamer`, so a schema name that clashes with a class member gets a `_` suffix (`select_`)
   - For an interface or union (`IsAbstract`): one `on<Type>()` variant per possible type (`TSFieldVariant`), each adding a `... on Type` fragment; the constructor always selects `__typename`, and the result type is a union discriminated on it
   - For each field (skip `__` fields): if the field type is composite (`IsObject=true`), set `NestedSelector` and the narrowed `ResultType`, including when the field refers to its own type (`Todo.parent: Todo`), which only skips the self-import; otherwise set `ResultType` via `fieldTypeFor()` and, when a codec applies, `DecodeArgs`
   - Collect imports for other selector classes, enums, scalars and decoders
3. Execute `ts_field_selector.tmpl` for each → `fields/{kebab-name}.ts`
4. Execute `ts_field_selector_index.tmpl` → `fields/index.ts`

#### Generated Output — Scalar Fields

```typescript
// Code generated by gqlkit. DO NOT EDIT.

import { FieldSelection } from "../builder/index.js";
import type { Role } from "../enums/index.js";

/** UserFields provides type-safe field selection for User */
export class UserFields<T extends object = {}> {
  readonly #selection: FieldSelection;

  constructor(selection: FieldSelection) {
    this.#selection = selection;
  }

  id(): UserFields<T & { id: string }> {
    this.#selection.addField("id");
    return new UserFields<T & { id: string }>(this.#selection);
  }

  name(): UserFields<T & { name: string }> {
    this.#selection.addField("name");
    return new UserFields<T & { name: string }>(this.#selection);
  }

  email(): UserFields<T & { email: string | null }> {
    this.#selection.addField("email");
    return new UserFields<T & { email: string | null }>(this.#selection);
  }

  role(): UserFields<T & { role: Role }> {
    this.#selection.addField("role");
    return new UserFields<T & { role: Role }>(this.#selection);
  }
}
```

#### Generated Output — Nested Object Fields

```typescript
// Code generated by gqlkit. DO NOT EDIT.

import { FieldSelection } from "../builder/index.js";
import { UserFields } from "./user.js";
import type { DateTime, Metadata } from "../scalars/index.js";
import { decodeDateTime } from "../scalars/index.js";

/** TodoFields provides type-safe field selection for Todo */
export class TodoFields<T extends object = {}> {
  readonly #selection: FieldSelection;

  constructor(selection: FieldSelection) {
    this.#selection = selection;
  }

  id(): TodoFields<T & { id: string }> {
    this.#selection.addField("id");
    return new TodoFields<T & { id: string }>(this.#selection);
  }

  text(): TodoFields<T & { text: string }> {
    this.#selection.addField("text");
    return new TodoFields<T & { text: string }>(this.#selection);
  }

  done(): TodoFields<T & { done: boolean }> {
    this.#selection.addField("done");
    return new TodoFields<T & { done: boolean }>(this.#selection);
  }

  priority(): TodoFields<T & { priority: number | null }> {
    this.#selection.addField("priority");
    return new TodoFields<T & { priority: number | null }>(this.#selection);
  }

  tags(): TodoFields<T & { tags: string[] }> {
    this.#selection.addField("tags");
    return new TodoFields<T & { tags: string[] }>(this.#selection);
  }

  user<U extends object>(selector: (f: UserFields) => UserFields<U>): TodoFields<T & { user: U }> {
    const child = new FieldSelection();
    selector(new UserFields(child));
    this.#selection.addChild("user", child);
    return new TodoFields<T & { user: U }>(this.#selection);
  }

  createdAt(): TodoFields<T & { createdAt: DateTime }> {
    this.#selection.addField("createdAt", decodeDateTime, 0);
    return new TodoFields<T & { createdAt: DateTime }>(this.#selection);
  }

  metadata(): TodoFields<T & { metadata: Metadata | null }> {
    this.#selection.addField("metadata");
    return new TodoFields<T & { metadata: Metadata | null }>(this.#selection);
  }

  parent<U extends object>(selector: (f: TodoFields) => TodoFields<U>): TodoFields<T & { parent: U | null }> {
    const child = new FieldSelection();
    selector(new TodoFields(child));
    this.#selection.addChild("parent", child);
    return new TodoFields<T & { parent: U | null }>(this.#selection);
  }
}
```

#### Generated Output — Barrel File

```typescript
// Code generated by gqlkit. DO NOT EDIT.

export { NodeFields } from "./node.js";
export { PageInfoFields } from "./page-info.js";
export { SearchResultFields } from "./search-result.js";
export { TodoFields } from "./todo.js";
export { TodoConnectionFields } from "./todo-connection.js";
export { TodoEdgeFields } from "./todo-edge.js";
export { UserFields } from "./user.js";
```

### 7. Operation Builders

**Method:** `generateOperationFiles()`
**Output:** `{queries|mutations}/{kebab-name}.ts`, `{queries|mutations}/root.ts`, `{queries|mutations}/index.ts`
**Templates:** `ts_operation.tmpl`, `ts_operation_root.tmpl`, `ts_operation_index.tmpl`

#### Data Structs

```go
// TSArgumentData describes a single argument on a TypeScript operation builder,
// including the original GraphQL name, the TS method name, and the TS/GraphQL
// type strings.
type TSArgumentData struct {
	ArgName     string // original GraphQL arg name
	MethodName  string // setter name: the arg name, suffixed only if it clashes with a builder member
	TSType      string // TypeScript type for the arg
	GraphQLType string // e.g., "Cursor", "Int", "[ChatbotOrder!]"
	// EncodeArg is appended to the setArg call when the arg's values need
	// encoding: ", { scalar: encodeDateTime, listDepth: 0 }". Empty otherwise.
	EncodeArg string
}

// TSOperationBuilderData holds all the template data for generating a single
// TypeScript query or mutation builder class. Each builder wraps BaseBuilder
// and adds typed argument setters, an optional select method, and an execute method.
type TSOperationBuilderData struct {
	BuilderName string
	OpType      opKind // written verbatim into the BaseBuilder call
	FieldName   string // original GraphQL field name
	MethodName  string // PascalCase operation name
	Arguments   []TSArgumentData
	HasSelect   bool // true for composite returns (object, interface, union), which need a selection set
	IsAbstract  bool // interface/union return: the builder requests __typename up front
	// NeedsSelect marks an object (not abstract) return: TResult starts as
	// SelectionRequired and execute/executeRaw/getOpFragment are typed to be
	// uncallable until select(), with a runtime guard behind it for batch()
	// and plain-JS callers. An abstract return is valid unselected ({ __typename }).
	NeedsSelect bool
	// Decodes routes execute()'s result through decodeResult: a select()
	// return that can reach a decoder, or a scalar return with one.
	Decodes bool
	// ResultDecoder is "decodeDateTime, 0" for a scalar return whose scalar
	// has a decoder (passed to setResultDecoder); empty otherwise.
	ResultDecoder  string
	SelectorName   string // e.g., "ChatbotConnectionFields"; empty when HasSelect is false
	ReturnType     string // TypeScript return type of execute() when HasSelect is false
	DefaultGeneric string // TResult before select(): the full type, e.g. "Todo | null", "(Todo | null)[]"
	SelectWrapper  string // TResult after select(): the same structure around T, e.g. "T[] | null"
}

// TSOperationImport describes a single import statement needed in an operation
// builder file, with optional type-only flag for TS "import type" syntax.
type TSOperationImport struct {
	Names    []string
	Path     string
	TypeOnly bool
}

// TSOperationTemplateData wraps the builder data and its imports for passing
// to the ts_operation.tmpl template.
type TSOperationTemplateData struct {
	Data    TSOperationBuilderData
	Imports []TSOperationImport
}

// TSOperationRootBuilder holds the data for one builder factory method in a
// root file (QueryRoot or MutationRoot).
type TSOperationRootBuilder struct {
	BuilderName string
	FieldName   string // GraphQL field name
	MethodName  string // factory method: the field name, suffixed only on a root-member clash
	FileName    string
}

// TSOperationRootData is the template data for generating queries/root.ts or
// mutations/root.ts, which contain the root class with factory methods.
type TSOperationRootData struct {
	ClassName string
	OpType    opKind
	Builders  []TSOperationRootBuilder
}

// TSOperationIndexData is the template data for generating the barrel
// index.ts file that re-exports the root class and all individual builders.
type TSOperationIndexData struct {
	RootClassName string
	Builders      []TSOperationRootBuilder
}
```

#### Builder Name Convention

| Operation Type | Suffix | Example |
|----------------|--------|---------|
| Query | `Builder` | `TodoBuilder`, `UsersBuilder` |
| Mutation | `MutationBuilder` | `CreateTodoMutationBuilder` |

A builder for an object return is generic over its result, `TodosBuilder<TResult = SelectionRequired>`; `select()` returns a new builder whose `TResult` is the narrowed type, wrapped as below.

#### SelectWrapper Logic

The `SelectWrapper` determines how the generic type `T` is wrapped in the `select()` method return. It is rendered by the same `outputTSType()` as every other output position, so nested lists follow the same rule:

| List return | Nullable return | SelectWrapper | Example |
|:---:|:---:|---|---|
| false | false | `T` | `T` |
| false | true | `T \| null` | `Todo \| null` |
| true | false | `T[]` | `Todo[]` |
| true | true | `T[] \| null` | `Todo[] \| null` |

#### Import Collection

`collectOperationImports()` gathers all imports for a single builder file:

1. Always: `BaseBuilder`, `GraphQLClient` and the type `RequestOptions` from `../builder/index.js`; also `SelectionRequired` when the result needs a selection, and `GraphQLErrors` when a codec must decode partial data
2. If `HasSelect`: `SelectorName` from `../fields/index.js`
3. Walk the return type and all argument types through `classifyTypeImport()`:
   - Scalar (non-simple) → `import type { ... } from "../scalars/index.js"`
   - Enum → `import type { ... } from "../enums/index.js"`
   - InputObject → `import type { ... } from "../inputs/index.js"`
4. With codecs: encoder functions and input encode plans, as value imports

#### Generated Output — Query Builder (with select)

```typescript
// Code generated by gqlkit. DO NOT EDIT.

import { BaseBuilder, GraphQLClient, GraphQLErrors } from "../builder/index.js";
import type { RequestOptions, SelectionRequired } from "../builder/index.js";
import { TodoFields } from "../fields/index.js";
import type { PaginationInput, TodoFilter } from "../inputs/index.js";

/** TodosBuilder builds a query for todos */
export class TodosBuilder<TResult = SelectionRequired> {
  readonly #builder: BaseBuilder;

  /** builder is internal: select() passes the shared state to the narrowed instance */
  constructor(client: GraphQLClient, builder?: BaseBuilder) {
    this.#builder = builder ?? new BaseBuilder(client, "query", "Todos", "todos");
  }

  /** Sets the filter argument */
  filter(v: TodoFilter | null): this {
    this.#builder.setArg("filter", v, "TodoFilter");
    return this;
  }

  /** Sets the pagination argument */
  pagination(v: PaginationInput | null): this {
    this.#builder.setArg("pagination", v, "PaginationInput");
    return this;
  }

  /** Configures which fields to return */
  select<T extends object>(selector: (f: TodoFields) => TodoFields<T>): TodosBuilder<T[]> {
    selector(new TodoFields(this.#builder.getSelection()));
    return new TodosBuilder<T[]>(this.#builder.getClient(), this.#builder);
  }

  /** Execute runs the query and returns the result; request.signal cancels it */
  async execute(this: TResult extends SelectionRequired ? never : this, request?: RequestOptions): Promise<TResult> {
    this.#requireSelection();
    const query = this.#builder.buildQuery();
    const variables = this.#builder.getVariables();

    let response: { todos: TResult };
    try {
      response = await this.#builder
        .getClient()
        .execute<{ todos: TResult }>(query, variables, request);
    } catch (e) {
      // Partial data arrives on the error as wire values; decode it first.
      if (e instanceof GraphQLErrors) this.#builder.decodePartial(e);
      throw e;
    }

    return this.#builder.decodeResult(response.todos);
  }

  /** ExecuteRaw runs the query and returns the raw response; request.signal cancels it */
  async executeRaw(this: TResult extends SelectionRequired ? never : this, request?: RequestOptions): Promise<Record<string, unknown>> {
    this.#requireSelection();
    return await this.#builder
      .getClient()
      .execute<Record<string, unknown>>(this.#builder.buildQuery(), this.#builder.getVariables(), request);
  }

  /** Internal — exposes this operation as a fragment for batch() */
  getOpFragment(this: TResult extends SelectionRequired ? never : this, alias: string) {
    this.#requireSelection();
    return this.#builder.getOpFragment(alias);
  }

  /** Fails before sending: the server rejects todos without a selection set. */
  #requireSelection(): void {
    if (this.#builder.getSelection().isEmpty()) {
      throw new Error("gqlkit: todos returns an object type; call select() with at least one field before running it");
    }
  }
}
```

#### Generated Output — Scalar Query (no select)

```typescript
// Code generated by gqlkit. DO NOT EDIT.

import { BaseBuilder, GraphQLClient } from "../builder/index.js";
import type { RequestOptions } from "../builder/index.js";

/** PingBuilder builds a query for ping */
export class PingBuilder {
  readonly #builder: BaseBuilder;

  constructor(client: GraphQLClient) {
    this.#builder = new BaseBuilder(client, "query", "Ping", "ping");
  }

  /** Execute runs the query and returns the result; request.signal cancels it */
  async execute(request?: RequestOptions): Promise<string> {
    const query = this.#builder.buildQuery();
    const variables = this.#builder.getVariables();

    const response = await this.#builder
      .getClient()
      .execute<{ ping: string }>(query, variables, request);

    return response.ping;
  }

  /** ExecuteRaw runs the query and returns the raw response; request.signal cancels it */
  async executeRaw(request?: RequestOptions): Promise<Record<string, unknown>> {
    return await this.#builder
      .getClient()
      .execute<Record<string, unknown>>(this.#builder.buildQuery(), this.#builder.getVariables(), request);
  }

  /** Internal — exposes this operation as a fragment for batch() */
  getOpFragment(alias: string) {
    return this.#builder.getOpFragment(alias);
  }
}
```

#### Generated Output — Root File

```typescript
// Code generated by gqlkit. DO NOT EDIT.

import { GraphQLClient } from "../builder/index.js";
import { PingBuilder } from "./ping.js";
import { TodoBuilder } from "./todo.js";
import { TodosBuilder } from "./todos.js";
import { TodosConnectionBuilder } from "./todos-connection.js";
import { UserBuilder } from "./user.js";
import { UsersBuilder } from "./users.js";
import { SearchBuilder } from "./search.js";
import { EchoBuilder } from "./echo.js";
import { SumBuilder } from "./sum.js";
import { ServerInfoBuilder } from "./server-info.js";
import { FailBuilder } from "./fail.js";
import { RequestHeaderBuilder } from "./request-header.js";
import { NodeBuilder } from "./node.js";

/** QueryRoot is the entry point for querys */
export class QueryRoot {
  readonly #client: GraphQLClient;

  constructor(client: GraphQLClient) {
    this.#client = client;
  }

  ping(): PingBuilder {
    return new PingBuilder(this.#client);
  }

  todo(): TodoBuilder {
    return new TodoBuilder(this.#client);
  }

  todos(): TodosBuilder {
    return new TodosBuilder(this.#client);
  }

  todosConnection(): TodosConnectionBuilder {
    return new TodosConnectionBuilder(this.#client);
  }

  user(): UserBuilder {
    return new UserBuilder(this.#client);
  }

  users(): UsersBuilder {
    return new UsersBuilder(this.#client);
  }

  search(): SearchBuilder {
    return new SearchBuilder(this.#client);
  }

  echo(): EchoBuilder {
    return new EchoBuilder(this.#client);
  }

  sum(): SumBuilder {
    return new SumBuilder(this.#client);
  }

  serverInfo(): ServerInfoBuilder {
    return new ServerInfoBuilder(this.#client);
  }

  fail(): FailBuilder {
    return new FailBuilder(this.#client);
  }

  requestHeader(): RequestHeaderBuilder {
    return new RequestHeaderBuilder(this.#client);
  }

  node(): NodeBuilder {
    return new NodeBuilder(this.#client);
  }
}
```

#### Generated Output — Barrel Index

```typescript
// Code generated by gqlkit. DO NOT EDIT.

export { QueryRoot } from "./root.js";
export { PingBuilder } from "./ping.js";
export { TodoBuilder } from "./todo.js";
export { TodosBuilder } from "./todos.js";
export { TodosConnectionBuilder } from "./todos-connection.js";
export { UserBuilder } from "./user.js";
export { UsersBuilder } from "./users.js";
export { SearchBuilder } from "./search.js";
export { EchoBuilder } from "./echo.js";
export { SumBuilder } from "./sum.js";
export { ServerInfoBuilder } from "./server-info.js";
export { FailBuilder } from "./fail.js";
export { RequestHeaderBuilder } from "./request-header.js";
export { NodeBuilder } from "./node.js";
```


## Templates

All templates are embedded in the Go binary via `//go:embed template/*` and compiled with these helper functions:

| Function | Behavior |
|----------|----------|
| `joinComma` | `strings.Join(s, ", ")` |
| `dirImport` | `"builder"` → `"../builder/index.js"`: a sibling SDK directory's barrel, with the explicit `.js` path Node ESM requires |
| `siblingImport` | `"todo"` → `"./todo.js"`: a file in the same directory |

### Template Reference

| Template File | Data Type | Output |
|---------------|-----------|--------|
| `ts_scalar.tmpl` | `[]TSScalar` | `scalars/index.ts` |
| `ts_enums.tmpl` | `[]TSEnumDef` | `enums/index.ts` |
| `ts_types.tmpl` | `TSTypesData` | `types/index.ts` |
| `ts_inputs.tmpl` | `TSInputsData` | `inputs/index.ts` |
| `ts_field_selector.tmpl` | `TSFieldSelectorData` | `fields/{name}.ts` |
| `ts_field_selector_index.tmpl` | `[]TSFieldIndexEntry` | `fields/index.ts` |
| `ts_operation.tmpl` | `TSOperationTemplateData` | `queries/{name}.ts` or `mutations/{name}.ts` |
| `ts_operation_root.tmpl` | `TSOperationRootData` | `queries/root.ts` or `mutations/root.ts` |
| `ts_operation_index.tmpl` | `TSOperationIndexData` | `queries/index.ts` or `mutations/index.ts` |

### Template: ts_scalar.tmpl

```
// Code generated by gqlkit. DO NOT EDIT.
{{ range . }}
{{- if .Import }}
{{- if eq .ImportType .Name }}
export type { {{ .Name }} } from "{{ .Import }}";
{{- else }}
export type { {{ .ImportType }} as {{ .Name }} } from "{{ .Import }}";
{{- end }}
{{- else }}
export type {{ .Name }} = {{ .TSType }};
{{- end }}
{{- if .CodecImport }}
export { {{ if .Decode }}{{ .Decode }} as {{ .DecoderName }}{{ end }}{{ if and .Decode .Encode }}, {{ end }}{{ if .Encode }}{{ .Encode }} as {{ .EncoderName }}{{ end }} } from "{{ .CodecImport }}";
{{- end }}
{{- end }}
```

### Template: ts_enums.tmpl

```
// Code generated by gqlkit. DO NOT EDIT.
{{ range $i, $enum := . }}
{{ if $i }}
{{ end -}}
export enum {{ $enum.Name }} {
{{- range $enum.Values }}
  {{ . }} = "{{ . }}",
{{- end }}
}
{{- end }}
```

### Template: ts_types.tmpl

`ts_inputs.tmpl` follows the same structure for input types, with optional `?:` fields and the codec encode plans:

```
// Code generated by gqlkit. DO NOT EDIT.
{{ if .EnumImports }}
import type { {{ joinComma .EnumImports }} } from "{{ dirImport "enums" }}";
{{- end }}
{{ if .ScalarImports }}
import type { {{ joinComma .ScalarImports }} } from "{{ dirImport "scalars" }}";
{{- end }}
{{ if or .EnumImports .ScalarImports }}
{{ end }}
{{- range $i, $td := .Types }}
{{ if $i }}
{{ end -}}
export interface {{ $td.Name }} {
{{- range $td.Fields }}
  {{ .Name }}{{ if .Optional }}?{{ end }}: {{ .TSType }};
{{- end }}
}
{{- end }}
{{- range .Unions }}

export type {{ .Name }} = {{ .Members }};
{{- end }}
```

### Template: ts_field_selector.tmpl

```
// Code generated by gqlkit. DO NOT EDIT.

import { FieldSelection } from "{{ dirImport "builder" }}";
{{- range .Imports }}
import { {{ .ClassName }} } from "{{ .FilePath }}";
{{- end }}
{{- if .EnumImports }}
import type { {{ joinComma .EnumImports }} } from "{{ dirImport "enums" }}";
{{- end }}
{{- if .ScalarImports }}
import type { {{ joinComma .ScalarImports }} } from "{{ dirImport "scalars" }}";
{{- end }}
{{- if .DecoderImports }}
import { {{ joinComma .DecoderImports }} } from "{{ dirImport "scalars" }}";
{{- end }}

/** {{ .SelectorName }} provides type-safe field selection for {{ .TypeName }} */
export class {{ .SelectorName }}<T extends object = {{ .DefaultResult }}> {
  readonly #selection: FieldSelection;
{{ if .IsAbstract }}
  /** typenameAdded is internal: chained calls reuse a selection that already requests __typename */
  constructor(selection: FieldSelection, typenameAdded = false) {
    this.#selection = selection;
    if (!typenameAdded) selection.addField("__typename");
  }
{{ range .Variants }}
  {{ .MethodName }}<U extends object>(selector: (f: {{ .SelectorName }}) => {{ .SelectorName }}<U>): {{ $.SelectorName }}<Exclude<T, { __typename: "{{ .TypeName }}" }> | (Extract<T, { __typename: "{{ .TypeName }}" }> & U)> {
    const child = new FieldSelection();
    selector(new {{ .SelectorName }}(child));
    this.#selection.addChild("... on {{ .TypeName }}", child);
    return new {{ $.SelectorName }}<Exclude<T, { __typename: "{{ .TypeName }}" }> | (Extract<T, { __typename: "{{ .TypeName }}" }> & U)>(this.#selection, true);
  }
{{ end }}
{{- else }}
  constructor(selection: FieldSelection) {
    this.#selection = selection;
  }
{{ end -}}
{{ range .Fields }}
{{ if .IsObject }}  {{ .MethodName }}<U extends object>(selector: (f: {{ .NestedSelector }}) => {{ .NestedSelector }}<U>): {{ $.SelectorName }}<T & { {{ .FieldName }}: {{ .ResultType }} }> {
    const child = new FieldSelection();
    selector(new {{ .NestedSelector }}(child));
    this.#selection.addChild("{{ .FieldName }}", child);
    return new {{ $.SelectorName }}<T & { {{ .FieldName }}: {{ .ResultType }} }>(this.#selection{{ if $.IsAbstract }}, true{{ end }});
  }
{{ else }}  {{ .MethodName }}(): {{ $.SelectorName }}<T & { {{ .FieldName }}: {{ .ResultType }} }> {
    this.#selection.addField("{{ .FieldName }}"{{ .DecodeArgs }});
    return new {{ $.SelectorName }}<T & { {{ .FieldName }}: {{ .ResultType }} }>(this.#selection{{ if $.IsAbstract }}, true{{ end }});
  }
{{ end }}
{{- end -}}
}
```

### Template: ts_operation.tmpl

```
// Code generated by gqlkit. DO NOT EDIT.
{{ range .Imports }}
{{- if .TypeOnly }}
import type { {{ joinComma .Names }} } from "{{ .Path }}";
{{- else }}
import { {{ joinComma .Names }} } from "{{ .Path }}";
{{- end }}
{{- end }}

/** {{ .Data.BuilderName }} builds a {{ .Data.OpType }} for {{ .Data.FieldName }} */
{{- if .Data.HasSelect }}
export class {{ .Data.BuilderName }}<TResult = {{ .Data.DefaultGeneric }}> {
{{- else }}
export class {{ .Data.BuilderName }} {
{{- end }}
  readonly #builder: BaseBuilder;
{{ if .Data.HasSelect }}
  /** builder is internal: select() passes the shared state to the narrowed instance */
  constructor(client: GraphQLClient, builder?: BaseBuilder) {
    this.#builder = builder ?? new BaseBuilder(client, "{{ .Data.OpType }}", "{{ .Data.MethodName }}", "{{ .Data.FieldName }}");
{{- if .Data.IsAbstract }}
    // An abstract result needs a selection even if select() is never called.
    if (!builder) this.#builder.getSelection().addField("__typename");
{{- end }}
  }
{{- else }}
  constructor(client: GraphQLClient) {
    this.#builder = new BaseBuilder(client, "{{ .Data.OpType }}", "{{ .Data.MethodName }}", "{{ .Data.FieldName }}");
{{- if .Data.ResultDecoder }}
    this.#builder.setResultDecoder({{ .Data.ResultDecoder }});
{{- end }}
  }
{{- end }}
{{ range .Data.Arguments }}
  /** Sets the {{ .ArgName }} argument */
  {{ .MethodName }}(v: {{ .TSType }}): this {
    this.#builder.setArg("{{ .ArgName }}", v, "{{ .GraphQLType }}"{{ .EncodeArg }});
    return this;
  }
{{ end }}
{{- if .Data.HasSelect }}
  /** Configures which fields to return */
  select<T extends object>(selector: (f: {{ .Data.SelectorName }}) => {{ .Data.SelectorName }}<T>): {{ .Data.BuilderName }}<{{ .Data.SelectWrapper }}> {
    selector(new {{ .Data.SelectorName }}(this.#builder.getSelection(){{ if .Data.IsAbstract }}, true{{ end }}));
    return new {{ .Data.BuilderName }}<{{ .Data.SelectWrapper }}>(this.#builder.getClient(), this.#builder);
  }
{{ end }}
  /** Execute runs the {{ .Data.OpType }} and returns the result; request.signal cancels it */
{{- $rt := "TResult" }}{{ if not .Data.HasSelect }}{{ $rt = .Data.ReturnType }}{{ end }}
{{- if .Data.HasSelect }}
  async execute({{ if .Data.NeedsSelect }}this: TResult extends SelectionRequired ? never : this, {{ end }}request?: RequestOptions): Promise<TResult> { {{- if .Data.NeedsSelect }}
    this.#requireSelection();
{{- end }}
{{- else }}
  async execute(request?: RequestOptions): Promise<{{ .Data.ReturnType }}> {
{{- end }}
    const query = this.#builder.buildQuery();
    const variables = this.#builder.getVariables();
{{ if .Data.Decodes }}
    let response: { {{ .Data.FieldName }}: {{ $rt }} };
    try {
      response = await this.#builder
        .getClient()
        .execute<{ {{ .Data.FieldName }}: {{ $rt }} }>(query, variables, request);
    } catch (e) {
      // Partial data arrives on the error as wire values; decode it first.
      if (e instanceof GraphQLErrors) this.#builder.decodePartial(e);
      throw e;
    }

    return this.#builder.decodeResult(response.{{ .Data.FieldName }});
{{- else }}
    const response = await this.#builder
      .getClient()
      .execute<{ {{ .Data.FieldName }}: {{ $rt }} }>(query, variables, request);

    return response.{{ .Data.FieldName }};
{{- end }}
  }

  /** ExecuteRaw runs the {{ .Data.OpType }} and returns the raw response; request.signal cancels it */
  async executeRaw({{ if .Data.NeedsSelect }}this: TResult extends SelectionRequired ? never : this, {{ end }}request?: RequestOptions): Promise<Record<string, unknown>> { {{- if .Data.NeedsSelect }}
    this.#requireSelection();
{{- end }}
    return await this.#builder
      .getClient()
      .execute<Record<string, unknown>>(this.#builder.buildQuery(), this.#builder.getVariables(), request);
  }

  /** Internal — exposes this operation as a fragment for batch() */
  getOpFragment({{ if .Data.NeedsSelect }}this: TResult extends SelectionRequired ? never : this, {{ end }}alias: string) { {{- if .Data.NeedsSelect }}
    this.#requireSelection();
{{- end }}
    return this.#builder.getOpFragment(alias);
  }
{{- if .Data.NeedsSelect }}

  /** Fails before sending: the server rejects {{ .Data.FieldName }} without a selection set. */
  #requireSelection(): void {
    if (this.#builder.getSelection().isEmpty()) {
      throw new Error("gqlkit: {{ .Data.FieldName }} returns an object type; call select() with at least one field before running it");
    }
  }
{{- end }}
}
```

### Template: ts_operation_root.tmpl

```
// Code generated by gqlkit. DO NOT EDIT.

import { GraphQLClient } from "{{ dirImport "builder" }}";
{{- range .Builders }}
import { {{ .BuilderName }} } from "{{ siblingImport .FileName }}";
{{- end }}

/** {{ .ClassName }} is the entry point for {{ .OpType }}s */
export class {{ .ClassName }} {
  readonly #client: GraphQLClient;

  constructor(client: GraphQLClient) {
    this.#client = client;
  }
{{ range .Builders }}
  {{ .MethodName }}(): {{ .BuilderName }} {
    return new {{ .BuilderName }}(this.#client);
  }
{{ end -}}
}
```

### Template: ts_operation_index.tmpl

```
// Code generated by gqlkit. DO NOT EDIT.

export { {{ .RootClassName }} } from "{{ siblingImport "root" }}";
{{- range .Builders }}
export { {{ .BuilderName }} } from "{{ siblingImport .FileName }}";
{{- end }}
```


## Runtime Library (gqlkit-ts)

The `gqlkit-ts` npm package provides the classes and helpers that the generated SDK depends on. It has zero runtime dependencies. Its user-facing API is documented in the [gqlkit-ts README](../gqlkit-ts/README.md); this section covers what the generator relies on.

### Public Exports

```typescript
export { FieldSelection, BaseBuilder, OperationType, OPERATION_TYPES } from "./builder";
export { GraphQLClient, GraphQLErrors, GraphQLHTTPError } from "./graphqlclient";
export type { ClientOptions, GraphQLError, RequestOptions } from "./graphqlclient";
export { batch, DEFAULT_BATCH_OP_NAME, GRAPHQL_NAME_PATTERN } from "./batch";
export { ScalarCodecError, CodecDirection } from "./codec";
export type { ScalarDecoder, ScalarEncoder, ArgEncoder, InputEncodePlan } from "./codec";
export type { BatchableBuilder, BatchResult, BatchOptions, OpFragment } from "./batch";
```

### FieldSelection

Tracks which fields are selected in a GraphQL selection tree: scalar fields (each with an optional decoder) and child selections, including inline fragments.

| Method | Description |
|--------|-------------|
| `addField(name, decode?, listDepth?)` | Add a leaf field; a decoder converts its value when a codec applies |
| `addChild(name, child, possibleTypes?)` | Add a nested selection; a name of the form `... on Type` renders an inline fragment |
| `hasDecoders()` | Whether any field in the tree needs decoding, so plain results skip the walk |
| `build(indent?)` | Serialize to a GraphQL selection set string |
| `isEmpty()` | Whether nothing is selected (used by the `select()` run-time guard) |

`build()` output example:

```
  id
  name
  address {
    city
    zip
  }
```

### BaseBuilder

Core builder for query and mutation operations. Generated classes hold one in a `#builder` field and delegate to it.

| Method | Description |
|--------|-------------|
| `setArg(name, value, graphqlType, encode?)` | Register an argument with its GraphQL type, and an encoder when a codec applies |
| `setResultDecoder(decode, listDepth?)` | Decoder for a scalar root result |
| `decodeResult(value, rootKey?)` | Apply the selection's decoders to a result |
| `decodePartial(err)` | Decode the partial `data` carried by a `GraphQLErrors` |
| `getSelection()` | Access the field selection tree |
| `getClient()` | Access the GraphQL client |
| `getVariables()` | Runtime variable values, encoded |
| `buildQuery()` | Serialize to a complete GraphQL operation string |
| `executeRaw()` | Build and execute, returning the raw `data` object |
| `getOpFragment(alias)` | The operation as an aliased fragment for `batch()` |

`buildQuery()` output example:

```graphql
query User($id: ID!) {
  user(id: $id) {
    id
    name
    email
  }
}
```

### GraphQLClient

HTTP transport layer. Sends GraphQL operations as JSON POST requests.

```typescript
class GraphQLClient {
  constructor(endpoint: string, options?: ClientOptions)
  async execute<T>(query: string, variables?: Record<string, unknown>, request?: RequestOptions): Promise<T>
  async rawQuery(query: string, variables?: Record<string, unknown>, request?: RequestOptions): Promise<unknown>
}

interface ClientOptions {
  headers?: Record<string, string>;   // extra HTTP headers
  authToken?: string;                 // Bearer token
  fetch?: typeof fetch;               // custom fetch (SSR/testing)
}

interface RequestOptions {
  signal?: AbortSignal;               // cancels this request
}
```

`execute()` flow:

1. If `request.signal` is already aborted, reject with its reason before sending
2. Resolve the fetch function per call (custom, else `globalThis.fetch`)
3. Build headers: `Content-Type: application/json` + user headers + optional Bearer auth (`authToken` wins over an `Authorization` header)
4. POST `{ query, variables }` to the endpoint, passing the signal
5. Parse the body; a body that is not a JSON object throws `GraphQLHTTPError` (status and body kept)
6. If the `errors` array is non-empty, throw `GraphQLErrors` with any partial `data` attached
7. If there is no `data`: a non-2xx status throws `GraphQLHTTPError`, otherwise a generic `Error`
8. Return `response.data`

### GraphQLErrors

```typescript
class GraphQLErrors extends Error {
  public errors: GraphQLError[];
  public data?: Record<string, unknown>;     // partial data sent with the errors
  public decodeError?: ScalarCodecError;     // set when partial data failed to decode
  // message = all error messages joined by "; "
}

interface GraphQLError {
  message: string;
  locations?: { line: number; column: number }[];
  path?: (string | number)[];
  extensions?: Record<string, unknown>;
}
```


## Generated Output Structure

```
sdk/
├── builder/
│   └── index.ts              ← re-exports from gqlkit-ts, SelectionRequired
├── scalars/
│   └── index.ts              ← type aliases, external re-exports, codec re-exports
├── enums/
│   └── index.ts              ← string enums
├── types/
│   └── index.ts              ← interfaces for Object/Interface types, union aliases
├── inputs/
│   └── index.ts              ← interfaces for InputObject types (+ encode plans with codecs)
├── fields/
│   ├── {type-name}.ts        ← field selector class per object, interface and union
│   └── index.ts              ← barrel re-exports
├── queries/
│   ├── {field-name}.ts       ← query builder class per field
│   ├── root.ts               ← QueryRoot factory class
│   └── index.ts              ← barrel re-exports
└── mutations/
    ├── {field-name}.ts       ← mutation builder class per field
    ├── root.ts               ← MutationRoot factory class
    └── index.ts              ← barrel re-exports
```

All generated files begin with `// Code generated by gqlkit. DO NOT EDIT.`, which is also how `--prune` recognises them. Relative imports use explicit `.js` paths (`./todo.js`, `../fields/index.js`).

File naming uses kebab-case: `TodoConnection` → `todo-connection.ts`.


## Data Flow Walkthrough

### Example: `todos(filter: TodoFilter, pagination: PaginationInput): [Todo!]!`

**Step 1 — Schema Parsing**

gqlparser produces an AST `FieldDefinition`:

```
Name: "todos"
Arguments: [
  { Name: "filter",     Type: TodoFilter (nullable) },
  { Name: "pagination", Type: PaginationInput (nullable) }
]
Type: [Todo!]!  // non-null list of non-null Todo
```

**Step 2 — buildOperationBuilderData(query, root, field)**

```
BuilderName    = "TodosBuilder"        (PascalCase + "Builder")
OpType         = query
FieldName      = "todos"
MethodName     = "Todos"
HasSelect      = true                  (Todo is an Object type)
NeedsSelect    = true                  (object result, not abstract)
DefaultGeneric = "SelectionRequired"   (execute() is not callable until select())
SelectWrapper  = "T[]"                 (outputTSType of [Todo!]! with leaf T)
SelectorName   = "TodoFields"
Arguments      = [
  { ArgName: "filter",     TSType: "TodoFilter | null",      GraphQLType: "TodoFilter" },
  { ArgName: "pagination", TSType: "PaginationInput | null", GraphQLType: "PaginationInput" }
]
```

**Step 3 — collectOperationImports()**

```
BaseBuilder, GraphQLClient, GraphQLErrors      → ../builder/index.js   (GraphQLErrors only because example-ts configures a codec)
type RequestOptions, SelectionRequired         → ../builder/index.js
TodoFields                                     → ../fields/index.js
type PaginationInput, TodoFilter               → ../inputs/index.js
```

**Step 4 — Template Renders TodosBuilder**

The `ts_operation.tmpl` template produces the complete builder class, shown as `queries/todos.ts` under [Operation Builders](#7-operation-builders).

**Step 5 — Runtime Execution**

```typescript
const result = await qr
  .todos()
  .filter({ done: false })
  .pagination({ limit: 10, offset: 0 })
  .select((t) => t.id().text().done())
  .execute();
// TypeScript type: { id: string; text: string; done: boolean }[]
```

`buildQuery()` produces:

```graphql
query Todos($filter: TodoFilter, $pagination: PaginationInput) {
  todos(filter: $filter, pagination: $pagination) {
    id
    text
    done
  }
}
```

`getVariables()` returns:

```json
{
  "filter": { "done": false },
  "pagination": { "limit": 10, "offset": 0 }
}
```


## Design Patterns

### Type-Safe Field Selection via Generic Intersection Types

Each method on a field selector returns a new selector instance whose generic intersects the previous type with the new field:

```typescript
class TodoFields<T extends object = {}> {
  id():   TodoFields<T & { id: string }>   { ... }
  text(): TodoFields<T & { text: string }> { ... }
  done(): TodoFields<T & { done: boolean }> { ... }
}

// Chaining: TodoFields<{} & {id: string} & {text: string}> = TodoFields<{id: string; text: string}>
```

TypeScript resolves the intersection type at compile time, giving full autocomplete and type checking on the result. No method casts: each returns `new TodoFields<...>(this.#selection)` over the shared selection.

### Nested Selection via Callbacks

Object fields use a callback that receives a fresh selector for the nested type. From the generated `fields/todo.ts`:

```typescript
user<U extends object>(selector: (f: UserFields) => UserFields<U>): TodoFields<T & { user: U }> {
  const child = new FieldSelection();
  selector(new UserFields(child));
  this.#selection.addChild("user", child);
  return new TodoFields<T & { user: U }>(this.#selection);
}
```

The callback's return type `UserFields<U>` captures what fields were selected, and `U` flows into the parent's generic.

### Discriminated Unions via on<Type>()

A selector for a union or interface always selects `__typename` and has one `on<Type>()` method per possible type. Each call adds a `... on Type` fragment and narrows the matching member of the result union, so `if (r.__typename === "Todo")` narrows `r` to the fields selected for `Todo`.

### Builder Pattern with Method Chaining

Argument setters return `this` for fluent chaining:

```typescript
qr.todo()
  .id("1")
  .select((t) => t.id().text())
  .execute()
```

### Transparent Scalar Re-exports

External package types are re-exported from `scalars/index.ts` under the GraphQL scalar name. All other generated files import from `../scalars/index.js` without knowing the original source:

```
luxon → DateTime → scalars/index.ts → types/index.ts, queries/*.ts, fields/*.ts
```

### Barrel File Pattern

Each module exports everything from a single `index.ts`:

```typescript
// fields/index.ts
export { TodoFields } from "./todo.js";
export { UserFields } from "./user.js";
```

Enables clean imports: `import { TodoFields, UserFields } from "./fields/index.js"`.

### File Naming Convention

- Go structs/types: PascalCase
- Generated TS files: kebab-case (e.g., `TodoConnection` → `todo-connection.ts`)
- Query builders: `{PascalField}Builder` (e.g., `TodoBuilder`)
- Mutation builders: `{PascalField}MutationBuilder` (e.g., `CreateTodoMutationBuilder`)
- Field selectors: `{TypeName}Fields` (e.g., `TodoFields`)
- Root classes: `QueryRoot`, `MutationRoot`
- Schema names that clash with a class member get a `_` suffix (`select_`)

### File Writer

`TSWriter` writes files to disk with the generated-code banner and no other formatting. Generated TypeScript is expected to be formatted by external tools (Prettier, ESLint, etc.) if the project wants that.

```go
type TSWriter struct {
	outputDir string          // root directory for all generated TS output
	written   map[string]bool // slash-separated paths written this run, for Prune
}

func (w *TSWriter) WriteFile(filename, content string) error
func (w *TSWriter) EnsureDir() error
func (w *TSWriter) Prune(dryRun bool) ([]string, error)
```


## Error Handling

### Generation-Time Errors

```go
// Sentinel errors
ErrSchemaPathRequired    = errors.New("schema path is required")
ErrDryRunWithoutPrune    = errors.New("--dry-run requires --prune")
ErrNullModeInvalid       = errors.New("unknown nullable mode")
ErrNullOverrideUnknown   = errors.New("unknown type or field")
ErrBindingTypeRequired   = errors.New(`missing TypeScript "type"`)
ErrCodecImportRequired   = errors.New("decode/encode need codecImport, the module that exports them")
ErrCodecWithoutFunctions = errors.New("codecImport is set but neither decode nor encode is")
ErrCodecNameInvalid      = errors.New("decode/encode must be a plain identifier exported by codecImport")

// Contextual errors (wrapped)
fmt.Errorf("failed to load client config: %w", err)
fmt.Errorf("failed to parse schema: %w", err)
fmt.Errorf("failed to execute scalar template: %w", err)
// etc.
```

### Runtime Errors

```typescript
// Server returned GraphQL errors (partial data on err.data)
throw new GraphQLErrors(errors, data)

// Response is not GraphQL (not JSON, or non-2xx with no data or errors)
throw new GraphQLHTTPError(status, body, reason)

// A codec's decode or encode threw
throw new ScalarCodecError(...)   // path names the failing value, e.g. "todos.0.createdAt"

// 2xx response with no data and no errors
throw new Error("No data returned from GraphQL query")
```
