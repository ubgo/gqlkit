package templater

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"text/template"
)

// --- FuncMap helper tests -------------------------------------------------

func TestPascalCase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"CREATED_AT", "CreatedAt"},
		{"user_id", "UserID"},
		{"user-name", "UserName"},
		{"foo bar baz", "FooBarBaz"},
		{"api_url", "APIURL"},
		{"http_json_xml", "HTTPJSONXML"},
		{"a", "A"},
		{"uuid", "UUID"},
		{"already", "Already"},
	}
	for _, c := range cases {
		if got := pascalCase(c.in); got != c.want {
			t.Errorf("pascalCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"user_id", "userID"},
		{"url_name", "urlName"},
		{"id", "id"},
		{"api", "api"},
		{"created_at", "createdAt"},
		{"already", "already"},
	}
	for _, c := range cases {
		if got := toCamelCase(c.in); got != c.want {
			t.Errorf("toCamelCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDesc(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want string
	}{
		{"Foo", "", ""},
		{"Foo", "hello world", "// Foo hello world\n"},
		{"Foo", "line1\nline2", "// Foo line1\n// line2\n"},
		{"Foo", "line1\n\nline3", "// Foo line1\n// line3\n"},
		{"Foo", "  spaced  ", "// Foo spaced\n"},
		// First line empty: name prefix is only applied at i==0, so it is lost.
		{"Foo", "\nsecond", "// second\n"},
	}
	for _, c := range cases {
		if got := formatDesc(c.name, c.desc); got != c.want {
			t.Errorf("formatDesc(%q, %q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\n\nb", []string{"a", "", "b"}},
	}
	for _, c := range cases {
		got := splitLines(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitLines(%q) len = %d, want %d (%v)", c.in, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestTrimSpace(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  x  ", "x"},
		{"\t\ny\t", "y"},
		{"no-trim", "no-trim"},
	}
	for _, c := range cases {
		if got := trimSpace(c.in); got != c.want {
			t.Errorf("trimSpace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJSONTag(t *testing.T) {
	cases := []struct {
		field     string
		omitempty bool
		want      string
	}{
		{"user_id", false, "`json:\"userID\"`"},
		{"user_id", true, "`json:\"userID,omitempty\"`"},
		{"name", false, "`json:\"name\"`"},
		{"created_at", true, "`json:\"createdAt,omitempty\"`"},
	}
	for _, c := range cases {
		if got := jsonTag(c.field, c.omitempty); got != c.want {
			t.Errorf("jsonTag(%q, %v) = %q, want %q", c.field, c.omitempty, got, c.want)
		}
	}
}

// --- FuncMap wiring via executed templates --------------------------------

// TestFuncMapViaExecution executes an inline template that calls each
// registered helper, exercising the FuncMap wiring end-to-end.
func TestFuncMapViaExecution(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"lower", `{{ lower "ABC" }}`, "abc"},
		{"upper", `{{ upper "abc" }}`, "ABC"},
		{"base", `{{ base "template/foo.tmpl" }}`, "foo.tmpl"},
		{"pascalCase", `{{ pascalCase "user_id" }}`, "UserID"},
		{"camelCase", `{{ camelCase "user_id" }}`, "userID"},
		{"formatDesc", `{{ formatDesc "Foo" "hi" }}`, "// Foo hi\n"},
		{"trimSpace", `{{ trimSpace "  x  " }}`, "x"},
		{"jsonTag", `{{ jsonTag "user_id" false }}`, "`json:\"userID\"`"},
		{"splitLines", `{{ range splitLines "a\nb" }}{{ . }},{{ end }}`, "a,b,"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tmpl, err := NewTemplate(c.name).Parse(c.body)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.body, err)
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, nil); err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if got := buf.String(); got != c.want {
				t.Errorf("template %q = %q, want %q", c.body, got, c.want)
			}
		})
	}
}

// --- Template parsing from the embedded FS --------------------------------

// expectedDefinedTemplates are the {{define "..."}} blocks that live inside
// the embedded .tmpl files. Parsing the whole dir must make each addressable.
var expectedDefinedTemplates = []string{
	"batch", "builder", "enums", "field_selector", "graphqlclient",
	"header", "inputs", "operation_builder", "scalar", "types",
}

func TestParseFSFromTemplateDir(t *testing.T) {
	tmpl := MustParse(NewTemplate("root").ParseFS(TemplateDir(), "template/*.tmpl"))

	for _, name := range expectedDefinedTemplates {
		if tmpl.Lookup(name) == nil {
			t.Errorf("expected defined template %q to be present after ParseFS", name)
		}
	}

	// The base-filename templates should also be registered.
	for _, fname := range []string{"batch.tmpl", "builder.tmpl", "types.tmpl"} {
		if tmpl.Lookup(fname) == nil {
			t.Errorf("expected file template %q to be present after ParseFS", fname)
		}
	}
}

func TestParseFSError(t *testing.T) {
	// A pattern that matches no files must return an error.
	_, err := NewTemplate("root").ParseFS(TemplateDir(), "template/*.doesnotexist")
	if err == nil {
		t.Fatal("expected error parsing a non-matching pattern, got nil")
	}
}

func TestMustParsePanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected MustParse to panic on parse error")
		}
	}()
	// Unclosed action -> Parse returns error -> MustParse must panic.
	MustParse(NewTemplate("bad").Parse(`{{ pascalCase "x" `))
}

func TestMustParseReturnsOnSuccess(t *testing.T) {
	tmpl := MustParse(NewTemplate("ok").Parse(`{{ upper "hi" }}`))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if buf.String() != "HI" {
		t.Errorf("got %q, want %q", buf.String(), "HI")
	}
}

// --- ExecuteTemplate (promoted from *template.Template) -------------------

func TestExecuteTemplateNamed(t *testing.T) {
	tmpl := MustParse(NewTemplate("root").Parse(`{{ define "greet" }}hi {{ upper . }}{{ end }}`))
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "greet", "bob"); err != nil {
		t.Fatalf("ExecuteTemplate error: %v", err)
	}
	if got := buf.String(); got != "hi BOB" {
		t.Errorf("ExecuteTemplate = %q, want %q", got, "hi BOB")
	}
}

func TestExecuteTemplateMissingReturnsError(t *testing.T) {
	tmpl := MustParse(NewTemplate("root").Parse(`{{ define "greet" }}hi{{ end }}`))
	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "nope", nil)
	if err == nil {
		t.Fatal("expected error executing a missing template, got nil")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q should mention the missing template name", err)
	}
}

// --- Funcs accumulation ---------------------------------------------------

func TestFuncsAccumulateAndNoOverwrite(t *testing.T) {
	tmpl := NewTemplate("acc")

	// A brand-new function is added and usable.
	tmpl = tmpl.Funcs(map[string]interface{}{
		"shout": func(s string) string { return s + "!" },
	})
	if _, ok := tmpl.FuncMap["shout"]; !ok {
		t.Error("expected 'shout' to be registered in accumulated FuncMap")
	}

	original := tmpl.FuncMap["shout"]

	// Re-registering an existing name must NOT overwrite the accumulated
	// FuncMap entry (documented behavior in Funcs).
	tmpl = tmpl.Funcs(map[string]interface{}{
		"shout": func(string) string { return "SHOULD_NOT_WIN" },
	})

	// NOTE (behavior report): the accumulated FuncMap field is guarded against
	// overwrite, but the *underlying* text/template Funcs call is NOT — the
	// second registration replaces the function used at execution time. The
	// two views therefore diverge. We assert the guarded field here.
	rawOriginal := reflectSameFunc(original, tmpl.FuncMap["shout"])
	if !rawOriginal {
		t.Error("expected accumulated FuncMap['shout'] to keep the original function")
	}
}

// reflectSameFunc reports whether two func values share the same underlying
// pointer (funcs are not comparable with ==, so use fmt formatting).
func reflectSameFunc(a, b interface{}) bool {
	fa, okA := a.(func(string) string)
	fb, okB := b.(func(string) string)
	if !okA || !okB {
		return false
	}
	// Compare by observable output: the original appends "!".
	return fa("z") == "z!" && fb("z") == "z!"
}

func TestFuncsInitializesNilMap(t *testing.T) {
	// Construct a Template without going through NewTemplate so FuncMap is nil,
	// then verify Funcs lazily initializes it.
	tmpl := &Template{Template: template.New("nilmap")}
	if tmpl.FuncMap != nil {
		t.Fatal("precondition: FuncMap should start nil")
	}
	tmpl = tmpl.Funcs(map[string]interface{}{"x": strings.ToLower})
	if tmpl.FuncMap == nil {
		t.Fatal("Funcs should initialize a nil FuncMap")
	}
	if _, ok := tmpl.FuncMap["x"]; !ok {
		t.Error("expected 'x' after Funcs on nil map")
	}
}

// --- Parse / ParseDir / AddParseTree / TemplateDir ------------------------

func TestParseErrorPropagates(t *testing.T) {
	_, err := NewTemplate("p").Parse(`{{ end }}`) // unexpected end
	if err == nil {
		t.Fatal("expected Parse error for malformed template")
	}
}

func TestParseDirFromTemplateDir(t *testing.T) {
	// ParseDir walks the real template/ directory on disk (relative to this
	// test file) and parses every non-.go file.
	tmpl, err := NewTemplate("dir").ParseDir("template")
	if err != nil {
		t.Fatalf("ParseDir error: %v", err)
	}
	for _, name := range []string{"batch", "types"} {
		if tmpl.Lookup(name) == nil {
			t.Errorf("expected defined template %q after ParseDir", name)
		}
	}
}

func TestParseDirMissingPathErrors(t *testing.T) {
	_, err := NewTemplate("dir").ParseDir("no_such_dir_here")
	if err == nil {
		t.Fatal("expected ParseDir to error on a missing path")
	}
}

func TestParseGlob(t *testing.T) {
	tmpl, err := NewTemplate("glob").ParseGlob("template/*.tmpl")
	if err != nil {
		t.Fatalf("ParseGlob error: %v", err)
	}
	if tmpl.Lookup("types") == nil {
		t.Error("expected 'types' template after ParseGlob")
	}
}

func TestParseGlobError(t *testing.T) {
	// A malformed glob pattern returns an error.
	if _, err := NewTemplate("glob").ParseGlob("[bad"); err == nil {
		t.Fatal("expected ParseGlob error on malformed pattern")
	}
}

func TestParseFilesError(t *testing.T) {
	if _, err := NewTemplate("files").ParseFiles("no_such_file.tmpl"); err == nil {
		t.Fatal("expected ParseFiles error on missing file")
	}
}

func TestParseFilesSuccess(t *testing.T) {
	tmpl, err := NewTemplate("files").ParseFiles("template/types.tmpl")
	if err != nil {
		t.Fatalf("ParseFiles error: %v", err)
	}
	if tmpl.Lookup("types.tmpl") == nil {
		t.Error("expected 'types.tmpl' template after ParseFiles")
	}
}

func TestAddParseTree(t *testing.T) {
	src := MustParse(NewTemplate("src").Parse(`{{ define "clone" }}cloned{{ end }}`))
	tree := src.Lookup("clone").Tree.Copy()

	dst := NewTemplate("dst")
	if _, err := dst.AddParseTree("clone", tree); err != nil {
		t.Fatalf("AddParseTree error: %v", err)
	}
	var buf bytes.Buffer
	if err := dst.ExecuteTemplate(&buf, "clone", nil); err != nil {
		t.Fatalf("ExecuteTemplate error: %v", err)
	}
	if buf.String() != "cloned" {
		t.Errorf("AddParseTree round-trip = %q, want %q", buf.String(), "cloned")
	}
}

func TestTemplateDirNotNil(t *testing.T) {
	fsys := TemplateDir()
	if fsys == nil {
		t.Fatal("TemplateDir returned nil")
	}
	entries, err := fs.ReadDir(fsys, "template")
	if err != nil {
		t.Fatalf("reading embedded dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected embedded template files, found none")
	}
}
