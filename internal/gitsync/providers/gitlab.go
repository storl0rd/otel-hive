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

// GitLab implements Provider against the GitLab REST API v4.
// Works with gitlab.com and self-hosted GitLab instances.
type GitLab struct {
	projectID string // URL-encoded "namespace/repo"
	token     string
	apiBase   string
	client    *http.Client
}

// NewGitLab creates a GitLab provider.
// repoURL must be https://gitlab.com/namespace/repo (or self-hosted equivalent).
func NewGitLab(repoURL, token string) *GitLab {
	projectID, apiBase := parseGitLabURL(repoURL)
	return &GitLab{
		projectID: projectID,
		token:     token,
		apiBase:   apiBase,
		client:    &http.Client{},
	}
}

func parseGitLabURL(raw string) (projectID, apiBase string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "https://gitlab.com/api/v4"
	}
	// Everything after the host is the project path (strip leading slash and .git)
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	projectID = url.PathEscape(path)
	apiBase = fmt.Sprintf("%s://%s/api/v4", u.Scheme, u.Host)
	return
}

func (g *GitLab) newRequest(ctx context.Context, method, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, g.apiBase+endpoint, nil)
	if err != nil {
		return nil, err
	}
	if g.token != "" {
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}
	return req, nil
}

func (g *GitLab) do(ctx context.Context, endpoint string) (*http.Response, error) {
	req, err := g.newRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("gitlab API %s: HTTP %d", endpoint, resp.StatusCode)
	}
	return resp, nil
}

// LatestCommitSHA returns the HEAD commit SHA on branch.
func (g *GitLab) LatestCommitSHA(ctx context.Context, branch string) (string, error) {
	endpoint := fmt.Sprintf("/projects/%s/repository/commits/%s",
		g.projectID, url.PathEscape(branch))
	resp, err := g.do(ctx, endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.ID, nil
}

// ListFiles returns all YAML files under rootPath using the recursive tree API.
func (g *GitLab) ListFiles(ctx context.Context, branch, rootPath string) ([]FileEntry, error) {
	var entries []FileEntry
	page := 1

	prefix := strings.TrimSuffix(rootPath, "/") + "/"

	for {
		endpoint := fmt.Sprintf(
			"/projects/%s/repository/tree?ref=%s&path=%s&recursive=true&per_page=100&page=%d",
			g.projectID, url.QueryEscape(branch), url.QueryEscape(rootPath), page,
		)
		resp, err := g.do(ctx, endpoint)
		if err != nil {
			return nil, err
		}

		var items []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			ID   string `json:"id"` // blob SHA in GitLab
		}
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		for _, item := range items {
			if item.Type != "blob" {
				continue
			}
			if !strings.HasPrefix(item.Path, prefix) && item.Path != rootPath {
				continue
			}
			if !strings.HasSuffix(item.Path, ".yaml") && !strings.HasSuffix(item.Path, ".yml") {
				continue
			}
			entries = append(entries, FileEntry{Path: item.Path, SHA: item.ID})
		}

		if len(items) < 100 {
			break
		}
		page++
	}
	return entries, nil
}

// FetchFile fetches a file's raw content by path.
// GitLab doesn't support blob-by-SHA for raw content; use path + branch.
func (g *GitLab) FetchFile(ctx context.Context, filePath, _ string) ([]byte, error) {
	// We need the branch — use the blob content endpoint which includes encoding.
	// The caller passes the path from ListFiles; we fetch from the default branch.
	// GitLab raw file API: GET /projects/:id/repository/files/:filepath/raw?ref=HEAD
	endpoint := fmt.Sprintf("/projects/%s/repository/files/%s/raw?ref=HEAD",
		g.projectID, url.PathEscape(filePath),
	)
	resp, err := g.do(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var buf []byte
	buf = make([]byte, 0, 4096)
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

// fetchFileByRef fetches a file using the files API and base64-decodes the response.
func (g *GitLab) fetchFileByRef(ctx context.Context, filePath, ref string) ([]byte, error) {
	endpoint := fmt.Sprintf("/projects/%s/repository/files/%s?ref=%s",
		g.projectID, url.PathEscape(filePath), url.QueryEscape(ref),
	)
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
