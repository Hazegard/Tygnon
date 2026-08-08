package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadAssetSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("release-archive-bytes"))
	}))
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	body, err := downloadAsset(req)
	if err != nil {
		t.Fatalf("downloadAsset: %v", err)
	}
	if string(body) != "release-archive-bytes" {
		t.Fatalf("body = %q, want %q", body, "release-archive-bytes")
	}
}

func TestDownloadAssetNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	if _, err := downloadAsset(req); err == nil {
		t.Fatalf("expected an error for a non-200 status")
	}
}

func TestDownloadAssetNetworkError(t *testing.T) {
	// An immediately-closed server gives a connection refused, no real network.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	if _, err := downloadAsset(req); err == nil {
		t.Fatalf("expected a network error against a closed server")
	}
}

func TestDownloadAssetWithLimitAtBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", 10)))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	body, err := downloadAssetWithLimit(req, 10)
	if err != nil {
		t.Fatalf("expected a body exactly at the limit to succeed: %v", err)
	}
	if len(body) != 10 {
		t.Fatalf("len(body) = %d, want 10", len(body))
	}
}

// An oversized body must error, not truncate (which would be hashed wrong).
func TestDownloadAssetWithLimitTruncationIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", 11)))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	body, err := downloadAssetWithLimit(req, 10)
	if err == nil {
		t.Fatalf("expected an error for a body exceeding the limit, got body of length %d", len(body))
	}
	if body != nil {
		t.Fatalf("expected no body to be returned on truncation error, got %d bytes", len(body))
	}
}
