package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
