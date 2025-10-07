package config

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateConfig validates the configuration for required fields and correctness
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	// Validate database configuration
	if err := validateDatabase(cfg); err != nil {
		return fmt.Errorf("database config: %w", err)
	}

	// Validate storage configuration
	if err := validateStorage(cfg); err != nil {
		return fmt.Errorf("storage config: %w", err)
	}

	// Validate auth configuration
	if err := validateAuth(cfg); err != nil {
		return fmt.Errorf("auth config: %w", err)
	}

	// Validate server configuration
	if err := validateServer(cfg); err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	return nil
}

func validateDatabase(cfg *Config) error {
	if cfg.DatabaseDSN == "" {
		return fmt.Errorf("database DSN is required")
	}

	// Basic DSN format validation
	if !strings.Contains(cfg.DatabaseDSN, "@tcp(") {
		return fmt.Errorf("invalid database DSN format (expected MySQL DSN)")
	}

	return nil
}

func validateStorage(cfg *Config) error {
	if cfg.S3Bucket == "" {
		return fmt.Errorf("S3 bucket is required")
	}

	if cfg.S3Region == "" {
		return fmt.Errorf("S3 region is required")
	}

	if cfg.S3Endpoint != "" {
		// Validate endpoint URL format
		if _, err := url.Parse(cfg.S3Endpoint); err != nil {
			return fmt.Errorf("invalid S3 endpoint URL: %w", err)
		}
	}

	return nil
}

func validateAuth(cfg *Config) error {
	validProviders := map[string]bool{
		"jwt":     true,
		"oauth":   true,
		"mtls":    true,
		"proxy":   true,
		"headers": true,
		"file":    true,
	}

	if cfg.Auth.Provider == "" {
		return fmt.Errorf("auth provider is required")
	}

	if !validProviders[cfg.Auth.Provider] {
		return fmt.Errorf("invalid auth provider '%s' (valid: jwt, oauth, mtls, proxy, headers, file)", cfg.Auth.Provider)
	}

	// Provider-specific validation
	switch cfg.Auth.Provider {
	case "jwt":
		if cfg.Auth.JWTSecret == "" || cfg.Auth.JWTSecret == "${JWT_SECRET}" {
			return fmt.Errorf("JWT secret is required when using jwt provider (set JWT_SECRET env var)")
		}
	case "oauth":
		if cfg.Auth.OAuthIntrospectionURL == "" {
			return fmt.Errorf("OAuth introspection URL is required when using oauth provider")
		}
		if cfg.Auth.OAuthClientID == "" {
			return fmt.Errorf("OAuth client ID is required when using oauth provider")
		}
		if cfg.Auth.OAuthClientSecret == "" || cfg.Auth.OAuthClientSecret == "${AUTH_OAUTH_CLIENT_SECRET}" {
			return fmt.Errorf("OAuth client secret is required when using oauth provider")
		}
	case "mtls":
		if cfg.Auth.MTLSCAFile == "" {
			return fmt.Errorf("mTLS CA file is required when using mtls provider")
		}
		if cfg.Auth.MTLSCertFile == "" {
			return fmt.Errorf("mTLS cert file is required when using mtls provider")
		}
		if cfg.Auth.MTLSKeyFile == "" {
			return fmt.Errorf("mTLS key file is required when using mtls provider")
		}
	case "proxy":
		if cfg.Auth.ProxySharedSecret == "" || cfg.Auth.ProxySharedSecret == "${AUTH_PROXY_SHARED_SECRET}" {
			return fmt.Errorf("proxy shared secret is required when using proxy provider")
		}
	case "file":
		if cfg.Auth.FileAuthDirectory == "" {
			return fmt.Errorf("file auth directory is required when using file provider")
		}
	}

	return nil
}

func validateServer(cfg *Config) error {
	if cfg.ServerPort == "" {
		return fmt.Errorf("server port is required")
	}

	return nil
}

// ValidateServiceConfig validates service-specific configuration
func ValidateServiceConfig(cfg *Config, serviceName string) error {
	// Common validation
	if err := ValidateConfig(cfg); err != nil {
		return err
	}

	// Service-specific validation would go here
	// For now, we just do common validation

	return nil
}
