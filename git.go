package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var versionRe = regexp.MustCompile(`(?i)\bv?(\d+(?:\.\d+){1,}(-.*)?)\b`)

type Git struct {
	Interactive bool
	Path        string
}

func NewGit(interactive bool, path string) *Git {
	return &Git{
		Interactive: interactive,
		Path:        path,
	}
}

func (g *Git) Add(config Config) error {
	args := []string{"-C", g.Path, "add"}
	if config.Interactive {
		args = append(args, "-p")
	} else {
		args = append(args, ".")
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (g *Git) GetCommitFiles() ([]string, error) {
	cmd := exec.Command("git", "-C", g.Path, "diff", "--cached", "--name-only")
	out := bytes.NewBuffer(nil)
	cmd.Stdout = out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	files := strings.Split(out.String(), "\n")
	var trimmedFiles []string
	for _, file := range files {
		trimmedFile := strings.TrimSuffix(file, ".rb")
		if trimmedFile == "" {
			continue
		}
		trimmedFiles = append(trimmedFiles, trimmedFile)
	}
	return trimmedFiles, nil
}

func (g *Git) CommitFiles() error {
	files, err := g.GetCommitFiles()
	if err != nil {
		return fmt.Errorf("get getting commit files: %w", err)
	}
	cmd := exec.Command("git", "-C", g.Path, "commit", "-m", fmt.Sprintf("Bump %s", strings.Join(files, "/")))
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("committing files: %w", err)
	}
	return nil
}

func (g *Git) Push() error {
	cmd := exec.Command("git", "-C", g.Path, "push")
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func ExtractProjectPath(u string) (string, error) {
	uu, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("error parsing url: %s", err)
	}
	return strings.Trim(uu.Path, "/"), nil
}

func GetOwnerRepo(u string) (string, string, error) {
	projectPath, err := ExtractProjectPath(u)
	if err != nil {
		return "", "", fmt.Errorf("error extracting project path: %w", err)
	}
	parts := strings.Split(projectPath, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid project path: %s", projectPath)
	}
	owner := parts[0]
	repo := parts[1]
	return owner, repo, nil
}

// IsInsideGitWorkTree returns true if dir is inside a Git working tree.
// It shells out to `git rev-parse --is-inside-work-tree`.
func IsInsideGitWorkTree(dir string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		// Any non-zero exit means “not a git repo” or Git not installed
		return false, nil
	}
	result := strings.TrimSpace(out.String())
	return result == "true", nil
}

func ExtractVersion(s string) (string, bool) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true // captured version without the leading v/V
}
