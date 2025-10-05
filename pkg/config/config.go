package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds application configuration
type Config struct {
	// Database
	DatabaseDSN string
	LogLevel    string

	// Server
	ServerPort string

	// Idempotency
	IdempotencyTTL time.Duration

	// Storage
	S3Bucket   string
	S3Region   string
	S3Endpoint string

	// Authentication
	Auth AuthConfig

	// Special Files Cache
	SchemaCacheTTL time.Duration
	PolicyCacheTTL time.Duration
	QuotaCacheTTL  time.Duration
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// Provider type: jwt, oauth, mtls, proxy, headers (dev only), custom
	Provider string

	// JWT Configuration
	JWTSecret string
	JWTIssuer string
	JWTTTL    time.Duration

	// OAuth Configuration
	OAuthIntrospectionURL string
	OAuthClientID         string
	OAuthClientSecret     string

	// Proxy Configuration (reverse proxy with HMAC)
	ProxySharedSecret string

	// mTLS Configuration
	MTLSCAFile   string
	MTLSCertFile string
	MTLSKeyFile  string

	// Optional: allow anonymous access (no auth required)
	AllowAnonymous bool

	// System Admin (environment-based, bypasses all authorization)
	SystemAdminToken string // Token that grants system admin access
	SystemAdminID    string // User ID for system admin (default: "system-admin")

	// File-based auth (.user files)
	FileAuthDirectory string // Directory containing .user file (default: "/")
	UserCacheTTL      time.Duration
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	return &Config{
		DatabaseDSN:    getEnv("DB_DSN", "root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		ServerPort:     getEnv("PORT", "8080"),
		IdempotencyTTL: getEnvDuration("IDEMPOTENCY_TTL_SECONDS", 24*time.Hour),
		S3Bucket:       getEnv("S3_BUCKET", "cc-vfs-files"),
		S3Region:       getEnv("S3_REGION", "us-east-1"),
		S3Endpoint:     getEnv("S3_ENDPOINT", "http://localhost:4566"),

		// Authentication
		Auth: AuthConfig{
			Provider:              getEnv("AUTH_PROVIDER", "headers"), // headers=dev only, jwt=production, file=.user files
			JWTSecret:             getEnv("AUTH_JWT_SECRET", "change-me-in-production"),
			JWTIssuer:             getEnv("AUTH_JWT_ISSUER", "mysql-vfs"),
			JWTTTL:                getEnvDuration("AUTH_JWT_TTL_SECONDS", 24*time.Hour),
			OAuthIntrospectionURL: getEnv("AUTH_OAUTH_INTROSPECTION_URL", ""),
			OAuthClientID:         getEnv("AUTH_OAUTH_CLIENT_ID", ""),
			OAuthClientSecret:     getEnv("AUTH_OAUTH_CLIENT_SECRET", ""),
			ProxySharedSecret:     getEnv("AUTH_PROXY_SHARED_SECRET", ""),
			MTLSCAFile:            getEnv("AUTH_MTLS_CA_FILE", ""),
			MTLSCertFile:          getEnv("AUTH_MTLS_CERT_FILE", ""),
			MTLSKeyFile:           getEnv("AUTH_MTLS_KEY_FILE", ""),
			AllowAnonymous:        getEnvBool("AUTH_ALLOW_ANONYMOUS", false),
			SystemAdminToken:      getEnv("SYSTEM_ADMIN_TOKEN", ""),
			SystemAdminID:         getEnv("SYSTEM_ADMIN_ID", "system-admin"),
			FileAuthDirectory:     getEnv("FILE_AUTH_DIRECTORY", "/"),
			UserCacheTTL:          getEnvDuration("USER_CACHE_TTL_SECONDS", 5*time.Minute),
		},

		// Special Files Cache
		SchemaCacheTTL: getEnvDuration("SCHEMA_CACHE_TTL_SECONDS", 5*time.Minute),
		PolicyCacheTTL: getEnvDuration("POLICY_CACHE_TTL_SECONDS", 5*time.Minute),
		QuotaCacheTTL:  getEnvDuration("QUOTA_CACHE_TTL_SECONDS", 5*time.Minute),
	}
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvDuration retrieves a duration from environment variable (in seconds) or returns default
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultValue
}

// getEnvBool retrieves a boolean from environment variable or returns default
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
