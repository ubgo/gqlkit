package clientgents

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// TSTypeMap maps GraphQL scalar names to their TypeScript type equivalents
// (e.g. "Int" -> "number", "Boolean" -> "boolean"). Custom scalars can be
// added via config bindings.
type TSTypeMap map[string]string

// BuiltInTSTypes returns the default mapping from GraphQL built-in and common
// extended scalars to TypeScript primitive types.
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

// TSBinding describes how a GraphQL scalar maps to a TypeScript type.
// When Import is set, the type is imported from that npm package instead
// of being defined as an inline type alias.
//
// A binding changes only the TS type unless it also names a codec: Decode
// converts each wire value of the scalar in a response (e.g. an ISO string
// into a luxon DateTime) and Encode converts each value sent as an argument or
// input field back to its wire form. Both name exports of CodecImport, a
// module path resolved like Import (relative paths are relative to the
// generated scalars/ directory). Either may be set alone. Validate enforces
// the combination; with no codec the generated SDK is unchanged.
type TSBinding struct {
	Type        string `json:"type"`                  // TypeScript type name (e.g. "DateTime", "JsonValue")
	Import      string `json:"import,omitempty"`      // npm package or path (e.g. "luxon", "type-fest")
	Decode      string `json:"decode,omitempty"`      // export of CodecImport: wire value → Type
	Encode      string `json:"encode,omitempty"`      // export of CodecImport: Type → wire value
	CodecImport string `json:"codecImport,omitempty"` // module exporting Decode / Encode
}

// HasCodec reports whether the binding converts values at runtime.
func (b TSBinding) HasCodec() bool {
	return b.Decode != "" || b.Encode != ""
}

// Validate checks the binding for scalar. Type must be set: an empty one
// used to be written out as `export type X = ;` — typically a Go-generator
// binding ({"model": "time.Time"}) copied into the TS config, whose unknown
// "model" key decodes to an empty Type. A codec needs a module to import
// from, a module needs a codec, and both names must be plain identifiers
// because they are emitted into an export statement.
func (b TSBinding) Validate(scalar string) error {
	switch {
	case strings.TrimSpace(b.Type) == "":
		return fmt.Errorf(`binding %q: %w; write "%s": "string" or "%s": { "type": "DateTime", "import": "luxon" } (a {"model": ...} binding is the Go generator's form)`, scalar, ErrBindingTypeRequired, scalar, scalar)
	case b.HasCodec() && b.CodecImport == "":
		return fmt.Errorf("binding %q: %w", scalar, ErrCodecImportRequired)
	case !b.HasCodec() && b.CodecImport != "":
		return fmt.Errorf("binding %q: %w", scalar, ErrCodecWithoutFunctions)
	}
	for _, name := range []string{b.Decode, b.Encode} {
		if name != "" && !tsIdentifier.MatchString(name) {
			return fmt.Errorf("binding %q: %q: %w", scalar, name, ErrCodecNameInvalid)
		}
	}
	return nil
}

// tsIdentifier matches a plain (ASCII) JavaScript identifier.
var tsIdentifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// UnmarshalJSON allows a binding to be either a plain string ("string") or
// an object ({"type": "DateTime", "import": "luxon"}) for backward compat.
func (b *TSBinding) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		b.Type = s
		return nil
	}
	type raw TSBinding
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*b = TSBinding(r)
	return nil
}

// ConfigTSBindings holds custom scalar-to-TypeScript type overrides loaded
// from the config.jsonc file.
type ConfigTSBindings map[string]TSBinding

// Validate checks every binding, in scalar-name order so the first error
// reported is deterministic.
func (bs ConfigTSBindings) Validate() error {
	names := make([]string, 0, len(bs))
	for name := range bs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := bs[name].Validate(name); err != nil {
			return err
		}
	}
	return nil
}

// Merge merges config bindings into the type map. For bindings with an
// external import, the map value is set to the scalar name (the key) because
// scalars/index.ts re-exports the imported type under that name.
func (m TSTypeMap) Merge(bindings ConfigTSBindings) TSTypeMap {
	for k, v := range bindings {
		if v.Import != "" {
			m[k] = k // other files import by scalar name from ../scalars
		} else {
			m[k] = v.Type
		}
	}
	return m
}

// Get returns the TS type for a GraphQL scalar name, defaulting to tsAny.
func (m TSTypeMap) Get(name string) string {
	if t, ok := m[name]; ok {
		return t
	}
	return tsAny
}

// toKebabCase converts PascalCase to kebab-case for TypeScript file naming
// (e.g. "ChatbotConnection" -> "chatbot-connection"). Handles acronyms
// gracefully so "URLParser" becomes "url-parser" not "u-r-l-parser".
func toKebabCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			// Look ahead to handle acronyms like "URL" → "url" not "u-r-l"
			if i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z' {
				b.WriteByte('-')
			} else if s[i-1] >= 'a' && s[i-1] <= 'z' {
				b.WriteByte('-')
			}
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// tsTypeOperatorPrefixes are the TypeScript type operators that bind looser
// than the array suffix: "keyof T[]" means keyof (T[]), not (keyof T)[].
var tsTypeOperatorPrefixes = []string{"keyof ", "typeof ", "readonly ", "unique ", "infer ", "new ", "asserts ", "abstract "}

// tsListOf renders a TypeScript array of elem. A GraphQL list becomes elem
// with a "[]" suffix, but "[]" binds tighter than "|", "&", "=>", a
// conditional "?:" and the keyof/typeof/readonly operators, so a binding such
// as "string | number" would otherwise render as "string | number[]" (a
// string, or an array of numbers). Such elements are wrapped in parentheses;
// simple references ("string", "Scalars.ID", "Array<string | number>",
// "Todo[]", "{ a: string }", "[a, b]") keep the bare suffix so existing
// output is unchanged.
func tsListOf(elem string) string {
	elem = strings.TrimSpace(elem)
	if tsNeedsParensForArray(elem) {
		return "(" + elem + ")" + tsArraySuffix
	}
	return elem + tsArraySuffix
}

// tsNeedsParensForArray reports whether elem must be parenthesised before a
// "[]" suffix: it starts with a type operator (tsTypeOperatorPrefixes) or has
// a top-level union, intersection, arrow or conditional (see tsTopLevelOps).
func tsNeedsParensForArray(elem string) bool {
	if elem == "" || tsFullyParenthesised(elem) {
		return false
	}
	for _, p := range tsTypeOperatorPrefixes {
		if strings.HasPrefix(elem, p) {
			return true
		}
	}
	unionLike, absorbing := tsTopLevelOps(elem)
	return unionLike || absorbing
}

// tsOrNull renders elem | null. Only a top-level arrow or conditional needs
// parentheses here: in "() => string | null" the "| null" would become part
// of the function's return type, and in "A extends B ? C : D | null" part of
// the false branch. Unions, intersections and type operators already bind
// tighter than (or as tight as) "|", so "string | number | null" stays flat.
func tsOrNull(elem string) string {
	return tsOrNullWith(elem, tsNullSuffix)
}

// tsOrNullWith is tsOrNull with a caller-chosen suffix (" | null" or
// " | null | undefined", see NullMode); the parenthesisation rule is the same.
func tsOrNullWith(elem, suffix string) string {
	elem = strings.TrimSpace(elem)
	if _, absorbing := tsTopLevelOps(elem); absorbing && !tsFullyParenthesised(elem) {
		elem = "(" + elem + ")"
	}
	return elem + suffix
}

// tsTopLevelOps scans elem outside any brackets or string literal and
// reports whether it has a union or intersection ("|", "&") and whether it
// has an arrow ("=>") or a conditional ("?") — the two forms that absorb
// anything written after them. Nesting is tracked over (), <>, {} and [] and
// quoted literals are skipped, so "Array<string | number>" and
// "{ a: 'x' | 'y' }" report neither.
func tsTopLevelOps(elem string) (unionLike, absorbing bool) {
	depth := 0
	var quote byte
	for i := 0; i < len(elem); i++ {
		c := elem[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++ // skip the escaped character
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '(', '<', '{', '[':
			depth++
		case ')', '>', '}', ']':
			depth--
		case '=':
			// "=>" is an arrow, not a closing angle bracket; skip its '>'
			// so the depth count stays balanced.
			if i+1 < len(elem) && elem[i+1] == '>' {
				if depth == 0 {
					absorbing = true
				}
				i++
			}
		case '?':
			if depth == 0 {
				absorbing = true
			}
		case '|', '&':
			if depth == 0 {
				unionLike = true
			}
		}
	}
	return unionLike, absorbing
}

// tsFullyParenthesised reports whether elem is one parenthesised group, e.g.
// "(a | b)" but not "(a) => b" or "(a) | (b)", so tsListOf never double-wraps.
func tsFullyParenthesised(elem string) bool {
	if !strings.HasPrefix(elem, "(") {
		return false
	}
	depth := 0
	var quote byte
	for i := 0; i < len(elem); i++ {
		c := elem[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++ // skip the escaped character
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i == len(elem)-1
			}
		}
	}
	return false
}
