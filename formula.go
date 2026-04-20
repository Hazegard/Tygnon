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
	Gitlab   GitType = iota
	XmGitlab GitType = iota
	Github   GitType = iota
	Gitea    GitType = iota
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
	info.Instance = info.gitInstance()

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

func (f *FormulaInfo) gitInstance() GitType {
	u, err := f.GetInstance()
	if err != nil {
		return GitType(-1)
	}
	switch u {
	case "github.com":
		return Github
	case "gitlab.com":
		return Gitlab
	case "git.example.fr":
		return XmGitlab
	case "git.hazegard.fr":
		return Gitea
	default:
		return f.fingerPrintInstance()
	}
}

func (f *FormulaInfo) fingerPrintInstance() GitType {
	u, err := f.GetInstance()
	if err != nil {
		return GitType(-1)
	}
	uu, err := url.Parse(u)
	if err != nil {
		return GitType(-1)
	}
	req, err := http.NewRequest("GET", uu.Hostname(), nil)
	if err != nil {
		return GitType(-1)
	}
	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return GitType(-1)
	}

	if strings.Contains(string(body), "<a href=\"https://about.gitlab.com\">About GitLab</a>") {
		return Gitlab
	}
	if strings.Contains(strings.ToLower(string(body)), "gitea") || strings.Contains(strings.ToLower(string(body)), "forgejo") {
		return Gitea
	}
	if strings.Contains(string(body), "<link rel=\"dns-prefetch\" href=\"https://github.githubassets.com\">") {
		return Github
	}
	return GitType(-1)
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
	case Gitlab, XmGitlab:
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
