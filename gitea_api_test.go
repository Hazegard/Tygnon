package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"code.gitea.io/sdk/gitea"
)

type giteaMockOptions struct {
	description   string
	tags          []string // first entry is what ListRepoTags returns as "latest"
	releaseTag    string   // empty means "no release" (404)
	defaultBranch string
	commitSHAs    []string // only the most recent SHA is served; len used to compute LastPage
}

func newGiteaMockServer(t *testing.T, m giteaMockOptions) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(path, "/version"):
			// gitea.NewClient checks the server version on construction.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version": "1.21.0"}`))

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
			if m.releaseTag == "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"tag_name": %q}`, m.releaseTag)))

		case strings.HasSuffix(path, "/commits"):
			total := len(m.commitSHAs)
			if total == 0 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("[]"))
				return
			}
			perPage, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if perPage < 1 {
				perPage = 30
			}
			// Like GitHub: only send rel="last" when page 1 isn't the last.
			if total > 1 {
				w.Header().Set("Link", fmt.Sprintf(
					`<%s?page=2&limit=%d>; rel="next", <%s?page=%d&limit=%d>; rel="last"`,
					path, perPage, path, total, perPage,
				))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"sha": %q}]`, m.commitSHAs[0])))

		case strings.Contains(path, "/repos/"):
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

func newTestGiteaApi(t *testing.T, server *httptest.Server) *GiteaApi {
	t.Helper()
	client, err := gitea.NewClient(server.URL, gitea.SetHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("gitea.NewClient: %v", err)
	}
	return &GiteaApi{client: client}
}

func TestGiteaGetLatestVersionReleaseBeatsTags(t *testing.T) {
	server := newGiteaMockServer(t, giteaMockOptions{
		tags:          []string{"v1.5.0"},
		releaseTag:    "v2.0.0",
		defaultBranch: "main",
	})
	defer server.Close()

	api := newTestGiteaApi(t, server)
	got, err := api.GetLatestVersion("https://gitea.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got != "v2.0.0" {
		t.Fatalf("GetLatestVersion() = %q, want v2.0.0", got)
	}
}

func TestGiteaGetLatestVersionFallsBackToTagWhenNoRelease(t *testing.T) {
	server := newGiteaMockServer(t, giteaMockOptions{
		tags:          []string{"v1.5.0"},
		releaseTag:    "",
		defaultBranch: "main",
	})
	defer server.Close()

	api := newTestGiteaApi(t, server)
	got, err := api.GetLatestVersion("https://gitea.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got != "v1.5.0" {
		t.Fatalf("GetLatestVersion() = %q, want v1.5.0", got)
	}
}

func TestGiteaGetLatestVersionErrorsWhenNeitherExists(t *testing.T) {
	server := newGiteaMockServer(t, giteaMockOptions{defaultBranch: "main"})
	defer server.Close()

	api := newTestGiteaApi(t, server)
	if _, err := api.GetLatestVersion("https://gitea.example.com/acme/foo"); err == nil {
		t.Fatalf("expected an error when neither a release nor a tag exists")
	}
}

func TestGiteaGetDescription(t *testing.T) {
	server := newGiteaMockServer(t, giteaMockOptions{description: "a nice project"})
	defer server.Close()

	api := newTestGiteaApi(t, server)
	got, err := api.GetDescription("https://gitea.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetDescription: %v", err)
	}
	if got != "a nice project" {
		t.Fatalf("GetDescription() = %q", got)
	}
}

func TestGiteaGetBranchVersionIdUsesLastPageAsCommitCount(t *testing.T) {
	shas := make([]string, 250)
	for i := range shas {
		shas[i] = fmt.Sprintf("%040d", i)
	}
	server := newGiteaMockServer(t, giteaMockOptions{
		defaultBranch: "main",
		commitSHAs:    shas,
	})
	defer server.Close()

	api := newTestGiteaApi(t, server)
	got, err := api.GetBranchVersionId("https://gitea.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetBranchVersionId: %v", err)
	}
	want := fmt.Sprintf("250.%s", shas[0][:8])
	if got != want {
		t.Fatalf("GetBranchVersionId() = %q, want %q", got, want)
	}
}

func TestGiteaGetBranchVersionIdSingleCommitOmitsLastLink(t *testing.T) {
	shas := []string{fmt.Sprintf("%040d", 1)}
	server := newGiteaMockServer(t, giteaMockOptions{
		defaultBranch: "main",
		commitSHAs:    shas,
	})
	defer server.Close()

	api := newTestGiteaApi(t, server)
	got, err := api.GetBranchVersionId("https://gitea.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetBranchVersionId: %v", err)
	}
	want := fmt.Sprintf("1.%s", shas[0][:8])
	if got != want {
		t.Fatalf("GetBranchVersionId() = %q, want %q", got, want)
	}
}

func TestGiteaGetBranchVersionIdNoCommits(t *testing.T) {
	server := newGiteaMockServer(t, giteaMockOptions{defaultBranch: "main"})
	defer server.Close()

	api := newTestGiteaApi(t, server)
	if _, err := api.GetBranchVersionId("https://gitea.example.com/acme/foo"); err == nil {
		t.Fatalf("expected an error when the branch has no commits")
	}
}
