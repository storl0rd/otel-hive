package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/audit"
)

// routeEvent maps a (method, Gin full-path) pair to an audit EventType and
// resource type. The Gin full-path uses the pattern string, e.g.
// "/api/v1/configs/:id", not the actual URL.
type routeEvent struct {
	eventType    audit.EventType
	resourceType string
}

// auditRoutes maps "METHOD /pattern" → routeEvent.
var auditRoutes = map[string]routeEvent{
	// Auth
	"POST /api/auth/setup":              {audit.EventUserSetup, "user"},
	"POST /api/auth/login":              {audit.EventUserLogin, "user"},
	"POST /api/auth/logout":             {audit.EventUserLogout, "user"},
	"POST /api/auth/api-keys":           {audit.EventAPIKeyCreated, "api_key"},
	"DELETE /api/auth/api-keys/:id":     {audit.EventAPIKeyRevoked, "api_key"},

	// Configs
	"POST /api/v1/configs":              {audit.EventConfigCreated, "config"},
	"PUT /api/v1/configs/:id":           {audit.EventConfigUpdated, "config"},
	"DELETE /api/v1/configs/:id":        {audit.EventConfigDeleted, "config"},
	"POST /api/v1/agents/:id/config":    {audit.EventConfigPushed, "agent"},

	// Agents
	"POST /api/v1/agents/:id/restart":   {audit.EventAgentRestarted, "agent"},

	// Groups
	"POST /api/v1/groups":               {audit.EventGroupCreated, "group"},
	"PUT /api/v1/groups/:id":            {audit.EventGroupUpdated, "group"},
	"DELETE /api/v1/groups/:id":         {audit.EventGroupDeleted, "group"},

	// Git sources
	"POST /api/v1/git-sources":          {audit.EventGitSourceCreated, "git_source"},
	"PUT /api/v1/git-sources/:id":       {audit.EventGitSourceUpdated, "git_source"},
	"DELETE /api/v1/git-sources/:id":    {audit.EventGitSourceDeleted, "git_source"},
	"POST /api/v1/git-sources/:id/sync": {audit.EventGitSourceSynced, "git_source"},
}

// Audit returns a Gin middleware that writes a record to the audit log after
// each successful mutating request. GET / OPTIONS / HEAD requests are skipped.
// Requests that return 4xx or 5xx are also skipped.
func Audit(store *audit.Store, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next() // run the actual handler first

		// Only audit mutating methods
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			return
		}

		// Only audit successful responses
		if c.Writer.Status() >= 400 {
			return
		}

		// Look up event type by method + Gin route pattern
		key := method + " " + c.FullPath()
		re, ok := auditRoutes[key]
		if !ok {
			return
		}

		// Resource ID: prefer the `:id` path param, fall back to sub-resource params
		resourceID := c.Param("id")
		if resourceID == "" {
			// For routes without :id (create operations), try to pull from response body?
			// Not easily accessible post-handler — leave blank; the DB auto-ID is fine.
		}

		entry := audit.Entry{
			ActorID:      c.GetString(CtxUserID),
			ActorName:    c.GetString(CtxUsername),
			EventType:    re.eventType,
			ResourceType: re.resourceType,
			ResourceID:   strings.TrimPrefix(resourceID, "/"),
			IPAddress:    c.ClientIP(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.Log(ctx, entry); err != nil {
			logger.Warn("audit log write failed", zap.Error(err))
		}
	}
}
