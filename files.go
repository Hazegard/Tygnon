package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func ReadFile(path string) (string, error) {
	// Read the file using os.ReadFile (available in Go 1.16+)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func WriteFile(path string, content string) error {
	// Convert the string content to a byte slice and write it to the file.
	// 0644 is the file permission (read and write for the owner, read for others).
	return os.WriteFile(path, []byte(content), 0o644)
}

func FindBrewFormulaFiles(dir string) ([]string, error) {
	var formulaFiles []string

	// Regex to match a brew formula class definition in Ruby.
	// e.g., "class Foo < Formula"
	re := regexp.MustCompile(`class\s+\S+\s+<\s+Formula`)

	// Walk the directory tree.
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Return the error if one occurs.
			return err
		}

		// Skip directories.
		if info.IsDir() || !strings.HasSuffix(path, ".rb") {
			return nil
		}

		// Read the file content.
		data, err := os.ReadFile(path)
		if err != nil {
			// Skip files that can't be read.
			return nil
		}

		// If the content matches our brew formula pattern, add the file to our list.
		if re.Match(data) {
			formulaFiles = append(formulaFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return formulaFiles, nil
}

func Sha256(data []byte) string {
	// Compute the SHA-256 hash
	hash := sha256.Sum256(data)

	// Convert the hash to a hexadecimal string
	hashHex := hex.EncodeToString(hash[:])
	return hashHex
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// GetContainingDir returns the directory for the given path.
// If path is already a directory, it returns path itself.
// If path is a file, it returns the directory containing that file.
func GetContainingDir(path string) (string, error) {
	// First, get file info
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("could not stat path %q: %w", path, err)
	}

	if info.IsDir() {
		// It's a directory: return as-is (but cleaned/absolute)
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("could not make absolute path %q: %w", path, err)
		}
		return abs, nil
	}

	// It's a file: return its parent directory
	dir := filepath.Dir(path)
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("could not make absolute path %q: %w", dir, err)
	}
	return abs, nil
}
