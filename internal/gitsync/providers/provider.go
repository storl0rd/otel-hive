// Package providers contains implementations of the git hosting provider interface.
package providers

import (
	"context"
)

// FileEntry is a file returned by a provider tree listing.
type FileEntry struct {
	// Path relative to the repo root.
	Path string
	// SHA of the blob — used for per-file change detection and fetching.
	SHA string
}

// Provider fetches file trees and content from a git hosting backend.
// Implementations must be safe for concurrent use.
type Provider interface {
	// ListFiles returns all YAML files under rootPath on the given branch.
	ListFiles(ctx context.Context, branch, rootPath string) ([]FileEntry, error)

	// FetchFile returns the raw content of a file at the given path + SHA.
	FetchFile(ctx context.Context, path, sha string) ([]byte, error)

	// LatestCommitSHA returns the SHA of the HEAD commit on branch.
	LatestCommitSHA(ctx context.Context, branch string) (string, error)
}
