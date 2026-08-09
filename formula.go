package main

import (
	"fmt"
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
	BottleURL   string
	SHA256      string
	Version     string
	Homepage    string
	License     string
	File        string
	Description string
	Instance    GitType
	Bottles     []Bottle
	Directory   string
	Revision    string
}
type Bottle struct {
	Url     string
	Sha256  string
	Options string
	target  string
}

func (b *Bottle) NewUrl(_ string, name string, version string) string {
	return strings.ReplaceAll(fmt.Sprintf("%s/%s-%s.%s.bottle.tar.gz", b.Url, name, version, b.target), "#{version}", version)
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

func (f *FormulaInfo) GetLocalFile() string {
	return TrimDir(f.Directory, f.File)
}

func (f *FormulaInfo) GetName() string {
	return strings.TrimSuffix(f.GetLocalFile(), ".rb")
}

func (f *FormulaInfo) ParseBottles(formula string) {
	startBottleRe := regexp.MustCompile(`bottle\s+do`)
	endBottleRe := regexp.MustCompile(`\s+end`)
	bottleRe := regexp.MustCompile(`sha256(?P<options>\s+.*,)?\s+(?P<target>.*):\s+"(?P<sha256>[a-fA-F0-9]{64})"`)
	inBottle := false
	for _, line := range strings.Split(formula, "\n") {
		if startBottleRe.MatchString(line) {
			inBottle = true
			continue
		}
		if endBottleRe.MatchString(line) {
			inBottle = false
		}
		if inBottle && bottleRe.MatchString(line) {

			match := bottleRe.FindStringSubmatch(line)
			bottle := Bottle{}
			for i, name := range bottleRe.SubexpNames() {
				if i == 0 {
					continue
				}
				if name == "target" {
					bottle.target = match[i]
				}
				if name == "sha256" {
					bottle.Sha256 = match[i]
				}
				if name == "options" {
					bottle.Options = match[i]
				}
			}
			f.Bottles = append(f.Bottles, bottle)
		}
	}
}

// ParseFormula parses a Homebrew formula (as a string)
// and extracts information such as URL, SHA256, version, etc.
func ParseFormula(formulaPath string, dir string) (FormulaInfo, error) {
	info := FormulaInfo{
		File:      formulaPath,
		Directory: dir,
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
	bottleUrlRe := regexp.MustCompile(`root_url\s+"([^"]+)"`)
	// revision is a bare integer (e.g. `revision 2`), not quoted.
	revisionRe := regexp.MustCompile(`(?m)^\s*revision\s+(\d+)\s*$`)

	// Extract URL.
	if match := urlRe.FindStringSubmatch(formula); len(match) > 1 {
		info.URL = strings.TrimSpace(match[1])
	}
	// Extract bottle URL.
	if match := bottleUrlRe.FindStringSubmatch(formula); len(match) > 1 {
		info.BottleURL = strings.TrimSpace(match[1])
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

	// Extract the revision.
	if match := revisionRe.FindStringSubmatch(formula); len(match) > 1 {
		info.Revision = strings.TrimSpace(match[1])
	}

	instance, err := info.gitInstance()
	if err != nil {
		return FormulaInfo{}, fmt.Errorf("error getting git instance: %s", err)
	}
	info.Instance = instance

	info.ParseBottles(formula)
	for i := range info.Bottles {
		info.Bottles[i].Url = info.BottleURL
	}

	return info, nil
}

// rubyStringEscaper escapes a value (desc/version come from the remote host)
// for a double-quoted Ruby string so it can't break out or interpolate.
// Only `#{`, `#@`, `#$` start interpolation, so a bare `#` is left alone.
var rubyStringEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"#{", `\#{`,
	"#@", `\#@`,
	"#$", `\#$`,
	"\n", `\n`,
	"\r", `\r`,
)

func escapeRubyString(s string) string {
	return rubyStringEscaper.Replace(s)
}

// escapeReplacement escapes a literal value for use inside a
// regexp.ReplaceAllString replacement, where an unescaped `$` would be treated
// as a capture-group reference. Untrusted values (version/desc) may contain `$`
// (e.g. a `$5` in a description), which would otherwise be silently dropped.
func escapeReplacement(s string) string {
	return strings.ReplaceAll(s, `$`, `$$`)
}

// Update updates the given Homebrew formula string with new version and sha256 values.
// It reports whether the file content actually changed, so callers can skip
// unchanged formulas (e.g. under -force, where the same version is re-written).
func (f *FormulaInfo) Update() (bool, error) {
	content, err := ReadFile(f.File)
	if err != nil {
		return false, fmt.Errorf("error reading formula file: %s", err)
	}
	original := content

	reVersion := regexp.MustCompile(`(?m)^(\s*version\s+")([^"]+)(".*)$`)
	// Replace the current version with the new version.
	content = reVersion.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", escapeReplacement(escapeRubyString(f.Version))))

	// Regular expression to match the sha256 line.
	reSHA256 := regexp.MustCompile(`(?m)^(\s*sha256\s+")([^"]+)(".*)$`)
	// Replace the current sha256 with the new sha256.
	content = reSHA256.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", f.SHA256))

	// Regular expression to match the desc line.
	reDescription := regexp.MustCompile(`(?m)^(\s*desc\s+")([^"]+)(".*)$`)
	// Replace the current description with the new description.
	content = reDescription.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", escapeReplacement(escapeRubyString(f.Description))))

	for _, bottle := range f.Bottles {
		reBottle := regexp.MustCompile(fmt.Sprintf("(sha256.*\\s+%s:.*\")[a-fA-F0-9]{64}\"", bottle.target))
		content = reBottle.ReplaceAllString(content, fmt.Sprintf("${1}%s\"", bottle.Sha256))
	}

	// revision is a bare integer, so set it in place or drop the whole line.
	reRevisionSet := regexp.MustCompile(`(?m)^(\s*revision\s+)\d+(\s*)$`)
	reRevisionLine := regexp.MustCompile(`(?m)^\s*revision\s+\d+\s*\r?\n`)
	if f.Revision != "" {
		content = reRevisionSet.ReplaceAllString(content, fmt.Sprintf("${1}%s${2}", f.Revision))
	} else {
		content = reRevisionLine.ReplaceAllString(content, "")
	}

	if content == original {
		return false, nil
	}
	if err := WriteFile(f.File, content); err != nil {
		return false, err
	}
	return true, nil
}

// GetInstance returns the forge hostname, used to pick the client and token.
// Derived from Homepage since every API call resolves owner/repo from it (the
// download URL may point elsewhere, e.g. a CDN mirror).
func (f *FormulaInfo) GetInstance() (string, error) {
	u, err := url.Parse(f.Homepage)
	if err != nil {
		return "", fmt.Errorf("error parsing formula homepage: %s", err)
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
		return fingerprinter.fingerPrintInstance(u)
	}
}

func (f *FormulaInfo) IsFollowingBranch() (bool, error) {
	u, err := url.Parse(f.URL)
	if err != nil {
		return false, fmt.Errorf("error parsing formula URL: %s", err)
	}
	switch f.Instance {
	case Github:
		re := regexp.MustCompile(`/archive/refs/heads/.*\.zip`)
		//if strings.HasSuffix(u.Path, "/archive/refs/heads/main.zip") || strings.HasSuffix(u.Path, "/archive/refs/heads/master.zip") {
		if re.MatchString(u.Path) {
			return true, nil
		}
	case Gitlab:
		re := regexp.MustCompile(`/archive/.*\.(zip|tar\.gz|tar|tar\.bz2)`)
		if re.MatchString(u.Path) {
			return true, nil
		}
	case Gitea:
		re := regexp.MustCompile(`/archive/.*.zip`)
		if re.MatchString(u.Path) {
			return true, nil
		}
	}
	return false, nil
}
