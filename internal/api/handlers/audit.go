package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/audit"
)

// AuditHandlers handles audit log API endpoints.
type AuditHandlers struct {
	store  *audit.Store
	logger *zap.Logger
}

// NewAuditHandlers creates a new AuditHandlers.
func NewAuditHandlers(store *audit.Store, logger *zap.Logger) *AuditHandlers {
	return &AuditHandlers{store: store, logger: logger}
}

// HandleList returns paginated audit log entries.
// GET /api/v1/audit-log?page=1&limit=50&event_type=config.pushed
func (h *AuditHandlers) HandleList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	eventType := c.Query("event_type")

	result, err := h.store.List(c.Request.Context(), audit.ListParams{
		Page:      page,
		Limit:     limit,
		EventType: eventType,
	})
	if err != nil {
		h.logger.Error("failed to list audit log", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query audit log"})
		return
	}
	c.JSON(http.StatusOK, result)
}
