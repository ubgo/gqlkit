package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFile is a small helper that reads a file and fails the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(b)
}

func TestNewWriter(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)
	if w == nil {
		t.Fatal("NewWriter returned nil")
	}
	if w.outputDir != dir {
		t.Fatalf("outputDir = %q, want %q", w.outputDir, dir)
	}
}

func TestEnsureDir(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	w := NewWriter(nested)

	if err := w.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}
	info, err := os.Stat(nested)
	if err != nil {
		t.Fatalf("expected dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}

	// Idempotent: calling again should not error.
	if err := w.EnsureDir(); err != nil {
		t.Fatalf("second EnsureDir failed: %v", err)
	}
}

func TestWriteFile_CreatesNestedDirsAndFormatsGo(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// Malformed-but-parseable Go: bad indentation / spacing that gofmt fixes.
	unformatted := "package foo\nfunc  Bar( ){\nx:=1\n_=x}\n"
	rel := filepath.Join("sub", "pkg", "bar.go")

	if err := w.WriteFile(rel, unformatted); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	full := filepath.Join(dir, rel)
	got := readFile(t, full)

	// gofmt normalizes double spaces after func and uses tabs for indentation.
	if strings.Contains(got, "func  Bar") {
		t.Errorf("expected gofmt to collapse spaces, got:\n%s", got)
	}
	if !strings.Contains(got, "func Bar()") {
		t.Errorf("expected formatted func signature, got:\n%s", got)
	}
	if !strings.Contains(got, "\tx := 1") {
		t.Errorf("expected tab-indented, spaced assignment, got:\n%q", got)
	}

	// Verify the nested directory was actually created.
	if _, err := os.Stat(filepath.Join(dir, "sub", "pkg")); err != nil {
		t.Errorf("nested dir not created: %v", err)
	}
}

func TestWriteFile_NonGoWrittenVerbatim(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// Content that is NOT valid Go — format.Source will fail and the raw
	// bytes must be written verbatim (with a warning printed to stdout).
	content := "This is    not  Go source.\n\tKeep   spacing.\n"
	if err := w.WriteFile("notes.txt", content); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got := readFile(t, filepath.Join(dir, "notes.txt"))
	if got != content {
		t.Errorf("non-Go content altered.\n got: %q\nwant: %q", got, content)
	}
}

func TestWriteFile_Overwrite(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	if err := w.WriteFile("a.txt", "first"); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := w.WriteFile("a.txt", "second"); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "a.txt")); got != "second" {
		t.Errorf("overwrite content = %q, want %q", got, "second")
	}
}

func TestWriteFile_InvalidPath(t *testing.T) {
	dir := t.TempDir()

	// Make a file, then try to use it as a parent directory — MkdirAll must
	// fail because a path component is a regular file, not a directory.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	w := NewWriter(dir)
	// "blocker/child.txt" forces MkdirAll(dir/blocker) which is a file.
	err := w.WriteFile(filepath.Join("blocker", "child.txt"), "data")
	if err == nil {
		t.Fatal("expected error writing under a file-as-directory, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create directory") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWriteFile_WriteError(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// The target itself is a directory, so os.WriteFile must fail even though
	// the parent-dir creation succeeds.
	sub := filepath.Join(dir, "adir")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	err := w.WriteFile("adir", "data")
	if err == nil {
		t.Fatal("expected error writing to a directory path, got nil")
	}
	if !strings.Contains(err.Error(), "failed to write file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWriteGoMod_WithModulePath(t *testing.T) {
	base := t.TempDir()
	out := filepath.Join(base, "gen") // does not exist yet
	w := NewWriter(out)

	if err := w.WriteGoMod("example.com/mymod", "ignored"); err != nil {
		t.Fatalf("WriteGoMod failed: %v", err)
	}
	got := readFile(t, filepath.Join(out, "go.mod"))
	if !strings.Contains(got, "module example.com/mymod") {
		t.Errorf("go.mod missing module path:\n%s", got)
	}
	if !strings.Contains(got, "go 1.21") {
		t.Errorf("go.mod missing go directive:\n%s", got)
	}
}

func TestWriteGoMod_EmptyModulePathUsesPackageName(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	if err := w.WriteGoMod("", "mypkg"); err != nil {
		t.Fatalf("WriteGoMod failed: %v", err)
	}
	got := readFile(t, filepath.Join(dir, "go.mod"))
	if !strings.Contains(got, "module mypkg") {
		t.Errorf("expected package name as module path:\n%s", got)
	}
}

func TestWriteGoMod_MkdirError(t *testing.T) {
	base := t.TempDir()
	// Make outputDir a regular file so MkdirAll fails.
	fileAsDir := filepath.Join(base, "notadir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	w := NewWriter(fileAsDir)
	err := w.WriteGoMod("m", "p")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create output directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteGoMod_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Pre-create go.mod as a directory so os.WriteFile fails while MkdirAll
	// (on the already-existing outputDir) succeeds.
	if err := os.MkdirAll(filepath.Join(dir, "go.mod"), 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	w := NewWriter(dir)
	err := w.WriteGoMod("m", "p")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to write go.mod") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClean_ReadDirError(t *testing.T) {
	base := t.TempDir()
	// outputDir is a regular file: Stat says it exists (not IsNotExist), then
	// ReadDir on a non-directory fails.
	fileAsDir := filepath.Join(base, "afile")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	w := NewWriter(fileAsDir)
	err := w.Clean()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read output directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFormatGoCode(t *testing.T) {
	// Valid but unformatted.
	out, err := FormatGoCode("package p\nvar  x=1\n")
	if err != nil {
		t.Fatalf("FormatGoCode failed: %v", err)
	}
	if !strings.Contains(out, "var x = 1") {
		t.Errorf("expected formatted output, got: %q", out)
	}

	// Invalid Go: returns the original code plus an error.
	bad := "this is not go"
	out, err = FormatGoCode(bad)
	if err == nil {
		t.Fatal("expected error for invalid Go, got nil")
	}
	if out != bad {
		t.Errorf("expected original code returned on error, got: %q", out)
	}
}

func TestWriteFormattedFile(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	if err := w.WriteFormattedFile("f.go", "package p\nvar  y=2\n"); err != nil {
		t.Fatalf("WriteFormattedFile failed: %v", err)
	}
	got := readFile(t, filepath.Join(dir, "f.go"))
	if !strings.Contains(got, "var y = 2") {
		t.Errorf("expected formatted content, got: %q", got)
	}

	// Invalid Go should surface a format error (nothing written).
	err := w.WriteFormattedFile("bad.go", "not go at all")
	if err == nil {
		t.Fatal("expected error for invalid Go, got nil")
	}
	if !strings.Contains(err.Error(), "failed to format") {
		t.Errorf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "bad.go")); !os.IsNotExist(statErr) {
		t.Errorf("expected no file written on format failure")
	}
}

func TestOutputPath(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)
	got := w.OutputPath("x/y.go")
	want := filepath.Join(dir, "x/y.go")
	if got != want {
		t.Errorf("OutputPath = %q, want %q", got, want)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	if w.Exists("nope.txt") {
		t.Error("Exists returned true for missing file")
	}
	if err := w.WriteFile("here.txt", "hi"); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if !w.Exists("here.txt") {
		t.Error("Exists returned false for existing file")
	}
}

func TestClean(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// Populate with a file and a nested directory.
	if err := w.WriteFile("top.txt", "x"); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := w.WriteFile(filepath.Join("nested", "deep.txt"), "y"); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := w.Clean(); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty dir after Clean, got %d entries", len(entries))
	}
	// The root dir itself must still exist.
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("root dir removed by Clean: %v", err)
	}
}

func TestClean_NonexistentDirIsNoop(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "does-not-exist")
	w := NewWriter(missing)
	if err := w.Clean(); err != nil {
		t.Errorf("Clean on missing dir should be nil, got: %v", err)
	}
}

func TestWriteFileWithHeader(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// Go content so the header + body still format cleanly.
	if err := w.WriteFileWithHeader("gen.go", "package p\n"); err != nil {
		t.Fatalf("WriteFileWithHeader failed: %v", err)
	}
	got := readFile(t, filepath.Join(dir, "gen.go"))
	if !strings.Contains(got, "Code generated by gqlsdk. DO NOT EDIT.") {
		t.Errorf("header missing:\n%s", got)
	}
	if !strings.Contains(got, "package p") {
		t.Errorf("body missing:\n%s", got)
	}
}

func TestBufferedWriter(t *testing.T) {
	bw := NewBufferedWriter()
	if bw == nil {
		t.Fatal("NewBufferedWriter returned nil")
	}

	n, err := bw.Write([]byte("abc"))
	if err != nil || n != 3 {
		t.Fatalf("Write = (%d, %v), want (3, nil)", n, err)
	}
	n, err = bw.WriteString("def")
	if err != nil || n != 3 {
		t.Fatalf("WriteString = (%d, %v), want (3, nil)", n, err)
	}
	if got := bw.String(); got != "abcdef" {
		t.Errorf("String = %q, want %q", got, "abcdef")
	}

	bw.Reset()
	if got := bw.String(); got != "" {
		t.Errorf("after Reset String = %q, want empty", got)
	}
}
