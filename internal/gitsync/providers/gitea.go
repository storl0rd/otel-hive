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

// Gitea implements Provider against the Gitea REST API.
// Gitea's API is largely GitHub-compatible, but the base URL differs.
type Gitea struct {
	owner   string
	repo    string
	token   string
	apiBase string
	client  *http.Client
}

// NewGitea creates a Gitea provider.
// repoURL must be https://gitea.example.com/owner/repo
func NewGitea(repoURL, token string) *Gitea {
	owner, repo, apiBase := parseGiteaURL(repoURL)
	return &Gitea{
		owner:   owner,
		repo:    repo,
		token:   token,
		apiBase: apiBase,
		client:  &http.Client{},
	}
}

func parseGiteaURL(raw string) (owner, repo, apiBase string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		owner = parts[0]
		repo = strings.TrimSuffix(parts[1], ".git")
	}
	apiBase = fmt.Sprintf("%s://%s/api/v1", u.Scheme, u.Host)
	return
}

func (g *Gitea) newRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+endpoint, nil)
	if err != nil {
		return nil, err
	}
	if g.token != "" {
		req.Header.Set("Authorization", "token "+g.token)
	}
	return req, nil
}

func (g *Gitea) do(ctx context.Context, endpoint string) (*http.Response, error) {
	req, err := g.newRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("gitea API %s: HTTP %d", endpoint, resp.StatusCode)
	}
	return resp, nil
}

// LatestCommitSHA returns the HEAD commit SHA on branch.
func (g *Gitea) LatestCommitSHA(ctx context.Context, branch string) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/branches/%s",
		url.PathEscape(g.owner), url.PathEscape(g.repo), url.PathEscape(branch))
	resp, err := g.do(ctx, endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Commit.ID, nil
}

// ListFiles returns all YAML files under rootPath using the git tree API.
func (g *Gitea) ListFiles(ctx context.Context, branch, rootPath string) ([]FileEntry, error) {
	// Gitea: GET /repos/{owner}/{repo}/git/trees/{sha}?recursive=1
	// First, get the commit SHA for the branch
	sha, err := g.LatestCommitSHA(ctx, branch)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1",
		url.PathEscape(g.owner), url.PathEscape(g.repo), sha)
	resp, err := g.do(ctx, endpoint)
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

// FetchFile fetches file content by blob SHA.
func (g *Gitea) FetchFile(ctx context.Context, filePath, sha string) ([]byte, error) {
	// Gitea supports raw file content via /raw endpoint
	endpoint := fmt.Sprintf("/repos/%s/%s/raw/%s",
		url.PathEscape(g.owner), url.PathEscape(g.repo), filePath)
	resp, err := g.do(ctx, endpoint)
	if err != nil {
		// Fall back to contents API
		return g.fetchViaContents(ctx, filePath)
	}
	defer resp.Body.Close()

	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

func (g *Gitea) fetchViaContents(ctx context.Context, filePath string) ([]byte, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/contents/%s",
		url.PathEscape(g.owner), url.PathEscape(g.repo), filePath)
	resp, err := g.do(ctx, endpoint)
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
		return nil, fmt.Errorf("unexpected encoding: %s", body.Encoding)
	}
	clean := strings.ReplaceAll(body.Content, "\n", "")
	return base64.StdEncoding.DecodeString(clean)
}
