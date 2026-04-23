package gitsync

import "time"

// Provider identifies which git hosting backend to use.
type Provider string

const (
	ProviderGitHub  Provider = "github"
	ProviderGitLab  Provider = "gitlab"
	ProviderGitea   Provider = "gitea"
	ProviderHTTP    Provider = "http"
)

// SyncStatus is the outcome of the last sync attempt.
type SyncStatus string

const (
	SyncStatusPending SyncStatus = "pending"
	SyncStatusSuccess SyncStatus = "success"
	SyncStatusFailed  SyncStatus = "failed"
)

// GitSource represents a git repository that is polled for collector configs.
type GitSource struct {
	ID string `json:"id"`

	// Human-readable label (e.g. "production-configs")
	Name string `json:"name"`

	// Full HTTPS URL of the repository, e.g. https://github.com/org/otel-configs
	RepoURL string `json:"repo_url"`

	// Personal access token or app token. Empty for public repos.
	// Stored in plaintext for now; encrypt-at-rest is a future hardening step.
	Token string `json:"token,omitempty"`

	// Branch to track. Defaults to "main".
	Branch string `json:"branch"`

	// Root path inside the repo that contains environment/group subdirs.
	// Defaults to "configs". Must NOT have a trailing slash.
	ConfigRoot string `json:"config_root"`

	// Which git hosting backend to use.
	ProviderType Provider `json:"provider"`

	// How often to poll for changes, in seconds. Defaults to 300 (5 min).
	PollIntervalSeconds int `json:"poll_interval_seconds"`

	// HMAC secret for verifying incoming webhooks. Empty disables webhook auth.
	WebhookSecret string `json:"webhook_secret,omitempty"`

	// State from last sync run.
	LastSyncSHA    string     `json:"last_sync_sha,omitempty"`
	LastSyncAt     *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus SyncStatus `json:"last_sync_status,omitempty"`
	LastSyncError  string     `json:"last_sync_error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SyncResult summarises one sync run.
type SyncResult struct {
	SourceID      string
	FilesChanged  int
	AgentsUpdated int
	Errors        []error
}

// LabelRule describes how a config file path maps to agent label selectors.
// All Selectors must match for the rule to apply (AND semantics).
type LabelRule struct {
	// Path of the config file relative to the repo root.
	FilePath string
	// SHA of the file blob (copied from providers.FileEntry).
	FileSHA string
	// Required agent label key=value pairs.
	Selectors map[string]string
}
