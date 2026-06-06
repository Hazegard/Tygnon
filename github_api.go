package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-github/v69/github"
)

// GithubApi now wraps a GitHub client while keeping the same interface.
type GithubApi struct {
	url    string
	client *github.Client
}

// NewGithubApi creates a new GitHub client using the provided token and base URL.
// If a non-default URL is provided, it attempts to create an Enterprise client.
func NewGithubApi(token string) *GithubApi {
	client := github.NewClient(nil)
	if token != "" {
		client = client.WithAuthToken(token)
	}
	baseURL := "https://api.github.com/"

	return &GithubApi{
		url:    baseURL,
		client: client,
	}
}

func (gapi *GithubApi) GetDescription(projectUrl string) (string, error) {
	owner, repoName, err := GetOwnerRepo(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting owner/repo info: %v", err)
	}
	repo, _, err := gapi.client.Repositories.Get(context.Background(), owner, repoName)
	if err != nil {
		return "", fmt.Errorf("error getting repo description: %v", err)
	}

	// GetDescription safely returns the repository description.
	description := repo.GetDescription()
	return description, nil
}

// GetLatestTagId retrieves the latest repository tag from a GitHub repository.
// The projectUrl should be a full URL (e.g. "https://github.com/owner/repo").
func (gapi *GithubApi) GetLatestTagId(projectUrl string) (string, error) {
	owner, repo, err := GetOwnerRepo(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting owner repo: %w", err)
	}
	ctx := context.Background()
	tags, _, err := gapi.client.Repositories.ListTags(ctx, owner, repo, nil)
	if err != nil {
		return "", fmt.Errorf("error getting tags: %w", err)
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("no tags found")
	}
	// Assuming the first tag in the list is the "latest" tag.
	//
	// sort.Slice(tags, func(i, j int) bool {
	// 	result := CompareVersions(tags[i].GetName(), tags[j].GetName())
	// 	return result > 0
	// })
	return tags[0].GetName(), nil
}

// GetLatestReleaseId retrieves the latest release from a GitHub repository.
// The projectUrl should be a full URL (e.g. "https://github.com/owner/repo").
func (gapi *GithubApi) GetLatestReleaseId(projectUrl string) (string, error) {
	projectPath, err := ExtractProjectPath(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error extracting project path: %w", err)
	}
	parts := strings.Split(projectPath, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid project path: %s", projectPath)
	}
	owner := parts[0]
	repo := parts[1]

	ctx := context.Background()
	release, resp, err := gapi.client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		// If a 404 is returned, it means there is no release.
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("no release found")
		}
		return "", fmt.Errorf("error getting latest release: %w", err)
	}
	return release.GetName(), nil
}

func (gapi *GithubApi) GetBranchVersionId(projectUrl string) (string, error) {
	ctx := context.Background()
	owner, repoName, err := GetOwnerRepo(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting owner/repo info: %v", err)
	}
	// Get repository details to obtain the default branch.
	repository, _, err := gapi.client.Repositories.Get(ctx, owner, repoName)
	if err != nil {
		return "", fmt.Errorf("error getting repository: %w", err)
	}
	opts := &github.CommitsListOptions{
		SHA: repository.GetDefaultBranch(),
		ListOptions: github.ListOptions{
			Page:    1,
			PerPage: 1,
		},
	}
	commits, resp, err := gapi.client.Repositories.ListCommits(ctx, owner, repoName, opts)
	if err != nil {
		return "", fmt.Errorf("error listing commits: %w", err)
	}
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found on branch %s", repository.GetDefaultBranch())
	}

	commitCount := resp.LastPage
	if commitCount == 0 {
		commitCount = 1
	}

	// Use the first commit's SHA (shortened to 8 characters).
	commitSHA := commits[0].GetSHA()
	shortSHA := commitSHA
	if len(commitSHA) > 8 {
		shortSHA = commitSHA[:8]
	}

	return fmt.Sprintf("%d.%s", commitCount, shortSHA), nil
}

// GetLatestVersion first attempts to get the latest release tag name.
// If no release is found, it falls back to getting the latest tag name.
func (gapi *GithubApi) GetLatestVersion(homepage string) (string, error) {
	release, err := gapi.GetLatestReleaseId(homepage)
	if err != nil {
		release = "0"
	}

	tag, err := gapi.GetLatestTagId(homepage)
	if err != nil {
		tag = "0"
	}
	cmp := CompareVersions(release, tag)
	if cmp > 0 {
		return release, nil
	}
	return tag, nil
}

// HttpGet downloads data from the given assetUrl.
// For GitHub, the token is added as an "Authorization" header if provided.
func (gapi *GithubApi) HttpGet(assetUrl string, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", assetUrl, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download asset, status: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read asset: %w", err)
	}
	return body, nil
}
