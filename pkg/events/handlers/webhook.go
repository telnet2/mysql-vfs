package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/events"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	CircuitStateClosed CircuitState = iota
	CircuitStateOpen
	CircuitStateHalfOpen
)

// CircuitBreaker implements circuit breaker pattern for webhooks
type CircuitBreaker struct {
	mu                sync.RWMutex
	state             CircuitState
	failureCount      int
	failureThreshold  int
	recoveryTimeout   time.Duration
	nextRetryTime     time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold int, recoveryTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitStateClosed,
		failureThreshold: failureThreshold,
		recoveryTimeout:  recoveryTimeout,
	}
}

// CanExecute checks if the circuit allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitStateClosed:
		return true
	case CircuitStateOpen:
		// Check if recovery timeout has passed
		if time.Now().After(cb.nextRetryTime) {
			return true // Move to half-open on next attempt
		}
		return false
	case CircuitStateHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful execution
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = CircuitStateClosed
}

// RecordFailure records a failed execution
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++

	if cb.state == CircuitStateHalfOpen {
		// Failed in half-open state, go back to open
		cb.state = CircuitStateOpen
		cb.nextRetryTime = time.Now().Add(cb.recoveryTimeout)
		return
	}

	if cb.failureCount >= cb.failureThreshold {
		// Reached threshold, open the circuit
		cb.state = CircuitStateOpen
		cb.nextRetryTime = time.Now().Add(cb.recoveryTimeout)
	}
}

// OnAttempt is called before attempting execution
func (cb *CircuitBreaker) OnAttempt() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitStateOpen && time.Now().After(cb.nextRetryTime) {
		// Move to half-open to test if service recovered
		cb.state = CircuitStateHalfOpen
	}
}

// WebhookHandler handles webhook events
type WebhookHandler struct {
	client          *http.Client
	circuitBreakers sync.Map // map[handlerName]*CircuitBreaker
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{
		client: &http.Client{
			Timeout: 30 * time.Second, // Default timeout
		},
	}
}

// Type returns the handler type
func (h *WebhookHandler) Type() events.HandlerType {
	return events.HandlerTypeWebhook
}

// Handle processes a webhook event
func (h *WebhookHandler) Handle(ctx context.Context, handler *events.EventHandler, payload interface{}) error {
	// Parse webhook config
	configBytes, err := json.Marshal(handler.Config)
	if err != nil {
		return fmt.Errorf("invalid webhook config: %w", err)
	}

	var config events.WebhookConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return fmt.Errorf("invalid webhook config: %w", err)
	}

	// Validate config
	if config.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Get or create circuit breaker
	var cb *CircuitBreaker
	if config.CircuitBreaker != nil && config.CircuitBreaker.Enabled {
		cbInterface, _ := h.circuitBreakers.LoadOrStore(handler.Name, NewCircuitBreaker(
			config.CircuitBreaker.FailureThreshold,
			time.Duration(config.CircuitBreaker.RecoveryTimeoutMs)*time.Millisecond,
		))
		cb = cbInterface.(*CircuitBreaker)

		// Check if circuit allows execution
		cb.OnAttempt()
		if !cb.CanExecute() {
			return fmt.Errorf("circuit breaker is open for handler %s", handler.Name)
		}
	}

	// Execute webhook with retries
	err = h.executeWithRetry(ctx, &config, payload)

	// Update circuit breaker
	if cb != nil {
		if err != nil {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
	}

	return err
}

// executeWithRetry executes the webhook with retry logic
func (h *WebhookHandler) executeWithRetry(ctx context.Context, config *events.WebhookConfig, payload interface{}) error {
	maxAttempts := 1
	if config.Retry != nil {
		maxAttempts = config.Retry.MaxAttempts
		if maxAttempts < 1 {
			maxAttempts = 1
		}
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Add delay before retry (except for first attempt)
		if attempt > 0 && config.Retry != nil {
			delay := h.calculateDelay(attempt, config.Retry)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Execute webhook
		err := h.executeWebhook(ctx, config, payload)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !h.isRetryable(err) {
			return err
		}

		log.Printf("webhook attempt %d/%d failed: %v", attempt+1, maxAttempts, err)
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", maxAttempts, lastErr)
}

// calculateDelay calculates the delay before next retry
func (h *WebhookHandler) calculateDelay(attempt int, retry *events.RetryConfig) time.Duration {
	initialDelay := time.Duration(retry.InitialDelayMs) * time.Millisecond
	maxDelay := time.Duration(retry.MaxDelayMs) * time.Millisecond
	if maxDelay == 0 {
		maxDelay = 60 * time.Second // Default max delay
	}

	var delay time.Duration
	switch retry.Backoff {
	case events.BackoffTypeExponential:
		// Exponential: 1s, 2s, 4s, 8s, ...
		delay = initialDelay * time.Duration(1<<uint(attempt-1))
	case events.BackoffTypeLinear:
		// Linear: 1s, 2s, 3s, 4s, ...
		delay = initialDelay * time.Duration(attempt)
	default:
		delay = initialDelay
	}

	// Cap at max delay
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// isRetryable checks if an error is retryable
func (h *WebhookHandler) isRetryable(err error) bool {
	// Network errors are retryable
	// 4xx errors (except 429) are not retryable
	// 5xx errors are retryable
	// For now, retry all errors (could be made smarter)
	return true
}

// executeWebhook executes a single webhook request
func (h *WebhookHandler) executeWebhook(ctx context.Context, config *events.WebhookConfig, payload interface{}) error {
	// Marshal payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create request
	method := config.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequestWithContext(ctx, method, config.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VFS-Webhook/1.0")
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Add HMAC signature if secret is provided
	if config.Secret != "" {
		signature := h.generateHMAC(payloadBytes, config.Secret)
		req.Header.Set("X-VFS-Signature", "sha256="+signature)
	}

	// Set timeout
	client := h.client
	if config.TimeoutMs > 0 {
		client = &http.Client{
			Timeout: time.Duration(config.TimeoutMs) * time.Millisecond,
		}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (for logging)
	body, _ := io.ReadAll(resp.Body)

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// generateHMAC generates HMAC-SHA256 signature
func (h *WebhookHandler) generateHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
