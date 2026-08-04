package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"gitlab.com/gitlab-org/api/client-go"
)

type GitlabApi struct {
	token  string
	url    string
	client *gitlab.Client
}

func NewGitlabApi(token string, url string) (*GitlabApi, error) {
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(url), gitlab.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("error creating gitlab client: %s", err)
	}
	return &GitlabApi{
		token:  token,
		url:    fmt.Sprintf("%s/api/v4", url),
		client: client,
	}, nil
}

func (gapi *GitlabApi) getProject(projectUrl string) (*gitlab.Project, error) {
	projectPath, err := ExtractProjectPath(projectUrl)
	if err != nil {
		return nil, fmt.Errorf("error extracting project path: %s", err)
	}
	projectPath = strings.Trim(projectPath, "/")
	project, _, err := gapi.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting project: %s", err)
	}
	return project, nil
}

func (gapi *GitlabApi) GetDescription(projectUrl string) (string, error) {
	project, err := gapi.getProject(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error getting project: %s", err)
	}
	return project.Description, nil
}

func (gapi *GitlabApi) GetLatestTagId(projectUrl string) (string, error) {
	project, err := gapi.getProject(projectUrl)
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

func (gapi *GitlabApi) GetBranchVersionId(projectUrl string) (string, error) {
	projectPath, err := ExtractProjectPath(projectUrl)
	if err != nil {
		return "", fmt.Errorf("error extracting project path: %s", err)
	}
	project, _, err := gapi.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return "", fmt.Errorf("error getting project: %s (%s)", err, projectPath)
	}

	commits, _, err := gapi.client.Commits.ListCommits(project.ID, &gitlab.ListCommitsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 1,
		},
		RefName: &project.DefaultBranch,
	})
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found on branch %s", project.DefaultBranch)
	}
	if err != nil {
		return "", fmt.Errorf("error listing commits: %s", err)
	}
	ID := commits[0].ShortID

	count := 0
	opts := &gitlab.ListCommitsOptions{
		RefName: &project.DefaultBranch,
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 50, // adjust as needed
		},
	}
	// Loop through pages until there are no more commits.
	for {
		commits, resp, err := gapi.client.Commits.ListCommits(project.ID, opts)
		if err != nil {
			return "", err
		}

		count += len(commits)

		// If there are no more pages, break out of the loop.
		if resp.NextPage <= 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return fmt.Sprintf("%d.%s", count, ID), nil
}

func (gapi *GitlabApi) GetLatestVersion(homepage string) (string, error) {
	release, releaseErr := gapi.GetLatestReleaseId(homepage)
	tag, tagErr := gapi.GetLatestTagId(homepage)
	if releaseErr != nil && tagErr != nil {
		return "", fmt.Errorf("no release or tag found: release: %w, tag: %w", releaseErr, tagErr)
	}
	if releaseErr != nil {
		return tag, nil
	}
	if tagErr != nil {
		return release, nil
	}
	if CompareVersions(release, tag) > 0 {
		return release, nil
	}
	return tag, nil
}

func (gapi *GitlabApi) HttpGet(assetUrl string, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", assetUrl, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []byte{}, fmt.Errorf("failed to download asset, status: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBodySize))
	if err != nil {
		return []byte{}, fmt.Errorf("failed to download asset: %w", err)

	}

	return body, nil
}
