package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTP implements Provider for a generic raw HTTPS URL.
// This is useful for single-file raw URLs (e.g. GitHub raw content, S3, etc.)
// It doesn't support ListFiles or LatestCommitSHA in a meaningful way —
// the URL IS the file and is always fetched on every sync.
type HTTP struct {
	rawURL string
	token  string
	client *http.Client
}

// NewHTTP creates an HTTP provider. rawURL should be the direct URL to a single
// YAML file, e.g. https://raw.githubusercontent.com/org/repo/main/collector.yaml
func NewHTTP(rawURL, token string) *HTTP {
	return &HTTP{
		rawURL: rawURL,
		token:  token,
		client: &http.Client{},
	}
}

// LatestCommitSHA is not supported for generic HTTP — returns a fixed sentinel.
// The service layer treats any non-empty SHA change as "changed", so returning ""
// means "unknown, always fetch".
func (h *HTTP) LatestCommitSHA(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ListFiles returns a single virtual FileEntry representing the raw URL.
func (h *HTTP) ListFiles(_ context.Context, _, rootPath string) ([]FileEntry, error) {
	// Derive a file path from the URL's last path segment.
	parts := strings.Split(strings.TrimRight(h.rawURL, "/"), "/")
	name := parts[len(parts)-1]
	if name == "" {
		name = "collector.yaml"
	}
	// Use rootPath as the directory prefix so the matcher can find it.
	path := strings.TrimSuffix(rootPath, "/") + "/" + name
	return []FileEntry{{Path: path, SHA: ""}}, nil
}

// FetchFile fetches the raw content from the URL.
// path and sha are ignored; the configured rawURL is always used.
func (h *HTTP) FetchFile(ctx context.Context, _, _ string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.rawURL, nil)
	if err != nil {
		return nil, err
	}
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP GET %s: status %d", h.rawURL, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
