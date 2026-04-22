// Copyright (c) 2024 Lawrence OSS Contributors
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"

	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/storage/applicationstore/types"
)

// Factory implements ApplicationStoreFactory and creates storage components backed by SQLite.
type Factory struct {
	dbPath string
	logger *zap.Logger
	store  *Storage
}

// NewFactory creates a new Factory with the given database path.
func NewFactory(dbPath string) *Factory {
	return &Factory{
		dbPath: dbPath,
	}
}

// Initialize implements storage.ApplicationStoreFactory
func (f *Factory) Initialize(logger *zap.Logger) error {
	f.logger = logger
	store, err := NewSQLiteStorage(f.dbPath, logger)
	if err != nil {
		return err
	}
	f.store = store.(*Storage)
	return nil
}

// CreateApplicationStore implements storage.ApplicationStoreFactory
func (f *Factory) CreateApplicationStore() (types.ApplicationStore, error) {
	return f.store, nil
}

// Purge removes all data from the Factory's underlying SQLite store.
// This function is intended for testing purposes only and should not be used in production environments.
func (f *Factory) Purge(ctx context.Context) error {
	f.logger.Info("Purging data from SQLite application store")

	// Delete all data from tables
	_, err := f.store.db.ExecContext(ctx, "DELETE FROM configs")
	if err != nil {
		return err
	}
	_, err = f.store.db.ExecContext(ctx, "DELETE FROM agents")
	if err != nil {
		return err
	}
	_, err = f.store.db.ExecContext(ctx, "DELETE FROM groups")
	if err != nil {
		return err
	}

	return nil
}

// DB returns the underlying *sql.DB so other packages (e.g. auth) can share
// the same connection without opening a second handle to the same file.
func (f *Factory) DB() *sql.DB {
	if f.store == nil {
		return nil
	}
	return f.store.db
}

// Close implements storage.Closer
func (f *Factory) Close() error {
	if f.store != nil {
		return f.store.Close()
	}
	return nil
}
