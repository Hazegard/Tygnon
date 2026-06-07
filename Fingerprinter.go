package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

var fingerprinter = new(Fingerprinter)

type Fingerprinter struct {
	fg map[string]GitType
}

func (f *Fingerprinter) fingerPrintInstance(url string) (GitType, error) {

	if v, ok := f.fg[url]; ok {
		return v, nil
	}
	res, err := http.Get(fmt.Sprintf("https://%s", url))
	if err != nil {
		f.fg[url] = GitType(-1)
		return GitType(-1), fmt.Errorf("error fetching fingerPrint instance: %s", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		f.fg[url] = GitType(-1)
		return GitType(-1), fmt.Errorf("error reading fingerPrint response: %s", err)
	}

	if strings.Contains(string(body), "<a href=\"https://about.gitlab.com\">About GitLab</a>") {
		f.fg[url] = Gitlab
		return Gitlab, nil
	}
	if strings.Contains(strings.ToLower(string(body)), "gitea") || strings.Contains(strings.ToLower(string(body)), "forgejo") {
		f.fg[url] = Gitea
		return Gitea, nil
	}
	if strings.Contains(string(body), "<link rel=\"dns-prefetch\" href=\"https://github.githubassets.com\">") {
		f.fg[url] = Github
		return Github, nil
	}
	f.fg[url] = GitType(-1)
	return GitType(-1), fmt.Errorf("no corresponding git instance found: %s", url)
}
