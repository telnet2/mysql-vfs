package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	// Set test environment variables
	os.Setenv("DB_DSN", "test:test@tcp(localhost:3306)/testdb")
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("S3_REGION", "us-west-2")
	os.Setenv("LOG_LEVEL", "debug")
	
	defer func() {
		os.Unsetenv("DB_DSN")
		os.Unsetenv("S3_BUCKET")
		os.Unsetenv("S3_REGION")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg := LoadFromEnv()
	
	if cfg.DatabaseDSN != "test:test@tcp(localhost:3306)/testdb" {
		t.Errorf("Expected DatabaseDSN to be 'test:test@tcp(localhost:3306)/testdb', got '%s'", cfg.DatabaseDSN)
	}
	
	if cfg.S3Bucket != "test-bucket" {
		t.Errorf("Expected S3Bucket to be 'test-bucket', got '%s'", cfg.S3Bucket)
	}
	
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel to be 'debug', got '%s'", cfg.LogLevel)
	}
}

func TestExpandEnvVars(t *testing.T) {
	os.Setenv("TEST_SECRET", "my-secret-value")
	defer os.Unsetenv("TEST_SECRET")

	cfg := &Config{
		Auth: AuthConfig{
			JWTSecret: "${TEST_SECRET}",
		},
	}
	
	expandEnvVars(cfg)
	
	if cfg.Auth.JWTSecret != "my-secret-value" {
		t.Errorf("Expected JWTSecret to be 'my-secret-value', got '%s'", cfg.Auth.JWTSecret)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *Config
		shouldErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				DatabaseDSN: "root:root@tcp(localhost:3306)/vfs",
				S3Bucket:    "test-bucket",
				S3Region:    "us-east-1",
				S3Endpoint:  "http://localhost:4566",
				ServerPort:  "8080",
				Auth: AuthConfig{
					Provider: "headers",
				},
			},
			shouldErr: false,
		},
		{
			name: "missing database DSN",
			cfg: &Config{
				S3Bucket:   "test-bucket",
				S3Region:   "us-east-1",
				ServerPort: "8080",
				Auth: AuthConfig{
					Provider: "headers",
				},
			},
			shouldErr: true,
		},
		{
			name: "missing S3 bucket",
			cfg: &Config{
				DatabaseDSN: "root:root@tcp(localhost:3306)/vfs",
				S3Region:    "us-east-1",
				ServerPort:  "8080",
				Auth: AuthConfig{
					Provider: "headers",
				},
			},
			shouldErr: true,
		},
		{
			name: "JWT provider without secret",
			cfg: &Config{
				DatabaseDSN: "root:root@tcp(localhost:3306)/vfs",
				S3Bucket:    "test-bucket",
				S3Region:    "us-east-1",
				ServerPort:  "8080",
				Auth: AuthConfig{
					Provider:  "jwt",
					JWTSecret: "${JWT_SECRET}", // Not expanded
				},
			},
			shouldErr: true,
		},
		{
			name: "invalid auth provider",
			cfg: &Config{
				DatabaseDSN: "root:root@tcp(localhost:3306)/vfs",
				S3Bucket:    "test-bucket",
				S3Region:    "us-east-1",
				ServerPort:  "8080",
				Auth: AuthConfig{
					Provider: "invalid-provider",
				},
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if (err != nil) != tt.shouldErr {
				t.Errorf("ValidateConfig() error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

func TestMergeEnvVars(t *testing.T) {
	os.Setenv("DB_DSN", "env-dsn")
	os.Setenv("PORT", "9090")
	defer func() {
		os.Unsetenv("DB_DSN")
		os.Unsetenv("PORT")
	}()

	cfg := &Config{
		DatabaseDSN: "config-dsn",
		ServerPort:  "8080",
	}
	
	mergeEnvVars(cfg)
	
	// Environment variables should override config file values
	if cfg.DatabaseDSN != "env-dsn" {
		t.Errorf("Expected DatabaseDSN to be 'env-dsn', got '%s'", cfg.DatabaseDSN)
	}
	
	if cfg.ServerPort != "9090" {
		t.Errorf("Expected ServerPort to be '9090', got '%s'", cfg.ServerPort)
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		defValue time.Duration
		expected time.Duration
	}{
		{
			name:     "valid seconds",
			envValue: "3600",
			defValue: 1 * time.Hour,
			expected: 3600 * time.Second,
		},
		{
			name:     "invalid value uses default",
			envValue: "invalid",
			defValue: 5 * time.Minute,
			expected: 5 * time.Minute,
		},
		{
			name:     "empty uses default",
			envValue: "",
			defValue: 10 * time.Second,
			expected: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("TEST_DURATION", tt.envValue)
				defer os.Unsetenv("TEST_DURATION")
			}
			
			result := getEnvDuration("TEST_DURATION", tt.defValue)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
