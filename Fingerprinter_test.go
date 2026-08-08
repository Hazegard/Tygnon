package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	gitlabSignatureHTML  = `<html><body><a href="https://about.gitlab.com">About GitLab</a></body></html>`
	giteaSignatureHTML   = `<html><body>Powered by Gitea</body></html>`
	forgejoSignatureHTML = `<html><body>Powered by Forgejo</body></html>`
	githubSignatureHTML  = `<html><head><link rel="dns-prefetch" href="https://github.githubassets.com"></head></html>`
	unknownSignatureHTML = `<html><body>just some random homepage</body></html>`
)

func fingerprintServer(t *testing.T, body string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	return server, &calls
}

func TestFingerPrintAtDetectsForges(t *testing.T) {
	cases := []struct {
		name string
		body string
		want GitType
	}{
		{"gitlab", gitlabSignatureHTML, Gitlab},
		{"gitea", giteaSignatureHTML, Gitea},
		{"forgejo (also detected as gitea)", forgejoSignatureHTML, Gitea},
		{"github", githubSignatureHTML, Github},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server, _ := fingerprintServer(t, c.body)
			defer server.Close()

			fp := NewFingerPrinter()
			got, err := fp.fingerPrintAt("key", server.Client(), server.URL)
			if err != nil {
				t.Fatalf("fingerPrintAt: %v", err)
			}
			if got != c.want {
				t.Fatalf("fingerPrintAt() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestFingerPrintAtUnknownForge(t *testing.T) {
	server, _ := fingerprintServer(t, unknownSignatureHTML)
	defer server.Close()

	fp := NewFingerPrinter()
	_, err := fp.fingerPrintAt("key", server.Client(), server.URL)
	if err == nil {
		t.Fatalf("expected an error for an unrecognized homepage")
	}
	if fp.fg["key"] != GitType(-1) {
		t.Errorf("expected the negative result to be cached, got %v", fp.fg["key"])
	}
}

func TestFingerPrintAtCachesResult(t *testing.T) {
	server, calls := fingerprintServer(t, githubSignatureHTML)
	defer server.Close()

	fp := NewFingerPrinter()
	if _, err := fp.fingerPrintAt("key", server.Client(), server.URL); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := fp.fingerPrintAt("key", server.Client(), server.URL); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("handler was called %d times, want 1 (second lookup should hit the cache)", got)
	}
}

func TestFingerPrintAtNetworkError(t *testing.T) {
	server, _ := fingerprintServer(t, githubSignatureHTML)
	server.Close() // closed immediately: connection refused

	fp := NewFingerPrinter()
	_, err := fp.fingerPrintAt("key", server.Client(), server.URL)
	if err == nil {
		t.Fatalf("expected a network error against a closed server")
	}
	if fp.fg["key"] != GitType(-1) {
		t.Errorf("expected the failure to be cached as -1, got %v", fp.fg["key"])
	}
}

func TestFingerPrintAtSignatureBeyondCapIsNotDetected(t *testing.T) {
	// A signature past the read cap is never seen (we only sniff the head).
	padding := strings.Repeat("x", maxFingerprintBodySize+1024)
	body := padding + githubSignatureHTML
	server, _ := fingerprintServer(t, body)
	defer server.Close()

	fp := NewFingerPrinter()
	_, err := fp.fingerPrintAt("key", server.Client(), server.URL)
	if err == nil {
		t.Fatalf("expected the signature past the read cap to go undetected")
	}
}

func TestFingerPrintAtSignatureWithinCapIsDetectedDespiteLargeBody(t *testing.T) {
	// A large body after the signature shouldn't prevent detection.
	body := githubSignatureHTML + strings.Repeat("y", 4096)
	server, _ := fingerprintServer(t, body)
	defer server.Close()

	fp := NewFingerPrinter()
	got, err := fp.fingerPrintAt("key", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fingerPrintAt: %v", err)
	}
	if got != Github {
		t.Fatalf("fingerPrintAt() = %v, want Github", got)
	}
}
