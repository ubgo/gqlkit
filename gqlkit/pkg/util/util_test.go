package util

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"github.com/vektah/gqlparser/v2/ast"
)

func TestIsSeparator(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"underscore", '_', true},
		{"dash", '-', true},
		{"space", ' ', true},
		{"tab", '\t', true},
		{"newline", '\n', true},
		{"carriage return", '\r', true},
		{"lower letter", 'a', false},
		{"upper letter", 'Z', false},
		{"digit", '5', false},
		{"dot", '.', false},
		{"slash", '/', false},
		{"unicode letter", 'é', false},
		{"unicode nbsp space", ' ', true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSeparator(tt.r); got != tt.want {
				t.Errorf("isSeparator(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestSplitCamel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single lower", "a", "a"},
		{"single upper", "A", "A"},
		{"all lower", "created", "created"},
		{"lower to upper once", "createdAt", "created_At"},
		{"multiple transitions", "oneTwoThree", "one_Two_Three"},
		{"already snake", "created_at", "created_at"},
		{"leading upper", "CreatedAt", "Created_At"},          // d(lower)->A(upper) splits
		{"all caps", "HTTP", "HTTP"},                          // no lower before upper
		{"acronym then word", "HTTPServer", "HTTPServer"},     // P(upper)->S(upper) no split; no lower->upper
		{"lower then acronym", "myHTTP", "my_HTTP"},           // y(lower)->H(upper) splits once
		{"digit before upper", "field1Name", "field1Name"},   // '1' is not lower, so no split before N
		{"upper then lower", "Ab", "Ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitCamel(tt.in); got != tt.want {
				t.Errorf("splitCamel(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPascalWords(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		want  string
	}{
		{"empty slice", []string{}, ""},
		{"single word", []string{"user"}, "User"},
		{"two words", []string{"user", "name"}, "UserName"},
		{"acronym id", []string{"user", "id"}, "UserID"},
		{"acronym http", []string{"http", "server"}, "HTTPServer"},
		{"empty word skipped", []string{"user", "", "name"}, "UserName"},
		{"all empty", []string{"", ""}, ""},
		{"already upper acronym", []string{"URL"}, "URL"},
		{"mixed case word normalized", []string{"hELLo"}, "Hello"},
		{"acronym mixed case", []string{"Json"}, "JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// pascalWords mutates its input slice; pass a copy to be safe.
			in := append([]string(nil), tt.words...)
			if got := pascalWords(in); got != tt.want {
				t.Errorf("pascalWords(%v) = %q, want %q", tt.words, got, tt.want)
			}
		})
	}
}

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single lower", "a", "A"},
		{"single upper", "A", "A"},
		{"snake_case", "created_at", "CreatedAt"},
		{"camelCase", "createdAt", "CreatedAt"},
		{"PascalCase passthrough", "CreatedAt", "CreatedAt"},
		{"user_id acronym", "user_id", "UserID"},
		{"http_server acronym", "http_server", "HTTPServer"},
		{"kebab-case", "created-at", "CreatedAt"},
		{"space separated", "created at", "CreatedAt"},
		{"leading separator", "_created_at", "CreatedAt"},
		{"trailing separator", "created_at_", "CreatedAt"},
		{"consecutive separators", "created__at", "CreatedAt"},
		{"mixed separators", "foo-bar_baz qux", "FooBarBazQux"},
		{"all caps single", "json", "JSON"},
		{"all caps upper", "JSON", "JSON"},
		{"url acronym", "url", "URL"},
		{"id acronym", "id", "ID"},
		{"digits", "field1", "Field1"},
		{"digit segments", "field_1_name", "Field1Name"},
		// NOTE: acronyms are only recognized when a whole separator-delimited
		// word equals the acronym. "HTTPServer" arrives as a single word, so
		// the acronym is NOT preserved — it is normalized to "Httpserver".
		{"acronym embedded not preserved", "myHTTPServer", "MyHttpserver"},
		{"unicode word", "café", "Café"},
		{"only separators", "___", ""},
		{"multiword camel", "oneTwoThree", "OneTwoThree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToPascalCase(tt.in); got != tt.want {
				t.Errorf("ToPascalCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single lower", "a", "a"},
		{"single upper", "A", "a"},
		{"snake_case", "user_name", "userName"},
		{"PascalCase", "UserName", "userName"},
		// Same acronym limitation as ToPascalCase: "HTTPServer" is one word,
		// normalized to "Httpserver", then first rune lowered.
		{"HTTPServer normalized", "HTTPServer", "httpserver"},
		{"camelCase passthrough", "userName", "userName"},
		{"kebab-case", "user-name", "userName"},
		{"acronym id first", "id", "iD"},
		{"leading separator", "_user_name", "userName"},
		{"digits", "field1", "field1"},
		{"unicode", "café", "café"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToCamelCase(tt.in); got != tt.want {
				t.Errorf("ToCamelCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single lower", "a", "a"},
		{"single upper", "A", "a"},
		{"PascalCase", "UserName", "user_name"},
		{"HTTPServer", "HTTPServer", "http_server"},
		{"userID", "userID", "user_id"},
		{"already snake", "user_name", "user_name"},
		{"camelCase", "createdAt", "created_at"},
		{"all lower", "created", "created"},
		{"all upper", "HTTP", "http"},
		{"digits", "field1", "field1"},
		{"digit then upper", "field1Name", "field1name"}, // prev '1' not a letter -> no boundary inserted
		{"leading upper single word", "User", "user"},
		{"two acronyms", "APIID", "apiid"},
		// UTF-8 safety: ToSnakeCase now ranges over runes, so multi-byte input
		// is preserved (previously "café" was mangled to "caf_ã©" by byte
		// indexing splitting the é).
		{"unicode preserved", "café", "café"},
		{"unicode with boundary", "légèreValue", "légère_value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToSnakeCase(tt.in); got != tt.want {
				t.Errorf("ToSnakeCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRoundTripSanity exercises the interplay between the case converters on a
// few representative inputs, asserting stable, idempotent-ish behavior.
func TestRoundTripSanity(t *testing.T) {
	pascal := ToPascalCase("http_server")
	if pascal != "HTTPServer" {
		t.Fatalf("setup: got %q", pascal)
	}
	if snake := ToSnakeCase(pascal); snake != "http_server" {
		t.Errorf("ToSnakeCase(ToPascalCase(\"http_server\")) = %q, want %q", snake, "http_server")
	}
	// ToPascalCase should be idempotent for already-Pascal acronym-free input.
	if got := ToPascalCase(ToPascalCase("user_name")); got != "UserName" {
		t.Errorf("idempotency broken: %q", got)
	}
}

func TestErrorf(t *testing.T) {
	err := Errorf(nil, "something %s at %d", "broke", 42)
	if err == nil {
		t.Fatal("Errorf returned nil")
	}
	if got, want := err.Error(), "something broke at 42"; got != want {
		t.Errorf("Errorf() = %q, want %q", got, want)
	}

	// pos parameter is accepted but unused; passing a real position must not panic.
	pos := &ast.Position{Line: 1, Column: 2}
	err2 := Errorf(pos, "no args here")
	if err2 == nil || err2.Error() != "no args here" {
		t.Errorf("Errorf() with pos = %v, want error %q", err2, "no args here")
	}
}

func TestSaveToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	content := "hello\nworld\n"
	if err := SaveToFile(path, content); err != nil {
		t.Fatalf("SaveToFile error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
	// Verify permissions are 0644.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("file perm = %o, want %o", perm, 0644)
	}
}

func TestSaveToFileError(t *testing.T) {
	// Writing into a non-existent directory should return an error.
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist", "out.txt")
	if err := SaveToFile(path, "x"); err == nil {
		t.Error("SaveToFile to invalid path expected error, got nil")
	}
}

func TestDumpStructToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.json")

	type inner struct {
		Count int    `json:"count"`
		Label string `json:"label"`
	}
	value := struct {
		Name  string   `json:"name"`
		Tags  []string `json:"tags"`
		Inner inner    `json:"inner"`
	}{
		Name:  "gqlkit",
		Tags:  []string{"a", "b"},
		Inner: inner{Count: 3, Label: "x"},
	}

	if err := DumpStructToFile(value, path); err != nil {
		t.Fatalf("DumpStructToFile error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	// Re-parse and compare structurally, independent of indentation style.
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("output is not valid JSON: %v\ncontent:\n%s", err, string(raw))
	}
	if back["name"] != "gqlkit" {
		t.Errorf("name = %v, want %q", back["name"], "gqlkit")
	}
	tags, ok := back["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %v, want [a b]", back["tags"])
	}
	innerMap, ok := back["inner"].(map[string]any)
	if !ok {
		t.Fatalf("inner is not an object: %v", back["inner"])
	}
	if innerMap["count"] != float64(3) {
		t.Errorf("inner.count = %v, want 3", innerMap["count"])
	}
	if innerMap["label"] != "x" {
		t.Errorf("inner.label = %v, want %q", innerMap["label"], "x")
	}

	// Output should be indented (pretty-printed), i.e. contain a newline.
	if !containsNewline(string(raw)) {
		t.Errorf("expected pretty-printed JSON with newlines, got: %s", string(raw))
	}
}

func TestDumpStructToFileUnmarshalable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	// A channel cannot be marshaled to JSON; expect an error and no panic.
	err := DumpStructToFile(make(chan int), path)
	if err == nil {
		t.Error("DumpStructToFile with unmarshalable value expected error, got nil")
	}
	// Sanity: the wrapped error should be inspectable.
	_ = errors.Unwrap(err)
}

func containsNewline(s string) bool {
	for _, r := range s {
		if r == '\n' {
			return true
		}
	}
	return false
}

// Ensure the acronyms table entries all upper-case cleanly (guards against a
// lowercase key sneaking in that would never match strings.ToUpper output).
func TestAcronymsTableIntegrity(t *testing.T) {
	for k := range acronyms {
		for _, r := range k {
			if unicode.IsLetter(r) && !unicode.IsUpper(r) {
				t.Errorf("acronym key %q contains non-upper letter %q; it can never match", k, r)
			}
		}
	}
}
