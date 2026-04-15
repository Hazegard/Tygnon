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
