package clientgen

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// miniSchema is the 3-type reproduction from issue #4 — not schema-specific,
// just enough to force cross-package imports (queries → types/fields, fields →
// builder).
const miniSchema = `
type Query { shop: Shop }
type Shop { id: ID! name: String plan: ShopPlan }
type ShopPlan { displayName: String }
`

// TestGenerateWithModulePathCompiles is the end-to-end guard for issue #4:
// running the generator with a --module value must (1) emit that module prefix
// on every local cross-package import and (2) produce an SDK that actually
// compiles. Per the codegen-testing rule, the build is the real test — a
// template that renders `import ( "" )` type-checks fine in isolation but the
// generated package does not compile.
func TestGenerateWithModulePathCompiles(t *testing.T) {
	const modulePath = "example.com/shopgql"

	// Generate() writes a schema.json into the working directory as a side
	// effect, so run the whole test from a temp cwd to avoid polluting the repo.
	work := t.TempDir()
	t.Chdir(work)

	schemaPath := filepath.Join(work, "mini.graphql")
	if err := os.WriteFile(schemaPath, []byte(miniSchema), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	outDir := filepath.Join(work, "out")
	gen, err := New(&Config{
		SchemaPath:  schemaPath,
		OutputDir:   outDir,
		PackageName: "shopgql",
		ModulePath:  modulePath,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// (1) Parse the actual import declarations of every generated file and
	// assert none is empty ("") or starts with "/" — those are the exact bug
	// signatures from issue #4 (a dropped module prefix). Parsing the AST is
	// precise where substring matching is not: builder.go/graphqlclient.go
	// legitimately contain `return ""` in their bodies. These checks need no Go
	// toolchain, so they run even under -short.
	fset := token.NewFileSet()
	var sawModulePrefix bool
	err = filepath.WalkDir(outDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(outDir, path)
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("%s: failed to parse: %v", rel, perr)
			return nil
		}
		for _, spec := range f.Imports {
			imp, uerr := strconv.Unquote(spec.Path.Value)
			if uerr != nil {
				t.Errorf("%s: unparseable import %s", rel, spec.Path.Value)
				continue
			}
			if imp == "" {
				t.Errorf("%s: has an empty import path (module prefix dropped)", rel)
			}
			if strings.HasPrefix(imp, "/") {
				t.Errorf("%s: import %q starts with %q — module prefix dropped", rel, imp, "/")
			}
			if strings.HasPrefix(imp, modulePath+"/") {
				sawModulePrefix = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk output: %v", err)
	}
	if !sawModulePrefix {
		t.Fatalf("no generated file imported %q/* — module prefix was not applied anywhere", modulePath)
	}

	// Spot-check the two packages named in the bug report.
	assertFileImports(t, filepath.Join(outDir, "fields", "field_shop.go"), modulePath+"/builder")
	assertFileImports(t, filepath.Join(outDir, "queries", "query_shop.go"), modulePath+"/types")

	// (2) The build is the real test. A go.mod matching --module makes every
	// local import resolvable; the generated SDK is self-contained (stdlib only),
	// so this builds offline with no `go mod tidy`. Skip under -short.
	if testing.Short() {
		t.Skip("skipping compile of generated SDK under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH; skipping compile check")
	}
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"),
		[]byte("module "+modulePath+"\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = outDir
	// GOFLAGS=-mod=mod / GOWORK=off keep the temp module from being pulled into
	// any enclosing workspace or requiring the network.
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated SDK does not compile: %v\n%s", err, out)
	}
}

// assertFileImports fails the test unless the given generated file imports want.
func assertFileImports(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(b), `"`+want+`"`) {
		t.Errorf("%s does not import %q", filepath.Base(path), want)
	}
}
