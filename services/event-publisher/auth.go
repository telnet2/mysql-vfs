package main

import (
	"fmt"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims represents the JWT claims structure
type JWTClaims struct {
	UserID string   `json:"user_id"`
	Groups []string `json:"groups"`
	jwt.RegisteredClaims
}

// AuthMiddleware validates JWT tokens for SSE connections
type AuthMiddleware struct {
	jwtSecret string
	enabled   bool
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(jwtSecret string, enabled bool) *AuthMiddleware {
	return &AuthMiddleware{
		jwtSecret: jwtSecret,
		enabled:   enabled,
	}
}

// ValidateToken validates a JWT token and returns user context
func (m *AuthMiddleware) ValidateToken(c *app.RequestContext) (interface{}, error) {
	// If auth is disabled, allow all connections
	if !m.enabled || m.jwtSecret == "" {
		return map[string]interface{}{
			"user_id": "anonymous",
			"groups":  []string{"user"},
		}, nil
	}

	// Get token from Authorization header
	authHeader := string(c.Request.Header.Get("Authorization"))
	if authHeader == "" {
		// Try query parameter as fallback (for EventSource which can't set headers easily)
		token := string(c.Query("token"))
		if token != "" {
			return m.parseToken(token)
		}
		return nil, fmt.Errorf("missing authorization header or token query parameter")
	}

	// Extract Bearer token
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return nil, fmt.Errorf("invalid authorization header format, expected 'Bearer <token>'")
	}

	tokenString := strings.TrimPrefix(authHeader, bearerPrefix)
	if tokenString == "" {
		return nil, fmt.Errorf("empty token")
	}

	return m.parseToken(tokenString)
}

// parseToken parses and validates a JWT token
func (m *AuthMiddleware) parseToken(tokenString string) (map[string]interface{}, error) {
	// Parse and validate JWT
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(m.jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid JWT: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return map[string]interface{}{
		"user_id": claims.UserID,
		"groups":  claims.Groups,
	}, nil
}

// CheckPermission checks if user has permission to access events
// For now, just checks if user is authenticated
// In the future, could filter events based on user groups/permissions
func (m *AuthMiddleware) CheckPermission(claims interface{}, filter string) bool {
	// Cast to map
	claimsMap, ok := claims.(map[string]interface{})
	if !ok {
		return false
	}

	// For now, allow all authenticated users to access all events
	// Could add group-based filtering here:
	// - "system-admin" can see all events
	// - Regular users can only see events for their own files
	_ = claimsMap // Use claimsMap for future filtering
	return true
}
