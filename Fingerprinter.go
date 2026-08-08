package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func NewFingerPrinter() Fingerprinter {
	fg := new(Fingerprinter)
	fg.fg = make(map[string]GitType)
	return *fg
}

var fingerprinter = NewFingerPrinter()

type Fingerprinter struct {
	fg map[string]GitType
}

func (f *Fingerprinter) fingerPrintInstance(host string) (GitType, error) {
	return f.fingerPrintAt(host, apiClient, fmt.Sprintf("https://%s", host))
}

// fingerPrintAt is fingerPrintInstance with the client and URL as params so
// tests can point it at a mock server.
func (f *Fingerprinter) fingerPrintAt(cacheKey string, client *http.Client, targetURL string) (GitType, error) {
	if v, ok := f.fg[cacheKey]; ok {
		return v, nil
	}
	res, err := client.Get(targetURL)
	if err != nil {
		f.fg[cacheKey] = GitType(-1)
		return GitType(-1), fmt.Errorf("error fetching fingerPrint instance: %s", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxFingerprintBodySize))
	if err != nil {
		f.fg[cacheKey] = GitType(-1)
		return GitType(-1), fmt.Errorf("error reading fingerPrint response: %s", err)
	}

	if strings.Contains(string(body), "<a href=\"https://about.gitlab.com\">About GitLab</a>") {
		f.fg[cacheKey] = Gitlab
		return Gitlab, nil
	}
	if strings.Contains(strings.ToLower(string(body)), "gitea") || strings.Contains(strings.ToLower(string(body)), "forgejo") {
		f.fg[cacheKey] = Gitea
		return Gitea, nil
	}
	if strings.Contains(string(body), "<link rel=\"dns-prefetch\" href=\"https://github.githubassets.com\">") {
		f.fg[cacheKey] = Github
		return Github, nil
	}
	f.fg[cacheKey] = GitType(-1)
	return GitType(-1), fmt.Errorf("no corresponding git instance found: %s", cacheKey)
}
