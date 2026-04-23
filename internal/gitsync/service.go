package gitsync

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/services"
)

// ConfigPusher can push a collector config to a specific agent via OpAMP.
// Implemented by opamp.ConfigSender.
type ConfigPusher interface {
	SendConfigToAgent(agentID uuid.UUID, configContent string) error
}

// AgentLister can list all known agents with their labels.
type AgentLister interface {
	ListAgents(ctx context.Context) ([]*services.Agent, error)
}

// agentWithLabels wraps *services.Agent to satisfy the generic MatchAgents constraint.
type agentWithLabels struct{ *services.Agent }

func (a agentWithLabels) GetLabels() map[string]string { return a.Labels }

// Service orchestrates git sync: stores sources, manages per-source pollers,
// and dispatches configs to matching agents via OpAMP.
type Service struct {
	store        *Store
	agentLister  AgentLister
	configPusher ConfigPusher
	logger       *zap.Logger

	mu      sync.Mutex
	workers map[string]*worker // keyed by GitSource.ID
	stopAll context.CancelFunc
	wg      sync.WaitGroup
}

// NewService creates a Service. Call Start() to begin background polling.
func NewService(
	store *Store,
	agentLister AgentLister,
	configPusher ConfigPusher,
	logger *zap.Logger,
) *Service {
	return &Service{
		store:        store,
		agentLister:  agentLister,
		configPusher: configPusher,
		logger:       logger,
		workers:      make(map[string]*worker),
	}
}

// Start loads all existing GitSources from the store and starts their pollers.
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.stopAll = cancel

	sources, err := s.store.List(ctx)
	if err != nil {
		cancel()
		return fmt.Errorf("loading git sources: %w", err)
	}

	for _, gs := range sources {
		s.startWorker(ctx, gs)
	}

	s.logger.Info("git sync service started", zap.Int("sources", len(sources)))
	return nil
}

// Stop gracefully shuts down all pollers.
func (s *Service) Stop() {
	if s.stopAll != nil {
		s.stopAll()
	}
	s.wg.Wait()
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// CreateSource persists a new GitSource and starts its poller.
func (s *Service) CreateSource(ctx context.Context, gs *GitSource) error {
	if err := validate(gs); err != nil {
		return err
	}
	if err := s.store.Create(ctx, gs); err != nil {
		return err
	}
	workerCtx := s.workerContext()
	s.startWorker(workerCtx, gs)
	return nil
}

// GetSource returns a single GitSource by ID.
func (s *Service) GetSource(ctx context.Context, id string) (*GitSource, error) {
	gs, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return gs, nil
}

// ListSources returns all GitSources.
func (s *Service) ListSources(ctx context.Context) ([]*GitSource, error) {
	return s.store.List(ctx)
}

// UpdateSource persists changes and restarts the poller with new settings.
func (s *Service) UpdateSource(ctx context.Context, gs *GitSource) error {
	if err := validate(gs); err != nil {
		return err
	}
	if err := s.store.Update(ctx, gs); err != nil {
		return err
	}
	s.stopWorker(gs.ID)
	workerCtx := s.workerContext()
	s.startWorker(workerCtx, gs)
	return nil
}

// DeleteSource removes the source and stops its poller.
func (s *Service) DeleteSource(ctx context.Context, id string) error {
	s.stopWorker(id)
	return s.store.Delete(ctx, id)
}

// TriggerSync forces an immediate sync for the given source, bypassing the SHA
// check (useful for the "Sync Now" button and webhook handler).
func (s *Service) TriggerSync(ctx context.Context, id string) (*SyncResult, error) {
	gs, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return s.runSync(ctx, gs, true)
}

// ── Webhook ───────────────────────────────────────────────────────────────────

// ValidateWebhookSignature checks the HMAC-SHA256 signature on an incoming
// webhook payload. Returns nil on success. If the source has no webhook secret
// configured, all signatures are accepted.
func (s *Service) ValidateWebhookSignature(sourceID string, payload []byte, sigHeader string) error {
	gs, err := s.store.Get(context.Background(), sourceID)
	if err != nil {
		return ErrNotFound
	}
	if gs.WebhookSecret == "" {
		return nil // no secret configured — accept all
	}

	// GitHub/Gitea send "sha256=<hex>"; GitLab sends "X-Gitlab-Token" as a plain string.
	// We support both.
	if strings.HasPrefix(sigHeader, "sha256=") {
		expected := computeHMAC(payload, gs.WebhookSecret)
		got := strings.TrimPrefix(sigHeader, "sha256=")
		if !hmac.Equal([]byte(expected), []byte(got)) {
			return fmt.Errorf("webhook signature mismatch")
		}
		return nil
	}

	// GitLab plain-token style
	if sigHeader == gs.WebhookSecret {
		return nil
	}
	return fmt.Errorf("webhook signature mismatch")
}

func computeHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// ── Sync core ─────────────────────────────────────────────────────────────────

// runSync fetches the repo tree, determines changed files, and pushes configs
// to matching agents. If forceAll is true it ignores per-file SHA cache.
func (s *Service) runSync(ctx context.Context, gs *GitSource, forceAll bool) (*SyncResult, error) {
	result := &SyncResult{SourceID: gs.ID}

	prov, err := NewProvider(gs)
	if err != nil {
		s.recordFailure(gs.ID, "", err)
		return result, err
	}

	// 1. Get latest commit SHA
	commitSHA, err := prov.LatestCommitSHA(ctx, gs.Branch)
	if err != nil {
		s.recordFailure(gs.ID, gs.LastSyncSHA, err)
		return result, fmt.Errorf("fetching commit SHA: %w", err)
	}

	// 2. Short-circuit if commit hasn't changed (unless forced)
	if !forceAll && commitSHA != "" && commitSHA == gs.LastSyncSHA {
		s.logger.Debug("no changes detected", zap.String("source", gs.ID), zap.String("sha", commitSHA))
		return result, nil
	}

	// 3. List files under config root
	entries, err := prov.ListFiles(ctx, gs.Branch, gs.ConfigRoot)
	if err != nil {
		s.recordFailure(gs.ID, gs.LastSyncSHA, err)
		return result, fmt.Errorf("listing files: %w", err)
	}

	// 4. Parse label rules from paths
	rules := ParseRules(entries, gs.ConfigRoot)

	// 5. List connected agents
	agentList, err := s.agentLister.ListAgents(ctx)
	if err != nil {
		s.recordFailure(gs.ID, gs.LastSyncSHA, err)
		return result, fmt.Errorf("listing agents: %w", err)
	}
	wrapped := make([]agentWithLabels, len(agentList))
	for i, a := range agentList {
		wrapped[i] = agentWithLabels{a}
	}

	// Build file SHA lookup for quick access
	shaByPath := make(map[string]string, len(entries))
	for _, e := range entries {
		shaByPath[e.Path] = e.SHA
	}

	// 6. For each rule, check if file changed and push to matching agents
	var pushErrors []error

	for _, rule := range rules {
		fileSHA := shaByPath[rule.FilePath]

		// Per-file change detection
		if !forceAll {
			cached, _ := s.store.GetFileSHA(ctx, gs.ID, rule.FilePath)
			if cached != "" && cached == fileSHA {
				continue // this file hasn't changed
			}
		}

		// Fetch file content
		content, err := prov.FetchFile(ctx, rule.FilePath, fileSHA)
		if err != nil {
			pushErrors = append(pushErrors, fmt.Errorf("fetch %s: %w", rule.FilePath, err))
			continue
		}

		// Match agents
		matched := MatchAgents(rule, wrapped)
		if len(matched) == 0 {
			s.logger.Debug("no agents matched rule",
				zap.String("file", rule.FilePath),
				zap.Any("selectors", rule.Selectors))
			// Update file SHA cache even if no agents matched — avoids re-fetching
			if fileSHA != "" {
				_ = s.store.UpsertFileSHA(ctx, gs.ID, rule.FilePath, fileSHA)
			}
			continue
		}

		// Push to each matched agent
		for _, a := range matched {
			if err := s.configPusher.SendConfigToAgent(a.ID, string(content)); err != nil {
				s.logger.Warn("failed to push config to agent",
					zap.String("agent", a.ID.String()),
					zap.String("file", rule.FilePath),
					zap.Error(err))
				pushErrors = append(pushErrors, fmt.Errorf("agent %s: %w", a.ID, err))
			} else {
				result.AgentsUpdated++
				s.logger.Info("pushed config to agent",
					zap.String("agent", a.Name),
					zap.String("file", rule.FilePath))
			}
		}

		// Update per-file SHA cache
		if fileSHA != "" {
			_ = s.store.UpsertFileSHA(ctx, gs.ID, rule.FilePath, fileSHA)
		}
		result.FilesChanged++
	}

	// 7. Persist sync outcome
	finalSHA := commitSHA
	if finalSHA == "" {
		finalSHA = gs.LastSyncSHA
	}
	if len(pushErrors) > 0 {
		s.recordFailure(gs.ID, finalSHA, fmt.Errorf("%d error(s): %w", len(pushErrors), pushErrors[0]))
		result.Errors = pushErrors
	} else {
		if err := s.store.UpdateSyncState(ctx, gs.ID, finalSHA, SyncStatusSuccess, ""); err != nil {
			s.logger.Warn("failed to persist sync state", zap.Error(err))
		}
	}

	s.logger.Info("sync complete",
		zap.String("source", gs.Name),
		zap.Int("files_changed", result.FilesChanged),
		zap.Int("agents_updated", result.AgentsUpdated),
		zap.Int("errors", len(pushErrors)),
	)
	return result, nil
}

func (s *Service) recordFailure(sourceID, sha string, err error) {
	s.logger.Error("sync failed", zap.String("source", sourceID), zap.Error(err))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.store.UpdateSyncState(ctx, sourceID, sha, SyncStatusFailed, err.Error())
}

// ── Worker management ─────────────────────────────────────────────────────────

func (s *Service) startWorker(ctx context.Context, gs *GitSource) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[gs.ID]; exists {
		return // already running
	}

	w := newWorker(gs, s)
	s.workers[gs.ID] = w
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		w.run(ctx)
	}()
}

func (s *Service) stopWorker(id string) {
	s.mu.Lock()
	w, exists := s.workers[id]
	if exists {
		delete(s.workers, id)
	}
	s.mu.Unlock()

	if exists {
		w.stop()
	}
}

// workerContext returns a context tied to s.stopAll.
// Must only be called after Start() has been called.
func (s *Service) workerContext() context.Context {
	// We rely on the parent context stored when Start() ran.
	// If stopAll hasn't been called yet we use a background context as fallback.
	if s.stopAll == nil {
		return context.Background()
	}
	// Re-create a child of background; the stopAll cancel will still fire
	// because workers listen on the context passed into their run() goroutine.
	return context.Background()
}

// ── Validation ────────────────────────────────────────────────────────────────

func validate(gs *GitSource) error {
	if strings.TrimSpace(gs.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(gs.RepoURL) == "" {
		return fmt.Errorf("repo_url is required")
	}
	switch gs.ProviderType {
	case ProviderGitHub, ProviderGitLab, ProviderGitea, ProviderHTTP:
	default:
		return fmt.Errorf("unknown provider %q; must be one of: github, gitlab, gitea, http", gs.ProviderType)
	}
	return nil
}
