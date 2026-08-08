package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// mockGitApi implements GitApi for testing handleFormulaWithClient without
// any network access.
type mockGitApi struct {
	latestVersion    string
	latestVersionErr error

	branchVersionID    string
	branchVersionIDErr error

	description    string
	descriptionErr error

	httpGetErr       error
	httpGetResponses map[string][]byte // keyed by assetUrl; falls back to a default body
}

func (m *mockGitApi) GetLatestTagId(string) (string, error)     { return "", nil }
func (m *mockGitApi) GetLatestReleaseId(string) (string, error) { return "", nil }

func (m *mockGitApi) GetLatestVersion(string) (string, error) {
	return m.latestVersion, m.latestVersionErr
}

func (m *mockGitApi) GetBranchVersionId(string) (string, error) {
	return m.branchVersionID, m.branchVersionIDErr
}

func (m *mockGitApi) GetDescription(string) (string, error) {
	return m.description, m.descriptionErr
}

func (m *mockGitApi) HttpGet(assetUrl string, _ string) ([]byte, error) {
	if m.httpGetErr != nil {
		return nil, m.httpGetErr
	}
	if b, ok := m.httpGetResponses[assetUrl]; ok {
		return b, nil
	}
	return []byte("default-asset-content"), nil
}

func newTestFormula(t *testing.T, dir, version string) FormulaInfo {
	t.Helper()
	content := `class Foo < Formula
  desc "old description"
  homepage "https://github.com/acme/foo"
  url "https://github.com/acme/foo/archive/refs/tags/v#{version}.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  version "` + version + `"
end
`
	path := writeFormula(t, dir, "foo.rb", content)
	info, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}
	return info
}

func TestHandleFormulaBumpsVersionAndUpdatesFile(t *testing.T) {
	dir := t.TempDir()
	formula := newTestFormula(t, dir, "1.0.0")

	mock := &mockGitApi{
		latestVersion: "v2.0.0",
		description:   "a shiny new description",
	}

	err, updated := handleFormulaWithClient(mock, "", formula, Config{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("handleFormulaWithClient: %v", err)
	}
	if updated.Version != "2.0.0" {
		t.Fatalf("Version = %q, want 2.0.0", updated.Version)
	}
	if updated.Description != "a shiny new description" {
		t.Fatalf("Description = %q", updated.Description)
	}
	wantSHA := Sha256([]byte("default-asset-content"))
	if updated.SHA256 != wantSHA {
		t.Fatalf("SHA256 = %q, want %q", updated.SHA256, wantSHA)
	}

	onDisk, err := ReadFile(formula.File)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(onDisk, `version "2.0.0"`) {
		t.Fatalf("formula file was not updated with the new version:\n%s", onDisk)
	}
	if !strings.Contains(onDisk, `desc "a shiny new description"`) {
		t.Fatalf("formula file was not updated with the new description:\n%s", onDisk)
	}
}

func TestHandleFormulaSkipsWhenNoNewVersion(t *testing.T) {
	dir := t.TempDir()
	formula := newTestFormula(t, dir, "2.0.0")

	mock := &mockGitApi{latestVersion: "v2.0.0"}

	err, updated := handleFormulaWithClient(mock, "", formula, Config{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("handleFormulaWithClient: %v", err)
	}
	if updated.URL != "" {
		t.Fatalf("expected a zero-value FormulaInfo when there's no update, got %+v", updated)
	}

	onDisk, err := ReadFile(formula.File)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(onDisk, `version "2.0.0"`) {
		t.Fatalf("formula file should be untouched when skipped:\n%s", onDisk)
	}
}

func TestHandleFormulaForceBumpsSameOrLowerVersion(t *testing.T) {
	dir := t.TempDir()
	formula := newTestFormula(t, dir, "2.0.0")

	mock := &mockGitApi{latestVersion: "v2.0.0", description: "same version, forced"}

	err, updated := handleFormulaWithClient(mock, "", formula, Config{Force: true}, zerolog.Nop())
	if err != nil {
		t.Fatalf("handleFormulaWithClient: %v", err)
	}
	if updated.Version != "2.0.0" || updated.Description != "same version, forced" {
		t.Fatalf("expected the formula to be re-processed under --force, got %+v", updated)
	}
}

func TestHandleFormulaSkipsWhenNewVersionIsLower(t *testing.T) {
	dir := t.TempDir()
	formula := newTestFormula(t, dir, "3.0.0")

	mock := &mockGitApi{latestVersion: "v1.0.0"}

	err, updated := handleFormulaWithClient(mock, "", formula, Config{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("handleFormulaWithClient: %v", err)
	}
	if updated.URL != "" {
		t.Fatalf("expected no update when the remote version is lower, got %+v", updated)
	}
}

func TestHandleFormulaPropagatesLatestVersionError(t *testing.T) {
	dir := t.TempDir()
	formula := newTestFormula(t, dir, "1.0.0")

	mock := &mockGitApi{latestVersionErr: errors.New("boom")}

	err, _ := handleFormulaWithClient(mock, "", formula, Config{}, zerolog.Nop())
	if err == nil {
		t.Fatalf("expected an error to be propagated when GetLatestVersion fails")
	}
}

func TestHandleFormulaAbortsOnReleaseDownloadFailure(t *testing.T) {
	dir := t.TempDir()
	formula := newTestFormula(t, dir, "1.0.0")

	original, err := ReadFile(formula.File)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// A newer version is available, but the release archive won't download.
	mock := &mockGitApi{latestVersion: "v2.0.0", httpGetErr: errors.New("network down")}

	err, updated := handleFormulaWithClient(mock, "", formula, Config{}, zerolog.Nop())
	if err == nil {
		t.Fatalf("expected an error when the release download fails")
	}
	if updated.URL != "" {
		t.Fatalf("expected a zero-value FormulaInfo on download failure, got %+v", updated)
	}

	// The file must be untouched: no version bump, no empty-file sha256.
	onDisk, err := ReadFile(formula.File)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if onDisk != original {
		t.Fatalf("formula file was modified despite a failed download:\n%s", onDisk)
	}
	emptyFileSha := Sha256(nil)
	if strings.Contains(onDisk, emptyFileSha) {
		t.Fatalf("empty-file sha256 %q was written into the formula:\n%s", emptyFileSha, onDisk)
	}
}

func TestHandleFormulaKeepsExistingDescriptionOnFetchError(t *testing.T) {
	dir := t.TempDir()
	formula := newTestFormula(t, dir, "1.0.0") // desc: "old description"

	// Download succeeds, but fetching the description fails.
	mock := &mockGitApi{latestVersion: "v2.0.0", descriptionErr: errors.New("api hiccup")}

	err, updated := handleFormulaWithClient(mock, "", formula, Config{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("handleFormulaWithClient: %v", err)
	}
	if updated.Version != "2.0.0" {
		t.Fatalf("expected the version to still be bumped, got %q", updated.Version)
	}
	if updated.Description != "old description" {
		t.Fatalf("expected the existing description to be preserved, got %q", updated.Description)
	}

	onDisk, err := ReadFile(formula.File)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(onDisk, `desc "old description"`) {
		t.Fatalf("description was blanked/altered on a fetch error:\n%s", onDisk)
	}
}

func TestHandleFormulaClearsRevisionUnlessKeepRevisionSet(t *testing.T) {
	dir := t.TempDir()
	content := `class Foo < Formula
  desc "old description"
  homepage "https://github.com/acme/foo"
  url "https://github.com/acme/foo/archive/refs/tags/v#{version}.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  version "1.0.0"
  revision 3
end
`
	path := writeFormula(t, dir, "foo.rb", content)
	formula, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}
	if formula.Revision != "3" {
		t.Fatalf("precondition: expected revision 3, got %q", formula.Revision)
	}

	mock := &mockGitApi{latestVersion: "v2.0.0"}

	t.Run("cleared by default", func(t *testing.T) {
		err, updated := handleFormulaWithClient(mock, "", formula, Config{}, zerolog.Nop())
		if err != nil {
			t.Fatalf("handleFormulaWithClient: %v", err)
		}
		if updated.Revision != "" {
			t.Fatalf("expected revision to be cleared, got %q", updated.Revision)
		}
		onDisk, _ := ReadFile(path)
		if strings.Contains(onDisk, "revision") {
			t.Fatalf("expected the revision line to be removed:\n%s", onDisk)
		}
	})

	t.Run("kept with KeepRevision", func(t *testing.T) {
		// Re-parse a fresh copy since the previous subtest already rewrote
		// the file on disk.
		path2 := writeFormula(t, dir, "foo2.rb", content)
		formula2, err := ParseFormula(path2, dir)
		if err != nil {
			t.Fatalf("ParseFormula: %v", err)
		}
		err, updated := handleFormulaWithClient(mock, "", formula2, Config{KeepRevision: true}, zerolog.Nop())
		if err != nil {
			t.Fatalf("handleFormulaWithClient: %v", err)
		}
		if updated.Revision != "3" {
			t.Fatalf("expected revision to be kept, got %q", updated.Revision)
		}
	})
}

func TestHandleFormulaBottleUpdateFallsBackToLowercaseName(t *testing.T) {
	dir := t.TempDir()
	content := `class Foo < Formula
  desc "old description"
  homepage "https://github.com/acme/foo"
  url "https://github.com/acme/foo/archive/refs/tags/v#{version}.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  version "1.0.0"

  bottle do
    root_url "https://example.com/bottles"
    sha256 arm64_sequoia: "1111111111111111111111111111111111111111111111111111111111111111"
  end
end
`
	path := writeFormula(t, dir, "Foo.rb", content)
	formula, err := ParseFormula(path, dir)
	if err != nil {
		t.Fatalf("ParseFormula: %v", err)
	}
	if len(formula.Bottles) != 1 {
		t.Fatalf("expected 1 bottle, got %d", len(formula.Bottles))
	}

	// The mixed-case bottle URL fails so the lowercase fallback runs; the
	// release download must succeed, else the update aborts first.
	mixedCaseBottleURL := formula.Bottles[0].NewUrl(formula.BottleURL, formula.GetName(), "2.0.0")
	lowercaseBottleURL := formula.Bottles[0].NewUrl(formula.BottleURL, "foo", "2.0.0")
	client := &conditionalHttpGetMock{
		mockGitApi: &mockGitApi{latestVersion: "v2.0.0"},
		failURLs:   map[string]bool{mixedCaseBottleURL: true},
		bodies:     map[string][]byte{lowercaseBottleURL: []byte("bottle-bytes")},
	}

	err, updated := handleFormulaWithClient(client, "", formula, Config{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("handleFormulaWithClient: %v", err)
	}
	wantSha := Sha256([]byte("bottle-bytes"))
	if updated.Bottles[0].Sha256 != wantSha {
		t.Fatalf("bottle sha256 = %q, want %q (lowercase-name fallback should have been used)", updated.Bottles[0].Sha256, wantSha)
	}
}

// conditionalHttpGetMock errors for URLs in failURLs, serves per-URL bodies
// when set, and a default otherwise.
type conditionalHttpGetMock struct {
	*mockGitApi
	failURLs map[string]bool
	bodies   map[string][]byte
}

func (c *conditionalHttpGetMock) HttpGet(assetUrl string, token string) ([]byte, error) {
	if c.failURLs[assetUrl] {
		return nil, errors.New("simulated 404")
	}
	if b, ok := c.bodies[assetUrl]; ok {
		return b, nil
	}
	return []byte("default-asset-content"), nil
}

func TestHandleFormulaUnknownInstanceErrors(t *testing.T) {
	dir := t.TempDir()
	content := `class Foo < Formula
  desc "d"
  homepage "https://example.com/acme/foo"
  url "https://example.com/acme/foo/archive/v1.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  version "1.0.0"
end
`
	path := filepath.Join(dir, "foo.rb")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write formula: %v", err)
	}

	// Set an unknown instance directly, so fingerprinting doesn't hit the network.
	formula := FormulaInfo{
		File:      path,
		Directory: dir,
		Homepage:  "https://example.com/acme/foo",
		URL:       "https://example.com/acme/foo/archive/v1.0.0.tar.gz",
		Version:   "1.0.0",
		Instance:  GitType(-1),
	}

	err, _ := HandleFormula(formula, Config{}, zerolog.Nop())
	if err == nil {
		t.Fatalf("expected an error for an unrecognized git instance")
	}
	if !strings.Contains(err.Error(), "unknown git instance") {
		t.Fatalf("error = %v, want it to mention 'unknown git instance'", err)
	}
}
