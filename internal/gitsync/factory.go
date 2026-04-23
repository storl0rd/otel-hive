package gitsync

import (
	"fmt"

	"github.com/storl0rd/otel-hive/internal/gitsync/providers"
)

// NewProvider constructs a providers.Provider from a GitSource configuration.
func NewProvider(gs *GitSource) (providers.Provider, error) {
	switch gs.ProviderType {
	case ProviderGitHub:
		return providers.NewGitHub(gs.RepoURL, gs.Token), nil
	case ProviderGitLab:
		return providers.NewGitLab(gs.RepoURL, gs.Token), nil
	case ProviderGitea:
		return providers.NewGitea(gs.RepoURL, gs.Token), nil
	case ProviderHTTP:
		return providers.NewHTTP(gs.RepoURL, gs.Token), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %q; must be one of: github, gitlab, gitea, http", gs.ProviderType)
	}
}
