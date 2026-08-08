package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractVersion(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   string
		wantOk bool
	}{
		{"v-prefixed", "v1.2.3", "1.2.3", true},
		{"bare", "2.5.0", "2.5.0", true},
		{"embedded in release title", "Version 2.5.0 release", "2.5.0", true},
		{"multi-digit", "v10.20.30", "10.20.30", true},
		{"prerelease suffix kept", "2.5.0-beta.1", "2.5.0-beta.1", true},
		{"uppercase V lowercased", "V1.0", "1.0", true},
		{
			"release title with trailing free text is NOT swallowed",
			"2.5.0 - now with bug fixes and stuff",
			"2.5.0",
			true,
		},
		{
			"quote-breakout payload after prerelease hyphen is NOT captured",
			`2.5.0-"; system("id"); x="`,
			"2.5.0",
			true,
		},
		{"no version present", "no version here", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ExtractVersion(c.input)
			if ok != c.wantOk {
				t.Fatalf("ExtractVersion(%q) ok = %v, want %v", c.input, ok, c.wantOk)
			}
			if got != c.want {
				t.Fatalf("ExtractVersion(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestExtractProjectPath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://github.com/owner/repo", "owner/repo"},
		{"https://github.com/owner/repo/", "owner/repo"},
		{"https://gitlab.example.com/group/sub/project", "group/sub/project"},
	}
	for _, c := range cases {
		got, err := ExtractProjectPath(c.input)
		if err != nil {
			t.Fatalf("ExtractProjectPath(%q) error: %v", c.input, err)
		}
		if got != c.want {
			t.Errorf("ExtractProjectPath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestGetOwnerRepo(t *testing.T) {
	owner, repo, err := GetOwnerRepo("https://github.com/acme/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "acme" || repo != "foo" {
		t.Fatalf("got owner=%q repo=%q, want acme/foo", owner, repo)
	}

	if _, _, err := GetOwnerRepo("https://github.com/onlyowner"); err == nil {
		t.Fatalf("expected an error for a path missing the repo segment")
	}
}

// requireGit skips the test if the git binary isn't available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "--local", "user.name", "Test User")
	run("config", "--local", "user.email", "test@example.com")
	run("config", "--local", "commit.gpgsign", "false")
	return dir
}

func TestIsInsideGitWorkTree(t *testing.T) {
	repo := newTestRepo(t)
	ok, err := IsInsideGitWorkTree(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected %s to be reported as a git work tree", repo)
	}

	notRepo := t.TempDir()
	ok, err = IsInsideGitWorkTree(notRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected %s to NOT be reported as a git work tree", notRepo)
	}
}

func TestGitAddAndCommitFiles(t *testing.T) {
	repo := newTestRepo(t)

	formulaPath := filepath.Join(repo, "foo.rb")
	if err := os.WriteFile(formulaPath, []byte("class Foo < Formula\nend\n"), 0o644); err != nil {
		t.Fatalf("write formula: %v", err)
	}

	g := NewGit(false, repo)
	if err := g.Add(Config{Interactive: false}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	files, err := g.GetCommitFiles()
	if err != nil {
		t.Fatalf("GetCommitFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "foo" {
		t.Fatalf("GetCommitFiles = %v, want [foo]", files)
	}

	if err := g.CommitFiles(); err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}

	cmd := exec.Command("git", "-C", repo, "log", "-1", "--pretty=%s")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	msg := strings.TrimSpace(string(out))
	if msg != "Bump foo" {
		t.Fatalf("commit message = %q, want %q", msg, "Bump foo")
	}
}
