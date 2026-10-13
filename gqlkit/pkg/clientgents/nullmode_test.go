package clientgents

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const nullModeSDL = `
input Patch { name: String }
type Item { name: String tags: [String] count: Int child: Item }
type Other { note: String }
type Query { item(patch: Patch): Item now: String other: Other }
`

// TestGenerateNullModes pins the NullMode seam: the default is "| null"; the
// global "null-undefined" loosens every nullable output field and return;
// a type override and a field override win in that order; list elements
// and inputs keep "| null" in every mode. Each SDK must compile.
func TestGenerateNullModes(t *testing.T) {
	cases := []struct {
		name, config string
		want, absent []string
	}{
		{
			name:   "default",
			config: `{}`,
			want:   []string{"name?: string | null;", "tags?: (string | null)[] | null;", "execute(request?: RequestOptions): Promise<string | null>"},
			absent: []string{"undefined"},
		},
		{
			name:   "global null-undefined",
			config: `{ "nullable": "null-undefined" }`,
			want: []string{
				"name?: string | null | undefined;",
				"tags?: (string | null)[] | null | undefined;", // elements stay | null
				"note?: string | null | undefined;",
				"name: string | null | undefined }",                                     // selector result
				"child: U | null | undefined }",                                         // object selector result
				"execute(request?: RequestOptions): Promise<string | null | undefined>", // scalar return
				"ItemBuilder<T | null | undefined>",                                     // select() result
			},
		},
		{
			name:   "type and field overrides",
			config: `{ "nullable": "null-undefined", "nullableOverrides": { "Item": "null", "Item.count": "null-undefined", "Query.now": "null" } }`,
			want: []string{
				"name?: string | null;",                                     // Item → null
				"count?: number | null | undefined;",                        // Item.count beats Item
				"note?: string | null | undefined;",                         // Other falls back to global
				"execute(request?: RequestOptions): Promise<string | null>", // Query.now → null
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sdk := generateSDK(t, nullModeSDL, c.config)
			all := read(t, sdk, "types/index.ts") + read(t, sdk, "fields/item.ts") + read(t, sdk, "queries/now.ts") + read(t, sdk, "queries/item.ts")
			mustContain(t, all, c.name, c.want...)
			for _, a := range c.absent {
				assertAbsent(t, all, a)
			}
			// Inputs are never loosened.
			mustContain(t, read(t, sdk, "inputs/index.ts"), "inputs", "name?: string | null;")
			typecheckSDK(t, sdk, nil)
		})
	}
}

// TestNullModeConfigErrors pins config validation: an unknown mode fails on
// load, and an override naming no output type or field fails in New.
func TestNullModeConfigErrors(t *testing.T) {
	cases := []struct {
		name, config string
		want         error
	}{
		{"unknown global mode", `{ "nullable": "loose" }`, ErrNullModeInvalid},
		{"unknown override mode", `{ "nullableOverrides": { "Item": "maybe" } }`, ErrNullModeInvalid},
		{"unknown type", `{ "nullableOverrides": { "Itme": "null" } }`, ErrNullOverrideUnknown},
		{"unknown field", `{ "nullableOverrides": { "Item.nmae": "null" } }`, ErrNullOverrideUnknown},
		{"input type is not output", `{ "nullableOverrides": { "Patch": "null" } }`, ErrNullOverrideUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			schema := filepath.Join(dir, "schema.graphql")
			cfg := filepath.Join(dir, "config.jsonc")
			if err := os.WriteFile(schema, []byte(nullModeSDL), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfg, []byte(c.config), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := New(&Config{SchemaPath: schema, OutputDir: filepath.Join(dir, "out"), ConfigPath: cfg})
			if !errors.Is(err, c.want) {
				t.Errorf("New() = %v, want %v", err, c.want)
			}
		})
	}
}
