package handlers

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/telnet2/mysql-vfs/pkg/events"
)

func TestLogHandler_Type(t *testing.T) {
	handler := NewLogHandler()
	if handler.Type() != events.HandlerTypeLog {
		t.Errorf("Expected type %s, got %s", events.HandlerTypeLog, handler.Type())
	}
}

func TestLogHandler_Handle_BasicLogging(t *testing.T) {
	handler := NewLogHandler()
	ctx := context.Background()

	tests := []struct {
		name     string
		level    events.LogLevel
		message  string
		expected string
	}{
		{
			name:     "info level",
			level:    events.LogLevelInfo,
			message:  "Test message",
			expected: "[INFO] test-handler: Test message",
		},
		{
			name:     "debug level",
			level:    events.LogLevelDebug,
			message:  "Debug message",
			expected: "[DEBUG] test-handler: Debug message",
		},
		{
			name:     "warn level",
			level:    events.LogLevelWarn,
			message:  "Warning message",
			expected: "[WARN] test-handler: Warning message",
		},
		{
			name:     "error level",
			level:    events.LogLevelError,
			message:  "Error message",
			expected: "[ERROR] test-handler: Error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			eventHandler := &events.EventHandler{
				Name: "test-handler",
				Type: events.HandlerTypeLog,
				Config: map[string]interface{}{
					"level":   tt.level,
					"message": tt.message,
				},
			}

			payload := map[string]interface{}{}

			resp := handler.Handle(ctx, eventHandler, payload)

			if !resp.Success {
				t.Errorf("Expected success, got error: %s", resp.Message)
			}

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected log output to contain %q, got: %s", tt.expected, output)
			}
		})
	}
}

func TestLogHandler_TemplateRendering(t *testing.T) {
	handler := NewLogHandler()
	ctx := context.Background()

	type TestPayload struct {
		Event    events.Event
		Resource map[string]interface{}
		User     map[string]interface{}
	}

	tests := []struct {
		name     string
		template string
		payload  TestPayload
		expected string
	}{
		{
			name:     "simple event type",
			template: "Event: {{event.type}}",
			payload: TestPayload{
				Event: events.Event{Type: "file.created"},
			},
			expected: "Event: file.created",
		},
		{
			name:     "multiple variables",
			template: "{{event.type}} on {{resource.path}} by {{user.user_id}}",
			payload: TestPayload{
				Event:    events.Event{Type: "file.created"},
				Resource: map[string]interface{}{"path": "/data/test.json"},
				User:     map[string]interface{}{"user_id": "alice"},
			},
			expected: "file.created on /data/test.json by alice",
		},
		{
			name:     "nested field access",
			template: "Resource: {{resource.path}}",
			payload: TestPayload{
				Resource: map[string]interface{}{"path": "/data/file.json"},
			},
			expected: "Resource: /data/file.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			eventHandler := &events.EventHandler{
				Name: "test-handler",
				Type: events.HandlerTypeLog,
				Config: map[string]interface{}{
					"level":   events.LogLevelInfo,
					"message": tt.template,
				},
			}

			resp := handler.Handle(ctx, eventHandler, tt.payload)

			if !resp.Success {
				t.Errorf("Expected success, got error: %s", resp.Message)
			}

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected log output to contain %q, got: %s", tt.expected, output)
			}
		})
	}
}

func TestLogHandler_RenderTemplate(t *testing.T) {
	handler := NewLogHandler()

	type TestPayload struct {
		Event    events.Event
		Resource map[string]interface{}
	}

	tests := []struct {
		name     string
		template string
		payload  interface{}
		expected string
	}{
		{
			name:     "no variables",
			template: "Static message",
			payload:  TestPayload{},
			expected: "Static message",
		},
		{
			name:     "single variable",
			template: "Event: {{event.type}}",
			payload: TestPayload{
				Event: events.Event{Type: "test.event"},
			},
			expected: "Event: test.event",
		},
		{
			name:     "missing variable",
			template: "Event: {{event.missing}}",
			payload:  TestPayload{},
			expected: "Event: ",
		},
		{
			name:     "map access",
			template: "Path: {{resource.path}}",
			payload: TestPayload{
				Resource: map[string]interface{}{"path": "/test"},
			},
			expected: "Path: /test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.renderTemplate(tt.template, tt.payload)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLogHandler_ExtractTemplateVars(t *testing.T) {
	handler := NewLogHandler()

	tests := []struct {
		name     string
		template string
		expected []string
	}{
		{
			name:     "no variables",
			template: "Static message",
			expected: nil,
		},
		{
			name:     "single variable",
			template: "{{event.type}}",
			expected: []string{"event.type"},
		},
		{
			name:     "multiple variables",
			template: "{{event.type}} on {{resource.path}}",
			expected: []string{"event.type", "resource.path"},
		},
		{
			name:     "variable with spaces",
			template: "{{ event.type }}",
			expected: []string{"event.type"},
		},
		{
			name:     "incomplete variable",
			template: "{{incomplete",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractTemplateVars(tt.template)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d variables, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Expected variable %d to be %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestLogHandler_GetNestedValue(t *testing.T) {
	handler := NewLogHandler()

	type Inner struct {
		Name string
		Age  int
	}

	type TestStruct struct {
		Event  events.Event
		Nested Inner
		Items  []string
	}

	tests := []struct {
		name     string
		path     string
		payload  interface{}
		expected string
	}{
		{
			name:     "struct field",
			path:     "event.type",
			payload:  TestStruct{Event: events.Event{Type: "test"}},
			expected: "test",
		},
		{
			name:     "nested struct",
			path:     "nested.name",
			payload:  TestStruct{Nested: Inner{Name: "Alice"}},
			expected: "Alice",
		},
		{
			name:     "int to string",
			path:     "nested.age",
			payload:  TestStruct{Nested: Inner{Age: 25}},
			expected: "25",
		},
		{
			name:     "missing path",
			path:     "missing.field",
			payload:  TestStruct{},
			expected: "",
		},
		{
			name:     "array field",
			path:     "items",
			payload:  TestStruct{Items: []string{"a", "b", "c"}},
			expected: "[a, b, c]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.getNestedValue(tt.payload, tt.path)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLogHandler_InvalidConfig(t *testing.T) {
	handler := NewLogHandler()
	ctx := context.Background()

	eventHandler := &events.EventHandler{
		Name:   "test-handler",
		Type:   events.HandlerTypeLog,
		Config: "invalid config", // Should be map
	}

	resp := handler.Handle(ctx, eventHandler, nil)

	if resp.Success {
		t.Error("Expected error for invalid config")
	}
	if !strings.Contains(resp.Message, "invalid log config") {
		t.Errorf("Expected 'invalid log config' error, got: %s", resp.Message)
	}
}
