package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSha256(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, c := range cases {
		got := Sha256([]byte(c.input))
		if got != c.want {
			t.Errorf("Sha256(%q) = %s, want %s", c.input, got, c.want)
		}
	}
}

func TestTrimDir(t *testing.T) {
	cases := []struct {
		dir, path, want string
	}{
		{"/foo/bar", "/foo/bar/baz.rb", "baz.rb"},
		{"/foo/bar/", "/foo/bar/baz.rb", "baz.rb"},
		{"/foo/bar", "/foo/bar/sub/baz.rb", "sub/baz.rb"},
	}
	for _, c := range cases {
		got := TrimDir(c.dir, c.path)
		if got != c.want {
			t.Errorf("TrimDir(%q, %q) = %q, want %q", c.dir, c.path, got, c.want)
		}
	}
}

func TestGetContainingDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "foo.rb")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	gotDir, err := GetContainingDir(dir)
	if err != nil {
		t.Fatalf("GetContainingDir(dir): %v", err)
	}
	if gotDir != dir {
		t.Errorf("GetContainingDir(%q) = %q, want %q", dir, gotDir, dir)
	}

	gotFileDir, err := GetContainingDir(file)
	if err != nil {
		t.Fatalf("GetContainingDir(file): %v", err)
	}
	// GetContainingDir appends a trailing separator when given a file path.
	if filepath.Clean(gotFileDir) != dir {
		t.Errorf("GetContainingDir(%q) = %q, want parent %q", file, gotFileDir, dir)
	}

	if _, err := GetContainingDir(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Errorf("expected an error for a nonexistent path")
	}
}

func TestReadWriteFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.txt")
	content := "hello\nworld\n"

	if err := WriteFile(path, content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != content {
		t.Errorf("ReadFile = %q, want %q", got, content)
	}

	if _, err := ReadFile(filepath.Join(dir, "missing.txt")); err == nil {
		t.Errorf("expected an error reading a nonexistent file")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if !fileExists(file) {
		t.Errorf("fileExists(%q) = false, want true", file)
	}
	if fileExists(filepath.Join(dir, "absent.txt")) {
		t.Errorf("fileExists(absent) = true, want false")
	}
	if fileExists(dir) {
		t.Errorf("fileExists(directory) = true, want false (directories aren't files)")
	}
}

func TestFindBrewFormulaFiles(t *testing.T) {
	dir := t.TempDir()

	formula := filepath.Join(dir, "foo.rb")
	if err := os.WriteFile(formula, []byte("class Foo < Formula\n  url \"https://example.com\"\nend\n"), 0o644); err != nil {
		t.Fatalf("write formula: %v", err)
	}

	// A .rb file that isn't a Homebrew formula should be ignored.
	notFormula := filepath.Join(dir, "helper.rb")
	if err := os.WriteFile(notFormula, []byte("puts 'hello'\n"), 0o644); err != nil {
		t.Fatalf("write non-formula ruby file: %v", err)
	}

	// A non-.rb file, even with matching content, should be ignored.
	wrongExt := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(wrongExt, []byte("class Foo < Formula\nend\n"), 0o644); err != nil {
		t.Fatalf("write wrong-extension file: %v", err)
	}

	// A nested formula should still be found (recursive walk).
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	nested := filepath.Join(subDir, "bar.rb")
	if err := os.WriteFile(nested, []byte("class Bar < Formula\nend\n"), 0o644); err != nil {
		t.Fatalf("write nested formula: %v", err)
	}

	got, err := FindBrewFormulaFiles(dir)
	if err != nil {
		t.Fatalf("FindBrewFormulaFiles: %v", err)
	}

	want := map[string]bool{formula: true, nested: true}
	if len(got) != len(want) {
		t.Fatalf("FindBrewFormulaFiles = %v, want exactly %v", got, want)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("unexpected file in results: %s", f)
		}
	}
}
