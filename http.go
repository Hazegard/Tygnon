package main

import (
	"net/http"
	"time"
)

// httpTimeout bounds every outbound request tygnon makes (fingerprinting,
// release/tag lookups, asset downloads). Without it, a slow or malicious
// remote host can hang the whole run indefinitely.
const httpTimeout = 30 * time.Second

// httpClient is shared by every git-hosting API client and the fingerprinter
// so all outbound requests are subject to the same timeout.
var httpClient = &http.Client{
	Timeout: httpTimeout,
}

// Only read the head of a homepage when fingerprinting the git instance.
const maxFingerprintBodySize = 1 << 20 // 1 MiB

// Cap the buffered asset size to bound memory usage.
const maxAssetBodySize = 2 << 30 // 2 GiB
