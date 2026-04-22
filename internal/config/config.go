package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
	Auth    AuthConfig    `yaml:"auth"`
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
	HTTPPort  int `yaml:"http_port"`
	OpAMPPort int `yaml:"opamp_port"`
}

// StorageConfig contains storage configuration
type StorageConfig struct {
	App AppStorageConfig `yaml:"app"`
}

// AppStorageConfig contains app storage configuration
type AppStorageConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	// JWTSecret is used to sign JWT tokens. If empty, a random secret is
	// generated at startup and persisted in the database (survives restarts).
	JWTSecret           string `yaml:"jwt_secret"`
	AccessTokenExpiry   string `yaml:"access_token_expiry"`  // default "15m"
	RefreshTokenExpiry  string `yaml:"refresh_token_expiry"` // default "168h"
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort:  8080,
			OpAMPPort: 4320,
		},
		Storage: StorageConfig{
			App: AppStorageConfig{
				Type: "sqlite",
				Path: "./data/app.db",
			},
		},
		Auth: AuthConfig{
			AccessTokenExpiry:  "15m",
			RefreshTokenExpiry: "168h",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
