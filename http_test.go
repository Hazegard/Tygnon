package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The asset transport must keep DefaultTransport's proxy support while adding
// the download timeouts.
func TestAssetTransportPreservesProxy(t *testing.T) {
	tr := newAssetTransport()

	if tr.Proxy == nil {
		t.Fatal("asset transport has a nil Proxy func: HTTP(S)_PROXY/NO_PROXY would be ignored")
	}

	// Check the Proxy func routes through a proxy (set one explicitly, since
	// ProxyFromEnvironment caches the env on first use).
	proxyURL, _ := url.Parse("http://proxy.internal:3128")
	tr.Proxy = http.ProxyURL(proxyURL)
	req, _ := http.NewRequest("GET", "https://example.com/asset.tar.gz", nil)
	got, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy func returned an error: %v", err)
	}
	if got == nil || got.Host != "proxy.internal:3128" {
		t.Fatalf("request was not routed through the configured proxy, got %v", got)
	}

	// The download-specific timeouts must be applied on top of the clone.
	if tr.ResponseHeaderTimeout != assetHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", tr.ResponseHeaderTimeout, assetHeaderTimeout)
	}
	if tr.TLSHandshakeTimeout != assetDialTimeout {
		t.Errorf("TLSHandshakeTimeout = %v, want %v", tr.TLSHandshakeTimeout, assetDialTimeout)
	}
	if tr.DialContext == nil {
		t.Error("DialContext should be set")
	}
}

// Mutating the asset transport must not touch http.DefaultTransport.
func TestAssetTransportIsIndependentClone(t *testing.T) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Skip("http.DefaultTransport is not *http.Transport on this build")
	}
	before := base.ResponseHeaderTimeout

	tr := newAssetTransport()
	tr.ResponseHeaderTimeout = 999 * time.Second

	if base.ResponseHeaderTimeout != before {
		t.Fatalf("mutating the asset transport leaked into http.DefaultTransport (%v != %v)", base.ResponseHeaderTimeout, before)
	}
}

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
