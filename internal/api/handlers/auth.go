package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/auth"
	"github.com/storl0rd/otel-hive/internal/middleware"
)

// AuthHandlers holds all auth-related HTTP handlers.
type AuthHandlers struct {
	svc    *auth.Service
	logger *zap.Logger
}

func NewAuthHandlers(svc *auth.Service, logger *zap.Logger) *AuthHandlers {
	return &AuthHandlers{svc: svc, logger: logger}
}

// HandleSetupStatus returns whether first-run setup is required.
// GET /api/auth/setup/status  — public
func (h *AuthHandlers) HandleSetupStatus(c *gin.Context) {
	required, err := h.svc.IsSetupRequired(c.Request.Context())
	if err != nil {
		h.logger.Error("setup status check failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"setup_required": required})
}

// HandleSetup creates the first admin account.
// POST /api/auth/setup  — public, only works when no users exist
func (h *AuthHandlers) HandleSetup(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pair, err := h.svc.Setup(c.Request.Context(), req.Username, req.Password)
	if err == auth.ErrSetupDone {
		c.JSON(http.StatusConflict, gin.H{"error": "setup already complete"})
		return
	}
	if err != nil {
		h.logger.Error("setup failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}

	h.logger.Info("initial admin created", zap.String("username", req.Username))
	c.JSON(http.StatusCreated, pair)
}

// HandleLogin authenticates with username + password.
// POST /api/auth/login  — public
func (h *AuthHandlers) HandleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pair, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err == auth.ErrInvalidPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err != nil {
		h.logger.Error("login failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}

	c.JSON(http.StatusOK, pair)
}

// HandleRefresh exchanges a refresh token for a new access token.
// POST /api/auth/refresh  — public
func (h *AuthHandlers) HandleRefresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err == auth.ErrInvalidToken || err == auth.ErrNotFound {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	if err == auth.ErrTokenExpired {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token expired"})
		return
	}
	if err != nil {
		h.logger.Error("token refresh failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "refresh failed"})
		return
	}

	c.JSON(http.StatusOK, pair)
}

// HandleLogout invalidates the current refresh token.
// POST /api/auth/logout  — public (token optional)
func (h *AuthHandlers) HandleLogout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	// Best-effort: ignore binding errors — client may already have lost the token
	_ = c.ShouldBindJSON(&req)
	if req.RefreshToken != "" {
		_ = h.svc.Logout(c.Request.Context(), req.RefreshToken)
	}
	c.Status(http.StatusNoContent)
}

// HandleListApiKeys returns all API keys for the authenticated user.
// GET /api/auth/api-keys  — requires auth
func (h *AuthHandlers) HandleListApiKeys(c *gin.Context) {
	userID := middleware.UserID(c)
	keys, err := h.svc.ListApiKeys(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("list api keys failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list keys"})
		return
	}
	if keys == nil {
		keys = []*auth.ApiKey{} // return [] not null
	}
	c.JSON(http.StatusOK, keys)
}

// HandleCreateApiKey generates a new API key. The plaintext key is returned
// only in this response and never stored.
// POST /api/auth/api-keys  — requires auth
func (h *AuthHandlers) HandleCreateApiKey(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := middleware.UserID(c)
	key, plainKey, err := h.svc.CreateApiKey(c.Request.Context(), userID, req.Name)
	if err != nil {
		h.logger.Error("create api key failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create key"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         key.ID,
		"name":       key.Name,
		"key":        plainKey, // shown once only
		"created_at": key.CreatedAt,
	})
}

// HandleRevokeApiKey deletes an API key owned by the authenticated user.
// DELETE /api/auth/api-keys/:id  — requires auth
func (h *AuthHandlers) HandleRevokeApiKey(c *gin.Context) {
	keyID := c.Param("id")
	userID := middleware.UserID(c)

	if err := h.svc.RevokeApiKey(c.Request.Context(), keyID, userID); err == auth.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	} else if err != nil {
		h.logger.Error("revoke api key failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke key"})
		return
	}

	c.Status(http.StatusNoContent)
}

// HandleMe returns the currently authenticated user's info.
// GET /api/auth/me  — requires auth
func (h *AuthHandlers) HandleMe(c *gin.Context) {
	v, _ := c.Get(middleware.CtxUserID)
	userID, _ := v.(string)
	v, _ = c.Get(middleware.CtxUsername)
	username, _ := v.(string)
	v, _ = c.Get(middleware.CtxRole)
	role, _ := v.(string)

	c.JSON(http.StatusOK, gin.H{
		"id":       userID,
		"username": username,
		"role":     role,
	})
}
