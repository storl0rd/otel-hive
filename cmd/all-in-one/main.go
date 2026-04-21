package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/storl0rd/otel-hive/internal/api"
	"github.com/storl0rd/otel-hive/internal/config"
	"github.com/storl0rd/otel-hive/internal/metrics"
	"github.com/storl0rd/otel-hive/internal/opamp"
	"github.com/storl0rd/otel-hive/internal/services"
	"github.com/storl0rd/otel-hive/internal/storage/applicationstore"
	"github.com/storl0rd/otel-hive/internal/utils"
)

const (
	appName = "otel-hive"
	version = "0.1.0"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "otel-hive",
		Short: "otel-hive - OpAMP-based OTel Collector management",
		Long: `otel-hive is an OpAMP-based management plane for OpenTelemetry Collectors:
- Agent registration and lifecycle management via OpAMP protocol
- Collector configuration push and version control
- Agent grouping and bulk configuration
- REST API for management operations`,
		RunE: runOtelHive,
	}

	rootCmd.AddCommand(versionCommand())
	rootCmd.AddCommand(configCommand())

	rootCmd.PersistentFlags().String("config", "./otel-hive.yaml", "Path to configuration file")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "json", "Log format (json, console)")

	_ = viper.BindPFlags(rootCmd.PersistentFlags())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runOtelHive(cmd *cobra.Command, args []string) error {
	configPath := viper.GetString("config")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := utils.NewLogger(cfg.Logging.Level, cfg.Logging.Format)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting otel-hive",
		zap.String("version", version),
		zap.String("config", configPath))

	// Application store (SQLite — agent metadata, configs, groups)
	appStoreFactory, err := applicationstore.NewFactoryFromAppConfig(cfg)
	if err != nil {
		logger.Fatal("Failed to create application store factory", zap.Error(err))
	}
	if err := appStoreFactory.Initialize(logger); err != nil {
		logger.Fatal("Failed to initialize application store factory", zap.Error(err))
	}
	appStore, err := appStoreFactory.CreateApplicationStore()
	if err != nil {
		logger.Fatal("Failed to create application store", zap.Error(err))
	}
	defer func() {
		if err := appStoreFactory.Close(); err != nil {
			logger.Error("Failed to close application store factory", zap.Error(err))
		}
	}()

	registry := prometheus.NewRegistry()
	metricsFactory := metrics.NewPrometheusFactory("otel_hive", registry)
	opampMetrics := metrics.NewOpAMPMetrics(metricsFactory)

	agents := opamp.NewAgents(logger)

	agentService := services.NewAgentService(appStore, logger)
	configSender := opamp.NewConfigSender(agents, logger)

	opampServer, err := opamp.NewServer(agents, agentService, opampMetrics, "", "", logger)
	if err != nil {
		logger.Fatal("Failed to create OpAMP server", zap.Error(err))
	}

	if err := opampServer.Start(cfg.Server.OpAMPPort); err != nil {
		logger.Fatal("Failed to start OpAMP server", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = opampServer.Stop(ctx)
	}()

	apiServer := api.NewServer(agentService, configSender, logger)

	go func() {
		if err := apiServer.Start(fmt.Sprintf("%d", cfg.Server.HTTPPort)); err != nil {
			logger.Fatal("Failed to start API server", zap.Error(err))
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = apiServer.Stop(ctx)
	}()

	logger.Info("otel-hive is running",
		zap.Int("opamp_port", cfg.Server.OpAMPPort),
		zap.Int("api_port", cfg.Server.HTTPPort))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down otel-hive...")
	return nil
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s v%s\n", appName, version)
		},
	}
}

func configCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			configPath := viper.GetString("config")
			_, err := config.LoadConfig(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Configuration loaded from: %s\n", configPath)
		},
	}
}
