package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
)

var (
	// ErrImpersonationNotAllowed is returned when user cannot impersonate
	ErrImpersonationNotAllowed = errors.New("user not authorized to impersonate")
)

// DelegationMiddleware extracts and validates on-behalf-of delegation headers
type DelegationMiddleware struct {
	// Future: could add policy loader for advanced validation
}

// NewDelegationMiddleware creates a new delegation middleware
func NewDelegationMiddleware() *DelegationMiddleware {
	return &DelegationMiddleware{}
}

// Handler returns a Hertz middleware handler for delegation
func (m *DelegationMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// Extract authenticated user from previous auth middleware
		userID, hasUser := GetUserID(ctx)
		groups, _ := GetUserGroups(ctx)

		if !hasUser {
			// No authenticated user - continue without delegation
			c.Next(ctx)
			return
		}

		// Create base auth context
		authCtx := &AuthContext{
			UserID:    userID,
			Groups:    groups,
			RequestID: getOrCreateRequestID(c),
		}

		// Check for delegation header
		principalUserID := string(c.Request.Header.Peek("X-VFS-On-Behalf-Of"))

		if principalUserID == "" {
			// No delegation - actor is acting for themselves
			ctx = context.WithValue(ctx, "authContext", authCtx)
			c.Next(ctx)
			return
		}

		// CRITICAL SECURITY CHECK
		// Verify actor has permission to impersonate principal
		if err := validateImpersonation(userID, principalUserID, groups); err != nil {
			// LOG SECURITY EVENT - potential attack attempt
			logSecurityEvent(c, "impersonation_denied", map[string]interface{}{
				"actor":      userID,
				"principal":  principalUserID,
				"reason":     err.Error(),
				"remote_ip":  c.ClientIP(),
				"user_agent": string(c.Request.Header.UserAgent()),
			})

			c.JSON(403, map[string]interface{}{
				"error":   "impersonation denied",
				"message": err.Error(),
			})
			c.Abort()
			return
		}

		// ONLY NOW DO WE TRUST THE HEADER
		authCtx.PrincipalUserID = principalUserID
		authCtx.DelegationReason = string(c.Request.Header.Peek("X-VFS-Delegation-Reason"))

		// LOG SUCCESSFUL DELEGATION for audit trail
		logSecurityEvent(c, "impersonation_granted", map[string]interface{}{
			"actor":     userID,
			"principal": principalUserID,
			"reason":    authCtx.DelegationReason,
		})

		// Store auth context in request context
		ctx = context.WithValue(ctx, "authContext", authCtx)
		c.Next(ctx)
	}
}

// validateImpersonation enforces delegation authorization
func validateImpersonation(actor, principal string, groups []string) error {
	// Prevent self-impersonation (no-op, but flag suspicious behavior)
	if actor == principal {
		return nil // Allow self-impersonation (no-op)
	}

	// Check if actor has impersonation permission
	hasImpersonatePermission := false
	for _, group := range groups {
		if group == "service-accounts" || group == "system-admin" {
			hasImpersonatePermission = true
			break
		}
	}

	if !hasImpersonatePermission {
		return ErrImpersonationNotAllowed
	}

	// TODO: Future enhancements:
	// - Policy-based authorization (Rego integration)
	// - Explicit allow-list (actor->principal mappings)
	// - Resource-scoped delegation
	// - Time-limited delegation

	return nil
}

// logSecurityEvent logs authentication/authorization events for audit
func logSecurityEvent(c *app.RequestContext, eventType string, details map[string]interface{}) {
	entry := map[string]interface{}{
		"timestamp":  time.Now().Format(time.RFC3339),
		"event_type": eventType,
		"request_id": getOrCreateRequestID(c),
		"path":       string(c.Request.Path()),
		"method":     string(c.Request.Method()),
	}

	for k, v := range details {
		entry[k] = v
	}

	logJSON, _ := json.Marshal(entry)
	log.Printf("SECURITY: %s", string(logJSON))
}

// getOrCreateRequestID gets or creates a request ID
func getOrCreateRequestID(c *app.RequestContext) string {
	// Check X-Request-ID header
	if reqID := c.Request.Header.Peek("X-Request-ID"); len(reqID) > 0 {
		return string(reqID)
	}

	// Generate new request ID
	return uuid.New().String()
}
