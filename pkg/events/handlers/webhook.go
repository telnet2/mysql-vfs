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

// Handle processes a webhook event and returns HandlerResponse
func (h *WebhookHandler) Handle(ctx context.Context, handler *events.EventHandler, payload interface{}) events.HandlerResponse {
	// Parse webhook config
	configBytes, err := json.Marshal(handler.Config)
	if err != nil {
		return events.ErrorResponse(fmt.Sprintf("invalid webhook config: %v", err))
	}

	var config events.WebhookConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return events.ErrorResponse(fmt.Sprintf("invalid webhook config: %v", err))
	}

	// Validate config
	if config.URL == "" {
		return events.ErrorResponse("webhook URL is required")
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
			msg := fmt.Sprintf("circuit breaker is open for handler %s", handler.Name)
			// Check on_error config
			if handler.IsVetoEnabled() && config.OnError == "abort" {
				return events.VetoResponse(msg, "circuit_open")
			}
			return events.ErrorResponse(msg)
		}
	}

	// Execute webhook with retries
	response := h.executeWithRetry(ctx, handler, &config, payload)

	// Update circuit breaker
	if cb != nil {
		if response.Success {
			cb.RecordSuccess()
		} else {
			cb.RecordFailure()
		}
	}

	return response
}

// executeWithRetry executes the webhook with retry logic
func (h *WebhookHandler) executeWithRetry(ctx context.Context, handler *events.EventHandler, config *events.WebhookConfig, payload interface{}) events.HandlerResponse {
	maxAttempts := 1
	if config.Retry != nil {
		maxAttempts = config.Retry.MaxAttempts
		if maxAttempts < 1 {
			maxAttempts = 1
		}
	}

	var lastResponse events.HandlerResponse
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Add delay before retry (except for first attempt)
		if attempt > 0 && config.Retry != nil {
			delay := h.calculateDelay(attempt, config.Retry)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				// Context timeout/cancellation
				if handler.IsVetoEnabled() && config.OnTimeout == "abort" {
					return events.VetoResponse("request timeout", "timeout")
				}
				return events.ErrorResponse(ctx.Err().Error())
			}
		}

		// Execute webhook
		response := h.executeWebhook(ctx, handler, config, payload)
		if response.Success {
			return response
		}

		lastResponse = response

		// If handler vetoed, don't retry
		if response.Veto {
			return response
		}

		// Check if error is retryable
		if !h.isRetryableResponse(response) {
			return response
		}

		// Log retry attempt (only if we have more attempts left)
		if attempt+1 < maxAttempts {
			log.Printf("webhook attempt %d/%d failed: %s", attempt+1, maxAttempts, response.Message)
		}
	}

	// All retries failed
	msg := fmt.Sprintf("webhook failed after %d attempts: %s", maxAttempts, lastResponse.Message)
	if handler.IsVetoEnabled() && config.OnError == "abort" {
		return events.VetoResponse(msg, "max_retries_exceeded")
	}
	return events.ErrorResponse(msg)
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

// isRetryableResponse checks if a response is retryable
func (h *WebhookHandler) isRetryableResponse(response events.HandlerResponse) bool {
	// Veto responses are never retried
	if response.Veto {
		return false
	}

	// Check error code to determine if retryable
	// Network errors, 5xx errors, and timeouts are retryable
	// 4xx errors (client errors) are not retryable
	switch response.Code {
	case "timeout", "network_error", "server_error":
		return true
	case "client_error", "forbidden", "unauthorized":
		return false
	default:
		// Unknown errors are retryable by default
		return true
	}
}

// WebhookResponse represents the response from a webhook endpoint
type WebhookResponse struct {
	Veto    bool   `json:"veto,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

// executeWebhook executes a single webhook request
func (h *WebhookHandler) executeWebhook(ctx context.Context, handler *events.EventHandler, config *events.WebhookConfig, payload interface{}) events.HandlerResponse {
	// Marshal payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return events.ErrorResponse(fmt.Sprintf("failed to marshal payload: %v", err))
	}

	// Create request
	method := config.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequestWithContext(ctx, method, config.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return events.ErrorResponse(fmt.Sprintf("failed to create request: %v", err))
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VFS-Webhook/2.0")
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
		// Network error or timeout
		if handler.IsVetoEnabled() && config.OnTimeout == "abort" {
			return events.VetoResponse(fmt.Sprintf("webhook request failed: %v", err), "network_error")
		}
		return events.HandlerResponse{
			Success: false,
			Veto:    false,
			Message: fmt.Sprintf("webhook request failed: %v", err),
			Code:    "network_error",
		}
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	// Parse response body for veto information (if JSON)
	var webhookResp WebhookResponse
	if err := json.Unmarshal(body, &webhookResp); err == nil {
		// Successfully parsed JSON response
		if handler.IsVetoEnabled() && webhookResp.Veto {
			return events.VetoResponse(webhookResp.Message, webhookResp.Code)
		}
	}

	// Check status code
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Success
		return events.SuccessResponse()
	}

	// Handle error status codes
	statusMsg := fmt.Sprintf("webhook returned status %d: %s", resp.StatusCode, string(body))

	// 403 Forbidden always means veto if veto is enabled
	if resp.StatusCode == 403 && handler.IsVetoEnabled() {
		message := webhookResp.Message
		if message == "" {
			message = statusMsg
		}
		code := webhookResp.Code
		if code == "" {
			code = "forbidden"
		}
		return events.VetoResponse(message, code)
	}

	// 401 Unauthorized
	if resp.StatusCode == 401 && handler.IsVetoEnabled() {
		message := webhookResp.Message
		if message == "" {
			message = statusMsg
		}
		code := webhookResp.Code
		if code == "" {
			code = "unauthorized"
		}
		return events.VetoResponse(message, code)
	}

	// 4xx client errors
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return events.HandlerResponse{
			Success: false,
			Veto:    false,
			Message: statusMsg,
			Code:    "client_error",
		}
	}

	// 5xx server errors
	if resp.StatusCode >= 500 {
		// Check on_error config
		if handler.IsVetoEnabled() && config.OnError == "abort" {
			return events.VetoResponse(statusMsg, "server_error")
		}
		return events.HandlerResponse{
			Success: false,
			Veto:    false,
			Message: statusMsg,
			Code:    "server_error",
		}
	}

	// Unexpected status code
	return events.ErrorResponse(statusMsg)
}

// generateHMAC generates HMAC-SHA256 signature
func (h *WebhookHandler) generateHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
