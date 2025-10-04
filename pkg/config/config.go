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
