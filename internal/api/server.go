package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/api/handlers"
	"github.com/storl0rd/otel-hive/internal/auth"
	"github.com/storl0rd/otel-hive/internal/metrics"
	"github.com/storl0rd/otel-hive/internal/middleware"
	"github.com/storl0rd/otel-hive/internal/services"
)

// AgentCommander defines the interface for sending commands to agents
type AgentCommander interface {
	SendConfigToAgent(agentId uuid.UUID, configContent string) error
	RestartAgent(agentId uuid.UUID) error
	RestartAgentsInGroup(groupId string) ([]uuid.UUID, []error)
	SendConfigToAgentsInGroup(groupId string, configContent string) ([]uuid.UUID, []error)
}

// Server represents the HTTP API server
type Server struct {
	router       *gin.Engine
	agentService services.AgentService
	authService  *auth.Service
	commander    AgentCommander
	logger       *zap.Logger
	httpServer   *http.Server
	metrics      *metrics.APIMetrics
	registry     *prometheus.Registry
}

// NewServer creates a new API server
func NewServer(agentService services.AgentService, authService *auth.Service, commander AgentCommander, logger *zap.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	registry := prometheus.NewRegistry()
	metricsFactory := metrics.NewPrometheusFactory("otel_hive", registry)
	apiMetrics := metrics.NewAPIMetrics(metricsFactory)

	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(loggingMiddleware(logger))

	server := &Server{
		router:       router,
		agentService: agentService,
		authService:  authService,
		commander:    commander,
		logger:       logger,
		metrics:      apiMetrics,
		registry:     registry,
	}

	router.Use(server.metricsMiddleware())
	server.registerRoutes()

	return server
}

// Start starts the HTTP server
func (s *Server) Start(port string) error {
	s.httpServer = &http.Server{
		Addr:    ":" + port,
		Handler: s.router,
	}

	s.logger.Info("Starting HTTP API server", zap.String("port", port))
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping HTTP API server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(shutdownCtx)
}

// registerRoutes registers all API routes
func (s *Server) registerRoutes() {
	agentHandlers := handlers.NewAgentHandlers(s.agentService, s.commander, s.logger)
	configHandlers := handlers.NewConfigHandlers(s.agentService, s.commander, s.logger)
	groupHandlers := handlers.NewGroupHandlers(s.agentService, s.commander, s.logger)
	healthHandlers := handlers.NewHealthHandlers(s.agentService, s.logger)
	authHandlers := handlers.NewAuthHandlers(s.authService, s.logger)

	// Metrics endpoint (unauthenticated — typically blocked at network level in prod)
	s.router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{})))

	// Health check (unauthenticated — used by Docker healthcheck)
	s.router.GET("/health", healthHandlers.HandleHealth)

	// Auth routes — public (no middleware)
	apiAuth := s.router.Group("/api/auth")
	{
		apiAuth.GET("/setup/status", authHandlers.HandleSetupStatus)
		apiAuth.POST("/setup", authHandlers.HandleSetup)
		apiAuth.POST("/login", authHandlers.HandleLogin)
		apiAuth.POST("/logout", authHandlers.HandleLogout)
		apiAuth.POST("/refresh", authHandlers.HandleRefresh)
	}

	// Authenticated auth routes
	apiAuthProtected := s.router.Group("/api/auth", middleware.Auth(s.authService))
	{
		apiAuthProtected.GET("/me", authHandlers.HandleMe)
		apiAuthProtected.GET("/api-keys", authHandlers.HandleListApiKeys)
		apiAuthProtected.POST("/api-keys", authHandlers.HandleCreateApiKey)
		apiAuthProtected.DELETE("/api-keys/:id", authHandlers.HandleRevokeApiKey)
	}

	// API v1 routes — all protected by auth middleware
	v1 := s.router.Group("/api/v1", middleware.Auth(s.authService))
	{
		// Agent routes
		agents := v1.Group("/agents")
		{
			agents.GET("", agentHandlers.HandleGetAgents)
			agents.GET("/stats", agentHandlers.HandleGetAgentStats)
			agents.GET("/:id", agentHandlers.HandleGetAgent)
			agents.PATCH("/:id/group", agentHandlers.HandleUpdateAgentGroup)
			agents.POST("/:id/config", agentHandlers.HandleSendConfigToAgent)
			agents.POST("/:id/restart", agentHandlers.HandleRestartAgent)
		}

		// Config routes
		configs := v1.Group("/configs")
		{
			configs.GET("", configHandlers.HandleGetConfigs)
			configs.POST("", configHandlers.HandleCreateConfig)
			configs.POST("/validate", configHandlers.HandleValidateConfig)
			configs.GET("/versions", configHandlers.HandleGetConfigVersions)
			configs.GET("/:id", configHandlers.HandleGetConfig)
			configs.PUT("/:id", configHandlers.HandleUpdateConfig)
			configs.DELETE("/:id", configHandlers.HandleDeleteConfig)
		}

		// Group routes
		groups := v1.Group("/groups")
		{
			groups.GET("", groupHandlers.HandleGetGroups)
			groups.POST("", groupHandlers.HandleCreateGroup)
			groups.GET("/:id", groupHandlers.HandleGetGroup)
			groups.PUT("/:id", groupHandlers.HandleUpdateGroup)
			groups.DELETE("/:id", groupHandlers.HandleDeleteGroup)
			groups.POST("/:id/config", groupHandlers.HandleAssignConfig)
			groups.GET("/:id/config", groupHandlers.HandleGetGroupConfig)
			groups.GET("/:id/agents", groupHandlers.HandleGetGroupAgents)
			groups.POST("/:id/restart", groupHandlers.HandleRestartGroup)
		}
	}

	// Serve static files for the UI
	s.router.Static("/assets", "./ui/dist/assets")

	// SPA catch-all route — must be last
	s.router.NoRoute(func(c *gin.Context) {
		filePath := filepath.Join("./ui/dist", c.Request.URL.Path)
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath)
			return
		}
		c.File("./ui/dist/index.html")
	})
}

// corsMiddleware adds CORS headers
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// loggingMiddleware adds request logging with reduced verbosity
func loggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		if param.Path == "/health" || param.Path == "/ready" {
			return ""
		}

		if param.StatusCode >= 400 {
			logger.Info("HTTP Request Error",
				zap.String("method", param.Method),
				zap.String("path", param.Path),
				zap.Int("status", param.StatusCode),
				zap.Duration("latency", param.Latency),
				zap.String("client_ip", param.ClientIP),
			)
			return ""
		}

		logger.Debug("HTTP Request",
			zap.String("method", param.Method),
			zap.String("path", param.Path),
			zap.Int("status", param.StatusCode),
			zap.Duration("latency", param.Latency),
			zap.String("client_ip", param.ClientIP),
		)
		return ""
	})
}

// metricsMiddleware tracks request metrics
func (s *Server) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		s.metrics.RequestCount.Inc(1)
		s.metrics.RequestDuration.Record(duration)

		if c.Writer.Status() >= 400 {
			s.metrics.RequestErrors.Inc(1)
		}

		path := c.FullPath()
		switch {
		case path == "/health":
			s.metrics.HealthCheckCount.Inc(1)
		case path == "/api/v1/agents/:id":
			s.metrics.AgentGetCount.Inc(1)
		case path == "/api/v1/agents":
			s.metrics.AgentListCount.Inc(1)
		case path == "/api/v1/groups/:id":
			s.metrics.GroupGetCount.Inc(1)
		case path == "/api/v1/groups":
			if c.Request.Method == "GET" {
				s.metrics.GroupListCount.Inc(1)
			} else if c.Request.Method == "POST" {
				s.metrics.GroupCreateCount.Inc(1)
			}
		case path == "/api/v1/configs/:id":
			s.metrics.ConfigGetCount.Inc(1)
		case path == "/api/v1/configs":
			if c.Request.Method == "GET" {
				s.metrics.ConfigListCount.Inc(1)
			} else if c.Request.Method == "POST" {
				s.metrics.ConfigCreateCount.Inc(1)
			}
		}
	}
}
