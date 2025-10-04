package events

import (
	"time"
)

// EventType represents the type of event
type EventType string

const (
	// File events
	EventFileCreated EventType = "file.created"
	EventFileUpdated EventType = "file.updated"
	EventFileDeleted EventType = "file.deleted"
	EventFileMoved   EventType = "file.moved"

	// Directory events
	EventDirectoryCreated EventType = "directory.created"
	EventDirectoryDeleted EventType = "directory.deleted"
)

// Event represents the core event information
type Event struct {
	ID            string    `json:"id"`
	Type          EventType `json:"type"`
	Timestamp     time.Time `json:"timestamp"`
	DirectoryPath string    `json:"directory_path"`
}

// ResourceType represents the type of resource
type ResourceType string

const (
	ResourceTypeFile      ResourceType = "file"
	ResourceTypeDirectory ResourceType = "directory"
)

// FileResource represents a file resource in an event
type FileResource struct {
	Type           ResourceType `json:"type"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Path           string       `json:"path"`
	SizeBytes      int64        `json:"size_bytes"`
	ContentType    string       `json:"content_type"`
	Version        int64        `json:"version"`
	ChecksumSHA256 string       `json:"checksum_sha256"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// DirectoryResource represents a directory resource in an event
type DirectoryResource struct {
	Type      ResourceType `json:"type"`
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	CreatedAt time.Time    `json:"created_at"`
}

// UserContext represents the user who triggered the event
type UserContext struct {
	UserID string   `json:"user_id"`
	Role   string   `json:"role"`
	Groups []string `json:"groups"`
}

// EventMetadata represents additional metadata for the event
type EventMetadata struct {
	RequestID string `json:"request_id"`
	IPAddress string `json:"ip_address,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

// FileEventPayload represents the complete payload for file events
type FileEventPayload struct {
	Event    Event         `json:"event"`
	Resource FileResource  `json:"resource"`
	User     UserContext   `json:"user"`
	Metadata EventMetadata `json:"metadata"`
}

// DirectoryEventPayload represents the complete payload for directory events
type DirectoryEventPayload struct {
	Event    Event             `json:"event"`
	Resource DirectoryResource `json:"resource"`
	User     UserContext       `json:"user"`
	Metadata EventMetadata     `json:"metadata"`
}

// MoveEventPayload represents the payload for file move events
type MoveEventPayload struct {
	Event        Event         `json:"event"`
	Resource     FileResource  `json:"resource"`
	OldPath      string        `json:"old_path"`
	NewPath      string        `json:"new_path"`
	OldDirectory string        `json:"old_directory"`
	NewDirectory string        `json:"new_directory"`
	User         UserContext   `json:"user"`
	Metadata     EventMetadata `json:"metadata"`
}

// HandlerType represents the type of event handler
type HandlerType string

const (
	HandlerTypeWebhook HandlerType = "webhook"
	HandlerTypeLog     HandlerType = "log"
	HandlerTypeMetrics HandlerType = "metrics"
)

// PatternType represents the type of pattern matching
type PatternType string

const (
	PatternTypeGlob  PatternType = "glob"
	PatternTypeRegex PatternType = "regex"
)

// EventFilter represents filtering criteria for events
type EventFilter struct {
	Pattern      string   `json:"pattern,omitempty"`
	Type         string   `json:"type,omitempty"` // "glob" or "regex"
	MinSizeBytes *int64   `json:"min_size_bytes,omitempty"`
	MaxSizeBytes *int64   `json:"max_size_bytes,omitempty"`
	ContentTypes []string `json:"content_types,omitempty"`
}

// BackoffType represents the type of retry backoff
type BackoffType string

const (
	BackoffTypeExponential BackoffType = "exponential"
	BackoffTypeLinear      BackoffType = "linear"
)

// RetryConfig represents retry configuration
type RetryConfig struct {
	MaxAttempts     int         `json:"max_attempts"`
	InitialDelayMs  int         `json:"initial_delay_ms"`
	MaxDelayMs      int         `json:"max_delay_ms,omitempty"`
	Backoff         BackoffType `json:"backoff"` // "exponential" or "linear"
}

// CircuitBreakerConfig represents circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled            bool `json:"enabled"`
	FailureThreshold   int  `json:"failure_threshold"`
	RecoveryTimeoutMs  int  `json:"recovery_timeout_ms"`
}

// WebhookConfig represents webhook handler configuration
type WebhookConfig struct {
	URL            string                  `json:"url"`
	Method         string                  `json:"method,omitempty"` // Default: POST
	Headers        map[string]string       `json:"headers,omitempty"`
	Secret         string                  `json:"secret,omitempty"` // For HMAC signature
	TimeoutMs      int                     `json:"timeout_ms,omitempty"`
	Retry          *RetryConfig            `json:"retry,omitempty"`
	CircuitBreaker *CircuitBreakerConfig   `json:"circuit_breaker,omitempty"`
	OnTimeout      string                  `json:"on_timeout,omitempty"`      // "abort" or "allow" (default: allow)
	OnError        string                  `json:"on_error,omitempty"`        // "abort" or "allow" (default: allow)
}

// LogLevel represents log severity level
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogConfig represents log handler configuration
type LogConfig struct {
	Level   LogLevel `json:"level"`
	Message string   `json:"message"` // Template with {{variable}} placeholders
}

// MetricsConfig represents metrics handler configuration
type MetricsConfig struct {
	MetricName string            `json:"metric_name"`
	Tags       map[string]string `json:"tags,omitempty"`       // Template tags with {{variable}} placeholders
	ValueField string            `json:"value_field,omitempty"` // Field to use as metric value (e.g., "resource.size_bytes")
}

// EventHandler represents a single event handler configuration
type EventHandler struct {
	Name         string        `json:"name"`
	Events       []EventType   `json:"events"` // Can include wildcards: "file.*.authorization.*"
	Type         HandlerType   `json:"type"`
	Enabled      *bool         `json:"enabled,omitempty"`       // nil = true, explicit true/false
	Synchronous  *bool         `json:"synchronous,omitempty"`   // nil = false, if true handler blocks operation
	VetoEnabled  *bool         `json:"veto_enabled,omitempty"`  // nil = false, if true handler can abort operation
	Filter       *EventFilter  `json:"filter,omitempty"`
	Config       interface{}   `json:"config"` // Will be WebhookConfig, LogConfig, or MetricsConfig
}

// IsEnabled returns whether the handler is enabled (default: true)
func (h *EventHandler) IsEnabled() bool {
	if h.Enabled == nil {
		return true
	}
	return *h.Enabled
}

// IsSynchronous returns whether the handler should be executed synchronously (default: false)
func (h *EventHandler) IsSynchronous() bool {
	if h.Synchronous == nil {
		return false
	}
	return *h.Synchronous
}

// IsVetoEnabled returns whether the handler can veto operations (default: false)
func (h *EventHandler) IsVetoEnabled() bool {
	if h.VetoEnabled == nil {
		return false
	}
	return *h.VetoEnabled
}

// EventsFile represents the structure of a .events file
type EventsFile struct {
	Handlers []EventHandler `json:"handlers"`
}
