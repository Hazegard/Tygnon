package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type GitType int

const (
	Gitlab GitType = iota
	Github GitType = iota
	Gitea  GitType = iota
)

// FormulaInfo holds the parsed information from a Homebrew formula.
type FormulaInfo struct {
	URL         string
	SHA256      string
	Version     string
	Homepage    string
	License     string
	File        string
	Description string
	Instance    GitType
}

func (f *FormulaInfo) GetNewVersionURL(version string) string {
	u := strings.ReplaceAll(f.URL, "#{version}", version)
	u = strings.ReplaceAll(u, "/vv", "/v")
	u = strings.ReplaceAll(u, "-vv", "-v")
	return u
}

func (f *FormulaInfo) GetVersionURL() string {
	return strings.ReplaceAll(f.URL, "#{version}", f.Version)
}

func (f *FormulaInfo) GetLocalFile(config Config) string {
	return strings.TrimPrefix(f.File, config.Path)
}

// ParseFormula parses a Homebrew formula (as a string)
// and extracts information such as URL, SHA256, version, etc.
func ParseFormula(formulaPath string) (FormulaInfo, error) {
	info := FormulaInfo{
		File: formulaPath,
	}
	formula, err := ReadFile(formulaPath)
	if err != nil {
		return FormulaInfo{}, fmt.Errorf("error reading formula file: %s", err)
	}

	// Define regular expression patterns for each field.
	urlRe := regexp.MustCompile(`url\s+"([^"]+)"`)
	sha256Re := regexp.MustCompile(`sha256\s+"([^"]+)"`)
	versionRe := regexp.MustCompile(`version\s+"([^"]+)"`)
	homepageRe := regexp.MustCompile(`homepage\s+"([^"]+)"`)
	descRe := regexp.MustCompile(`desc\s+"([^"]+)"`)

	// Extract URL.
	if match := urlRe.FindStringSubmatch(formula); len(match) > 1 {
		info.URL = strings.TrimSpace(match[1])
	}

	// Extract SHA256.
	if match := sha256Re.FindStringSubmatch(formula); len(match) > 1 {
		info.SHA256 = strings.TrimSpace(match[1])
	}

	// Extract version.
	if match := versionRe.FindStringSubmatch(formula); len(match) > 1 {
		info.Version = strings.TrimSpace(match[1])
	}

	// Extract homepage.
	if match := homepageRe.FindStringSubmatch(formula); len(match) > 1 {
		info.Homepage = strings.TrimSpace(match[1])
	}

	// Extract description.
	if match := descRe.FindStringSubmatch(formula); len(match) > 1 {
		info.Description = strings.TrimSpace(match[1])
	}
	instance, err := info.gitInstance()
	if err != nil {
		return FormulaInfo{}, fmt.Errorf("error getting git instance: %s", err)
	}
	info.Instance = instance

	return info, nil
}

// Update updates the given Homebrew formula string with new version and sha256 values.
// It returns the updated formula as a string.
func (f *FormulaInfo) Update() error {
	content, err := ReadFile(f.File)
	if err != nil {
		return fmt.Errorf("error reading formula file: %s", err)
	}

	reVersion := regexp.MustCompile(`(?m)^(\s*version\s+")([^"]+)(".*)$`)
	// Replace the current version with the new version.
	content = reVersion.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", f.Version))

	// Regular expression to match the sha256 line.
	reSHA256 := regexp.MustCompile(`(?m)^(\s*sha256\s+")([^"]+)(".*)$`)
	// Replace the current sha256 with the new sha256.
	content = reSHA256.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", f.SHA256))

	// Regular expression to match the sha256 line.
	reDescription := regexp.MustCompile(`(?m)^(\s*desc\s+")([^"]+)(".*)$`)
	// Replace the current sha256 with the new sha256.
	content = reDescription.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", f.Description))

	return WriteFile(f.File, content)
}

func (f *FormulaInfo) GetInstance() (string, error) {
	u, err := url.Parse(f.URL)
	if err != nil {
		return "", fmt.Errorf("error parsing formula URL: %s", err)
	}
	return u.Hostname(), nil
}

func (f *FormulaInfo) gitInstance() (GitType, error) {
	u, err := f.GetInstance()
	if err != nil {
		return GitType(-1), fmt.Errorf("error parsing git instance url: %s", err)
	}
	switch u {
	case "github.com":
		return Github, nil
	case "gitlab.com":
		return Gitlab, nil
	default:
		return f.fingerPrintInstance()
	}
}

func (f *FormulaInfo) fingerPrintInstance() (GitType, error) {
	u, err := f.GetInstance()
	if err != nil {
		return GitType(-1), nil
	}
	res, err := http.Get(fmt.Sprintf("https://%s", u))
	if err != nil {
		return GitType(-1), fmt.Errorf("error fetching fingerPrint instance: %s", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return GitType(-1), fmt.Errorf("error reading fingerPrint response: %s", err)
	}

	if strings.Contains(string(body), "<a href=\"https://about.gitlab.com\">About GitLab</a>") {
		return Gitlab, nil
	}
	if strings.Contains(strings.ToLower(string(body)), "gitea") || strings.Contains(strings.ToLower(string(body)), "forgejo") {
		return Gitea, nil
	}
	if strings.Contains(string(body), "<link rel=\"dns-prefetch\" href=\"https://github.githubassets.com\">") {
		return Github, nil
	}
	return GitType(-1), fmt.Errorf("no corresponding git instance found: %s", u)
}

func (f *FormulaInfo) IsOnMaster() (bool, error) {
	u, err := url.Parse(f.URL)
	if err != nil {
		return false, fmt.Errorf("error parsing formula URL: %s", err)
	}
	switch f.Instance {
	case Github:
		if strings.HasSuffix(u.Path, "/archive/refs/heads/main.zip") {
			return true, nil
		}
	case Gitlab:
		re := regexp.MustCompile(`/archive/master/.*\.zip`)
		if re.MatchString(u.Path) {
			return true, nil
		}
	case Gitea:
		if strings.HasSuffix(u.Path, "/archive/main.zip") {
			return true, nil
		}
	}
	return false, nil
}
