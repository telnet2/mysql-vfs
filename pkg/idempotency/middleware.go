package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
)

const (
	RequestIDHeader    = "X-Request-ID"
	DefaultIdempotencyTTL = 24 * time.Hour
)

// For backwards compatibility - will be deprecated
var IdempotencyTTL = DefaultIdempotencyTTL

// Service handles idempotency logic
type Service struct {
	db  *gorm.DB
	ttl time.Duration
}

// NewService creates a new idempotency service with default 24-hour TTL
func NewService(db *gorm.DB) *Service {
	return &Service{
		db:  db,
		ttl: DefaultIdempotencyTTL,
	}
}

// NewServiceWithTTL creates a new idempotency service with custom TTL (useful for testing)
func NewServiceWithTTL(db *gorm.DB, ttl time.Duration) *Service {
	return &Service{
		db:  db,
		ttl: ttl,
	}
}

// Middleware provides idempotency checking for mutation operations
func (s *Service) Middleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// Only apply to mutation operations (POST, PUT, DELETE)
		method := string(c.Method())
		if method != "POST" && method != "PUT" && method != "DELETE" {
			c.Next(ctx)
			return
		}

		// Extract request ID from header
		requestID := string(c.Request.Header.Peek(RequestIDHeader))
		if requestID == "" {
			c.JSON(consts.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Missing %s header for mutation operation", RequestIDHeader),
			})
			c.Abort()
			return
		}

		// Validate UUID format
		if _, err := uuid.Parse(requestID); err != nil {
			c.JSON(consts.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Invalid %s format, must be UUIDv4", RequestIDHeader),
			})
			c.Abort()
			return
		}

		// Check if request has been processed before
		var record models.IdempotencyRecord
		err := s.db.Where("request_id = ?", requestID).First(&record).Error
		if err == nil {
			// Request already processed, return cached response
			c.Data(consts.StatusOK, "application/json", []byte(record.ResponseBody))
			c.Abort()
			return
		} else if err != gorm.ErrRecordNotFound {
			// Database error
			c.JSON(consts.StatusInternalServerError, map[string]string{
				"error": "Failed to check idempotency",
			})
			c.Abort()
			return
		}

		// Store request ID in context for later use
		c.Set("request_id", requestID)

		// Continue processing
		c.Next(ctx)
	}
}

// CacheResponse stores the response for future idempotency checks
func (s *Service) CacheResponse(requestID string, response interface{}) error {
	// Serialize response
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// Calculate hash
	hash := sha256.Sum256(responseBytes)
	hashStr := hex.EncodeToString(hash[:])

	// Store in database
	record := models.IdempotencyRecord{
		RequestID:    requestID,
		ResponseHash: hashStr,
		ResponseBody: string(responseBytes),
		ExpiresAt:    time.Now().Add(s.ttl),
		CreatedAt:    time.Now(),
	}

	if err := s.db.Create(&record).Error; err != nil {
		return fmt.Errorf("failed to create idempotency record: %w", err)
	}

	return nil
}

// GetRequestID extracts request ID from context
func GetRequestID(c *app.RequestContext) string {
	if val, exists := c.Get("request_id"); exists {
		if requestID, ok := val.(string); ok {
			return requestID
		}
	}
	return ""
}

// CleanupExpired removes expired idempotency records
func (s *Service) CleanupExpired() error {
	result := s.db.Where("expires_at < ?", time.Now()).Delete(&models.IdempotencyRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup expired records: %w", result.Error)
	}
	return nil
}

// StartCleanupWorker starts a background worker that periodically cleans up expired records
func (s *Service) StartCleanupWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.CleanupExpired(); err != nil {
				// Log error but continue
				fmt.Printf("Idempotency cleanup error: %v\n", err)
			}
		}
	}
}
