package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Store handles all auth-related persistence against the shared SQLite DB.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store and runs schema migrations.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("auth store migration: %w", err)
	}
	return s, nil
}

// migrate creates auth tables if they don't exist. Idempotent.
func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id          TEXT PRIMARY KEY,
			username    TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role        TEXT NOT NULL DEFAULT 'operator',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS api_keys (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL,
			name        TEXT NOT NULL,
			key_hash    TEXT NOT NULL UNIQUE,
			last_used   DATETIME,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS refresh_tokens (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL,
			token_hash  TEXT NOT NULL UNIQUE,
			expires_at  DATETIME NOT NULL,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
		CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
	`)
	return err
}

// --- Users ---

func (s *Store) CreateUser(ctx context.Context, u *User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, string(u.Role),
	)
	return err
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	var u User
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &u, nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// --- API Keys ---

func (s *Store) CreateApiKey(ctx context.Context, k *ApiKey) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, name, key_hash) VALUES (?, ?, ?, ?)`,
		k.ID, k.UserID, k.Name, k.KeyHash,
	)
	return err
}

func (s *Store) GetApiKeyByHash(ctx context.Context, keyHash string) (*ApiKey, error) {
	var k ApiKey
	var lastUsed sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, key_hash, last_used, created_at FROM api_keys WHERE key_hash = ?`,
		keyHash,
	).Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &lastUsed, &createdAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", lastUsed.String)
		k.LastUsed = &t
	}
	k.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &k, nil
}

func (s *Store) ListApiKeysByUser(ctx context.Context, userID string) ([]*ApiKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, last_used, created_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*ApiKey
	for rows.Next() {
		var k ApiKey
		var lastUsed sql.NullString
		var createdAt string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &lastUsed, &createdAt); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", lastUsed.String)
			k.LastUsed = &t
		}
		k.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteApiKey(ctx context.Context, id, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = ? AND user_id = ?`, id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchApiKeyLastUsed(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	return err
}

// --- Refresh Tokens ---

func (s *Store) StoreRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	id, err := generateID()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		id, userID, tokenHash, expiresAt.UTC().Format("2006-01-02 15:04:05"),
	)
	return err
}

func (s *Store) GetRefreshToken(ctx context.Context, tokenHash string) (userID string, expiresAt time.Time, err error) {
	var expStr string
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM refresh_tokens WHERE token_hash = ?`, tokenHash,
	).Scan(&userID, &expStr)
	if err == sql.ErrNoRows {
		return "", time.Time{}, ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt, _ = time.Parse("2006-01-02 15:04:05", expStr)
	return userID, expiresAt, nil
}

func (s *Store) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE token_hash = ?`, tokenHash,
	)
	return err
}

func (s *Store) DeleteUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE user_id = ?`, userID,
	)
	return err
}

func (s *Store) PruneExpiredTokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE expires_at < CURRENT_TIMESTAMP`,
	)
	return err
}

// --- Helpers ---

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
