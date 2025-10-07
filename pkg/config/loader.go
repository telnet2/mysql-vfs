package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// LoadConfig loads configuration from a YAML file with environment variable support
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Auto-discover config files in order of preference
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		
		// Search paths (in order)
		v.AddConfigPath(".")                      // Current directory
		v.AddConfigPath("./config")               // ./config subdirectory
		v.AddConfigPath("/etc/vfs")               // System config
		v.AddConfigPath("$HOME/.vfs")             // User home
	}

	// Enable environment variable support
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, fall back to env vars only
			return loadFromViperWithEnvOnly(v)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Set defaults
	v.SetDefault("database.table_prefix", "vfs_")
	v.SetDefault("logging.level", "info")
	v.SetDefault("server.port", "8080")
	v.SetDefault("idempotency.ttl", "24h")
	v.SetDefault("cache.schema_ttl", "5m")
	v.SetDefault("cache.policy_ttl", "5m")
	v.SetDefault("cache.quota_ttl", "5m")
	v.SetDefault("cache.user_ttl", "5m")
	v.SetDefault("auth.provider", "headers")
	v.SetDefault("auth.jwt.issuer", "mysql-vfs")
	v.SetDefault("auth.jwt.ttl", "24h")
	v.SetDefault("auth.system_admin.id", "system-admin")
	v.SetDefault("auth.file.directory", "/")

	// Manually extract config values from nested YAML structure
	cfg := &Config{
		DatabaseDSN:    v.GetString("database.dsn"),
		TablePrefix:    v.GetString("database.table_prefix"),
		LogLevel:       v.GetString("logging.level"),
		ServerPort:     v.GetString("server.port"),
		IdempotencyTTL: v.GetDuration("idempotency.ttl"),
		S3Bucket:       v.GetString("storage.s3.bucket"),
		S3Region:       v.GetString("storage.s3.region"),
		S3Endpoint:     v.GetString("storage.s3.endpoint"),
		SchemaCacheTTL: v.GetDuration("cache.schema_ttl"),
		PolicyCacheTTL: v.GetDuration("cache.policy_ttl"),
		QuotaCacheTTL:  v.GetDuration("cache.quota_ttl"),
		Auth: AuthConfig{
			Provider:              v.GetString("auth.provider"),
			JWTSecret:             v.GetString("auth.jwt.secret"),
			JWTIssuer:             v.GetString("auth.jwt.issuer"),
			JWTTTL:                v.GetDuration("auth.jwt.ttl"),
			OAuthIntrospectionURL: v.GetString("auth.oauth.introspection_url"),
			OAuthClientID:         v.GetString("auth.oauth.client_id"),
			OAuthClientSecret:     v.GetString("auth.oauth.client_secret"),
			ProxySharedSecret:     v.GetString("auth.proxy.shared_secret"),
			MTLSCAFile:            v.GetString("auth.mtls.ca_file"),
			MTLSCertFile:          v.GetString("auth.mtls.cert_file"),
			MTLSKeyFile:           v.GetString("auth.mtls.key_file"),
			AllowAnonymous:        v.GetBool("auth.allow_anonymous"),
			SystemAdminToken:      v.GetString("auth.system_admin.token"),
			SystemAdminID:         v.GetString("auth.system_admin.id"),
			FileAuthDirectory:     v.GetString("auth.file.directory"),
			UserCacheTTL:          v.GetDuration("cache.user_ttl"),
		},
	}

	// Expand environment variables in string fields
	expandEnvVars(cfg)

	// Validate configuration
	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// LoadConfigWithEnv loads config with environment variable fallback
// Priority: CLI flags > Env vars > Config file > Defaults
func LoadConfigWithEnv() (*Config, error) {
	configPath := os.Getenv("VFS_CONFIG_FILE")
	
	// Try to load from config file first
	cfg, err := LoadConfig(configPath)
	if err != nil {
		// If config file doesn't exist, fall back to env vars (backward compatibility)
		if os.IsNotExist(err) || strings.Contains(err.Error(), "Not Found") {
			return LoadFromEnv(), nil
		}
		return nil, err
	}

	// Merge with environment variables (env vars take precedence)
	mergeEnvVars(cfg)

	return cfg, nil
}

// loadFromViperWithEnvOnly creates config from env vars only when no config file exists
func loadFromViperWithEnvOnly(v *viper.Viper) (*Config, error) {
	// Fall back to env-only config
	return LoadFromEnv(), nil
}

// expandEnvVars expands ${VAR_NAME} syntax in config values
func expandEnvVars(cfg *Config) {
	cfg.DatabaseDSN = os.ExpandEnv(cfg.DatabaseDSN)
	cfg.S3Endpoint = os.ExpandEnv(cfg.S3Endpoint)
	cfg.S3Bucket = os.ExpandEnv(cfg.S3Bucket)
	cfg.S3Region = os.ExpandEnv(cfg.S3Region)
	
	cfg.Auth.JWTSecret = os.ExpandEnv(cfg.Auth.JWTSecret)
	cfg.Auth.JWTIssuer = os.ExpandEnv(cfg.Auth.JWTIssuer)
	cfg.Auth.OAuthIntrospectionURL = os.ExpandEnv(cfg.Auth.OAuthIntrospectionURL)
	cfg.Auth.OAuthClientID = os.ExpandEnv(cfg.Auth.OAuthClientID)
	cfg.Auth.OAuthClientSecret = os.ExpandEnv(cfg.Auth.OAuthClientSecret)
	cfg.Auth.ProxySharedSecret = os.ExpandEnv(cfg.Auth.ProxySharedSecret)
	cfg.Auth.MTLSCAFile = os.ExpandEnv(cfg.Auth.MTLSCAFile)
	cfg.Auth.MTLSCertFile = os.ExpandEnv(cfg.Auth.MTLSCertFile)
	cfg.Auth.MTLSKeyFile = os.ExpandEnv(cfg.Auth.MTLSKeyFile)
	cfg.Auth.SystemAdminToken = os.ExpandEnv(cfg.Auth.SystemAdminToken)
	cfg.Auth.SystemAdminID = os.ExpandEnv(cfg.Auth.SystemAdminID)
	cfg.Auth.FileAuthDirectory = os.ExpandEnv(cfg.Auth.FileAuthDirectory)
}

// mergeEnvVars overlays environment variables onto config (env vars win)
func mergeEnvVars(cfg *Config) {
	// Database
	if v := os.Getenv("DB_DSN"); v != "" {
		cfg.DatabaseDSN = v
	}
	if v := os.Getenv("TABLE_PREFIX"); v != "" {
		cfg.TablePrefix = v
	}
	
	// Server
	if v := os.Getenv("PORT"); v != "" {
		cfg.ServerPort = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	
	// Storage
	if v := os.Getenv("S3_ENDPOINT"); v != "" {
		cfg.S3Endpoint = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		cfg.S3Bucket = v
	}
	if v := os.Getenv("S3_REGION"); v != "" {
		cfg.S3Region = v
	}
	
	// Auth
	if v := os.Getenv("AUTH_PROVIDER"); v != "" {
		cfg.Auth.Provider = v
	}
	if v := os.Getenv("AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" && cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("SYSTEM_ADMIN_TOKEN"); v != "" {
		cfg.Auth.SystemAdminToken = v
	}
}

// DiscoverConfigFile finds the first available config file
func DiscoverConfigFile() (string, error) {
	// Check environment variable first
	if configPath := os.Getenv("VFS_CONFIG_FILE"); configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
	}

	// Search in standard locations
	searchPaths := []string{
		"config.yaml",
		"config.yml",
		"./config/config.yaml",
		"./config/config.yml",
		"/etc/vfs/config.yaml",
		"/etc/vfs/config.yml",
	}

	// Add home directory paths
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths,
			filepath.Join(home, ".vfs", "config.yaml"),
			filepath.Join(home, ".vfs", "config.yml"),
		)
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no config file found in standard locations")
}
