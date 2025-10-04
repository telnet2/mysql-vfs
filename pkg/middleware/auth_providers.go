package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/telnet2/mysql-vfs/pkg/config"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/repository"
)

// NewAuthExtractorFromConfig creates an auth extractor based on config
func NewAuthExtractorFromConfig(cfg config.AuthConfig, fileRepo repository.FileRepository, dirRepo repository.DirectoryRepository) (AuthExtractor, error) {
	// ALWAYS check super user first (hybrid auth)
	baseExtractor, err := createBaseExtractor(cfg, fileRepo, dirRepo)
	if err != nil {
		return nil, err
	}

	// Wrap with super user check
	return NewHybridAuthExtractor(cfg, baseExtractor), nil
}

func createBaseExtractor(cfg config.AuthConfig, fileRepo repository.FileRepository, dirRepo repository.DirectoryRepository) (AuthExtractor, error) {
	switch cfg.Provider {
	case "jwt":
		return NewJWTAuthExtractor(cfg.JWTSecret, cfg.JWTIssuer)

	case "oauth":
		return NewOAuthAuthExtractor(
			cfg.OAuthIntrospectionURL,
			cfg.OAuthClientID,
			cfg.OAuthClientSecret,
		)

	case "mtls":
		return NewMTLSAuthExtractor(cfg.MTLSCAFile, cfg.MTLSCertFile, cfg.MTLSKeyFile)

	case "proxy":
		return NewProxyAuthExtractor(cfg.ProxySharedSecret)

	case "file":
		// File-based auth using .user files
		return NewFileAuthExtractor(cfg, fileRepo, dirRepo)

	case "headers":
		// Development only - UNSAFE for production
		if cfg.AllowAnonymous {
			return NewHeaderAuthExtractor(true), nil
		}
		return NewHeaderAuthExtractor(false), nil

	default:
		return nil, fmt.Errorf("unknown auth provider: %s (valid: jwt, oauth, mtls, proxy, file, headers)", cfg.Provider)
	}
}

// ============================================================================
// JWT Auth Extractor
// ============================================================================

type JWTClaims struct {
	UserID string   `json:"user_id"`
	Role   string   `json:"role"`
	Groups []string `json:"groups"`
	jwt.RegisteredClaims
}

// NewJWTAuthExtractor creates a JWT-based auth extractor
func NewJWTAuthExtractor(secretKey, issuer string) (AuthExtractor, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("JWT secret key is required")
	}

	return func(tokenString string) (AuthContext, error) {
		// Parse and validate JWT
		token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			// Verify signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secretKey), nil
		})

		if err != nil {
			return AuthContext{}, fmt.Errorf("invalid JWT: %w", err)
		}

		claims, ok := token.Claims.(*JWTClaims)
		if !ok || !token.Valid {
			return AuthContext{}, fmt.Errorf("invalid token claims")
		}

		// Verify issuer if specified
		if issuer != "" && claims.Issuer != issuer {
			return AuthContext{}, fmt.Errorf("invalid issuer: expected %s, got %s", issuer, claims.Issuer)
		}

		return AuthContext{
			UserID: claims.UserID,
			Role:   claims.Role,
			Groups: claims.Groups,
		}, nil
	}, nil
}

// ============================================================================
// OAuth Auth Extractor
// ============================================================================

// NewOAuthAuthExtractor creates an OAuth-based auth extractor
func NewOAuthAuthExtractor(introspectionURL, clientID, clientSecret string) (AuthExtractor, error) {
	if introspectionURL == "" {
		return nil, fmt.Errorf("OAuth introspection URL is required")
	}

	return func(tokenString string) (AuthContext, error) {
		// TODO: Implement OAuth token introspection
		// Call introspectionURL with token, verify with clientID/clientSecret
		return AuthContext{}, fmt.Errorf("OAuth not implemented yet")
	}, nil
}

// ============================================================================
// mTLS Auth Extractor
// ============================================================================

// NewMTLSAuthExtractor creates an mTLS-based auth extractor
func NewMTLSAuthExtractor(caFile, certFile, keyFile string) (AuthExtractor, error) {
	// TODO: Implement mTLS certificate verification
	return func(tokenString string) (AuthContext, error) {
		return AuthContext{}, fmt.Errorf("mTLS not implemented yet")
	}, nil
}

// ============================================================================
// Reverse Proxy Auth Extractor (with HMAC signature)
// ============================================================================

// NewProxyAuthExtractor creates a reverse proxy auth extractor with HMAC verification
func NewProxyAuthExtractor(sharedSecret string) (AuthExtractor, error) {
	if sharedSecret == "" {
		return nil, fmt.Errorf("proxy shared secret is required")
	}

	return func(tokenString string) (AuthContext, error) {
		// Token format: "userID:role:groups:timestamp:signature"
		parts := strings.Split(tokenString, ":")
		if len(parts) != 5 {
			return AuthContext{}, fmt.Errorf("invalid proxy token format")
		}

		userID, role, groupsStr, timestampStr, signature := parts[0], parts[1], parts[2], parts[3], parts[4]

		// Verify signature (HMAC)
		message := fmt.Sprintf("%s:%s:%s:%s", userID, role, groupsStr, timestampStr)
		expectedSig := computeHMAC(message, sharedSecret)

		if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
			return AuthContext{}, fmt.Errorf("invalid signature")
		}

		// Check timestamp (prevent replay attacks - 5 minute window)
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			return AuthContext{}, fmt.Errorf("invalid timestamp")
		}

		if time.Now().Unix()-timestamp > 300 {
			return AuthContext{}, fmt.Errorf("token expired")
		}

		groups := []string{}
		if groupsStr != "" {
			groups = strings.Split(groupsStr, ",")
		}

		return AuthContext{
			UserID: userID,
			Role:   role,
			Groups: groups,
		}, nil
	}, nil
}

func computeHMAC(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// ============================================================================
// Header-Based Auth Extractor (Development Only)
// ============================================================================

// NewHeaderAuthExtractor creates a header-based extractor (UNSAFE - dev only!)
func NewHeaderAuthExtractor(allowAnonymous bool) AuthExtractor {
	return func(tokenString string) (AuthContext, error) {
		// This extractor is NOT called with a token - it's used differently
		// The auth middleware should handle this case specially

		if allowAnonymous {
			return AuthContext{
				UserID: "anonymous",
				Role:   "user",
				Groups: []string{},
			}, nil
		}

		return AuthContext{}, fmt.Errorf("header-based auth requires headers to be set by middleware")
	}
}

// ============================================================================
// Hybrid Auth Extractor (Super User + Base Provider)
// ============================================================================

// NewHybridAuthExtractor wraps any auth extractor with super user check
func NewHybridAuthExtractor(cfg config.AuthConfig, baseExtractor AuthExtractor) AuthExtractor {
	return func(tokenString string) (AuthContext, error) {
		// 1. Check if token matches super user token
		if cfg.SuperUserToken != "" && tokenString == cfg.SuperUserToken {
			return AuthContext{
				UserID: cfg.SuperUserID,
				Role:   cfg.SuperUserRole,
				Groups: []string{"super-admins"},
				Metadata: map[string]interface{}{
					"auth_type": "super_user",
				},
			}, nil
		}

		// 2. Fall back to base extractor
		return baseExtractor(tokenString)
	}
}

// ============================================================================
// File-Based Auth Extractor (.user files)
// ============================================================================

// NewFileAuthExtractor creates a file-based auth extractor
func NewFileAuthExtractor(cfg config.AuthConfig, fileRepo repository.FileRepository, dirRepo repository.DirectoryRepository) (AuthExtractor, error) {
	userLoader := domain.NewUserLoader(fileRepo, dirRepo, cfg.UserCacheTTL)

	return func(tokenString string) (AuthContext, error) {
		// Try to authenticate using token
		user, err := userLoader.LoadUserByToken(context.Background(), cfg.FileAuthDirectory, tokenString)
		if err != nil {
			return AuthContext{}, fmt.Errorf("invalid token: %w", err)
		}

		return AuthContext{
			UserID: user.UserID,
			Role:   user.Role,
			Groups: user.Groups,
			Metadata: map[string]interface{}{
				"auth_type": "file_based",
			},
		}, nil
	}, nil
}
