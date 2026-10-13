package clientgen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestValidateDetectsPackageFromGoMod covers the go.mod auto-detection fallback
// (detectPackagePath / readModulePath): when neither --module nor a slashed
// --package is given, Package is derived from the current dir's go.mod + the
// output dir name.
func TestValidateDetectsPackageFromGoMod(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module github.com/acme/proj\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cfg := Config{SchemaPath: "x.graphql", OutputDir: "./sdk", PackageName: "sdk"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if want := "github.com/acme/proj/sdk"; cfg.Package != want {
		t.Errorf("Package = %q, want %q", cfg.Package, want)
	}
}

// TestValidateDefaults covers the OutputDir / PackageName default application.
func TestValidateDefaults(t *testing.T) {
	t.Chdir(t.TempDir()) // no go.mod and no -m: Package undeterminable
	cfg := Config{SchemaPath: "x.graphql"}
	if err := cfg.Validate(); !errors.Is(err, ErrPackageUndetermined) {
		t.Fatalf("Validate: err = %v, want ErrPackageUndetermined", err)
	}
	if cfg.OutputDir != "./sdk" {
		t.Errorf("OutputDir default = %q, want ./sdk", cfg.OutputDir)
	}
	if cfg.PackageName != "sdk" {
		t.Errorf("PackageName default = %q, want sdk", cfg.PackageName)
	}
}

// TestValidateRequiresSchema covers the required-field guard.
func TestValidateRequiresSchema(t *testing.T) {
	cfg := Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate should error when SchemaPath is empty")
	}
}

// TestValidateModulePathThreadsIntoPackage pins the regression from issue #4:
// the -m / --module flag stored its value in Config.ModulePath, but the
// generator threads Config.Package into every cross-package import. Because
// ModulePath was never copied into Package, Package stayed empty and imports
// rendered as `import ( "" )` / `"/types"`, so the generated SDK didn't compile.
// Validate() must copy ModulePath into an empty Package.
func TestValidateModulePathThreadsIntoPackage(t *testing.T) {
	tests := []struct {
		name            string
		in              Config
		wantPackage     string
		wantPackageName string
	}{
		{
			name:            "module flag becomes the SDK root import path",
			in:              Config{SchemaPath: "x.graphql", ModulePath: "mymod/shopgql", PackageName: "shopgql"},
			wantPackage:     "mymod/shopgql",
			wantPackageName: "shopgql",
		},
		{
			name:            "module flag wins over the -p-with-slash convenience",
			in:              Config{SchemaPath: "x.graphql", ModulePath: "mymod/shopgql", PackageName: "other/pkgname"},
			wantPackage:     "mymod/shopgql",
			wantPackageName: "other/pkgname", // untouched: ModulePath branch ran first, so the slash branch is skipped
		},
		{
			name:            "no module flag falls back to -p-with-slash",
			in:              Config{SchemaPath: "x.graphql", PackageName: "example.com/api"},
			wantPackage:     "example.com/api",
			wantPackageName: "api",
		},
		{
			name:            "explicit Package is never overwritten by ModulePath",
			in:              Config{SchemaPath: "x.graphql", ModulePath: "mymod/shopgql", Package: "already/set", PackageName: "shopgql"},
			wantPackage:     "already/set",
			wantPackageName: "shopgql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run in a temp cwd so the go.mod auto-detection fallback can't leak
			// the repo's own module path into cases that expect empty/other values.
			t.Chdir(t.TempDir())

			cfg := tt.in
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if cfg.Package != tt.wantPackage {
				t.Errorf("Package = %q, want %q", cfg.Package, tt.wantPackage)
			}
			if cfg.PackageName != tt.wantPackageName {
				t.Errorf("PackageName = %q, want %q", cfg.PackageName, tt.wantPackageName)
			}
		})
	}
}

// TestDetectPackagePathMatrix crosses where the command runs (module root,
// a sub-directory) with where the output goes (relative, nested, absolute,
// the module root itself, outside the module), plus the no-go.mod case. The
// old code only read ./go.mod and blindly joined OutputDir, so a sub-directory
// run produced no Package at all and generation emitted "/types" with exit 0.
func TestDetectPackagePathMatrix(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/proj\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "cmd", "gen")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	cases := []struct {
		name, cwd, out, want string
		wantErr              bool
	}{
		{"root ./sdk", root, "./sdk", "example.com/proj/sdk", false},
		{"root nested", root, "internal/gql/sdk", "example.com/proj/internal/gql/sdk", false},
		{"root absolute", root, filepath.Join(root, "pkg", "sdk"), "example.com/proj/pkg/sdk", false},
		{"root itself", root, ".", "example.com/proj", false},
		{"sub dir walks up", sub, "./sdk", "example.com/proj/cmd/gen/sdk", false},
		{"sub dir parent output", sub, "../../sdk", "example.com/proj/sdk", false},
		{"output outside module", root, outside, "", true},
		{"no go.mod anywhere", outside, "./sdk", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Chdir(c.cwd)
			got, err := detectPackagePath(c.out)
			if c.wantErr {
				if !errors.Is(err, ErrPackageUndetermined) {
					t.Fatalf("err = %v, want ErrPackageUndetermined", err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("detectPackagePath(%q) = %q, %v; want %q", c.out, got, err, c.want)
			}
		})
	}
}
