package middleware

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// ContextKey is a custom type for context keys
type ContextKey string

const (
	// UserIDKey is the context key for user ID
	UserIDKey ContextKey = "user_id"

	// UserGroupsKey is the context key for user groups (includes special groups like "system-admin", "admin", "user")
	UserGroupsKey ContextKey = "user_groups"

	// UserMetadataKey is the context key for additional user metadata
	UserMetadataKey ContextKey = "user_metadata"
)

// AuthExtractor is a function type that extracts auth context from a token
// This allows external auth providers to inject their own token validation
type AuthExtractor func(token string) (AuthContext, error)

// AuthContext holds the authenticated user information
type AuthContext struct {
	UserID   string
	Groups   []string               // User's group memberships (e.g., ["admin"], ["user"], ["system-admin"])
	Metadata map[string]interface{} // Additional custom fields
}

// AuthMiddleware handles authentication via external providers
type AuthMiddleware struct {
	extractor AuthExtractor
	optional  bool
}

// NewAuthMiddleware creates a new authentication middleware
// The extractor function is provided by external auth systems (JWT, OAuth, etc.)
func NewAuthMiddleware(extractor AuthExtractor, optional bool) *AuthMiddleware {
	return &AuthMiddleware{
		extractor: extractor,
		optional:  optional,
	}
}

// Handler returns a Hertz middleware handler
func (m *AuthMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// Extract token from Authorization header
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			if m.optional {
				// If auth is optional, continue without user context
				c.Next(ctx)
				return
			}

			c.JSON(401, map[string]string{
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		// Check for "Bearer " prefix
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			c.JSON(401, map[string]string{
				"error": "invalid authorization header format, expected 'Bearer <token>'",
			})
			c.Abort()
			return
		}

		// Extract token
		tokenString := strings.TrimPrefix(authHeader, bearerPrefix)
		if tokenString == "" {
			c.JSON(401, map[string]string{
				"error": "empty token",
			})
			c.Abort()
			return
		}

		// Use the external auth extractor to validate and extract context
		authContext, err := m.extractor(tokenString)
		if err != nil {
			c.JSON(401, map[string]string{
				"error": "authentication failed",
			})
			c.Abort()
			return
		}

		// Store auth context in request context
		ctx = context.WithValue(ctx, UserIDKey, authContext.UserID)
		ctx = context.WithValue(ctx, UserGroupsKey, authContext.Groups)
		ctx = context.WithValue(ctx, UserMetadataKey, authContext.Metadata)

		c.Next(ctx)
	}
}

// RequireGroup returns a middleware that requires a specific group membership
func RequireGroup(requiredGroup string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		groups, ok := ctx.Value(UserGroupsKey).([]string)
		if !ok || len(groups) == 0 {
			c.JSON(401, map[string]string{
				"error": "authentication required",
			})
			c.Abort()
			return
		}

		// Check if user is in required group
		hasGroup := false
		for _, g := range groups {
			if g == requiredGroup {
				hasGroup = true
				break
			}
		}

		if !hasGroup {
			c.JSON(403, map[string]string{
				"error": "insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}

// RequireAdmin returns a middleware that requires admin group membership
func RequireAdmin() app.HandlerFunc {
	return RequireGroup("admin")
}

// GetUserID extracts the user ID from context
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok
}

// GetUserGroups extracts the user groups from context
func GetUserGroups(ctx context.Context) ([]string, bool) {
	groups, ok := ctx.Value(UserGroupsKey).([]string)
	return groups, ok
}

// HasGroup checks if user has a specific group
func HasGroup(ctx context.Context, group string) bool {
	groups, ok := GetUserGroups(ctx)
	if !ok {
		return false
	}
	for _, g := range groups {
		if g == group {
			return true
		}
	}
	return false
}

// GetUserMetadata extracts custom user metadata from context
func GetUserMetadata(ctx context.Context) (map[string]interface{}, bool) {
	metadata, ok := ctx.Value(UserMetadataKey).(map[string]interface{})
	return metadata, ok
}

// IsAdmin checks if the user in context is an admin
func IsAdmin(ctx context.Context) bool {
	return HasGroup(ctx, "admin")
}

// IsSystemAdmin checks if the user in context is a system admin
func IsSystemAdmin(ctx context.Context) bool {
	return HasGroup(ctx, "system-admin")
}
