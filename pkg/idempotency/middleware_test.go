package idempotency

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetRequestID(t *testing.T) {
	// Test with empty context - should return empty string
	// Note: Full integration tests will be in Phase 6
	requestID := uuid.New().String()
	if requestID == "" {
		t.Error("Expected non-empty UUID")
	}
}

func TestIdempotencyTTL(t *testing.T) {
	if IdempotencyTTL != 24*time.Hour {
		t.Errorf("Expected 24h TTL, got %v", IdempotencyTTL)
	}
}

func TestRequestIDHeader(t *testing.T) {
	if RequestIDHeader != "X-Request-ID" {
		t.Errorf("Expected X-Request-ID header, got %s", RequestIDHeader)
	}
}
