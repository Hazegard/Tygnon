package main

import (
	"fmt"
	"io"
	"net/http"

	"code.gitea.io/sdk/gitea"
)

type GiteaApi struct {
	token  string
	url    string
	client *gitea.Client
}

func NewGiteaApi(token string, url string) (*GiteaApi, error) {

	client, err := gitea.NewClient(url, gitea.SetToken(token))
	if err != nil {
		return nil, fmt.Errorf("error creating gitea client: %s", err)
	}
	return &GiteaApi{
		token:  token,
		url:    url,
		client: client,
	}, nil
}

func (api *GiteaApi) getRepo(owner string, repoName string) (*gitea.Repository, error) {
	repo, _, err := api.client.GetRepo(owner, repoName)
	if err != nil {
		return nil, fmt.Errorf("error getting repo: %s", err)
	}
	return repo, nil
}

func (api *GiteaApi) GetDescription(projectUrl string) (string, error) {
	owner, repoName, err := GetOwnerRepo(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting owner repo: %s", err)
	}
	repo, err := api.getRepo(owner, repoName)
	if err != nil {
		return "", fmt.Errorf("error getting repo: %s", err)
	}
	return repo.Description, nil
}

func (api *GiteaApi) GetLatestTagId(projectUrl string) (string, error) {
	owner, repoName, err := GetOwnerRepo(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting owner repo: %s", err)
	}
	tags, _, err := api.client.ListRepoTags(owner, repoName, gitea.ListRepoTagsOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting tags: %s", err)
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("no tags found")
	}
	return tags[0].Name, nil
}

func (api *GiteaApi) GetLatestReleaseId(projectUrl string) (string, error) {
	owner, repoName, err := GetOwnerRepo(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting owner repo: %s", err)
	}
	release, _, err := api.client.GetLatestRelease(owner, repoName)
	if err != nil {
		return "", fmt.Errorf("error getting latest release: %s", err)
	}
	return release.TagName, nil
}

func (api *GiteaApi) GetMasterVersionId(projectUrl string) (string, error) {
	owner, repoName, err := GetOwnerRepo(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting owner repo: %s", err)
	}
	// Get repository details to obtain the default branch.
	repository, _, err := api.client.GetRepo(owner, repoName)
	if err != nil {
		return "", fmt.Errorf("error getting repository: %w", err)
	}

	// List commits on the default branch with a page size of 1.
	commits, resp, err := api.client.ListRepoCommits(owner, repoName, gitea.ListCommitOptions{
		SHA: repository.DefaultBranch,
		ListOptions: gitea.ListOptions{
			Page:     1,
			PageSize: 1,
		},
	})
	if err != nil {
		return "", fmt.Errorf("error listing commits: %w", err)
	}
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found on branch %s", repository.DefaultBranch)
	}

	opts := gitea.ListCommitOptions{
		SHA: repository.DefaultBranch,
		ListOptions: gitea.ListOptions{
			Page:     1,
			PageSize: 1,
		},
	}
	count := 0
	// Loop through pages until there are no more commits.
	for {
		commits, resp, err := api.client.ListRepoCommits(owner, repoName, opts)
		if err != nil {
			return "", err
		}

		count += len(commits)

		// If there are no more pages, break out of the loop.
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	if count == 0 {
		return "", fmt.Errorf("no commits found on branch %s", repository.DefaultBranch)
	}
	commitCount := resp.LastPage
	if commitCount == 0 {
		commitCount = 1
	}
	// Get the short commit SHA (first 8 characters).
	shortCommit := commits[0].SHA
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}
	return fmt.Sprintf("%d.%s", commitCount, shortCommit), nil
}

func (api *GiteaApi) GetLatestVersion(projectUrl string) (string, error) {
	release, err := api.GetLatestReleaseId(projectUrl)
	if err != nil {
		release = "0"
	}

	tag, err := api.GetLatestTagId(projectUrl)
	if err != nil {
		tag = "0"
	}

	cmp, err := CompareVersions(release, tag)
	if err != nil {
		return "", fmt.Errorf("error comparing versions: %w", err)
	}

	if cmp > 0 {
		return release, nil
	}
	return tag, nil
}

func (api *GiteaApi) HttpGet(assetUrl string, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", assetUrl, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []byte{}, fmt.Errorf("failed to download asset, status: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to download asset: %w", err)
	}
	return body, nil
}
