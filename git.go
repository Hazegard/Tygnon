package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

type Git struct {
	Config Config
}

func NewGit(config Config) *Git {
	return &Git{Config: config}
}

func (g *Git) AddP() error {
	cmd := exec.Command("git", "-C", g.Config.Path, "add", "-p")
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
	cmd := exec.Command("git", "-C", g.Config.Path, "diff", "--cached", "--name-only")
	out := bytes.NewBuffer(nil)
	cmd.Stdout = out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	files := strings.Split(out.String(), "\n")
	var trimmedFiles []string
	for _, file := range files {
		trimmedFiles = append(trimmedFiles, strings.TrimSuffix(file, ".rb"))
	}
	return trimmedFiles, nil
}

func (g *Git) CommitFiles() error {
	files, err := g.GetCommitFiles()
	if err != nil {
		return fmt.Errorf("get getting commit files: %w", err)
	}
	cmd := exec.Command("git", "-C", g.Config.Path, "commit", "-m", fmt.Sprintf("Bump %s", strings.Join(files, "/")))
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("committing files: %w", err)
	}
	return nil
}

func (g *Git) Push() error {
	cmd := exec.Command("git", "-C", g.Config.Path, "push")
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
