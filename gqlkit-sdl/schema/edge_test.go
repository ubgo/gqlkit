package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// TestFetchSchemaResponseMatrix walks the space of answers a real endpoint
// can give to the introspection POST and pins what FetchSchema does with each.
// Every row that is not a usable schema must be an error — a silent success
// would let the CLI write an empty schema file and print "Done".
func TestFetchSchemaResponseMatrix(t *testing.T) {
	valid := string(cannedIntrospectionJSON(t))

	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string // substring; "" means success expected
		wantIs  error  // optional sentinel
	}{
		{name: "200 valid schema", status: http.StatusOK, body: valid},
		{name: "200 with GraphQL errors (introspection disabled)", status: http.StatusOK,
			body:    `{"errors":[{"message":"introspection is disabled"},{"message":"second"}],"data":null}`,
			wantErr: "GraphQL errors: introspection is disabled; second"},
		{name: "200 errors alongside partial data", status: http.StatusOK,
			body:    `{"errors":[{"message":"partial"}],"data":{"__schema":{"queryType":{"name":"Query"}}}}`,
			wantErr: "partial"},
		{name: "200 data null without errors", status: http.StatusOK, body: `{"data":null}`, wantIs: ErrNoSchema},
		{name: "200 empty JSON object", status: http.StatusOK, body: `{}`, wantIs: ErrNoSchema},
		{name: "200 schema without queryType", status: http.StatusOK,
			body: `{"data":{"__schema":{"types":[]}}}`, wantIs: ErrNoSchema},
		{name: "200 empty body", status: http.StatusOK, body: "", wantErr: "failed to unmarshal response"},
		{name: "200 HTML page", status: http.StatusOK, body: "<html>login</html>", wantErr: "failed to unmarshal response"},
		{name: "200 JSON array", status: http.StatusOK, body: `[1,2]`, wantErr: "failed to unmarshal response"},
		{name: "401 with body", status: http.StatusUnauthorized, body: "no token", wantErr: "unexpected status code 401: no token"},
		{name: "404 empty body", status: http.StatusNotFound, body: "", wantErr: "unexpected status code 404"},
		{name: "502 gateway", status: http.StatusBadGateway, body: "upstream down", wantErr: "unexpected status code 502"},
		{name: "201 is not 200", status: http.StatusCreated, body: valid, wantErr: "unexpected status code 201"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := FetchSchema(srv.URL, nil)
			switch {
			case tc.wantErr == "" && tc.wantIs == nil:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil || got.QueryType == nil {
					t.Fatalf("expected a schema, got %+v", got)
				}
			default:
				if err == nil {
					t.Fatalf("expected error, got schema %+v", got)
				}
				if got != nil {
					t.Errorf("error path must not return a schema, got %+v", got)
				}
				if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err, tc.wantErr)
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Errorf("error %v is not %v", err, tc.wantIs)
				}
			}
		})
	}
}

// TestFetchSchemaTimeout pins that a server which accepts the connection but
// never answers fails within FetchOptions.Timeout instead of hanging forever.
func TestFetchSchemaTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	start := time.Now()
	_, err := FetchSchema(srv.URL, &FetchOptions{Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout not honoured: took %v", elapsed)
	}
}

// TestFetchSchemaRedirect covers the two redirect families. 307/308 keep the
// POST body, so introspection succeeds at the new location; 301/302 turn the
// POST into a body-less GET, which a GraphQL endpoint rejects — that must
// surface as an error, never as an empty schema.
func TestFetchSchemaRedirect(t *testing.T) {
	valid := cannedIntrospectionJSON(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "GET not supported", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write(valid)
	}))
	defer target.Close()

	cases := []struct {
		code    int
		wantErr bool
	}{
		{http.StatusTemporaryRedirect, false},
		{http.StatusPermanentRedirect, false},
		{http.StatusMovedPermanently, true},
		{http.StatusFound, true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			redir := httptest.NewServer(http.RedirectHandler(target.URL, tc.code))
			defer redir.Close()
			got, err := FetchSchema(redir.URL, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error after %d redirect, got %+v", tc.code, got)
				}
				return
			}
			if err != nil || got == nil || got.QueryType == nil {
				t.Fatalf("redirect %d: schema=%+v err=%v", tc.code, got, err)
			}
		})
	}
}

// TestFetchSchemaHeaders pins header forwarding, including "Host", which
// net/http drops from Request.Header and must instead be set as Request.Host.
func TestFetchSchemaHeaders(t *testing.T) {
	var gotHost, gotAuth, gotCustom, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost, gotAuth, gotCustom, gotCT = r.Host, r.Header.Get("Authorization"), r.Header.Get("X-Tenant"), r.Header.Get("Content-Type")
		_, _ = w.Write(cannedIntrospectionJSON(t))
	}))
	defer srv.Close()

	_, err := FetchSchema(srv.URL, &FetchOptions{Headers: map[string]string{
		"host":          "api.example.test",
		"Authorization": "Bearer x",
		"x-tenant":      "acme",
		"Content-Type":  "application/graphql+json",
	}})
	if err != nil {
		t.Fatalf("FetchSchema: %v", err)
	}
	if gotHost != "api.example.test" {
		t.Errorf("Host = %q, want api.example.test", gotHost)
	}
	if gotAuth != "Bearer x" || gotCustom != "acme" {
		t.Errorf("headers not forwarded: auth=%q tenant=%q", gotAuth, gotCustom)
	}
	// A user-supplied Content-Type overrides the default, by design.
	if gotCT != "application/graphql+json" {
		t.Errorf("Content-Type = %q, want user override", gotCT)
	}
}

// TestFetchSchemaInvalidHeaderName pins that a header the HTTP layer cannot
// send is an error, not a silently dropped header.
func TestFetchSchemaInvalidHeaderName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(cannedIntrospectionJSON(t))
	}))
	defer srv.Close()
	if _, err := FetchSchema(srv.URL, &FetchOptions{Headers: map[string]string{"": "v"}}); err == nil {
		t.Fatal("expected error for empty header name")
	}
}

// TestFetchSchemaHugeSchema exercises a schema far larger than a typical one
// through fetch, convert and parse, so a size-dependent truncation or
// quadratic blow-up would show here.
func TestFetchSchemaHugeSchema(t *testing.T) {
	const typeCount = 3000
	s := IntrospectionSchema{QueryType: &TypeRef{Name: "Query"}}
	var queryFields []Field
	for i := range typeCount {
		name := fmt.Sprintf("T%d", i)
		s.Types = append(s.Types, FullType{Kind: kindObject, Name: name, Fields: []Field{
			{Name: "id", Type: nonNull(named(kindScalar, "ID"))},
			{Name: "label", Type: named(kindScalar, "String")},
		}})
		queryFields = append(queryFields, Field{Name: fmt.Sprintf("t%d", i), Type: named(kindObject, name)})
	}
	s.Types = append(s.Types, FullType{Kind: kindObject, Name: "Query", Fields: queryFields})
	body, err := json.Marshal(IntrospectionResponse{Data: IntrospectionData{Schema: s}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, err := FetchSchema(srv.URL, nil)
	if err != nil {
		t.Fatalf("FetchSchema: %v", err)
	}
	if len(got.Types) != typeCount+1 {
		t.Fatalf("got %d types, want %d", len(got.Types), typeCount+1)
	}
	sdl := ConvertToSDL(got)
	doc, gqlErr := gqlparser.LoadSchema(&ast.Source{Name: "huge.graphql", Input: sdl})
	if gqlErr != nil {
		t.Fatalf("generated SDL does not parse: %v", gqlErr)
	}
	// The parser adds the implicit __schema / __type meta-fields to Query.
	userFields := 0
	for _, f := range doc.Query.Fields {
		if !strings.HasPrefix(f.Name, metaTypePrefix) {
			userFields++
		}
	}
	if userFields != typeCount {
		t.Errorf("parsed Query has %d fields, want %d", userFields, typeCount)
	}
}

// TestConvertToSDLParsesBack is the round-trip contract: for descriptions and
// deprecation reasons containing every character that needs escaping, the
// emitted SDL must parse with a spec-compliant parser and keep the text.
func TestConvertToSDLParsesBack(t *testing.T) {
	cases := []struct {
		name string
		desc string
	}{
		{"plain", "simple"},
		{"double quote", `say "hi"`},
		{"backslash", `C:\path`},
		{"multi-line", "line one\nline two"},
		{"triple quote in block", "first\nhas \"\"\" inside"},
		{"triple quote single line", `has """ inside`},
		{"lone carriage return", "a\rb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := tc.desc
			s := &IntrospectionSchema{
				QueryType: &TypeRef{Name: "Query"},
				Types: []FullType{{Kind: kindObject, Name: "Query", Description: &desc, Fields: []Field{
					{Name: "f", Description: &desc, Type: named(kindScalar, "String")},
				}}},
			}
			sdl := ConvertToSDL(s)
			doc, err := gqlparser.LoadSchema(&ast.Source{Name: "t.graphql", Input: sdl})
			if err != nil {
				t.Fatalf("SDL does not parse: %v\n%s", err, sdl)
			}
			if doc.Query == nil || doc.Query.Fields.ForName("f") == nil {
				t.Fatalf("parsed schema lost the field:\n%s", sdl)
			}
			// Block strings normalise line endings, so compare with \r
			// treated as a line break.
			want := strings.ReplaceAll(tc.desc, "\r", "\n")
			if got := doc.Query.Fields.ForName("f").Description; got != want {
				t.Errorf("description round-trip = %q, want %q", got, want)
			}
		})
	}
}

// TestConvertToSDLEmptyAndMetaOnly covers degenerate inputs: no types, and a
// schema holding only introspection meta-types and built-in scalars. Both must
// produce empty output rather than a stray block or a redeclared builtin.
func TestConvertToSDLEmptyAndMetaOnly(t *testing.T) {
	if got := ConvertToSDL(&IntrospectionSchema{}); got != "" {
		t.Errorf("empty schema produced %q", got)
	}
	s := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types: []FullType{
			{Kind: kindObject, Name: "__Type"},
			{Kind: kindScalar, Name: "String"},
			{Kind: kindScalar, Name: "ID"},
		},
		Directives: []Directive{{Name: "skip"}, {Name: "deprecated"}},
	}
	if got := ConvertToSDL(s); got != "" {
		t.Errorf("meta-only schema produced %q", got)
	}
}

// TestConvertToSDLDeterministic pins that type order in the introspection
// response does not change the output, so regenerated files diff cleanly.
func TestConvertToSDLDeterministic(t *testing.T) {
	a := []FullType{
		{Kind: kindObject, Name: "Query", Fields: []Field{{Name: "x", Type: named(kindScalar, "Int")}}},
		{Kind: kindEnum, Name: "Color", EnumValues: []EnumValue{{Name: "RED"}}},
		{Kind: kindScalar, Name: "Date"},
	}
	b := []FullType{a[2], a[0], a[1]}
	sa := ConvertToSDL(&IntrospectionSchema{QueryType: &TypeRef{Name: "Query"}, Types: a})
	sb := ConvertToSDL(&IntrospectionSchema{QueryType: &TypeRef{Name: "Query"}, Types: b})
	if sa != sb {
		t.Errorf("output depends on input order:\n%s\n---\n%s", sa, sb)
	}
	// The input slice must not be reordered as a side effect.
	if a[0].Name != "Query" {
		t.Errorf("ConvertToSDL mutated its input order")
	}
}

// TestSaveOutputMatrix covers the output-writing space: a fresh path, an
// existing file (overwritten, never appended), an idempotent re-run, a
// missing parent directory and an unwritable one.
func TestSaveOutputMatrix(t *testing.T) {
	s := &IntrospectionSchema{QueryType: &TypeRef{Name: "Query"}, Types: []FullType{
		{Kind: kindObject, Name: "Query", Fields: []Field{{Name: "ok", Type: named(kindScalar, "Boolean")}}},
	}}

	t.Run("overwrites a longer existing file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "schema.graphql")
		if err := os.WriteFile(p, []byte(strings.Repeat("stale\n", 1000)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := SaveToFile("type Query { ok: Boolean }\n", p); err != nil {
			t.Fatalf("SaveToFile: %v", err)
		}
		got, _ := os.ReadFile(p)
		if strings.Contains(string(got), "stale") {
			t.Errorf("old content survived:\n%s", got)
		}
	})

	t.Run("idempotent re-run", func(t *testing.T) {
		dir := t.TempDir()
		for _, write := range []func(string) error{
			func(p string) error { return SaveToFile(ConvertToSDL(s), p) },
			func(p string) error { return SaveAsJSON(s, p) },
		} {
			p := filepath.Join(dir, "out")
			if err := write(p); err != nil {
				t.Fatal(err)
			}
			first, _ := os.ReadFile(p)
			if err := write(p); err != nil {
				t.Fatal(err)
			}
			second, _ := os.ReadFile(p)
			if string(first) != string(second) {
				t.Errorf("second run differs:\n%s\n---\n%s", first, second)
			}
		}
	})

	t.Run("JSON round-trips through FetchSchema's parser", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "schema.json")
		if err := SaveAsJSON(s, p); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(p)
		var resp IntrospectionResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("saved JSON does not parse: %v", err)
		}
		if resp.Data.Schema.QueryType == nil || len(resp.Data.Schema.Types) != 1 {
			t.Errorf("round-trip lost data: %+v", resp.Data.Schema)
		}
	})

	t.Run("missing parent directory", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "nope", "schema.graphql")
		if err := SaveToFile("x", p); err == nil {
			t.Error("SaveToFile: expected error for missing directory")
		}
		if err := SaveAsJSON(s, p); err == nil {
			t.Error("SaveAsJSON: expected error for missing directory")
		}
	})

	t.Run("unwritable directory", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("directory permission bits are not enforced here")
		}
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		if err := SaveToFile("x", filepath.Join(dir, "schema.graphql")); err == nil {
			t.Error("expected permission error")
		}
	})
}

// TestFetchSchemaErrorBodyTruncated pins the non-200 excerpt: short bodies
// are quoted whole, bodies past maxErrorBodyBytes are cut and marked.
func TestFetchSchemaErrorBodyTruncated(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantTrunc bool
	}{
		{"short", "denied", false},
		{"exactly the cap", strings.Repeat("a", maxErrorBodyBytes), false},
		{"one past the cap", strings.Repeat("b", maxErrorBodyBytes+1), true},
		{"whole HTML page", "<html>" + strings.Repeat("x", 100_000) + "</html>", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := FetchSchema(srv.URL, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			msg := err.Error()
			if !strings.Contains(msg, "unexpected status code 403") {
				t.Errorf("status missing: %q", msg)
			}
			if got := strings.HasSuffix(msg, truncatedSuffix); got != tc.wantTrunc {
				t.Errorf("truncated = %v, want %v (len %d)", got, tc.wantTrunc, len(msg))
			}
			if !tc.wantTrunc && !strings.HasSuffix(msg, tc.body) {
				t.Errorf("short body not quoted whole: %q", msg)
			}
			if limit := len("unexpected status code 403: ") + maxErrorBodyBytes + len(truncatedSuffix); len(msg) > limit {
				t.Errorf("message length %d exceeds %d", len(msg), limit)
			}
		})
	}
}

// TestFetchSchemaDebugOutput: the curl line goes to DebugOutput, nothing is
// sent, and the result is (nil, nil).
func TestFetchSchemaDebugOutput(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	var buf strings.Builder
	got, err := FetchSchema(srv.URL, &FetchOptions{Debug: true, DebugOutput: &buf, Headers: map[string]string{"X-K": "v"}})
	if err != nil || got != nil {
		t.Fatalf("got %+v, %v; want nil, nil", got, err)
	}
	if hit {
		t.Error("debug mode sent the request")
	}
	if out := buf.String(); !strings.HasPrefix(out, "curl -X 'POST'") || !strings.Contains(out, "X-K: v") || !strings.Contains(out, srv.URL) {
		t.Errorf("curl output = %q", out)
	}
}

// TestDeprecationReasonParsesBack is the round-trip contract for
// @deprecated(reason: ...) on fields and enum values: every character class
// that needs escaping must survive a spec-compliant parse unchanged. Go's %q,
// used before, emitted \x01-style escapes that the parser rejects.
func TestDeprecationReasonParsesBack(t *testing.T) {
	reasons := []string{
		"plain",
		`use "other" instead`,
		`C:\old\path`,
		"two\nlines",
		"tab\there",
		"cr\rhere",
		"bell\x07and\x01ctl",
		"del\x7fchar",
		"unicode é 漢字 ✓",
		"form\ffeed and back\bspace",
	}
	for _, reason := range reasons {
		t.Run(fmt.Sprintf("%q", reason), func(t *testing.T) {
			s := &IntrospectionSchema{
				QueryType: &TypeRef{Name: "Query"},
				Types: []FullType{
					{Kind: kindObject, Name: "Query", Fields: []Field{
						{Name: "old", Type: named(kindEnum, "Color"), IsDeprecated: true, DeprecationReason: &reason},
					}},
					{Kind: kindEnum, Name: "Color", EnumValues: []EnumValue{
						{Name: "RED"},
						{Name: "OLD", IsDeprecated: true, DeprecationReason: &reason},
					}},
				},
			}
			sdl := ConvertToSDL(s)
			doc, err := gqlparser.LoadSchema(&ast.Source{Name: "d.graphql", Input: sdl})
			if err != nil {
				t.Fatalf("SDL does not parse: %v\n%s", err, sdl)
			}
			fieldReason := doc.Query.Fields.ForName("old").Directives.ForName("deprecated").Arguments.ForName("reason").Value.Raw
			enumReason := doc.Types["Color"].EnumValues.ForName("OLD").Directives.ForName("deprecated").Arguments.ForName("reason").Value.Raw
			if fieldReason != reason || enumReason != reason {
				t.Errorf("round-trip: field %q enum %q, want %q", fieldReason, enumReason, reason)
			}
		})
	}
}

// TestFilterPatternMatrix pins whole-name matching for every pattern shape.
// The regression rows are the alternations: `^`+p+`$` without a group made
// "users|posts" keep anything starting with users or ending with posts.
func TestFilterPatternMatrix(t *testing.T) {
	names := []string{"todo", "todos", "todosConnection", "echo", "echoAll", "myecho", "user"}
	s := &IntrospectionSchema{QueryType: &TypeRef{Name: "Query"}}
	var fields []Field
	for _, n := range names {
		fields = append(fields, Field{Name: n, Type: named(kindScalar, "String")})
	}
	s.Types = []FullType{{Kind: kindObject, Name: "Query", Fields: fields}}

	cases := []struct {
		pattern string
		want    []string // kept by OnlyQueries
	}{
		{"todo", []string{"todo"}},
		{"todo|echo", []string{"todo", "echo"}},
		{"todo|echo|user", []string{"todo", "echo", "user"}},
		{"todos.*", []string{"todos", "todosConnection"}},
		{"(todo|echo)", []string{"todo", "echo"}},
		{"^todo$", []string{"todo"}},
		{"^todo|echo$", []string{"todo", "echo"}},
		{".*echo", []string{"echo", "myecho"}},
		{"to.o", []string{"todo"}},
	}
	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			out, err := FilterSchema(s, &FilterOptions{OnlyQueries: []string{tc.pattern}})
			if err != nil {
				t.Fatal(err)
			}
			if got := fieldNames(out, "Query"); strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("only %q kept %v, want %v", tc.pattern, got, tc.want)
			}
			// Exclude is the complement over the same names.
			out, err = FilterSchema(s, &FilterOptions{ExcludeQueries: []string{tc.pattern}, OnlyQueries: []string{".*"}})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(fieldNames(out, "Query")); got != len(names)-len(tc.want) {
				t.Errorf("exclude %q kept %d fields, want %d", tc.pattern, got, len(names)-len(tc.want))
			}
		})
	}
}

// TestFilterInvalidPatternErrors: every filter list reports a bad regex,
// naming it, even when its root type is absent.
func TestFilterInvalidPatternErrors(t *testing.T) {
	noMutation := &IntrospectionSchema{QueryType: &TypeRef{Name: "Query"}, Types: []FullType{
		{Kind: kindObject, Name: "Query", Fields: []Field{{Name: "a", Type: named(kindScalar, "String")}}},
	}}
	for name, opts := range map[string]*FilterOptions{
		"only-queries":      {OnlyQueries: []string{"user("}},
		"exclude-queries":   {ExcludeQueries: []string{"user("}},
		"only-mutations":    {OnlyMutations: []string{"user("}},
		"exclude-mutations": {ExcludeMutations: []string{"user("}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := FilterSchema(noMutation, opts)
			if err == nil || !strings.Contains(err.Error(), `"user("`) {
				t.Errorf("err = %v, want one naming %q", err, "user(")
			}
		})
	}
}

// TestFilterEmptyRoots: an emptied mutation root is dropped with its schema
// entry (and, with RemoveUnused, the types only it used); an emptied query
// root is ErrEmptyQueryRoot. Either way the result must be parseable SDL.
func TestFilterEmptyRoots(t *testing.T) {
	t.Run("mutation dropped, SDL parses", func(t *testing.T) {
		out, err := FilterSchema(sampleSchema(), &FilterOptions{ExcludeMutations: []string{".*"}, RemoveUnused: true})
		if err != nil {
			t.Fatal(err)
		}
		if out.MutationType != nil || hasType(out, "Mutation") {
			t.Errorf("empty Mutation kept: root=%v", out.MutationType)
		}
		if hasType(out, "CreateUserInput") {
			t.Error("input only the dropped mutation used was not pruned")
		}
		sdl := ConvertToSDL(out)
		if _, err := gqlparser.LoadSchema(&ast.Source{Name: "m.graphql", Input: sdl}); err != nil {
			t.Fatalf("SDL does not parse: %v\n%s", err, sdl)
		}
	})
	t.Run("only-mutations matching nothing drops mutation", func(t *testing.T) {
		out, err := FilterSchema(sampleSchema(), &FilterOptions{OnlyMutations: []string{"nope"}})
		if err != nil || out.MutationType != nil {
			t.Errorf("root=%v err=%v", out.MutationType, err)
		}
	})
	t.Run("custom-named mutation root dropped", func(t *testing.T) {
		s := sampleSchema()
		s.MutationType = &TypeRef{Name: "RootMutation"}
		for i := range s.Types {
			if s.Types[i].Name == "Mutation" {
				s.Types[i].Name = "RootMutation"
			}
		}
		out, err := FilterSchema(s, &FilterOptions{ExcludeMutations: []string{".*"}})
		if err != nil || out.MutationType != nil || hasType(out, "RootMutation") {
			t.Errorf("root=%v err=%v", out.MutationType, err)
		}
		if strings.Contains(ConvertToSDL(out), "mutation:") {
			t.Error("schema block still names the dropped mutation root")
		}
	})
	t.Run("empty subscription root dropped", func(t *testing.T) {
		s := sampleSchema()
		s.SubscriptionType = &TypeRef{Name: "Subscription"}
		s.Types = append(s.Types, FullType{Kind: kindObject, Name: "Subscription"})
		out, err := FilterSchema(s, &FilterOptions{ExcludeQueries: []string{"posts"}})
		if err != nil || out.SubscriptionType != nil || hasType(out, "Subscription") {
			t.Errorf("root=%v err=%v", out.SubscriptionType, err)
		}
	})
	t.Run("partially filtered mutation kept", func(t *testing.T) {
		out, err := FilterSchema(sampleSchema(), &FilterOptions{ExcludeMutations: []string{"deleteTask"}})
		if err != nil || out.MutationType == nil || len(fieldNames(out, "Mutation")) != 1 {
			t.Errorf("root=%v fields=%v err=%v", out.MutationType, fieldNames(out, "Mutation"), err)
		}
	})
	for name, opts := range map[string]*FilterOptions{
		"exclude every query": {ExcludeQueries: []string{".*"}},
		"only matches none":   {OnlyQueries: []string{"nope"}},
	} {
		t.Run("query emptied: "+name, func(t *testing.T) {
			s := sampleSchema()
			out, err := FilterSchema(s, opts)
			if !errors.Is(err, ErrEmptyQueryRoot) || out != nil {
				t.Errorf("out=%v err=%v, want ErrEmptyQueryRoot", out, err)
			}
			if len(fieldNames(s, "Query")) != 3 {
				t.Error("input schema was mutated")
			}
		})
	}
}
