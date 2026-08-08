package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// requireUnixShell skips the test on platforms without /bin/sh (hooks are
// arbitrary executables; this suite only exercises them via a shell script).
func requireUnixShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no /bin/sh available")
	}
}

// newArgCaptureScript writes a shell script that dumps its args (one per line)
// to outputPath, so tests can check what a hook was called with.
func newArgCaptureScript(t *testing.T, dir, name string) (scriptPath, outputPath string) {
	t.Helper()
	requireUnixShell(t)

	outputPath = filepath.Join(dir, name+".out")
	scriptPath = filepath.Join(dir, name+".sh")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\n", outputPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return scriptPath, outputPath
}

func readCapturedArgs(t *testing.T, outputPath string) []string {
	t.Helper()
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("hook did not run (no output file): %v", err)
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestHookApplyHookVersionArgument(t *testing.T) {
	cases := []struct {
		name        string
		formula     FormulaInfo
		wantVersion string
	}{
		{
			name: "not following a branch uses the full version",
			formula: FormulaInfo{
				File:     "/tap/foo.rb",
				Version:  "1.2.3",
				Instance: Github,
				URL:      "https://github.com/acme/foo/archive/refs/tags/v1.2.3.tar.gz",
			},
			wantVersion: "1.2.3",
		},
		{
			name: "following a branch uses the commit hash component",
			formula: FormulaInfo{
				File:     "/tap/foo.rb",
				Version:  "123.abcd1234",
				Instance: Github,
				URL:      "https://github.com/acme/foo/archive/refs/heads/main.zip",
			},
			wantVersion: "abcd1234",
		},
		{
			name: "following a branch but version has no dot falls back to the full version",
			formula: FormulaInfo{
				File:     "/tap/foo.rb",
				Version:  "onlyonepart",
				Instance: Github,
				URL:      "https://github.com/acme/foo/archive/refs/heads/main.zip",
			},
			wantVersion: "onlyonepart",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			script, output := newArgCaptureScript(t, dir, "hook")
			hook := Hook{Name: "Test", Path: script, Parameters: []string{"extra1", "extra2"}}

			err := hook.ApplyHook(c.formula, hook.Name, Config{}, zerolog.Nop())
			if err != nil {
				t.Fatalf("ApplyHook: %v", err)
			}

			args := readCapturedArgs(t, output)
			want := []string{c.formula.File, c.wantVersion, "extra1", "extra2"}
			if len(args) != len(want) {
				t.Fatalf("captured args = %v, want %v", args, want)
			}
			for i := range want {
				if args[i] != want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, args[i], want[i])
				}
			}
		})
	}
}

func TestHookApplyHookPropagatesFailure(t *testing.T) {
	hook := Hook{Name: "Failing", Path: "/definitely/does/not/exist"}
	err := hook.ApplyHook(FormulaInfo{File: "foo.rb", Version: "1.0.0"}, hook.Name, Config{}, zerolog.Nop())
	if err == nil {
		t.Fatalf("expected an error when the hook script doesn't exist")
	}
}

func TestHooksApplyHookDispatchesByFormulaNameOrFile(t *testing.T) {
	dir := t.TempDir()

	byLocalFileScript, byLocalFileOut := newArgCaptureScript(t, dir, "by-local-file")
	byNameScript, byNameOut := newArgCaptureScript(t, dir, "by-name")
	_, unrelatedOut := newArgCaptureScript(t, dir, "unrelated")

	hooks := Hooks{
		{Name: "ByLocalFile", Path: byLocalFileScript, Formulas: []string{"foo.rb"}},
		{Name: "ByName", Path: byNameScript, Formulas: []string{"foo"}},
		{Name: "Unrelated", Path: "/does/not/matter", Formulas: []string{"bar.rb", "bar"}},
	}

	formula := FormulaInfo{
		File:      "/tap/foo.rb",
		Directory: "/tap",
		Version:   "1.0.0",
		Instance:  Github,
		URL:       "https://github.com/acme/foo/archive/refs/tags/v1.0.0.tar.gz",
	}

	hooks.ApplyHook(formula, Config{}, zerolog.Nop())

	if _, err := os.Stat(byLocalFileOut); err != nil {
		t.Errorf("hook matching by local file name should have run")
	}
	if _, err := os.Stat(byNameOut); err != nil {
		t.Errorf("hook matching by formula name should have run")
	}
	if _, err := os.Stat(unrelatedOut); err == nil {
		t.Errorf("unrelated hook should NOT have run")
	}
}
