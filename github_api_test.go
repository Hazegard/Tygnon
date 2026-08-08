package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/v69/github"
)

type githubMockOptions struct {
	description   string
	tags          []string // first entry is what ListTags returns as "latest"
	releaseName   string   // empty means "no release" (404); this is the release's free-text Name, not TagName
	defaultBranch string
	commitSHAs    []string // only the most recent SHA is actually served; len used to compute LastPage
}

func newGithubMockServer(t *testing.T, m githubMockOptions) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(path, "/tags"):
			var sb strings.Builder
			sb.WriteString("[")
			for i, name := range m.tags {
				if i > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(fmt.Sprintf(`{"name": %q}`, name))
			}
			sb.WriteString("]")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sb.String()))

		case strings.HasSuffix(path, "/releases/latest"):
			if m.releaseName == "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"name": %q, "tag_name": %q}`, m.releaseName, m.releaseName)))

		case strings.HasSuffix(path, "/commits"):
			total := len(m.commitSHAs)
			if total == 0 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("[]"))
				return
			}
			perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
			if perPage < 1 {
				perPage = 30
			}
			// Only send the next/last links when page 1 isn't already the last.
			if total > 1 {
				w.Header().Set("Link", fmt.Sprintf(
					`<%s?page=2&per_page=%d>; rel="next", <%s?page=%d&per_page=%d>; rel="last"`,
					path, perPage, path, total, perPage,
				))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"sha": %q}]`, m.commitSHAs[0])))

		case strings.HasPrefix(path, "/repos/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"default_branch": %q, "description": %q}`,
				m.defaultBranch, m.description,
			)))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return httptest.NewServer(mux)
}

// newTestGithubApi points a GithubApi at a mock server via the client BaseURL.
func newTestGithubApi(t *testing.T, server *httptest.Server) *GithubApi {
	t.Helper()
	client := github.NewClient(server.Client())
	base, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client.BaseURL = base
	return &GithubApi{client: client}
}

func TestGithubGetLatestVersionReleaseBeatsTags(t *testing.T) {
	server := newGithubMockServer(t, githubMockOptions{
		tags:          []string{"v1.5.0"},
		releaseName:   "v2.0.0",
		defaultBranch: "main",
	})
	defer server.Close()

	api := newTestGithubApi(t, server)
	got, err := api.GetLatestVersion("https://github.com/acme/foo")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got != "v2.0.0" {
		t.Fatalf("GetLatestVersion() = %q, want v2.0.0", got)
	}
}

func TestGithubGetLatestVersionFallsBackToTagWhenNoRelease(t *testing.T) {
	server := newGithubMockServer(t, githubMockOptions{
		tags:          []string{"v1.5.0"},
		releaseName:   "",
		defaultBranch: "main",
	})
	defer server.Close()

	api := newTestGithubApi(t, server)
	got, err := api.GetLatestVersion("https://github.com/acme/foo")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got != "v1.5.0" {
		t.Fatalf("GetLatestVersion() = %q, want v1.5.0", got)
	}
}

func TestGithubGetLatestVersionErrorsWhenNeitherExists(t *testing.T) {
	server := newGithubMockServer(t, githubMockOptions{defaultBranch: "main"})
	defer server.Close()

	api := newTestGithubApi(t, server)
	if _, err := api.GetLatestVersion("https://github.com/acme/foo"); err == nil {
		t.Fatalf("expected an error when neither a release nor a tag exists")
	}
}

func TestGithubGetDescription(t *testing.T) {
	server := newGithubMockServer(t, githubMockOptions{description: "a nice project"})
	defer server.Close()

	api := newTestGithubApi(t, server)
	got, err := api.GetDescription("https://github.com/acme/foo")
	if err != nil {
		t.Fatalf("GetDescription: %v", err)
	}
	if got != "a nice project" {
		t.Fatalf("GetDescription() = %q", got)
	}
}

func TestGithubGetBranchVersionIdUsesLastPageAsCommitCount(t *testing.T) {
	shas := make([]string, 250)
	for i := range shas {
		shas[i] = fmt.Sprintf("%040d", i) // 40-char fake SHAs
	}
	server := newGithubMockServer(t, githubMockOptions{
		defaultBranch: "main",
		commitSHAs:    shas,
	})
	defer server.Close()

	api := newTestGithubApi(t, server)
	got, err := api.GetBranchVersionId("https://github.com/acme/foo")
	if err != nil {
		t.Fatalf("GetBranchVersionId: %v", err)
	}
	want := fmt.Sprintf("250.%s", shas[0][:8])
	if got != want {
		t.Fatalf("GetBranchVersionId() = %q, want %q", got, want)
	}
}

func TestGithubGetBranchVersionIdSingleCommitOmitsLastLink(t *testing.T) {
	// One commit: no rel="last" link, so LastPage stays 0 and must become 1.
	shas := []string{fmt.Sprintf("%040d", 1)}
	server := newGithubMockServer(t, githubMockOptions{
		defaultBranch: "main",
		commitSHAs:    shas,
	})
	defer server.Close()

	api := newTestGithubApi(t, server)
	got, err := api.GetBranchVersionId("https://github.com/acme/foo")
	if err != nil {
		t.Fatalf("GetBranchVersionId: %v", err)
	}
	want := fmt.Sprintf("1.%s", shas[0][:8])
	if got != want {
		t.Fatalf("GetBranchVersionId() = %q, want %q", got, want)
	}
}

func TestGithubGetBranchVersionIdNoCommits(t *testing.T) {
	server := newGithubMockServer(t, githubMockOptions{defaultBranch: "main"})
	defer server.Close()

	api := newTestGithubApi(t, server)
	if _, err := api.GetBranchVersionId("https://github.com/acme/foo"); err == nil {
		t.Fatalf("expected an error when the branch has no commits")
	}
}
