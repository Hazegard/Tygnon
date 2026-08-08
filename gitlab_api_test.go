package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gitlab.com/gitlab-org/api/client-go"
)

type gitlabMockOptions struct {
	description       string
	tags              []string // first entry is what ListTags returns as "latest"
	releaseTag        string   // empty means "no release" (404)
	defaultBranch     string
	commitShortIDs    []string // full commit history, index 0 = most recent
	provideTotalItems bool     // whether the mock reports X-Total on the per_page=1 commits call
}

// The returned counter counts commit pages fetched with per_page != 1, i.e.
// the pagination fallback, so tests can tell the fast path from it.
func newGitlabMockServer(t *testing.T, m gitlabMockOptions) (*httptest.Server, *int32) {
	t.Helper()
	var countingPageCalls int32

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(path, "/repository/tags"):
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

		case strings.Contains(path, "/releases"):
			if m.releaseTag == "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "404 Not Found"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"tag_name": %q}`, m.releaseTag)))

		case strings.Contains(path, "/repository/commits"):
			q := r.URL.Query()
			page, _ := strconv.Atoi(q.Get("page"))
			if page < 1 {
				page = 1
			}
			perPage, _ := strconv.Atoi(q.Get("per_page"))
			if perPage < 1 {
				perPage = 20
			}
			if perPage != 1 {
				atomic.AddInt32(&countingPageCalls, 1)
			}

			total := len(m.commitShortIDs)
			start := (page - 1) * perPage
			if start > total {
				start = total
			}
			end := start + perPage
			if end > total {
				end = total
			}

			var sb strings.Builder
			sb.WriteString("[")
			for i, id := range m.commitShortIDs[start:end] {
				if i > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(fmt.Sprintf(`{"id": %q, "short_id": %q}`, id, id))
			}
			sb.WriteString("]")

			if perPage == 1 && m.provideTotalItems {
				w.Header().Set("X-Total", strconv.Itoa(total))
				w.Header().Set("X-Total-Pages", strconv.Itoa(total))
			}
			if end < total {
				w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sb.String()))

		case strings.Contains(path, "/projects/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"id": 1, "default_branch": %q, "description": %q}`,
				m.defaultBranch, m.description,
			)))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return httptest.NewServer(mux), &countingPageCalls
}

func newTestGitlabApi(t *testing.T, server *httptest.Server) *GitlabApi {
	t.Helper()
	client, err := gitlab.NewClient("", gitlab.WithBaseURL(server.URL), gitlab.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("gitlab.NewClient: %v", err)
	}
	return &GitlabApi{client: client}
}

func TestGitlabGetLatestVersionReleaseBeatsTags(t *testing.T) {
	server, _ := newGitlabMockServer(t, gitlabMockOptions{
		tags:          []string{"v1.5.0"},
		releaseTag:    "v2.0.0",
		defaultBranch: "main",
	})
	defer server.Close()

	api := newTestGitlabApi(t, server)
	got, err := api.GetLatestVersion("https://gitlab.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got != "v2.0.0" {
		t.Fatalf("GetLatestVersion() = %q, want v2.0.0", got)
	}
}

func TestGitlabGetLatestVersionFallsBackToTagWhenNoRelease(t *testing.T) {
	server, _ := newGitlabMockServer(t, gitlabMockOptions{
		tags:          []string{"v1.5.0"},
		releaseTag:    "", // no release
		defaultBranch: "main",
	})
	defer server.Close()

	api := newTestGitlabApi(t, server)
	got, err := api.GetLatestVersion("https://gitlab.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got != "v1.5.0" {
		t.Fatalf("GetLatestVersion() = %q, want v1.5.0", got)
	}
}

func TestGitlabGetLatestVersionErrorsWhenNeitherExists(t *testing.T) {
	server, _ := newGitlabMockServer(t, gitlabMockOptions{
		tags:          nil,
		releaseTag:    "",
		defaultBranch: "main",
	})
	defer server.Close()

	api := newTestGitlabApi(t, server)
	if _, err := api.GetLatestVersion("https://gitlab.example.com/acme/foo"); err == nil {
		t.Fatalf("expected an error when neither a release nor a tag exists")
	}
}

func TestGitlabGetDescription(t *testing.T) {
	server, _ := newGitlabMockServer(t, gitlabMockOptions{description: "a nice project"})
	defer server.Close()

	api := newTestGitlabApi(t, server)
	got, err := api.GetDescription("https://gitlab.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetDescription: %v", err)
	}
	if got != "a nice project" {
		t.Fatalf("GetDescription() = %q", got)
	}
}

func commitIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("commit%04d", i)
	}
	return ids
}

func TestGitlabGetBranchVersionIdFastPathUsesTotalItems(t *testing.T) {
	commits := commitIDs(250)
	server, pageCalls := newGitlabMockServer(t, gitlabMockOptions{
		defaultBranch:     "main",
		commitShortIDs:    commits,
		provideTotalItems: true,
	})
	defer server.Close()

	api := newTestGitlabApi(t, server)
	got, err := api.GetBranchVersionId("https://gitlab.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetBranchVersionId: %v", err)
	}
	want := "250." + commits[0]
	if got != want {
		t.Fatalf("GetBranchVersionId() = %q, want %q", got, want)
	}
	if calls := atomic.LoadInt32(pageCalls); calls != 0 {
		t.Fatalf("expected the fast path to avoid paginating through commits, got %d paginated calls", calls)
	}
}

func TestGitlabGetBranchVersionIdFallsBackToPaginationWithoutTotalItems(t *testing.T) {
	commits := commitIDs(250)
	server, pageCalls := newGitlabMockServer(t, gitlabMockOptions{
		defaultBranch:     "main",
		commitShortIDs:    commits,
		provideTotalItems: false,
	})
	defer server.Close()

	api := newTestGitlabApi(t, server)
	got, err := api.GetBranchVersionId("https://gitlab.example.com/acme/foo")
	if err != nil {
		t.Fatalf("GetBranchVersionId: %v", err)
	}
	want := "250." + commits[0]
	if got != want {
		t.Fatalf("GetBranchVersionId() = %q, want %q", got, want)
	}
	if calls := atomic.LoadInt32(pageCalls); calls < 2 {
		t.Fatalf("expected the fallback to paginate across multiple pages, got %d paginated calls", calls)
	}
}

func TestGitlabGetBranchVersionIdNoCommits(t *testing.T) {
	server, _ := newGitlabMockServer(t, gitlabMockOptions{defaultBranch: "main"})
	defer server.Close()

	api := newTestGitlabApi(t, server)
	if _, err := api.GetBranchVersionId("https://gitlab.example.com/acme/foo"); err == nil {
		t.Fatalf("expected an error when the branch has no commits")
	}
}
