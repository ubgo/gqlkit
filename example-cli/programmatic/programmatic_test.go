// Package programmatic_test runs the two example programs end to end, the way
// docs/cli/programmatic.md describes them: sdlapi fetches and trims the
// mockapi schema, then gen generates a Go and a TypeScript SDK from it, and
// the Go SDK must compile. Everything happens in a temporary directory, so a
// run leaves nothing behind in the repository.
package programmatic_test

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockapiURL is the endpoint sdlapi/main.go fetches from. It is hard-coded in
// that program because the docs quote it verbatim.
const mockapiURL = "http://localhost:8081/query"

// sdkModule is the module the generated Go SDK lives in; gen/main.go sets
// ModulePath to sdkModule + "/sdk".
const sdkModule = "example.com/prog"

// requireMockapi fails the test, with the command that fixes it, when nothing
// answers GraphQL at mockapiURL. A skip would let a missing server pass as
// green, which is exactly what this example exists to prevent.
func requireMockapi(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, mockapiURL,
		strings.NewReader(`{"query":"{__typename}"}`))
	if err != nil {
		t.Fatalf("building the mockapi probe: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("mockapi is not running at %s (%v); start it with: task mockapi:run", mockapiURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mockapi at %s answered %d; start it with: task mockapi:run", mockapiURL, resp.StatusCode)
	}
}

// buildProgram compiles ./<name> from this module into dir and returns the
// binary's path, so the program runs with its working directory set to a
// temporary directory rather than the module root.
func buildProgram(t *testing.T, name, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, name)
	run(t, ".", nil, "go", "build", "-o", bin, "./"+name)
	return bin
}

// run executes a command in dir and fails the test with its output on error.
func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// copyFile copies src into dir under the same base name.
func copyFile(t *testing.T, src, dir string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(src)), data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", src, err)
	}
}

// requireContains fails when the file at path does not contain want.
func requireContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s does not contain %q", path, want)
	}
}

func TestSdlapiThenGen(t *testing.T) {
	requireMockapi(t)

	work := t.TempDir()
	bins := t.TempDir()
	sdlapi := buildProgram(t, "sdlapi", bins)
	gen := buildProgram(t, "gen", bins)

	// sdlapi: fetch, keep todo and todos, prune, write SDL and JSON.
	out := run(t, work, nil, sdlapi)
	if !strings.Contains(out, "wrote schema.graphql and schema.json") {
		t.Fatalf("sdlapi output missing its final line:\n%s", out)
	}
	requireContains(t, filepath.Join(work, "schema.graphql"), "todo(id: ID!): Todo")
	requireContains(t, filepath.Join(work, "schema.json"), `"__schema"`)
	sdl, err := os.ReadFile(filepath.Join(work, "schema.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sdl), "ping:") {
		t.Fatal("schema.graphql still has ping: --only-queries todo,todos did not filter")
	}

	// gen: both SDKs from that schema, with the two config formats.
	copyFile(t, filepath.Join("testdata", "go.jsonc"), work)
	copyFile(t, filepath.Join("testdata", "ts.jsonc"), work)
	run(t, work, nil, gen)
	requireContains(t, filepath.Join(work, "sdk", "scalars", "scalars.go"), "DateTime = time.Time")
	requireContains(t, filepath.Join(work, "sdk", "queries", "query_todos.go"), sdkModule+"/sdk/types")
	requireContains(t, filepath.Join(work, "web", "src", "sdk", "scalars", "index.ts"), "export type DateTime = string;")

	// The Go SDK must compile on its own: no gqlkit import, no workspace.
	if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte("module "+sdkModule+"\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, []string{"GOWORK=off", "GOFLAGS=-mod=mod"}, "go", "build", "./...")
}
