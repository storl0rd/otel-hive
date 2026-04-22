package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/storl0rd/otel-hive/internal/auth"
)

// Context keys for values set by the auth middleware.
const (
	CtxUserID   = "auth_user_id"
	CtxUsername = "auth_username"
	CtxRole     = "auth_role"
)

// Auth returns a Gin middleware that enforces authentication.
//
// Priority:
//  1. Authorization: Bearer <jwt>
//  2. X-API-Key: ohk_<key>
//
// On first-run (no users exist) every protected route returns 503 with
// {"setup_required": true} so the frontend can redirect to /setup.
//
// If svc is nil (e.g. in tests using an in-memory backend that has no auth
// store), authentication is bypassed and the request passes through.
func Auth(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Bypass auth when no auth service is configured (test/dev only).
		if svc == nil {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// First-run guard: if no users exist, block everything except the
		// setup and health endpoints (those are explicitly excluded from this
		// middleware in server.go).
		required, err := svc.IsSetupRequired(ctx)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth check failed"})
			return
		}
		if required {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"setup_required": true,
				"message":        "Initial setup required. Visit /setup to create the admin account.",
			})
			return
		}

		// Try Bearer JWT first
		if header := c.GetHeader("Authorization"); strings.HasPrefix(header, "Bearer ") {
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := svc.ValidateAccessToken(tokenStr)
			if err != nil {
				abortUnauthorized(c, err)
				return
			}
			setUser(c, claims.UserID, claims.Username, string(claims.Role))
			c.Next()
			return
		}

		// Try X-API-Key header
		if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
			user, _, err := svc.ValidateApiKey(ctx, apiKey)
			if err != nil {
				abortUnauthorized(c, err)
				return
			}
			setUser(c, user.ID, user.Username, string(user.Role))
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
	}
}

func setUser(c *gin.Context, userID, username, role string) {
	c.Set(CtxUserID, userID)
	c.Set(CtxUsername, username)
	c.Set(CtxRole, role)
}

func abortUnauthorized(c *gin.Context, err error) {
	msg := "unauthorized"
	if err == auth.ErrTokenExpired {
		msg = "token expired"
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": msg})
}

// UserID extracts the authenticated user's ID from context (set by Auth middleware).
func UserID(c *gin.Context) string {
	v, _ := c.Get(CtxUserID)
	s, _ := v.(string)
	return s
}

// UserRole extracts the authenticated user's role from context.
func UserRole(c *gin.Context) auth.Role {
	v, _ := c.Get(CtxRole)
	s, _ := v.(string)
	return auth.Role(s)
}
