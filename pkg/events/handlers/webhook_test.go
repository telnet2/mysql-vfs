package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/telnet2/mysql-vfs/pkg/events"
)

func TestWebhookHandler_VetoViaHTTPStatus(t *testing.T) {
	tests := []struct {
		name           string
		serverStatus   int
		serverResponse string
		vetoEnabled    bool
		wantVeto       bool
		wantSuccess    bool
	}{
		{
			name:         "200 OK - success",
			serverStatus: http.StatusOK,
			vetoEnabled:  true,
			wantVeto:     false,
			wantSuccess:  true,
		},
		{
			name:         "403 Forbidden - veto when enabled",
			serverStatus: http.StatusForbidden,
			vetoEnabled:  true,
			wantVeto:     true,
			wantSuccess:  false,
		},
		{
			name:         "403 Forbidden - no veto when disabled",
			serverStatus: http.StatusForbidden,
			vetoEnabled:  false,
			wantVeto:     false,
			wantSuccess:  false,
		},
		{
			name:         "401 Unauthorized - veto when enabled",
			serverStatus: http.StatusUnauthorized,
			vetoEnabled:  true,
			wantVeto:     true,
			wantSuccess:  false,
		},
		{
			name:         "500 Internal Server Error - no veto by default",
			serverStatus: http.StatusInternalServerError,
			vetoEnabled:  true,
			wantVeto:     false, // 5xx doesn't veto unless on_error=abort
			wantSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != "" {
					w.Write([]byte(tt.serverResponse))
				}
			}))
			defer server.Close()

			// Create handler
			handler := NewWebhookHandler()

			// Create event handler config
			vetoEnabled := tt.vetoEnabled
			eventHandler := &events.EventHandler{
				Name:        "test-webhook",
				VetoEnabled: &vetoEnabled,
				Config: events.WebhookConfig{
					URL: server.URL,
				},
			}

			// Create test payload
			payload := &events.FileEventPayload{
				Event: events.Event{
					ID:   "evt_123",
					Type: "file.create.completion.succeeded",
				},
			}

			// Execute handler
			response := handler.Handle(context.Background(), eventHandler, payload)

			// Check response
			if response.Veto != tt.wantVeto {
				t.Errorf("Handle() veto = %v, want %v", response.Veto, tt.wantVeto)
			}
			if response.Success != tt.wantSuccess {
				t.Errorf("Handle() success = %v, want %v", response.Success, tt.wantSuccess)
			}
		})
	}
}

func TestWebhookHandler_VetoViaJSONResponse(t *testing.T) {
	tests := []struct {
		name         string
		serverStatus int
		serverBody   WebhookResponse
		vetoEnabled  bool
		wantVeto     bool
		wantCode     string
	}{
		{
			name:         "JSON veto response",
			serverStatus: http.StatusOK,
			serverBody: WebhookResponse{
				Veto:    true,
				Message: "Content policy violation",
				Code:    "content_violation",
			},
			vetoEnabled: true,
			wantVeto:    true,
			wantCode:    "content_violation",
		},
		{
			name:         "JSON veto ignored when veto disabled",
			serverStatus: http.StatusOK,
			serverBody: WebhookResponse{
				Veto:    true,
				Message: "Should be ignored",
				Code:    "ignored",
			},
			vetoEnabled: false,
			wantVeto:    false,
			wantCode:    "",
		},
		{
			name:         "JSON non-veto response",
			serverStatus: http.StatusOK,
			serverBody: WebhookResponse{
				Veto:    false,
				Message: "All good",
			},
			vetoEnabled: true,
			wantVeto:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				json.NewEncoder(w).Encode(tt.serverBody)
			}))
			defer server.Close()

			// Create handler
			handler := NewWebhookHandler()

			// Create event handler config
			vetoEnabled := tt.vetoEnabled
			eventHandler := &events.EventHandler{
				Name:        "test-webhook",
				VetoEnabled: &vetoEnabled,
				Config: events.WebhookConfig{
					URL: server.URL,
				},
			}

			// Create test payload
			payload := &events.FileEventPayload{
				Event: events.Event{
					ID:   "evt_123",
					Type: "file.create.completion.succeeded",
				},
			}

			// Execute handler
			response := handler.Handle(context.Background(), eventHandler, payload)

			// Check response
			if response.Veto != tt.wantVeto {
				t.Errorf("Handle() veto = %v, want %v", response.Veto, tt.wantVeto)
			}
			if tt.wantCode != "" && response.Code != tt.wantCode {
				t.Errorf("Handle() code = %v, want %v", response.Code, tt.wantCode)
			}
		})
	}
}

func TestWebhookHandler_OnError(t *testing.T) {
	tests := []struct {
		name        string
		onError     string
		vetoEnabled bool
		wantVeto    bool
	}{
		{
			name:        "500 error with on_error=abort",
			onError:     "abort",
			vetoEnabled: true,
			wantVeto:    true,
		},
		{
			name:        "500 error with on_error=allow",
			onError:     "allow",
			vetoEnabled: true,
			wantVeto:    false,
		},
		{
			name:        "500 error with on_error=abort but veto disabled",
			onError:     "abort",
			vetoEnabled: false,
			wantVeto:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server that always returns 500
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			// Create handler
			handler := NewWebhookHandler()

			// Create event handler config
			vetoEnabled := tt.vetoEnabled
			eventHandler := &events.EventHandler{
				Name:        "test-webhook",
				VetoEnabled: &vetoEnabled,
				Config: events.WebhookConfig{
					URL:     server.URL,
					OnError: tt.onError,
				},
			}

			// Create test payload
			payload := &events.FileEventPayload{
				Event: events.Event{
					ID:   "evt_123",
					Type: "file.create.completion.succeeded",
				},
			}

			// Execute handler
			response := handler.Handle(context.Background(), eventHandler, payload)

			// Check response
			if response.Veto != tt.wantVeto {
				t.Errorf("Handle() veto = %v, want %v", response.Veto, tt.wantVeto)
			}
		})
	}
}

func TestWebhookHandler_HMACSignature(t *testing.T) {
	secret := "test-secret-key"
	var receivedSignature string

	// Create test server that captures the HMAC signature
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-VFS-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create handler
	handler := NewWebhookHandler()

	// Create event handler config
	eventHandler := &events.EventHandler{
		Name: "test-webhook",
		Config: events.WebhookConfig{
			URL:    server.URL,
			Secret: secret,
		},
	}

	// Create test payload
	payload := &events.FileEventPayload{
		Event: events.Event{
			ID:   "evt_123",
			Type: "file.create.completion.succeeded",
		},
	}

	// Execute handler
	handler.Handle(context.Background(), eventHandler, payload)

	// Verify signature was sent
	if receivedSignature == "" {
		t.Error("Expected HMAC signature header, got none")
	}

	if len(receivedSignature) < 10 {
		t.Errorf("HMAC signature looks invalid: %s", receivedSignature)
	}

	// Signature should start with "sha256="
	if receivedSignature[:7] != "sha256=" {
		t.Errorf("Expected signature to start with 'sha256=', got: %s", receivedSignature[:7])
	}
}

func TestWebhookHandler_PayloadStructure(t *testing.T) {
	var receivedPayload events.FileEventPayload

	// Create test server that captures the payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create handler
	handler := NewWebhookHandler()

	// Create event handler config
	eventHandler := &events.EventHandler{
		Name: "test-webhook",
		Config: events.WebhookConfig{
			URL: server.URL,
		},
	}

	// Create test payload
	expectedPayload := &events.FileEventPayload{
		Event: events.Event{
			ID:   "evt_123",
			Type: "file.create.completion.succeeded",
		},
		Resource: events.FileResource{
			Type: events.ResourceTypeFile,
			ID:   "file_abc",
			Name: "test.json",
			Path: "/data/test.json",
		},
		User: events.UserContext{
			UserID: "alice",
			Groups: []string{"admin"},
		},
	}

	// Execute handler
	handler.Handle(context.Background(), eventHandler, expectedPayload)

	// Verify payload structure
	if receivedPayload.Event.ID != expectedPayload.Event.ID {
		t.Errorf("Expected event ID %s, got %s", expectedPayload.Event.ID, receivedPayload.Event.ID)
	}

	if receivedPayload.Resource.Name != expectedPayload.Resource.Name {
		t.Errorf("Expected resource name %s, got %s", expectedPayload.Resource.Name, receivedPayload.Resource.Name)
	}

	if receivedPayload.User.UserID != expectedPayload.User.UserID {
		t.Errorf("Expected user ID %s, got %s", expectedPayload.User.UserID, receivedPayload.User.UserID)
	}
}

func TestWebhookHandler_CircuitBreaker(t *testing.T) {
	failureCount := 0

	// Create test server that fails first 5 times
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failureCount++
		if failureCount <= 5 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Create handler
	handler := NewWebhookHandler()

	// Create event handler config with circuit breaker
	eventHandler := &events.EventHandler{
		Name: "test-webhook",
		Config: events.WebhookConfig{
			URL: server.URL,
			CircuitBreaker: &events.CircuitBreakerConfig{
				Enabled:           true,
				FailureThreshold:  3,
				RecoveryTimeoutMs: 100,
			},
		},
	}

	// Create test payload
	payload := &events.FileEventPayload{
		Event: events.Event{
			ID:   "evt_123",
			Type: "file.create.completion.succeeded",
		},
	}

	// Execute handler multiple times
	for i := 0; i < 10; i++ {
		response := handler.Handle(context.Background(), eventHandler, payload)

		// After 3 failures, circuit should open
		if i >= 3 && i < 6 {
			// Circuit is open, requests should fail immediately
			if response.Success {
				t.Errorf("Expected circuit breaker to be open at iteration %d", i)
			}
		}
	}

	// Circuit breaker should have prevented some requests
	if failureCount >= 10 {
		t.Error("Circuit breaker did not prevent requests after threshold")
	}
}
