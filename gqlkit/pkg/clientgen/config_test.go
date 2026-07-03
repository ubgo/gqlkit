package clientgen

import "testing"

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
