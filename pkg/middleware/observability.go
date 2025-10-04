package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// RequestIDMiddleware adds a unique request ID to each request
type RequestIDMiddleware struct {
	headerName string
}

// NewRequestIDMiddleware creates a new request ID middleware
func NewRequestIDMiddleware(headerName string) *RequestIDMiddleware {
	if headerName == "" {
		headerName = "X-Request-ID"
	}
	return &RequestIDMiddleware{
		headerName: headerName,
	}
}

// Handler returns a Hertz middleware handler
func (m *RequestIDMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// Check if request ID already exists in header
		requestID := string(c.GetHeader(m.headerName))
		if requestID == "" {
			// Generate new request ID
			requestID = uuid.New().String()
		}

		// Add to response headers
		c.Header(m.headerName, requestID)

		// Add to context
		ctx = context.WithValue(ctx, "request_id", requestID)
		ctx = context.WithValue(ctx, "requestID", requestID) // Alias for compatibility

		c.Next(ctx)
	}
}

// ObservabilityMiddleware provides logging and metrics for requests
type ObservabilityMiddleware struct {
	serviceName string
}

// NewObservabilityMiddleware creates a new observability middleware
func NewObservabilityMiddleware(serviceName string) *ObservabilityMiddleware {
	return &ObservabilityMiddleware{
		serviceName: serviceName,
	}
}

// Handler returns a Hertz middleware handler
func (m *ObservabilityMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()
		path := string(c.FullPath())
		method := string(c.Method())

		// Get request ID from context
		requestID, _ := ctx.Value("request_id").(string)

		// Log request start
		log.Debug().
			Str("service", m.serviceName).
			Str("request_id", requestID).
			Str("method", method).
			Str("path", path).
			Str("remote_addr", c.ClientIP()).
			Msg("request started")

		// Process request
		c.Next(ctx)

		// Calculate duration
		duration := time.Since(start)

		// Get status code
		statusCode := c.Response.StatusCode()

		// Log request completion
		logLevel := log.Info()
		if statusCode >= 500 {
			logLevel = log.Error()
		} else if statusCode >= 400 {
			logLevel = log.Warn()
		}

		logLevel.
			Str("service", m.serviceName).
			Str("request_id", requestID).
			Str("method", method).
			Str("path", path).
			Int("status", statusCode).
			Dur("duration", duration).
			Int("response_size", c.Response.Header.ContentLength()).
			Msg("request completed")

		// TODO: Record metrics (Prometheus)
		// metrics.RequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
		// metrics.RequestCount.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
	}
}

// RecoveryMiddleware recovers from panics and returns 500 error
type RecoveryMiddleware struct{}

// NewRecoveryMiddleware creates a new recovery middleware
func NewRecoveryMiddleware() *RecoveryMiddleware {
	return &RecoveryMiddleware{}
}

// Handler returns a Hertz middleware handler
func (m *RecoveryMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer func() {
			if err := recover(); err != nil {
				requestID, _ := ctx.Value("request_id").(string)

				log.Error().
					Str("request_id", requestID).
					Str("method", string(c.Method())).
					Str("path", string(c.FullPath())).
					Interface("error", err).
					Msg("panic recovered")

				c.JSON(500, map[string]string{
					"error":      "internal server error",
					"request_id": requestID,
				})
				c.Abort()
			}
		}()

		c.Next(ctx)
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing
type CORSMiddleware struct {
	allowedOrigins []string
	allowedMethods []string
	allowedHeaders []string
	maxAge         int
}

// CORSConfig configures CORS middleware
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

// NewCORSMiddleware creates a new CORS middleware
func NewCORSMiddleware(config CORSConfig) *CORSMiddleware {
	if len(config.AllowedOrigins) == 0 {
		config.AllowedOrigins = []string{"*"}
	}
	if len(config.AllowedMethods) == 0 {
		config.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}
	}
	if len(config.AllowedHeaders) == 0 {
		config.AllowedHeaders = []string{"Content-Type", "Authorization", "X-Request-ID"}
	}
	if config.MaxAge == 0 {
		config.MaxAge = 86400 // 24 hours
	}

	return &CORSMiddleware{
		allowedOrigins: config.AllowedOrigins,
		allowedMethods: config.AllowedMethods,
		allowedHeaders: config.AllowedHeaders,
		maxAge:         config.MaxAge,
	}
}

// Handler returns a Hertz middleware handler
func (m *CORSMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		origin := string(c.GetHeader("Origin"))

		// Set CORS headers
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight requests
		if string(c.Method()) == "OPTIONS" {
			c.Header("Access-Control-Allow-Methods", joinStrings(m.allowedMethods, ", "))
			c.Header("Access-Control-Allow-Headers", joinStrings(m.allowedHeaders, ", "))
			c.Header("Access-Control-Max-Age", strconv.Itoa(m.maxAge))
			c.Status(204)
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}

// joinStrings joins strings with a separator
func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// TimeoutMiddleware enforces a timeout on request processing
type TimeoutMiddleware struct {
	timeout time.Duration
}

// NewTimeoutMiddleware creates a new timeout middleware
func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &TimeoutMiddleware{
		timeout: timeout,
	}
}

// Handler returns a Hertz middleware handler
func (m *TimeoutMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// Create context with timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		// Channel to signal completion
		done := make(chan struct{})

		go func() {
			c.Next(timeoutCtx)
			close(done)
		}()

		select {
		case <-done:
			// Request completed successfully
			return
		case <-timeoutCtx.Done():
			// Timeout occurred
			requestID, _ := ctx.Value("request_id").(string)

			log.Warn().
				Str("request_id", requestID).
				Str("method", string(c.Method())).
				Str("path", string(c.FullPath())).
				Dur("timeout", m.timeout).
				Msg("request timeout")

			c.JSON(504, map[string]string{
				"error":      fmt.Sprintf("request timeout after %v", m.timeout),
				"request_id": requestID,
			})
			c.Abort()
		}
	}
}
