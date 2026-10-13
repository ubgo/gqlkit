package clientgen

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// optSchema has three input types with the same field set, one per way a
// mode can be chosen (see optConfig), and every value kind: scalar, enum,
// list, nested input, slice-bound scalar — nullable and non-null.
const optSchema = `
scalar Bytes
enum Status { ACTIVE DONE }
input Sub { x: Int }
input OptIn  { s: String e: Status l: [String!] o: Sub b: Bytes self: OptIn sNN: String! lNN: [String!]! }
input PtrIn  { s: String e: Status l: [String!] o: Sub b: Bytes self: PtrIn sNN: String! lNN: [String!]! }
input MixIn  { s: String e: Status l: [String!] o: Sub b: Bytes self: MixIn sNN: String! lNN: [String!]! }
type Query { q(a: OptIn, b: PtrIn, c: MixIn): Int }
`

// optConfig sets the mode at all three levels: global opt, PtrIn back to
// pointer by type override, and MixIn.s back to pointer by field override
// (the rest of MixIn inherits the global opt).
const optConfig = `{
  "bindings": { "Bytes": { "model": "[]byte" } },
  "inputs": {
    "optional": "opt",
    "overrides": { "PtrIn": "pointer", "MixIn.s": "pointer" }
  }
}`

// newOptGenerator builds a Generator over optSchema with the given config.
func newOptGenerator(t *testing.T, cfg, outDir string) (*Generator, error) {
	t.Helper()
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.graphql")
	cfgPath := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(schemaPath, []byte(optSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if outDir == "" {
		outDir = filepath.Join(dir, "out")
	}
	return New(&Config{SchemaPath: schemaPath, ConfigPath: cfgPath, OutputDir: outDir, PackageName: "opt", ModulePath: "example.com/opt"})
}

// TestOptionalModeFieldMatrix pins Go type and JSON tag for every
// mode-source × nullability × kind combination.
func TestOptionalModeFieldMatrix(t *testing.T) {
	g, err := newOptGenerator(t, optConfig, "")
	if err != nil {
		t.Fatal(err)
	}
	opt := map[string][2]string{
		"s":    {"Opt[string]", "omitzero"},
		"e":    {"Opt[enums.Status]", "omitzero"},
		"l":    {"Opt[[]string]", "omitzero"},
		"o":    {"Opt[Sub]", "omitzero"},
		"b":    {"Opt[[]byte]", "omitzero"},
		"self": {"", "omitzero"}, // type name differs per struct, checked below
		"sNN":  {"string", ""},
		"lNN":  {"[]string", ""},
	}
	ptr := map[string][2]string{
		"s":    {"*string", "omitempty"},
		"e":    {"*enums.Status", "omitempty"},
		"l":    {"[]string", "omitempty"}, // omitzero only in WireTag (go1.24 MarshalJSON)
		"o":    {"*Sub", "omitempty"},
		"b":    {"[]byte", "omitempty"}, // slice-bound scalar, same as l
		"self": {"", "omitempty"},
		"sNN":  {"string", ""},
		"lNN":  {"[]string", ""},
	}
	want := map[string]map[string][2]string{"OptIn": opt, "PtrIn": ptr, "MixIn": {}}
	for k, v := range opt {
		want["MixIn"][k] = v
	}
	want["MixIn"]["s"] = ptr["s"] // field override

	for typeName, fields := range want {
		for _, fd := range g.buildFieldDefs(g.schema.Types[typeName]) {
			// The wire name is the first element of the json tag.
			gql, _, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(fd.JSONTag, "`json:\""), "\"`"), ",")
			w, ok := fields[gql]
			if !ok {
				t.Fatalf("%s: unexpected field %s", typeName, fd.Name)
			}
			wantType := w[0]
			if gql == "self" {
				wantType = map[bool]string{true: "Opt[" + typeName + "]", false: "*" + typeName}[strings.HasPrefix(w[1], "omitzero")]
			}
			if fd.GoType != wantType {
				t.Errorf("%s.%s type = %s, want %s", typeName, gql, fd.GoType, wantType)
			}
			wantTag := "`json:\"" + gql + "\"`"
			if w[1] != "" {
				wantTag = "`json:\"" + gql + "," + w[1] + "\"`"
			}
			if fd.JSONTag != wantTag {
				t.Errorf("%s.%s tag = %s, want %s", typeName, gql, fd.JSONTag, wantTag)
			}
			// Pointer-mode slices: the struct tag stays omitempty (safe on
			// toolchains that ignore omitzero); only WireTag uses omitzero.
			wantWire := wantTag
			if (gql == "l" || gql == "b") && w[0] == fd.GoType && !strings.HasPrefix(fd.GoType, "Opt[") {
				wantWire = "`json:\"" + gql + ",omitzero\"`"
			}
			if fd.WireTag != wantWire {
				t.Errorf("%s.%s wire tag = %s, want %s", typeName, gql, fd.WireTag, wantWire)
			}
		}
	}
}

// TestDefaultOptionalModeIsPointer pins the release default: without an
// inputs section nothing changes for existing callers (except the slice fix).
func TestDefaultOptionalModeIsPointer(t *testing.T) {
	g, err := newOptGenerator(t, `{ "bindings": { "Bytes": { "model": "[]byte" } } }`, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range g.buildFieldDefs(g.schema.Types["OptIn"]) {
		if strings.HasPrefix(fd.GoType, "Opt[") {
			t.Errorf("default mode produced %s %s", fd.Name, fd.GoType)
		}
	}
}

// TestInputsConfigValidation enumerates the config mistakes New must reject.
func TestInputsConfigValidation(t *testing.T) {
	cases := map[string]string{
		"unknown global mode":   `{"inputs":{"optional":"maybe"}}`,
		"unknown override mode": `{"inputs":{"overrides":{"OptIn":"nullable"}}}`,
		"unknown type":          `{"inputs":{"overrides":{"Nope":"opt"}}}`,
		"object, not input":     `{"inputs":{"overrides":{"Query":"opt"}}}`,
		"unknown field":         `{"inputs":{"overrides":{"OptIn.zzz":"opt"}}}`,
		"non-null field":        `{"inputs":{"overrides":{"OptIn.sNN":"opt"}}}`,
	}
	for name, cfg := range cases {
		if _, err := newOptGenerator(t, cfg, ""); !errors.Is(err, ErrInvalidInputsConfig) {
			t.Errorf("%s: err = %v, want ErrInvalidInputsConfig", name, err)
		}
	}
	if _, err := newOptGenerator(t, `{"inputs":{"optional":"pointer","overrides":{"OptIn.s":"opt","PtrIn":"pointer"}}}`, ""); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

// optWireProgram marshals every state of every field kind through the
// generated inputs package and prints one JSON document per line.
const optWireProgram = `package main

import (
	"encoding/json"
	"fmt"

	"example.com/opt/enums"
	"example.com/opt/inputs"
)

func p(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}

func main() {
	// opt mode: unset / null / value / empty list, for each kind.
	p(inputs.OptIn{})
	p(inputs.OptIn{S: inputs.Null[string](), E: inputs.Null[enums.Status](), L: inputs.Null[[]string](), O: inputs.Null[inputs.Sub](), B: inputs.Null[[]byte](), Self: inputs.Null[inputs.OptIn]()})
		p(inputs.OptIn{S: inputs.Some(""), E: inputs.Some(enums.StatusDone), L: inputs.Some([]string{}), O: inputs.Some(inputs.Sub{X: inputs.Some(1)}), Self: inputs.Some(inputs.OptIn{SNn: "inner"}), SNn: "v", LNn: []string{"a"}})
	// pointer mode: nil omitted, empty slice sent as [], value sent.
	p(inputs.PtrIn{})
	s := ""
	p(inputs.PtrIn{S: &s, L: []string{}, B: []byte{}})
	// mixed: S is pointer (field override), the rest opt.
	p(inputs.MixIn{S: &s, L: inputs.Null[[]string]()})
	// Get / IsNull round trip.
	v, ok := inputs.Some(5).Get()
	_, okNull := inputs.Null[int]().Get()
	fmt.Println(v, ok, okNull, inputs.Null[int]().IsNull(), inputs.Opt[int]{}.IsZero())
	var back inputs.OptIn
	_ = json.Unmarshal([]byte(` + "`" + `{"s":null,"e":"DONE"}` + "`" + `), &back)
	fmt.Println(back.S.IsNull(), back.E.IsZero(), back.L.IsZero())
}
`

// TestOptionalWireMatrix is the build + wire gate: generate the SDK, compile
// it with a program that marshals every state, and compare the JSON.
func TestOptionalWireMatrix(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	outDir := filepath.Join(work, "out")
	g, err := newOptGenerator(t, optConfig, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	requireGoToolchain(t)
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte("module example.com/opt\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "cmd", "wire"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "cmd", "wire", "main.go"), []byte(optWireProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := goBuild(t, outDir, "./..."); err != nil {
		t.Fatalf("opt SDK does not compile: %v\n%s", err, out)
	}
	cmd := exec.Command("go", "run", "./cmd/wire")
	cmd.Dir = outDir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wire program: %v\n%s", err, out)
	}
	want := strings.Join([]string{
		`{"sNN":"","lNN":null}`,
		`{"s":null,"e":null,"l":null,"o":null,"b":null,"self":null,"sNN":"","lNN":null}`,
		`{"s":"","e":"DONE","l":[],"o":{"x":1},"self":{"sNN":"inner","lNN":null},"sNN":"v","lNN":["a"]}`,
		`{"sNN":"","lNN":null}`,
		`{"s":"","l":[],"b":"","sNN":"","lNN":null}`,
		`{"s":"","l":null,"sNN":"","lNN":null}`,
		`5 true false true true`,
		`true false true`,
	}, "\n") + "\n"
	if string(out) != want {
		t.Errorf("wire output mismatch\n got:\n%s\nwant:\n%s", out, want)
	}

	// Simulate a toolchain older than Go 1.24, where the go1.24-tagged
	// inputs_json.go is excluded and encoding/json ignores omitzero: remove
	// the file and rerun. Unset (nil) lists must STILL be omitted, never sent
	// as null (null would clear the value on the server); an empty list then
	// degrades to omitted, the pre-fix behaviour.
	if err := os.Remove(filepath.Join(outDir, "inputs", "inputs_json.go")); err != nil {
		t.Fatal(err)
	}
	legacy, err := exec.Command("sh", "-c", "cd "+outDir+" && GOWORK=off GOFLAGS=-mod=mod go run ./cmd/wire").CombinedOutput()
	if err != nil {
		t.Fatalf("legacy wire program: %v\n%s", err, legacy)
	}
	lines := strings.Split(string(legacy), "\n")
	if lines[3] != `{"sNN":"","lNN":null}` || lines[4] != `{"s":"","sNN":"","lNN":null}` {
		t.Errorf("legacy pointer-mode encoding changed:\n%s\n%s", lines[3], lines[4])
	}
	if strings.Contains(lines[3], `"l":null`) || strings.Contains(lines[3], `"b":null`) {
		t.Errorf("legacy encoding sent null for an unset list: %s", lines[3])
	}
}

// TestGoVersionGuard pins the minimum-Go enforcement: builder/ and inputs/
// carry a `//go:build !go1.24` guard. The installed toolchain is newer, so
// the SDK builds (every compile test proves that); to exercise the old
// toolchain path, the guard is compiled with its build constraint removed,
// which is exactly what a pre-1.24 toolchain does, and must fail with the
// explanatory identifier.
func TestGoVersionGuard(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	outDir := filepath.Join(work, "out")
	g, err := newOptGenerator(t, optConfig, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	var guard string
	for _, pkg := range []string{pkgBuilder, pkgInputs} {
		src := readFile(t, filepath.Join(outDir, pkg, minGoVersionGuardFile))
		if !strings.Contains(src, "//go:build !go1.24\n") {
			t.Fatalf("%s guard lacks the !go1.24 constraint:\n%s", pkg, src)
		}
		guard = src
	}

	requireGoToolchain(t)
	scratch := filepath.Join(work, "old")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "go.mod"), []byte("module old\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	asOld := strings.Replace(guard, "//go:build !go1.24\n", "", 1)
	if err := os.WriteFile(filepath.Join(scratch, "guard.go"), []byte(asOld), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := goBuild(t, scratch, ".")
	if err == nil || !strings.Contains(out, "gqlkit_SDK_requires_Go_1_24_or_newer") {
		t.Fatalf("guard did not stop an old-toolchain build: err=%v\n%s", err, out)
	}
}
