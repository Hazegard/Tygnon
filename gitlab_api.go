package main

import (
	"fmt"
	"gitlab.com/gitlab-org/api/client-go"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type GitlabApi struct {
	token  string
	url    string
	client *gitlab.Client
}

func NewGitlabApi(token string, url string) (*GitlabApi, error) {
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(url))
	if err != nil {
		return nil, fmt.Errorf("error creating gitlab client: %s", err)
	}
	return &GitlabApi{
		token:  token,
		url:    "https://git.example.fr/api/v4/",
		client: client,
	}, nil
}

func (gapi *GitlabApi) GetLatestTagId(projectUrl string) (string, error) {
	projectPath, err := ExtractProjectPath(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error extracting project path: %s", err)
	}
	projectPath = strings.Trim(projectPath, "/")
	project, _, err := gapi.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return "", fmt.Errorf("error getting project: %s", err)
	}
	tags, _, err := gapi.client.Tags.ListTags(project.ID, nil)
	if len(tags) == 0 {
		return "", fmt.Errorf("error getting latest tag: %s", err)
	}
	return tags[0].Name, nil
}

func (gapi *GitlabApi) GetLatestReleaseId(projectUrl string) (string, error) {
	projectPath, err := ExtractProjectPath(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error extracting project path: %s", err)
	}
	project, _, err := gapi.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return "", fmt.Errorf("error getting project: %s", err)
	}
	release, _, err := gapi.client.Releases.GetLatestRelease(project.ID)
	if release == nil {
		return "", fmt.Errorf("error getting latest release: %s", err)
	}
	return release.TagName, nil
}

func ExtractProjectPath(u string) (string, error) {
	uu, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("error parsing url: %s", err)
	}
	return strings.Trim(uu.Path, "/"), nil
}

func (gapi *GitlabApi) GetLatestVersion(homepage string) (string, error) {
	newVersion := ""
	release, err := gapi.GetLatestReleaseId(homepage)
	if err == nil {
		newVersion = release
	} else {
		tag, err := gapi.GetLatestTagId(homepage)
		if err != nil {
			return "", fmt.Errorf("error getting latest tag (%s): %s\n", homepage, err)
		}
		newVersion = tag
	}
	return newVersion, nil
}

func (gapi *GitlabApi) HttpGet(assetUrl string, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", assetUrl, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
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
