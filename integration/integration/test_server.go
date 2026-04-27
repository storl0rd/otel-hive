// Copyright (c) 2024 Lawrence OSS Contributors
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/api"
	"github.com/storl0rd/otel-hive/internal/auth"
	"github.com/storl0rd/otel-hive/internal/metrics"
	"github.com/storl0rd/otel-hive/internal/opamp"
	"github.com/storl0rd/otel-hive/internal/services"
	"github.com/storl0rd/otel-hive/internal/storage/applicationstore"
	"github.com/storl0rd/otel-hive/internal/storage/applicationstore/memory"
	"github.com/storl0rd/otel-hive/internal/storage/applicationstore/sqlite"
)

const testJWTSecret = "integration-test-secret-do-not-use-in-production"

// TestServer represents a test instance of otel-hive for integration testing
type TestServer struct {
	// Configuration
	HTTPPort  int
	OpAMPPort int

	// Storage
	appStoreFactory applicationstore.ApplicationStoreFactory

	// Services
	agentService services.AgentService
	authService  *auth.Service

	// Servers
	apiServer   *api.Server
	opampServer *opamp.Server

	// Metrics
	opampMetrics *metrics.OpAMPMetrics

	// Utilities
	logger  *zap.Logger
	baseURL string
	tempDir string
	t       *testing.T
}

// NewTestServer creates a new test server instance
func NewTestServer(t *testing.T, useMemory bool) *TestServer {
	logger := zap.NewNop()

	tempDir := t.TempDir()

	ts := &TestServer{
		HTTPPort:  findFreePort(),
		OpAMPPort: findFreePort(),
		logger:    logger,
		tempDir:   tempDir,
		t:         t,
	}

	ts.baseURL = fmt.Sprintf("http://localhost:%d", ts.HTTPPort)

	if useMemory {
		ts.initMemoryStorage()
	} else {
		ts.initDatabaseStorage()
	}

	ts.initMetrics()
	ts.initServices()
	ts.initAuth()
	ts.initServers()

	return ts
}

// initMemoryStorage initializes in-memory application storage
func (ts *TestServer) initMemoryStorage() {
	appFactory := memory.NewFactory()
	if err := appFactory.Initialize(ts.logger); err != nil {
		ts.t.Fatalf("Failed to initialize memory app store: %v", err)
	}
	ts.appStoreFactory = appFactory
}

// initDatabaseStorage initializes SQLite application storage
func (ts *TestServer) initDatabaseStorage() {
	appDBPath := ts.tempDir + "/app.db"
	appFactory := sqlite.NewFactory(appDBPath)
	if err := appFactory.Initialize(ts.logger); err != nil {
		ts.t.Fatalf("Failed to initialize SQLite app store: %v", err)
	}
	ts.appStoreFactory = appFactory
}

// initMetrics initializes metrics components
func (ts *TestServer) initMetrics() {
	registry := prometheus.NewRegistry()
	metricsFactory := metrics.NewPrometheusFactory("otel_hive", registry)
	ts.opampMetrics = metrics.NewOpAMPMetrics(metricsFactory)
}

// initServices initializes service layer
func (ts *TestServer) initServices() {
	appStore, err := ts.appStoreFactory.CreateApplicationStore()
	if err != nil {
		ts.t.Fatalf("Failed to create app store: %v", err)
	}
	ts.agentService = services.NewAgentService(appStore, ts.logger)
}

// initAuth initializes the auth service for testing.
// For SQLite backends the auth store shares the same DB connection.
// For in-memory backends auth is unavailable (authService remains nil).
func (ts *TestServer) initAuth() {
	provider, ok := ts.appStoreFactory.(applicationstore.DBProvider)
	if !ok {
		return
	}
	db := provider.DB()
	if db == nil {
		return
	}
	authStore, err := auth.NewStore(db)
	if err != nil {
		ts.t.Fatalf("Failed to create auth store: %v", err)
	}
	ts.authService = auth.NewService(authStore, testJWTSecret, 15*time.Minute, 7*24*time.Hour)
}

// initServers initializes all servers
func (ts *TestServer) initServers() {
	agents := opamp.NewAgents(ts.logger)
	configSender := opamp.NewConfigSender(agents, ts.logger)

	opampServer, err := opamp.NewServer(agents, ts.agentService, ts.opampMetrics, "", "", ts.logger)
	if err != nil {
		ts.t.Fatalf("Failed to create OpAMP server: %v", err)
	}
	ts.opampServer = opampServer

	ts.apiServer = api.NewServer(ts.agentService, ts.authService, nil, nil, configSender, ts.logger)
}

// Start starts all servers
func (ts *TestServer) Start() {
	go func() {
		if err := ts.apiServer.Start(fmt.Sprintf("%d", ts.HTTPPort)); err != nil && err != http.ErrServerClosed {
			ts.t.Logf("API server error: %v", err)
		}
	}()

	if err := ts.opampServer.Start(ts.OpAMPPort); err != nil {
		ts.t.Fatalf("Failed to start OpAMP server: %v", err)
	}

	ts.WaitForReady()
}

// Stop stops all servers
func (ts *TestServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if ts.apiServer != nil {
		_ = ts.apiServer.Stop(ctx)
	}

	if ts.opampServer != nil {
		_ = ts.opampServer.Stop(ctx)
	}

	if closer, ok := ts.appStoreFactory.(applicationstore.Closer); ok {
		closer.Close()
	}
}

// WaitForReady waits for the server to be ready to accept requests
func (ts *TestServer) WaitForReady() {
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		resp, err := http.Get(ts.baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	ts.t.Fatal("Server did not become ready in time")
}

// GET makes an HTTP GET request
func (ts *TestServer) GET(path string) (*http.Response, error) {
	return http.Get(ts.baseURL + path)
}

// POST makes an HTTP POST request
func (ts *TestServer) POST(path string, contentType string, body io.Reader) (*http.Response, error) {
	return http.Post(ts.baseURL+path, contentType, body)
}

// DELETE makes an HTTP DELETE request
func (ts *TestServer) DELETE(path string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", ts.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

// findFreePort finds an available port
func findFreePort() int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
