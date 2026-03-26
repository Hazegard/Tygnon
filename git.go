package main

import (
	"bytes"
	"fmt"
	"github.com/rs/zerolog"
	"os"
	"os/exec"
	"strings"
)

type Git struct {
	l      *zerolog.Logger
	Config Config
}

func NewGit(l *zerolog.Logger, config Config) *Git {
	return &Git{l: l, Config: config}
}

func (g *Git) AddP() error {
	cmd := exec.Command("git", "-C", g.Config.Path, "add", "-p")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	if err != nil {
		g.l.Error().Err(err).Str("Path", g.Config.Path).Msg("git add failed")
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
		g.l.Error().Err(err).Str("Path", g.Config.Path).Msg("git diff failed")
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
	cmd := exec.Command("git", "-C", g.Config.Path, "commit", "-m", strings.Join(files, "/"))
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		g.l.Error().Err(err).Str("Path", g.Config.Path).Msg("git commit failed")
		return err
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
