package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
)

// ConvertToSDL converts an IntrospectionSchema into a GraphQL SDL (Schema
// Definition Language) string. It outputs the schema definition (if custom root
// types exist), all user-defined types sorted alphabetically, and any custom
// directives. Built-in types (prefixed with "__") and built-in scalars/directives
// are omitted from the output.
func ConvertToSDL(schema *IntrospectionSchema) string {
	var sb strings.Builder

	// Write the explicit schema block only if root types have non-default names.
	writeSchemaDefinition(&sb, schema)

	// Sort types alphabetically by name for deterministic, diff-friendly output.
	types := make([]FullType, len(schema.Types))
	copy(types, schema.Types)
	sort.Slice(types, func(i, j int) bool {
		return types[i].Name < types[j].Name
	})

	// Write each user-defined type, skipping introspection meta-types (e.g., __Type, __Field).
	for _, t := range types {
		if strings.HasPrefix(t.Name, metaTypePrefix) {
			continue
		}

		writeType(&sb, t)
	}

	// Write custom directives, skipping built-in ones (@skip, @include, @deprecated, @specifiedBy).
	for _, d := range schema.Directives {
		if isBuiltInDirective(d.Name) {
			continue
		}
		writeDirective(&sb, d)
	}

	return sb.String()
}

// writeSchemaDefinition writes an explicit `schema { ... }` block only when
// the root operation types use non-standard names (i.e., something other than
// "Query", "Mutation", "Subscription"). If all root types use the default names,
// the schema block is omitted since GraphQL infers them by convention.
func writeSchemaDefinition(sb *strings.Builder, schema *IntrospectionSchema) {
	hasCustomRootTypes := (schema.QueryType != nil && schema.QueryType.Name != defaultQueryTypeName) ||
		(schema.MutationType != nil && schema.MutationType.Name != defaultMutationTypeName) ||
		(schema.SubscriptionType != nil && schema.SubscriptionType.Name != defaultSubscriptionTypeName)

	if hasCustomRootTypes {
		sb.WriteString("schema {\n")
		if schema.QueryType != nil {
			fmt.Fprintf(sb, "  query: %s\n", schema.QueryType.Name)
		}
		if schema.MutationType != nil {
			fmt.Fprintf(sb, "  mutation: %s\n", schema.MutationType.Name)
		}
		if schema.SubscriptionType != nil {
			fmt.Fprintf(sb, "  subscription: %s\n", schema.SubscriptionType.Name)
		}
		sb.WriteString("}\n\n")
	}
}

// writeType dispatches to the appropriate writer based on the GraphQL type kind.
func writeType(sb *strings.Builder, t FullType) {
	switch t.Kind {
	case kindScalar:
		writeScalar(sb, t)
	case kindObject:
		writeObject(sb, t)
	case kindInterface:
		writeInterface(sb, t)
	case kindUnion:
		writeUnion(sb, t)
	case kindEnum:
		writeEnum(sb, t)
	case kindInputObject:
		writeInputObject(sb, t)
	}
}

// writeScalar writes a `scalar <Name>` declaration. Built-in scalars
// (Int, Float, String, Boolean, ID) are skipped.
func writeScalar(sb *strings.Builder, t FullType) {
	if isBuiltInScalar(t.Name) {
		return
	}

	writeDescription(sb, t.Description, "")
	fmt.Fprintf(sb, "scalar %s\n\n", t.Name)
}

// writeObject writes a `type <Name> [implements ...] { fields }` block,
// including any interfaces the object type implements.
func writeObject(sb *strings.Builder, t FullType) {
	writeDescription(sb, t.Description, "")
	fmt.Fprintf(sb, "type %s", t.Name)

	// Append "implements X & Y" clause if the type implements interfaces.
	if len(t.Interfaces) > 0 {
		interfaces := make([]string, len(t.Interfaces))
		for i, iface := range t.Interfaces {
			if iface.Name != nil {
				interfaces[i] = *iface.Name
			}
		}
		fmt.Fprintf(sb, " implements %s", strings.Join(interfaces, " & "))
	}

	sb.WriteString(" {\n")
	writeFields(sb, t.Fields)
	sb.WriteString("}\n\n")
}

// writeInterface writes an `interface <Name> [implements ...] { fields }` block.
// Interfaces can implement other interfaces (per the GraphQL spec).
func writeInterface(sb *strings.Builder, t FullType) {
	writeDescription(sb, t.Description, "")
	fmt.Fprintf(sb, "interface %s", t.Name)

	// Append parent interface implementations if present.
	if len(t.Interfaces) > 0 {
		interfaces := make([]string, len(t.Interfaces))
		for i, iface := range t.Interfaces {
			if iface.Name != nil {
				interfaces[i] = *iface.Name
			}
		}
		fmt.Fprintf(sb, " implements %s", strings.Join(interfaces, " & "))
	}

	sb.WriteString(" {\n")
	writeFields(sb, t.Fields)
	sb.WriteString("}\n\n")
}

// writeUnion writes a `union <Name> = TypeA | TypeB | ...` declaration
// listing all possible member types.
func writeUnion(sb *strings.Builder, t FullType) {
	writeDescription(sb, t.Description, "")

	types := make([]string, len(t.PossibleTypes))
	for i, pt := range t.PossibleTypes {
		if pt.Name != nil {
			types[i] = *pt.Name
		}
	}

	fmt.Fprintf(sb, "union %s = %s\n\n", t.Name, strings.Join(types, " | "))
}

// writeEnum writes an `enum <Name> { VALUE1 VALUE2 ... }` block, including
// @deprecated directives on individual enum values when applicable.
func writeEnum(sb *strings.Builder, t FullType) {
	writeDescription(sb, t.Description, "")
	fmt.Fprintf(sb, "enum %s {\n", t.Name)

	for _, ev := range t.EnumValues {
		writeDescription(sb, ev.Description, "  ")
		fmt.Fprintf(sb, "  %s", ev.Name)

		// Append @deprecated directive with optional reason.
		if ev.IsDeprecated {
			if ev.DeprecationReason != nil && *ev.DeprecationReason != "" {
				fmt.Fprintf(sb, " @deprecated(reason: \"%s\")", escapeString(*ev.DeprecationReason))
			} else {
				sb.WriteString(" @deprecated")
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("}\n\n")
}

// writeInputObject writes an `input <Name> { field1: Type ... }` block
// for INPUT_OBJECT types, including default values where specified.
func writeInputObject(sb *strings.Builder, t FullType) {
	writeDescription(sb, t.Description, "")
	fmt.Fprintf(sb, "input %s {\n", t.Name)

	for _, f := range t.InputFields {
		writeDescription(sb, f.Description, "  ")
		fmt.Fprintf(sb, "  %s: %s", f.Name, formatType(f.Type))

		// Append default value literal if one is defined.
		if f.DefaultValue != nil {
			fmt.Fprintf(sb, " = %s", *f.DefaultValue)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("}\n\n")
}

// writeFields writes a list of field definitions inside a type body. Each field
// includes its arguments (if any), return type, and deprecation status.
func writeFields(sb *strings.Builder, fields []Field) {
	for _, f := range fields {
		writeDescription(sb, f.Description, "  ")
		fmt.Fprintf(sb, "  %s", f.Name)

		if len(f.Args) > 0 {
			writeArguments(sb, f.Args)
		}

		fmt.Fprintf(sb, ": %s", formatType(f.Type))

		// Append @deprecated directive with optional reason.
		if f.IsDeprecated {
			if f.DeprecationReason != nil && *f.DeprecationReason != "" {
				fmt.Fprintf(sb, " @deprecated(reason: \"%s\")", escapeString(*f.DeprecationReason))
			} else {
				sb.WriteString(" @deprecated")
			}
		}
		sb.WriteString("\n")
	}
}

// writeArguments writes a field's argument list in either single-line or
// multi-line format. Multi-line format is used when any argument has a
// description or there are more than 2 arguments, for readability.
func writeArguments(sb *strings.Builder, args []InputValue) {
	if len(args) == 0 {
		return
	}

	// Determine if any argument has a description to decide on formatting.
	hasDescription := false
	for _, arg := range args {
		if arg.Description != nil && *arg.Description != "" {
			hasDescription = true
			break
		}
	}

	if hasDescription || len(args) > 2 {
		// Multi-line format: each argument on its own line with extra indentation.
		sb.WriteString("(\n")
		for _, arg := range args {
			writeDescription(sb, arg.Description, "    ")
			fmt.Fprintf(sb, "    %s: %s", arg.Name, formatType(arg.Type))
			if arg.DefaultValue != nil {
				fmt.Fprintf(sb, " = %s", *arg.DefaultValue)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("  )")
	} else {
		// Single-line format: compact "(arg1: Type, arg2: Type)" for simple cases.
		sb.WriteString("(")
		argStrs := make([]string, len(args))
		for i, arg := range args {
			argStr := fmt.Sprintf("%s: %s", arg.Name, formatType(arg.Type))
			if arg.DefaultValue != nil {
				argStr += fmt.Sprintf(" = %s", *arg.DefaultValue)
			}
			argStrs[i] = argStr
		}
		sb.WriteString(strings.Join(argStrs, ", "))
		sb.WriteString(")")
	}
}

// writeDirective writes a `directive @<name>(...) on LOCATION | LOCATION` declaration.
func writeDirective(sb *strings.Builder, d Directive) {
	writeDescription(sb, d.Description, "")
	fmt.Fprintf(sb, "directive @%s", d.Name)

	if len(d.Args) > 0 {
		writeArguments(sb, d.Args)
	}

	if len(d.Locations) > 0 {
		sb.WriteString(" on ")
		sb.WriteString(strings.Join(d.Locations, " | "))
	}

	sb.WriteString("\n\n")
}

// blockQuote delimits a GraphQL block string; escapedBlockQuote is how the
// spec writes that sequence inside one.
const (
	blockQuote        = `"""`
	escapedBlockQuote = `\"""`
)

// writeDescription writes a GraphQL description string above a definition.
// Single-line descriptions use the `"text"` format; multi-line descriptions
// use the triple-quote `"""..."""` block format. The indent parameter controls
// the indentation level (e.g., "" for top-level, "  " for fields).
func writeDescription(sb *strings.Builder, description *string, indent string) {
	if description == nil || *description == "" {
		return
	}

	desc := *description
	// A lone carriage return is a line terminator too: inside a "..." literal
	// it would make the SDL unparseable, so it forces the block format.
	if strings.ContainsAny(desc, "\r\n") {
		// Multi-line block string format.
		fmt.Fprintf(sb, "%s\"\"\"\n", indent)
		// A literal """ inside a block string would end it early and leave
		// the rest of the description as invalid SDL; the spec escape is \""".
		// Line endings are normalised first so every line gets the indent;
		// a bare \r left inside a line would split it without one.
		normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(desc)
		lines := strings.Split(strings.ReplaceAll(normalized, blockQuote, escapedBlockQuote), "\n")
		for _, line := range lines {
			fmt.Fprintf(sb, "%s%s\n", indent, line)
		}
		fmt.Fprintf(sb, "%s\"\"\"\n", indent)
	} else {
		// Single-line string literal format.
		fmt.Fprintf(sb, "%s\"%s\"\n", indent, escapeString(desc))
	}
}

// formatType recursively converts a TypeInfo wrapper chain into an SDL type
// string. NON_NULL appends "!", LIST wraps in "[]", and named types return
// their name directly. Example: NON_NULL(LIST(NON_NULL(String))) -> "[String!]!"
func formatType(t TypeInfo) string {
	switch t.Kind {
	case kindNonNull:
		if t.OfType != nil {
			return formatType(*t.OfType) + "!"
		}
	case kindList:
		if t.OfType != nil {
			return "[" + formatType(*t.OfType) + "]"
		}
	default:
		if t.Name != nil {
			return *t.Name
		}
	}
	return ""
}

// escapeString escapes s for use inside a GraphQL single-line string literal
// ("..."), using only the escapes the spec defines: \" \\ \b \f \n \r \t
// and \uXXXX for any other control character. Everything else, including
// non-ASCII text, is written as-is (SDL is UTF-8). Go's %q is not a
// substitute: it emits \x.. and \a escapes that GraphQL parsers reject.
func escapeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < firstPrintableASCII || r == asciiDelete {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// Control-character bounds for escapeString: code points below
// firstPrintableASCII (C0 controls) and asciiDelete are written as \uXXXX.
const (
	firstPrintableASCII = 0x20
	asciiDelete         = 0x7F
)

// isBuiltInScalar reports whether name is one of the five built-in GraphQL
// scalars (see builtInScalars), which are implicit and never declared in SDL.
func isBuiltInScalar(name string) bool {
	return slices.Contains(builtInScalars, name)
}

// isBuiltInDirective reports whether name is a spec-defined directive (see
// builtInDirectives), which every server declares and SDL must not redefine.
func isBuiltInDirective(name string) bool {
	return slices.Contains(builtInDirectives, name)
}

// outputFileMode is the permission for written schema files: world-readable,
// owner-writable, because a schema is meant to be committed and shared.
const outputFileMode os.FileMode = 0o644

// SaveToFile writes the given SDL string to a file at the specified path,
// replacing any existing file. This is the final step in the
// introspection-to-SDL pipeline.
func SaveToFile(sdl string, filepath string) error {
	return os.WriteFile(filepath, []byte(sdl), outputFileMode)
}

// SaveAsJSON marshals the introspection schema as pretty-printed JSON and
// writes it to the given file path.
func SaveAsJSON(schema *IntrospectionSchema, filepath string) error {
	wrapper := IntrospectionResponse{
		Data: IntrospectionData{
			Schema: *schema,
		},
	}

	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return os.WriteFile(filepath, data, outputFileMode)
}
