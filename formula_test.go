package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleFormula = `class Foo < Formula
  desc "A nice tool"
  homepage "https://github.com/acme/foo"
  url "https://github.com/acme/foo/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  version "1.0.0"
  revision 2
  license "MIT"

  bottle do
    root_url "https://example.com/bottles"
    sha256 arm64_sequoia: "1111111111111111111111111111111111111111111111111111111111111111"
    sha256 cellar: :any, sonoma: "2222222222222222222222222222222222222222222222222222222222222222"
  end
end
`

func writeFormula(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write formula %s: %v", name, err)
	}
	return path
}

func TestParseFormula(t *testing.T) {
	dir := t.TempDir()
	path := writeFormula(t, dir, "foo.rb", sampleFormula)

	info, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}

	if info.URL != "https://github.com/acme/foo/archive/refs/tags/v1.0.0.tar.gz" {
		t.Errorf("URL = %q", info.URL)
	}
	if info.SHA256 != strings.Repeat("0", 64) {
		t.Errorf("SHA256 = %q", info.SHA256)
	}
	if info.Version != "1.0.0" {
		t.Errorf("Version = %q", info.Version)
	}
	if info.Homepage != "https://github.com/acme/foo" {
		t.Errorf("Homepage = %q", info.Homepage)
	}
	if info.Description != "A nice tool" {
		t.Errorf("Description = %q", info.Description)
	}
	if info.Revision != "2" {
		t.Errorf("Revision = %q, want %q (bare integer syntax)", info.Revision, "2")
	}
	if info.Instance != Github {
		t.Errorf("Instance = %v, want Github", info.Instance)
	}
	if info.BottleURL != "https://example.com/bottles" {
		t.Errorf("BottleURL = %q", info.BottleURL)
	}
	if len(info.Bottles) != 2 {
		t.Fatalf("expected 2 bottles, got %d: %+v", len(info.Bottles), info.Bottles)
	}
	if info.Bottles[0].target != "arm64_sequoia" || info.Bottles[0].Sha256 != strings.Repeat("1", 64) {
		t.Errorf("bottle[0] = %+v", info.Bottles[0])
	}
	if info.Bottles[1].target != "sonoma" || info.Bottles[1].Options == "" {
		t.Errorf("bottle[1] = %+v", info.Bottles[1])
	}

	if info.GetLocalFile() != "foo.rb" {
		t.Errorf("GetLocalFile() = %q", info.GetLocalFile())
	}
	if info.GetName() != "foo" {
		t.Errorf("GetName() = %q", info.GetName())
	}
}

func TestParseFormulaNoRevision(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(sampleFormula, "  revision 2\n", "", 1)
	path := writeFormula(t, dir, "foo.rb", content)

	info, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}
	if info.Revision != "" {
		t.Errorf("Revision = %q, want empty when no revision line present", info.Revision)
	}
}

func TestGetInstanceUsesHomepageNotURL(t *testing.T) {
	dir := t.TempDir()
	// url points at a different host (e.g. a CDN mirror) than homepage.
	content := `class Foo < Formula
  desc "mirrored asset"
  homepage "https://github.com/acme/foo"
  url "https://cdn.example.com/mirror/foo-1.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  version "1.0.0"
end
`
	path := writeFormula(t, dir, "foo.rb", content)

	info, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}
	if info.Instance != Github {
		t.Fatalf("Instance = %v, want Github (derived from homepage, not the cdn URL)", info.Instance)
	}
	host, err := info.GetInstance()
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if host != "github.com" {
		t.Fatalf("GetInstance() = %q, want github.com", host)
	}
}

func TestIsFollowingBranch(t *testing.T) {
	cases := []struct {
		name     string
		instance GitType
		url      string
		want     bool
	}{
		{"github release archive", Github, "https://github.com/acme/foo/archive/refs/tags/v1.0.0.tar.gz", false},
		{"github branch archive", Github, "https://github.com/acme/foo/archive/refs/heads/main.zip", true},
		{"gitlab release archive", Gitlab, "https://gitlab.com/acme/foo/-/archive/v1.0.0/foo-v1.0.0.tar.gz", true},
		{"gitea release archive", Gitea, "https://gitea.example.com/acme/foo/archive/v1.0.0.tar.gz", false},
		{"gitea branch archive", Gitea, "https://gitea.example.com/acme/foo/archive/main.zip", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &FormulaInfo{URL: c.url, Instance: c.instance}
			got, err := f.IsFollowingBranch()
			if err != nil {
				t.Fatalf("IsFollowingBranch: %v", err)
			}
			if got != c.want {
				t.Errorf("IsFollowingBranch() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestGetNewVersionURL(t *testing.T) {
	f := &FormulaInfo{URL: "https://github.com/acme/foo/archive/refs/tags/v#{version}.tar.gz"}
	got := f.GetNewVersionURL("1.2.3")
	want := "https://github.com/acme/foo/archive/refs/tags/v1.2.3.tar.gz"
	if got != want {
		t.Errorf("GetNewVersionURL = %q, want %q", got, want)
	}

	// double-v cleanup when the template and version both carry a "v".
	f2 := &FormulaInfo{URL: "https://example.com/archive/v#{version}.zip"}
	got2 := f2.GetNewVersionURL("v2.0.0")
	if strings.Contains(got2, "vv") {
		t.Errorf("GetNewVersionURL left a double v: %q", got2)
	}
}

func TestGetVersionURL(t *testing.T) {
	f := &FormulaInfo{URL: "https://example.com/#{version}/asset.tar.gz", Version: "3.4.5"}
	got := f.GetVersionURL()
	want := "https://example.com/3.4.5/asset.tar.gz"
	if got != want {
		t.Errorf("GetVersionURL = %q, want %q", got, want)
	}
}

func TestBottleNewUrl(t *testing.T) {
	b := &Bottle{Url: "https://example.com/bottles", target: "arm64_sequoia"}
	got := b.NewUrl("ignored", "foo", "1.2.3")
	want := "https://example.com/bottles/foo-1.2.3.arm64_sequoia.bottle.tar.gz"
	if got != want {
		t.Errorf("Bottle.NewUrl = %q, want %q", got, want)
	}
}

// A quote-breakout or interpolation payload in desc/version must come out
// escaped, never as live Ruby.
func TestUpdateEscapesInjection(t *testing.T) {
	dir := t.TempDir()
	path := writeFormula(t, dir, "foo.rb", sampleFormula)

	info, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}

	info.Description = `evil "; system("id"); x="`
	info.Version = `2.0.0"; require 'open3'; Open3.capture2("id"); x="`
	info.SHA256 = strings.Repeat("a", 64)

	if err := info.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}

	updated, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The raw, unescaped breakout must never appear.
	if strings.Contains(updated, `system("id"); x="`) {
		t.Fatalf("quote-breakout was not escaped:\n%s", updated)
	}
	if !strings.Contains(updated, `evil \"; system(\"id\"); x=\"`) {
		t.Fatalf("expected escaped description, got:\n%s", updated)
	}
	if !strings.Contains(updated, `2.0.0\"; require 'open3'; Open3.capture2(\"id\"); x=\"`) {
		t.Fatalf("expected escaped version, got:\n%s", updated)
	}

	// The updated file must still parse.
	reparsed, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("re-parsing updated formula failed: %v", err)
	}
	if reparsed.SHA256 != strings.Repeat("a", 64) {
		t.Errorf("sha256 not updated correctly: %q", reparsed.SHA256)
	}
}

func TestUpdateEscapesInterpolationOnlyNotBareHash(t *testing.T) {
	dir := t.TempDir()
	path := writeFormula(t, dir, "foo.rb", sampleFormula)
	info, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}

	info.Description = `A C# wrapper, fixes #123`
	info.Version = `1.0.1`
	if err := info.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}

	updated, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(updated, `desc "A C# wrapper, fixes #123"`) {
		t.Fatalf("benign '#' should not be escaped, got:\n%s", updated)
	}

	info.Description = `evil #{system("id")} interpolation`
	if err := info.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err = ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(updated, `\#{system(\"id\")}`) {
		t.Fatalf("expected #{...} interpolation to be escaped, got:\n%s", updated)
	}
}

func TestUpdatePreservesDollarSigns(t *testing.T) {
	dir := t.TempDir()
	path := writeFormula(t, dir, "foo.rb", sampleFormula)
	info, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}

	// A bare '$' must survive verbatim: it's a capture-group reference in the
	// regexp replacement, so an unescaped one would silently drop text.
	info.Description = `Costs $5, honours ${HOME} and $1`
	info.Version = `1.2.3+build$99`
	if err := info.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}

	updated, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(updated, `desc "Costs $5, honours ${HOME} and $1"`) {
		t.Fatalf("dollar signs in description were not preserved, got:\n%s", updated)
	}
	if !strings.Contains(updated, `version "1.2.3+build$99"`) {
		t.Fatalf("dollar sign in version was not preserved, got:\n%s", updated)
	}
}

func TestUpdateRevisionLifecycle(t *testing.T) {
	t.Run("set replaces existing revision", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFormula(t, dir, "foo.rb", sampleFormula)
		info, err := ParseFormula(path, dir)
		if err != nil {
			t.Fatalf("ParseFormula: %v", err)
		}
		info.Revision = "7"
		if err := info.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		updated, _ := ReadFile(path)
		if !strings.Contains(updated, "\n  revision 7\n") {
			t.Fatalf("expected revision line updated to 7, got:\n%s", updated)
		}
	})

	t.Run("clear removes existing revision line", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFormula(t, dir, "foo.rb", sampleFormula)
		info, err := ParseFormula(path, dir)
		if err != nil {
			t.Fatalf("ParseFormula: %v", err)
		}
		info.Revision = ""
		if err := info.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		updated, _ := ReadFile(path)
		if strings.Contains(updated, "revision") {
			t.Fatalf("expected revision line removed, got:\n%s", updated)
		}
		// The rest of the file must remain intact.
		if !strings.Contains(updated, `license "MIT"`) {
			t.Fatalf("clearing revision corrupted surrounding content:\n%s", updated)
		}
	})

	t.Run("absent revision line stays absent", func(t *testing.T) {
		dir := t.TempDir()
		content := strings.Replace(sampleFormula, "  revision 2\n", "", 1)
		path := writeFormula(t, dir, "foo.rb", content)
		info, err := ParseFormula(path, dir)
		if err != nil {
			t.Fatalf("ParseFormula: %v", err)
		}
		if info.Revision != "" {
			t.Fatalf("expected no revision parsed")
		}
		if err := info.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		updated, _ := ReadFile(path)
		if strings.Contains(updated, "revision") {
			t.Fatalf("did not expect a revision line to appear:\n%s", updated)
		}
	})
}
