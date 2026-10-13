package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// introspectionJSON is a minimal but valid introspection response: a Query root
// with one scalar field. Served raw so this package-main test doesn't depend on
// the schema package's (unexported) test fixtures.
const introspectionJSON = `{"data":{"__schema":{
  "queryType":{"name":"Query"},
  "mutationType":null,
  "subscriptionType":null,
  "types":[
    {"kind":"OBJECT","name":"Query","fields":[
      {"name":"hello","args":[],"type":{"kind":"SCALAR","name":"String","ofType":null}}
    ],"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null}
  ],
  "directives":[]
}}}`

// introspectionServer returns an httptest server that answers introspection.
func introspectionServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(introspectionJSON))
	}))
	t.Cleanup(s.Close)
	return s
}

// TestFetchCmdSDL drives the fetch command end to end and asserts an SDL file.
func TestFetchCmdSDL(t *testing.T) {
	srv := introspectionServer(t)
	out := filepath.Join(t.TempDir(), "schema.graphql")

	cmd := fetchCmd()
	cmd.SetArgs([]string{"--url", srv.URL, "-o", out, "-H", "Authorization: Bearer x"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(b) == 0 {
		t.Error("expected non-empty SDL output")
	}
}

// TestFetchCmdJSON covers the --format json branch.
func TestFetchCmdJSON(t *testing.T) {
	srv := introspectionServer(t)
	out := filepath.Join(t.TempDir(), "schema.json")

	cmd := fetchCmd()
	cmd.SetArgs([]string{"--url", srv.URL, "-o", out, "-f", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fetch json: %v", err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Errorf("expected non-empty JSON output (err=%v)", err)
	}
}

// TestFetchCmdWithFilters exercises the filter branch (--only-queries + --remove-unused).
func TestFetchCmdWithFilters(t *testing.T) {
	srv := introspectionServer(t)
	out := filepath.Join(t.TempDir(), "schema.graphql")

	cmd := fetchCmd()
	cmd.SetArgs([]string{"--url", srv.URL, "-o", out, "--only-queries", "hello", "--remove-unused"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fetch with filters: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected output file: %v", err)
	}
}

// TestFetchCmdInvalidHeader covers the malformed-header error path.
func TestFetchCmdInvalidHeader(t *testing.T) {
	cmd := fetchCmd()
	cmd.SetArgs([]string{"--url", "http://example.invalid", "-H", "no-colon-here"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for malformed header")
	}
}

// TestFetchCmdFetchError covers the FetchSchema failure path (server 500).
func TestFetchCmdFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cmd := fetchCmd()
	cmd.SetArgs([]string{"--url", srv.URL, "-o", filepath.Join(t.TempDir(), "s.graphql")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

// TestVersionCmd covers the version subcommand.
func TestVersionCmd(t *testing.T) {
	cmd := versionCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
}

// runFetch executes the fetch command with args and returns its stdout.
func runFetch(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := fetchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// TestFetchCmdFlagMatrix walks the CLI's flag space against one server and
// pins, per combination, whether a file is written, what it starts with, and
// the progress lines printed.
func TestFetchCmdFlagMatrix(t *testing.T) {
	srv := introspectionServer(t)

	cases := []struct {
		name       string
		args       func(out string) []string
		wantFile   bool
		filePrefix string
		wantOut    []string
		notOut     []string
	}{
		{
			name:       "default format",
			args:       func(o string) []string { return []string{"--url", srv.URL, "-o", o} },
			wantFile:   true,
			filePrefix: "type Query {",
			wantOut:    []string{"Fetching schema from " + srv.URL, "Converting to SDL format...", "Done!"},
			notOut:     []string{"Applying filters"},
		},
		{
			name:       "explicit graphql format",
			args:       func(o string) []string { return []string{"--url", srv.URL, "-o", o, "--format", formatGraphQL} },
			wantFile:   true,
			filePrefix: "type Query {",
		},
		{
			name:       "json format",
			args:       func(o string) []string { return []string{"--url", srv.URL, "-o", o, "-f", formatJSON} },
			wantFile:   true,
			filePrefix: "{",
			wantOut:    []string{"Saving JSON to"},
			notOut:     []string{"Converting to SDL"},
		},
		{
			name:     "remove-unused alone triggers filtering",
			args:     func(o string) []string { return []string{"--url", srv.URL, "-o", o, "--remove-unused"} },
			wantFile: true,
			wantOut:  []string{"Applying filters..."},
		},
		{
			name:     "repeated headers",
			args:     func(o string) []string { return []string{"--url", srv.URL, "-o", o, "-H", "A: 1", "-H", "B:2"} },
			wantFile: true,
		},
		{
			name: "debug prints curl and writes nothing",
			args: func(o string) []string {
				return []string{"--url", srv.URL, "-o", o, "--debug", "-H", "Authorization: Bearer s"}
			},
			wantFile: false,
			wantOut:  []string{"curl -X 'POST'", "Authorization: Bearer s"},
			notOut:   []string{"Fetching schema", "Done!"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out")
			out, err := runFetch(t, tc.args(path)...)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			b, statErr := os.ReadFile(path)
			if tc.wantFile != (statErr == nil) {
				t.Fatalf("file written = %v, want %v", statErr == nil, tc.wantFile)
			}
			if tc.filePrefix != "" && !strings.HasPrefix(string(b), tc.filePrefix) {
				t.Errorf("file starts %q, want prefix %q", string(b), tc.filePrefix)
			}
			for _, w := range tc.wantOut {
				if !strings.Contains(out, w) {
					t.Errorf("stdout missing %q:\n%s", w, out)
				}
			}
			for _, n := range tc.notOut {
				if strings.Contains(out, n) {
					t.Errorf("stdout should not contain %q:\n%s", n, out)
				}
			}
		})
	}
}

// TestFetchCmdErrorMatrix pins that every failure exits non-nil and leaves
// no output file behind.
func TestFetchCmdErrorMatrix(t *testing.T) {
	gqlErrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"introspection disabled"}]}`))
	}))
	defer gqlErrSrv.Close()
	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer emptySrv.Close()
	okSrv := introspectionServer(t)

	cases := []struct {
		name    string
		args    func(out string) []string
		wantErr string
	}{
		{"unknown format", func(o string) []string { return []string{"--url", okSrv.URL, "-o", o, "-f", "yaml"} }, `unknown --format "yaml" (valid: graphql, json)`},
		{"format is case-sensitive", func(o string) []string { return []string{"--url", okSrv.URL, "-o", o, "-f", "JSON"} }, "unknown --format"},
		{"missing --url", func(o string) []string { return []string{"-o", o} }, `required flag(s) "url" not set`},
		{"empty --url", func(o string) []string { return []string{"--url", "", "-o", o} }, "fetching schema"},
		{"unsupported scheme", func(o string) []string { return []string{"--url", "ftp://x", "-o", o} }, "fetching schema"},
		{"GraphQL errors", func(o string) []string { return []string{"--url", gqlErrSrv.URL, "-o", o} }, "introspection disabled"},
		{"no schema in data", func(o string) []string { return []string{"--url", emptySrv.URL, "-o", o} }, "no introspection schema"},
		{"header without colon", func(o string) []string { return []string{"--url", okSrv.URL, "-o", o, "-H", "Bearer"} }, "invalid header format"},
		{"output dir missing", func(o string) []string {
			return []string{"--url", okSrv.URL, "-o", filepath.Join(o, "missing", "s.graphql")}
		}, "saving file"},
		{"json output dir missing", func(o string) []string {
			return []string{"--url", okSrv.URL, "-f", formatJSON, "-o", filepath.Join(o, "missing", "s.json")}
		}, "saving JSON file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out")
			_, err := runFetch(t, tc.args(path)...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Errorf("failed run left a file at %s", path)
			}
		})
	}
}

// TestFetchCmdOverwriteAndRerun: an existing output is replaced, and a second
// identical run produces byte-identical output.
func TestFetchCmdOverwriteAndRerun(t *testing.T) {
	srv := introspectionServer(t)
	path := filepath.Join(t.TempDir(), "schema.graphql")
	if err := os.WriteFile(path, []byte(strings.Repeat("old ", 5000)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runFetch(t, "--url", srv.URL, "-o", path); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if strings.Contains(string(first), "old") {
		t.Fatal("old content survived the overwrite")
	}
	if _, err := runFetch(t, "--url", srv.URL, "-o", path); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Errorf("re-run not idempotent:\n%s\n---\n%s", first, second)
	}
}

// TestVersionCmdOutput pins the printed shape "gqlkit-sdl <version>".
func TestVersionCmdOutput(t *testing.T) {
	cmd := versionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "gqlkit-sdl ") {
		t.Errorf("version output = %q", out.String())
	}
}

// fullIntrospectionServer answers with a Query and a Mutation root, for the
// filter cases that empty one of them.
func fullIntrospectionServer(t *testing.T) *httptest.Server {
	t.Helper()
	const body = `{"data":{"__schema":{
  "queryType":{"name":"Query"},"mutationType":{"name":"Mutation"},"subscriptionType":null,
  "types":[
    {"kind":"OBJECT","name":"Query","fields":[{"name":"todo","args":[],"type":{"kind":"SCALAR","name":"String"}},{"name":"todos","args":[],"type":{"kind":"SCALAR","name":"String"}},{"name":"echo","args":[],"type":{"kind":"SCALAR","name":"String"}}]},
    {"kind":"OBJECT","name":"Mutation","fields":[{"name":"createTodo","args":[],"type":{"kind":"SCALAR","name":"String"}}]}
  ],"directives":[]}}}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

// TestFetchCmdFilterEdges covers the filter outcomes a user sees: alternation
// keeps exactly the named fields, an emptied Mutation is dropped with a note
// on stderr, an emptied Query and an invalid regex are errors with no file.
func TestFetchCmdFilterEdges(t *testing.T) {
	srv := fullIntrospectionServer(t)

	t.Run("alternation keeps whole names only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "s.graphql")
		if _, err := runFetch(t, "--url", srv.URL, "-o", path, "--only-queries", "todo|echo"); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(path)
		if !strings.Contains(string(b), "  todo: String") || !strings.Contains(string(b), "  echo: String") || strings.Contains(string(b), "todos") {
			t.Errorf("output:\n%s", b)
		}
	})

	t.Run("emptied mutation dropped with stderr note", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "s.graphql")
		cmd := fetchCmd()
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"--url", srv.URL, "-o", path, "--exclude-mutations", ".*"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "Mutation") {
			t.Errorf("empty Mutation written:\n%s", b)
		}
		if !strings.Contains(stderr.String(), "dropped the empty Mutation type") {
			t.Errorf("stderr = %q", stderr.String())
		}
		if strings.Contains(stdout.String(), "Note: filters removed") {
			t.Errorf("note went to stdout: %q", stdout.String())
		}
	})

	for name, args := range map[string][]string{
		"emptied query": {"--exclude-queries", ".*"},
		"invalid regex": {"--only-queries", "todo("},
	} {
		t.Run(name+" is an error", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.graphql")
			_, err := runFetch(t, append([]string{"--url", srv.URL, "-o", path}, args...)...)
			if err == nil || !strings.Contains(err.Error(), "filtering schema") {
				t.Fatalf("err = %v", err)
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Error("failed run wrote a file")
			}
		})
	}
}
