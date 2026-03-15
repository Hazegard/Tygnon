package main

import (
	"fmt"
	"gitlab.com/gitlab-org/api/client-go"
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

func (gapi *GitlabApi) GetLatestTagId(projectUrl string) (*gitlab.Tag, error) {
	projectPath, err := ExtractProjectPath(projectUrl)
	if err != nil {
		return nil, fmt.Errorf("error extracting project path: %s", err)
	}
	projectPath = strings.Trim(projectPath, "/")
	project, _, err := gapi.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting project: %s", err)
	}
	tags, _, err := gapi.client.Tags.ListTags(project.ID, nil)
	if len(tags) == 0 {
		return nil, fmt.Errorf("error getting latest tag: %s", err)
	}
	return tags[0], nil
}

func (gapi *GitlabApi) GetLatestReleaseId(projectUrl string) (*gitlab.Release, error) {
	projectPath, err := ExtractProjectPath(projectUrl)
	if err != nil {
		return nil, fmt.Errorf("error extracting project path: %s", err)
	}
	projectPath = strings.Trim(projectPath, "/")
	project, _, err := gapi.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting project: %s", err)
	}
	release, _, err := gapi.client.Releases.GetLatestRelease(project.ID)
	if release == nil {
		return nil, fmt.Errorf("error getting latest release: %s", err)
	}
	return release, nil
}

func ExtractProjectPath(u string) (string, error) {
	uu, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("error parsing url: %s", err)
	}
	return uu.Path, nil
}
