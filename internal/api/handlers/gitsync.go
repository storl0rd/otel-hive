package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/gitsync"
)

// GitSyncHandlers handles all git-sync HTTP endpoints.
type GitSyncHandlers struct {
	svc    *gitsync.Service
	logger *zap.Logger
}

// NewGitSyncHandlers creates a new GitSyncHandlers.
func NewGitSyncHandlers(svc *gitsync.Service, logger *zap.Logger) *GitSyncHandlers {
	return &GitSyncHandlers{svc: svc, logger: logger}
}

// ── Authenticated CRUD ────────────────────────────────────────────────────────

// HandleListSources returns all git sources.
// GET /api/v1/git-sources
func (h *GitSyncHandlers) HandleListSources(c *gin.Context) {
	sources, err := h.svc.ListSources(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sources == nil {
		sources = []*gitsync.GitSource{}
	}
	c.JSON(http.StatusOK, gin.H{"git_sources": sources})
}

// HandleGetSource returns a single git source by ID.
// GET /api/v1/git-sources/:id
func (h *GitSyncHandlers) HandleGetSource(c *gin.Context) {
	id := c.Param("id")
	gs, err := h.svc.GetSource(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "git source not found"})
		return
	}
	c.JSON(http.StatusOK, sanitise(gs))
}

// HandleCreateSource creates a new git source.
// POST /api/v1/git-sources
func (h *GitSyncHandlers) HandleCreateSource(c *gin.Context) {
	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gs := &gitsync.GitSource{
		Name:                req.Name,
		RepoURL:             req.RepoURL,
		Token:               req.Token,
		Branch:              req.Branch,
		ConfigRoot:          req.ConfigRoot,
		ProviderType:        gitsync.Provider(req.Provider),
		PollIntervalSeconds: req.PollIntervalSeconds,
		WebhookSecret:       req.WebhookSecret,
	}

	if err := h.svc.CreateSource(c.Request.Context(), gs); err != nil {
		status := http.StatusInternalServerError
		if isValidationErr(err) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, sanitise(gs))
}

// HandleUpdateSource replaces a git source's configuration.
// PUT /api/v1/git-sources/:id
func (h *GitSyncHandlers) HandleUpdateSource(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.svc.GetSource(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "git source not found"})
		return
	}

	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing.Name = req.Name
	existing.RepoURL = req.RepoURL
	if req.Token != "" {
		existing.Token = req.Token // only update token if provided (allow omitting to keep existing)
	}
	existing.Branch = req.Branch
	existing.ConfigRoot = req.ConfigRoot
	existing.ProviderType = gitsync.Provider(req.Provider)
	if req.PollIntervalSeconds > 0 {
		existing.PollIntervalSeconds = req.PollIntervalSeconds
	}
	existing.WebhookSecret = req.WebhookSecret

	if err := h.svc.UpdateSource(c.Request.Context(), existing); err != nil {
		status := http.StatusInternalServerError
		if isValidationErr(err) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sanitise(existing))
}

// HandleDeleteSource deletes a git source.
// DELETE /api/v1/git-sources/:id
func (h *GitSyncHandlers) HandleDeleteSource(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.svc.GetSource(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "git source not found"})
		return
	}
	if err := h.svc.DeleteSource(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// HandleTriggerSync forces an immediate sync for a source.
// POST /api/v1/git-sources/:id/sync
func (h *GitSyncHandlers) HandleTriggerSync(c *gin.Context) {
	id := c.Param("id")
	result, err := h.svc.TriggerSync(c.Request.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == gitsync.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"source_id":      result.SourceID,
		"files_changed":  result.FilesChanged,
		"agents_updated": result.AgentsUpdated,
		"error_count":    len(result.Errors),
	})
}

// ── Webhook ───────────────────────────────────────────────────────────────────

// HandleWebhook receives a webhook push event and triggers a sync.
// POST /api/webhook/git/:id
// This endpoint is intentionally unauthenticated but HMAC-validated.
func (h *GitSyncHandlers) HandleWebhook(c *gin.Context) {
	id := c.Param("id")

	// Read body for HMAC validation before handing to JSON parser
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 5<<20)) // 5 MB limit
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// Check signature — GitHub: X-Hub-Signature-256, GitLab: X-Gitlab-Token, Gitea: X-Gitea-Signature
	sig := c.GetHeader("X-Hub-Signature-256")
	if sig == "" {
		sig = c.GetHeader("X-Gitea-Signature")
	}
	if sig == "" {
		sig = c.GetHeader("X-Gitlab-Token")
	}

	if err := h.svc.ValidateWebhookSignature(id, body, sig); err != nil {
		h.logger.Warn("webhook signature validation failed",
			zap.String("source_id", id), zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook signature"})
		return
	}

	// Trigger sync asynchronously so the webhook returns quickly
	go func() {
		result, err := h.svc.TriggerSync(c.Request.Context(), id)
		if err != nil {
			h.logger.Error("webhook-triggered sync failed", zap.String("source_id", id), zap.Error(err))
			return
		}
		h.logger.Info("webhook sync complete",
			zap.String("source_id", id),
			zap.Int("files", result.FilesChanged),
			zap.Int("agents", result.AgentsUpdated))
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "sync triggered"})
}

// ── Request / response types ──────────────────────────────────────────────────

type createSourceRequest struct {
	Name                string `json:"name" binding:"required"`
	RepoURL             string `json:"repo_url" binding:"required"`
	Token               string `json:"token"`
	Branch              string `json:"branch"`
	ConfigRoot          string `json:"config_root"`
	Provider            string `json:"provider" binding:"required"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	WebhookSecret       string `json:"webhook_secret"`
}

// sanitise returns a copy of the GitSource with the token redacted.
func sanitise(gs *gitsync.GitSource) map[string]interface{} {
	hasToken := gs.Token != ""
	return map[string]interface{}{
		"id":                    gs.ID,
		"name":                  gs.Name,
		"repo_url":              gs.RepoURL,
		"has_token":             hasToken,
		"branch":                gs.Branch,
		"config_root":           gs.ConfigRoot,
		"provider":              string(gs.ProviderType),
		"poll_interval_seconds": gs.PollIntervalSeconds,
		"has_webhook_secret":    gs.WebhookSecret != "",
		"last_sync_sha":         gs.LastSyncSHA,
		"last_sync_at":          gs.LastSyncAt,
		"last_sync_status":      string(gs.LastSyncStatus),
		"last_sync_error":       gs.LastSyncError,
		"created_at":            gs.CreatedAt,
		"updated_at":            gs.UpdatedAt,
	}
}

// isValidationErr reports whether err is a user-facing validation error
// (as opposed to a storage error).
func isValidationErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "name is required" ||
		msg == "repo_url is required" ||
		len(msg) > 0 && msg[:7] == "unknown"
}
