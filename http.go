package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// apiTimeout bounds lightweight API/metadata requests (tags, releases, commits, fingerprinting).
const apiTimeout = 30 * time.Second

// apiClient is used for API/metadata requests by every client and the fingerprinter.
var apiClient = &http.Client{
	Timeout: apiTimeout,
}

// Timeouts for asset downloads. The overall timeout bounds the whole request
// (body included), the others just connection setup and the wait for headers.
const (
	assetDialTimeout    = 10 * time.Second
	assetHeaderTimeout  = 30 * time.Second
	assetOverallTimeout = 10 * time.Minute
)

// assetClient downloads release/bottle archives, which can be large, so it
// avoids http.Client.Timeout (which would bound the body read too).
var assetClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: assetDialTimeout,
		}).DialContext,
		TLSHandshakeTimeout:   assetDialTimeout,
		ResponseHeaderTimeout: assetHeaderTimeout,
	},
}

// Only read the head of a homepage when fingerprinting the git instance.
const maxFingerprintBodySize = 1 << 20 // 1 MiB

// Cap the buffered asset size to bound memory usage.
const maxAssetBodySize = 2 << 30 // 2 GiB

func downloadAsset(req *http.Request) ([]byte, error) {
	return downloadAssetWithLimit(req, maxAssetBodySize)
}

// downloadAssetWithLimit downloads req and errors (instead of truncating) if
// the body is bigger than maxSize, so we never hash a partial download.
func downloadAssetWithLimit(req *http.Request, maxSize int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(req.Context(), assetOverallTimeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := assetClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download asset, status: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}
	if int64(len(body)) > maxSize {
		return nil, fmt.Errorf("asset exceeds maximum allowed size of %d bytes", maxSize)
	}
	return body, nil
}
