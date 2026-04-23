package gitsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store persists GitSource records in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store and runs migrations.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	return s, s.migrate()
}

const schema = `
CREATE TABLE IF NOT EXISTS git_sources (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL,
    repo_url             TEXT NOT NULL,
    token                TEXT NOT NULL DEFAULT '',
    branch               TEXT NOT NULL DEFAULT 'main',
    config_root          TEXT NOT NULL DEFAULT 'configs',
    provider             TEXT NOT NULL,
    poll_interval_secs   INTEGER NOT NULL DEFAULT 300,
    webhook_secret       TEXT NOT NULL DEFAULT '',
    last_sync_sha        TEXT NOT NULL DEFAULT '',
    last_sync_at         DATETIME,
    last_sync_status     TEXT NOT NULL DEFAULT 'pending',
    last_sync_error      TEXT NOT NULL DEFAULT '',
    created_at           DATETIME NOT NULL,
    updated_at           DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS git_source_file_shas (
    source_id  TEXT NOT NULL,
    file_path  TEXT NOT NULL,
    sha        TEXT NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (source_id, file_path),
    FOREIGN KEY (source_id) REFERENCES git_sources(id) ON DELETE CASCADE
);
`

func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

// Create inserts a new GitSource. Sets ID, CreatedAt, UpdatedAt.
func (s *Store) Create(ctx context.Context, gs *GitSource) error {
	if gs.ID == "" {
		gs.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	gs.CreatedAt = now
	gs.UpdatedAt = now

	if gs.Branch == "" {
		gs.Branch = "main"
	}
	if gs.ConfigRoot == "" {
		gs.ConfigRoot = "configs"
	}
	if gs.PollIntervalSeconds == 0 {
		gs.PollIntervalSeconds = 300
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO git_sources
		  (id, name, repo_url, token, branch, config_root, provider,
		   poll_interval_secs, webhook_secret,
		   last_sync_sha, last_sync_status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		gs.ID, gs.Name, gs.RepoURL, gs.Token, gs.Branch, gs.ConfigRoot,
		string(gs.ProviderType), gs.PollIntervalSeconds, gs.WebhookSecret,
		"", string(SyncStatusPending), now, now,
	)
	return err
}

// Get retrieves a single GitSource by ID.
func (s *Store) Get(ctx context.Context, id string) (*GitSource, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, repo_url, token, branch, config_root, provider,
		       poll_interval_secs, webhook_secret,
		       last_sync_sha, last_sync_at, last_sync_status, last_sync_error,
		       created_at, updated_at
		FROM git_sources WHERE id = ?`, id)
	return scanSource(row)
}

// List returns all GitSources.
func (s *Store) List(ctx context.Context) ([]*GitSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, repo_url, token, branch, config_root, provider,
		       poll_interval_secs, webhook_secret,
		       last_sync_sha, last_sync_at, last_sync_status, last_sync_error,
		       created_at, updated_at
		FROM git_sources ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*GitSource
	for rows.Next() {
		gs, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, gs)
	}
	return out, rows.Err()
}

// Update persists name, repo_url, token, branch, config_root, provider,
// poll_interval_secs, and webhook_secret fields.
func (s *Store) Update(ctx context.Context, gs *GitSource) error {
	gs.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE git_sources SET
		  name=?, repo_url=?, token=?, branch=?, config_root=?, provider=?,
		  poll_interval_secs=?, webhook_secret=?, updated_at=?
		WHERE id=?`,
		gs.Name, gs.RepoURL, gs.Token, gs.Branch, gs.ConfigRoot,
		string(gs.ProviderType), gs.PollIntervalSeconds, gs.WebhookSecret,
		gs.UpdatedAt, gs.ID,
	)
	return err
}

// Delete removes a GitSource and its file SHA cache.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM git_sources WHERE id = ?`, id)
	return err
}

// UpdateSyncState persists the outcome of a sync run.
func (s *Store) UpdateSyncState(ctx context.Context, id string, sha string, status SyncStatus, syncErr string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE git_sources SET
		  last_sync_sha=?, last_sync_at=?, last_sync_status=?, last_sync_error=?, updated_at=?
		WHERE id=?`,
		sha, now, string(status), syncErr, now, id,
	)
	return err
}

// ── File SHA cache ────────────────────────────────────────────────────────────

// GetFileSHA returns the last known SHA for a file path within a source,
// or "" if not cached.
func (s *Store) GetFileSHA(ctx context.Context, sourceID, path string) (string, error) {
	var sha string
	err := s.db.QueryRowContext(ctx,
		`SELECT sha FROM git_source_file_shas WHERE source_id=? AND file_path=?`,
		sourceID, path,
	).Scan(&sha)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sha, err
}

// UpsertFileSHA stores or updates the SHA for a file path.
func (s *Store) UpsertFileSHA(ctx context.Context, sourceID, path, sha string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO git_source_file_shas (source_id, file_path, sha, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(source_id, file_path) DO UPDATE SET sha=excluded.sha, updated_at=excluded.updated_at`,
		sourceID, path, sha, now,
	)
	return err
}

// ── Scanner helpers ───────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanSource(row scanner) (*GitSource, error) {
	var gs GitSource
	var syncAt sql.NullTime
	var provider, status string

	err := row.Scan(
		&gs.ID, &gs.Name, &gs.RepoURL, &gs.Token, &gs.Branch, &gs.ConfigRoot,
		&provider, &gs.PollIntervalSeconds, &gs.WebhookSecret,
		&gs.LastSyncSHA, &syncAt, &status, &gs.LastSyncError,
		&gs.CreatedAt, &gs.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	gs.ProviderType = Provider(provider)
	gs.LastSyncStatus = SyncStatus(status)
	if syncAt.Valid {
		t := syncAt.Time
		gs.LastSyncAt = &t
	}
	return &gs, nil
}

// ── JSON helpers (for API serialisation) ─────────────────────────────────────

func (gs *GitSource) MarshalPublic() ([]byte, error) {
	type public struct {
		ID                  string     `json:"id"`
		Name                string     `json:"name"`
		RepoURL             string     `json:"repo_url"`
		Branch              string     `json:"branch"`
		ConfigRoot          string     `json:"config_root"`
		ProviderType        Provider   `json:"provider"`
		PollIntervalSeconds int        `json:"poll_interval_seconds"`
		LastSyncSHA         string     `json:"last_sync_sha,omitempty"`
		LastSyncAt          *time.Time `json:"last_sync_at,omitempty"`
		LastSyncStatus      SyncStatus `json:"last_sync_status"`
		LastSyncError       string     `json:"last_sync_error,omitempty"`
		CreatedAt           time.Time  `json:"created_at"`
		UpdatedAt           time.Time  `json:"updated_at"`
	}
	return json.Marshal(public{
		ID:                  gs.ID,
		Name:                gs.Name,
		RepoURL:             gs.RepoURL,
		Branch:              gs.Branch,
		ConfigRoot:          gs.ConfigRoot,
		ProviderType:        gs.ProviderType,
		PollIntervalSeconds: gs.PollIntervalSeconds,
		LastSyncSHA:         gs.LastSyncSHA,
		LastSyncAt:          gs.LastSyncAt,
		LastSyncStatus:      gs.LastSyncStatus,
		LastSyncError:       gs.LastSyncError,
		CreatedAt:           gs.CreatedAt,
		UpdatedAt:           gs.UpdatedAt,
	})
}

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = fmt.Errorf("git source not found")
