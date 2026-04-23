package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GitHub implements Provider against the GitHub REST API v3.
// Works with github.com and GitHub Enterprise (pass the enterprise base URL as repoURL).
type GitHub struct {
	owner    string
	repo     string
	token    string
	apiBase  string
	client   *http.Client
}

// NewGitHub creates a GitHub provider.
// repoURL must be https://github.com/owner/repo or https://github.enterprise.example.com/owner/repo
func NewGitHub(repoURL, token string) *GitHub {
	owner, repo, apiBase := parseGitHubURL(repoURL)
	return &GitHub{
		owner:   owner,
		repo:    repo,
		token:   token,
		apiBase: apiBase,
		client:  &http.Client{},
	}
}

func parseGitHubURL(raw string) (owner, repo, apiBase string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "https://api.github.com"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		owner = parts[0]
		repo = strings.TrimSuffix(parts[1], ".git")
	}
	if u.Host == "github.com" {
		apiBase = "https://api.github.com"
	} else {
		// GitHub Enterprise
		apiBase = fmt.Sprintf("%s://%s/api/v3", u.Scheme, u.Host)
	}
	return
}

func (g *GitHub) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return req, nil
}

func (g *GitHub) do(ctx context.Context, method, endpoint string) (*http.Response, error) {
	req, err := g.newRequest(ctx, method, g.apiBase+endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("github API %s: HTTP %d", endpoint, resp.StatusCode)
	}
	return resp, nil
}

// LatestCommitSHA returns the HEAD commit SHA on the given branch.
func (g *GitHub) LatestCommitSHA(ctx context.Context, branch string) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s",
		url.PathEscape(g.owner), url.PathEscape(g.repo), url.PathEscape(branch))
	resp, err := g.do(ctx, http.MethodGet, endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.SHA, nil
}

// ListFiles returns all files under rootPath on the given branch using the recursive tree API.
func (g *GitHub) ListFiles(ctx context.Context, branch, rootPath string) ([]FileEntry, error) {
	// First get the tree SHA for the commit
	commitSHA, err := g.LatestCommitSHA(ctx, branch)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1",
		url.PathEscape(g.owner), url.PathEscape(g.repo), commitSHA)
	resp, err := g.do(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	prefix := strings.TrimSuffix(rootPath, "/") + "/"
	var entries []FileEntry
	for _, item := range body.Tree {
		if item.Type != "blob" {
			continue
		}
		if !strings.HasPrefix(item.Path, prefix) && item.Path != rootPath {
			continue
		}
		if !strings.HasSuffix(item.Path, ".yaml") && !strings.HasSuffix(item.Path, ".yml") {
			continue
		}
		entries = append(entries, FileEntry{Path: item.Path, SHA: item.SHA})
	}
	return entries, nil
}

// FetchFile fetches a blob by its SHA.
func (g *GitHub) FetchFile(ctx context.Context, _ string, sha string) ([]byte, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/git/blobs/%s",
		url.PathEscape(g.owner), url.PathEscape(g.repo), sha)
	resp, err := g.do(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected blob encoding: %s", body.Encoding)
	}
	// GitHub wraps base64 in newlines
	clean := strings.ReplaceAll(body.Content, "\n", "")
	return base64.StdEncoding.DecodeString(clean)
}
