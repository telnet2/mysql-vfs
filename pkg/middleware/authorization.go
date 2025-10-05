package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/open-policy-agent/opa/rego"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// UserContext represents the user making the request
type UserContext struct {
	UserID   string
	Username string
	Groups   []string // User's group memberships (e.g., ["admin"], ["user"], ["system-admin"])
}

// AuthorizationMiddleware checks access permissions using .rego policies
type AuthorizationMiddleware struct {
	policyLoader   *domain.PolicyLoader
	ownerLoader    *domain.OwnerLoader
	dirRepo        db.DirectoryRepository
	timeout        time.Duration
	skipRoutes     map[string]bool // Routes that don't require authorization
	extractUserCtx func(*app.RequestContext) (*UserContext, error)
}

// AuthorizationConfig configures the authorization middleware
type AuthorizationConfig struct {
	PolicyLoader   *domain.PolicyLoader
	OwnerLoader    *domain.OwnerLoader
	DirRepo        db.DirectoryRepository
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
		policyLoader:   config.PolicyLoader,
		ownerLoader:    config.OwnerLoader,
		dirRepo:        config.DirRepo,
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

		// System admin bypasses all rego authorization
		// Check if "system-admin" is in user's groups
		hasSystemAdmin := false
		for _, group := range userCtx.Groups {
			if group == "system-admin" {
				hasSystemAdmin = true
				break
			}
		}
		if hasSystemAdmin {
			ctx = context.WithValue(ctx, "authorized", true)
			ctx = context.WithValue(ctx, "user_context", userCtx)
			c.Next(ctx)
			return
		}

		// Extract resource path from request
		resourcePath := extractResourcePath(c)

		// Determine action from HTTP method and route
		action := determineAction(c)

		// Create context with timeout
		authCtx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		// Load .rego policy for the resource path
		// Extract directory path from resource path
		dirPath := extractDirectoryPath(resourcePath)

		// Load the .rego policy (with inheritance)
		regoPolicy, err := m.policyLoader.LoadPolicy(authCtx, dirPath)
		if err != nil {
			if err == domain.ErrNotFound {
				// No policy found - fall back to built-in default policy
				// This should only happen if bootstrap hasn't run yet
				// or if all .rego files have been deleted
				regoPolicy = DefaultRegoPolicy
				// Log warning
				fmt.Printf("WARNING: No .rego policy found for path %s, using built-in default. Run bootstrap to create /.rego\n", dirPath)
			} else {
				// Fail closed on other errors
				c.JSON(500, map[string]string{
					"error": fmt.Sprintf("failed to load authorization policy: %v", err),
				})
				c.Abort()
				return
			}
		}

		// Get ownership information for the directory
		var ownerGroups []string
		if m.ownerLoader != nil && m.dirRepo != nil {
			dir, err := m.dirRepo.FindByPath(authCtx, dirPath)
			if err == nil {
				ownerGroups, _ = m.ownerLoader.GetOwnerGroups(authCtx, dir.ID)
			}
		}

		// Build input for OPA evaluation
		input := map[string]interface{}{
			"user": map[string]interface{}{
				"id":       userCtx.UserID,
				"username": userCtx.Username,
				"groups":   userCtx.Groups,
			},
			"resource": map[string]interface{}{
				"path":   resourcePath,
				"type":   determineResourceType(route),
				"owners": ownerGroups,
			},
			"action": action,
		}

		// TODO: Implement actual REGO evaluation using OPA engine
		// For now, this is a placeholder that allows all requests
		// In the next phase, we'll use github.com/open-policy-agent/opa
		allowed := evaluateRegoPolicy(regoPolicy, input)

		if !allowed {
			c.JSON(403, map[string]string{
				"error": "forbidden: access denied by policy",
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

// defaultUserContextExtractor extracts user context from the request context
// This works with the generic auth middleware or external auth headers
func defaultUserContextExtractor(c *app.RequestContext) (*UserContext, error) {
	// Get context from Hertz request context
	ctx := c

	// Try to get from auth middleware context first
	userID, hasUserID := ctx.Value(UserIDKey).(string)
	groups, hasGroups := ctx.Value(UserGroupsKey).([]string)

	// If not found, try from headers (for external auth via reverse proxy)
	if !hasUserID {
		userID = string(c.GetHeader("X-User-ID"))
		if userID == "" {
			userID = "anonymous"
		}
	}

	// Get groups from header if not in context
	if !hasGroups || len(groups) == 0 {
		if groupHeader := string(c.GetHeader("X-User-Groups")); groupHeader != "" {
			groups = strings.Split(groupHeader, ",")
			// Trim whitespace from each group
			for i := range groups {
				groups[i] = strings.TrimSpace(groups[i])
			}
		} else {
			// Default to "user" group if no groups specified
			groups = []string{"user"}
		}
	}

	username := string(c.GetHeader("X-Username"))
	if username == "" {
		username = userID
	}

	return &UserContext{
		UserID:   userID,
		Username: username,
		Groups:   groups,
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

// extractDirectoryPath extracts the directory path from a resource path
func extractDirectoryPath(resourcePath string) string {
	// If the resource is a file, get its directory
	// For now, use simple logic - can be enhanced based on actual path structure
	if strings.Contains(resourcePath, "/files/") {
		// Extract directory from file path
		parts := strings.Split(resourcePath, "/files/")
		if len(parts) > 0 {
			return parts[0]
		}
	}

	// For directory operations, use the path directly
	if strings.Contains(resourcePath, "/directories/") {
		parts := strings.Split(resourcePath, "/directories/")
		if len(parts) > 1 {
			return "/" + parts[1]
		}
	}

	// Default to root
	return "/"
}

// evaluateRegoPolicy evaluates a Rego policy against input using OPA
func evaluateRegoPolicy(regoPolicy string, input map[string]interface{}) bool {
	// Create a new Rego query
	// The policy should define a rule named "allow" in the package "vfs.authz"
	ctx := context.Background()

	// Compile the Rego policy
	query, err := rego.New(
		rego.Query("data.vfs.authz.allow"),
		rego.Module("policy.rego", regoPolicy),
	).PrepareForEval(ctx)

	if err != nil {
		// Policy compilation failed - fail closed
		return false
	}

	// Evaluate the policy with the input
	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		// Evaluation failed - fail closed
		return false
	}

	// Check if the policy allows the request
	// The "allow" rule should return a boolean value
	if len(results) > 0 && len(results[0].Expressions) > 0 {
		allowed, ok := results[0].Expressions[0].Value.(bool)
		if ok {
			return allowed
		}
	}

	// Default deny if no clear allow decision
	return false
}
