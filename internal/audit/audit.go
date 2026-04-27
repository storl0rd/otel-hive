// Package audit provides a write-ahead audit log backed by the shared SQLite database.
package audit

import (
	"context"
	"database/sql"
	"time"
)

// EventType names a category of audited action.
type EventType string

const (
	// Auth events
	EventUserLogin    EventType = "user.login"
	EventUserLogout   EventType = "user.logout"
	EventUserSetup    EventType = "user.setup"
	EventAPIKeyCreated EventType = "api_key.created"
	EventAPIKeyRevoked EventType = "api_key.revoked"

	// Config events
	EventConfigCreated EventType = "config.created"
	EventConfigUpdated EventType = "config.updated"
	EventConfigDeleted EventType = "config.deleted"
	EventConfigPushed  EventType = "config.pushed"

	// Agent events
	EventAgentRestarted EventType = "agent.restarted"

	// Group events
	EventGroupCreated EventType = "group.created"
	EventGroupUpdated EventType = "group.updated"
	EventGroupDeleted EventType = "group.deleted"

	// Git source events
	EventGitSourceCreated EventType = "git_source.created"
	EventGitSourceUpdated EventType = "git_source.updated"
	EventGitSourceDeleted EventType = "git_source.deleted"
	EventGitSourceSynced  EventType = "git_source.synced"
)

// Entry is a single audit log record.
type Entry struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	ActorID      string    `json:"actor_id,omitempty"`
	ActorName    string    `json:"actor_name,omitempty"`
	EventType    EventType `json:"event_type"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Details      string    `json:"details,omitempty"`
	IPAddress    string    `json:"ip_address,omitempty"`
}

// Store writes and queries audit log entries using the shared SQLite connection.
type Store struct {
	db *sql.DB
}

// NewStore creates an audit Store. db must be the same handle used by the
// application store so that all writes share a single WAL journal.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Log writes a single audit entry. Errors are non-fatal — callers should log
// and continue rather than failing the user request.
func (s *Store) Log(ctx context.Context, e Entry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log
		 (actor_id, actor_name, event_type, resource_type, resource_id, details, ip_address)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullStr(e.ActorID), nullStr(e.ActorName), string(e.EventType),
		nullStr(e.ResourceType), nullStr(e.ResourceID),
		nullStr(e.Details), nullStr(e.IPAddress),
	)
	return err
}

// ListParams controls the query for List.
type ListParams struct {
	Page      int    // 1-based; defaults to 1
	Limit     int    // rows per page; defaults to 50, capped at 200
	EventType string // optional filter
}

// ListResult is the paginated response from List.
type ListResult struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
	Page    int     `json:"page"`
	Limit   int     `json:"limit"`
}

// List returns a paginated, optionally-filtered slice of audit entries ordered
// newest-first.
func (s *Store) List(ctx context.Context, p ListParams) (*ListResult, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	offset := (p.Page - 1) * p.Limit

	baseWhere := ""
	var filterArgs []any
	if p.EventType != "" {
		baseWhere = " WHERE event_type = ?"
		filterArgs = append(filterArgs, p.EventType)
	}

	// Total count
	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log"+baseWhere, filterArgs...,
	).Scan(&total); err != nil {
		return nil, err
	}

	// Rows
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, timestamp, actor_id, actor_name, event_type,
		        resource_type, resource_id, details, ip_address
		 FROM audit_log`+baseWhere+` ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		append(filterArgs, p.Limit, offset)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]Entry, 0)
	for rows.Next() {
		var e Entry
		var actorID, actorName, resourceType, resourceID, details, ip sql.NullString
		if err := rows.Scan(
			&e.ID, &e.Timestamp, &actorID, &actorName, &e.EventType,
			&resourceType, &resourceID, &details, &ip,
		); err != nil {
			return nil, err
		}
		e.ActorID = actorID.String
		e.ActorName = actorName.String
		e.ResourceType = resourceType.String
		e.ResourceID = resourceID.String
		e.Details = details.String
		e.IPAddress = ip.String
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ListResult{
		Entries: entries,
		Total:   total,
		Page:    p.Page,
		Limit:   p.Limit,
	}, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
