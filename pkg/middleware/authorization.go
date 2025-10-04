package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/telnet2/mysql-vfs/pkg/services"
)

// UserContext represents the user making the request
type UserContext struct {
	UserID   string
	Username string
	Roles    []string
	Groups   []string
}

// AuthorizationMiddleware checks access permissions using OPA
type AuthorizationMiddleware struct {
	opaService     *services.OPAService
	timeout        time.Duration
	skipRoutes     map[string]bool // Routes that don't require authorization
	extractUserCtx func(*app.RequestContext) (*UserContext, error)
}

// AuthorizationConfig configures the authorization middleware
type AuthorizationConfig struct {
	OPAService     *services.OPAService
	Timeout        time.Duration
	SkipRoutes     []string
	ExtractUserCtx func(*app.RequestContext) (*UserContext, error)
}

// NewAuthorizationMiddleware creates a new authorization middleware
func NewAuthorizationMiddleware(config AuthorizationConfig) *AuthorizationMiddleware {
	if config.Timeout == 0 {
		config.Timeout = 200 * time.Millisecond
	}

	if config.ExtractUserCtx == nil {
		config.ExtractUserCtx = defaultUserContextExtractor
	}

	skipRoutes := make(map[string]bool)
	for _, route := range config.SkipRoutes {
		skipRoutes[route] = true
	}

	return &AuthorizationMiddleware{
		opaService:     config.OPAService,
		timeout:        config.Timeout,
		skipRoutes:     skipRoutes,
		extractUserCtx: config.ExtractUserCtx,
	}
}

// Handler returns a Hertz middleware handler
func (m *AuthorizationMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		route := string(c.FullPath())

		// Skip authorization for certain routes (e.g., health checks)
		if m.skipRoutes[route] {
			c.Next(ctx)
			return
		}

		// Extract user context from request
		userCtx, err := m.extractUserCtx(c)
		if err != nil {
			c.JSON(401, map[string]string{
				"error": "unauthorized: failed to extract user context",
			})
			c.Abort()
			return
		}

		// Extract resource path from request
		resourcePath := extractResourcePath(c)

		// Determine action from HTTP method and route
		action := determineAction(c)

		// Create context with timeout
		authCtx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		// Check authorization via OPA
		// Build input for OPA
		input := map[string]interface{}{
			"user": map[string]interface{}{
				"id":       userCtx.UserID,
				"username": userCtx.Username,
				"roles":    userCtx.Roles,
				"groups":   userCtx.Groups,
			},
			"resource": map[string]interface{}{
				"path": resourcePath,
				"type": determineResourceType(route),
			},
			"action": action,
		}

		allowed, err := m.opaService.Evaluate(authCtx, "vfs/authz/allow", input)
		if err != nil {
			// Fail closed on errors
			c.JSON(500, map[string]string{
				"error": fmt.Sprintf("authorization check failed: %v", err),
			})
			c.Abort()
			return
		}

		if !allowed {
			c.JSON(403, map[string]string{
				"error": "forbidden: access denied",
			})
			c.Abort()
			return
		}

		// Add authorization decision and user context to context
		ctx = context.WithValue(ctx, "authorized", true)
		ctx = context.WithValue(ctx, "user_context", userCtx)
		c.Next(ctx)
	}
}

// defaultUserContextExtractor is the default user context extractor
// In production, this should extract from JWT or session
func defaultUserContextExtractor(c *app.RequestContext) (*UserContext, error) {
	// Extract from headers (placeholder implementation)
	userID := string(c.GetHeader("X-User-ID"))
	username := string(c.GetHeader("X-Username"))

	if userID == "" {
		// For now, allow anonymous access with default user
		userID = "anonymous"
		username = "anonymous"
	}

	return &UserContext{
		UserID:   userID,
		Username: username,
		Roles:    []string{"user"},
		Groups:   []string{},
	}, nil
}

// extractResourcePath extracts the resource path from the request
func extractResourcePath(c *app.RequestContext) string {
	// Get path parameters
	path := string(c.Request.URI().Path())

	// Remove API prefix if present
	path = strings.TrimPrefix(path, "/api/v1")

	return path
}

// determineAction determines the action based on HTTP method and route
func determineAction(c *app.RequestContext) string {
	method := string(c.Method())
	route := string(c.FullPath())

	switch method {
	case "GET":
		return "read"
	case "POST":
		if strings.Contains(route, "/move") {
			return "move"
		}
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "unknown"
	}
}

// determineResourceType determines the resource type from the route
func determineResourceType(route string) string {
	if strings.Contains(route, "/files") {
		return "file"
	}
	if strings.Contains(route, "/directories") {
		return "directory"
	}
	return "unknown"
}
