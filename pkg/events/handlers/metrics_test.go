package handlers

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/telnet2/mysql-vfs/pkg/events"
)

func TestMetricsHandler_Type(t *testing.T) {
	handler := NewMetricsHandler()
	if handler.Type() != events.HandlerTypeMetrics {
		t.Errorf("Expected type %s, got %s", events.HandlerTypeMetrics, handler.Type())
	}
}

func TestMetricsHandler_Handle_BasicMetrics(t *testing.T) {
	handler := NewMetricsHandler()
	ctx := context.Background()

	tests := []struct {
		name         string
		metricName   string
		tags         map[string]string
		valueField   string
		payload      interface{}
		expectMetric string
		expectValue  string
	}{
		{
			name:         "simple counter",
			metricName:   "file_created",
			tags:         map[string]string{},
			valueField:   "",
			payload:      map[string]interface{}{},
			expectMetric: "METRIC file_created 1.000000",
		},
		{
			name:       "counter with tags",
			metricName: "file_created",
			tags: map[string]string{
				"operation": "create",
				"status":    "success",
			},
			valueField:   "",
			payload:      map[string]interface{}{},
			expectMetric: "METRIC file_created{",
			expectValue:  "1.000000",
		},
		{
			name:       "gauge with value",
			metricName: "file_size_bytes",
			tags:       map[string]string{},
			valueField: "size",
			payload: struct {
				Size int
			}{Size: 1024},
			expectMetric: "METRIC file_size_bytes 1024.000000",
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
				Type: events.HandlerTypeMetrics,
				Config: map[string]interface{}{
					"metric_name": tt.metricName,
					"tags":        tt.tags,
					"value_field": tt.valueField,
				},
			}

			resp := handler.Handle(ctx, eventHandler, tt.payload)

			if !resp.Success {
				t.Errorf("Expected success, got error: %s", resp.Message)
			}

			output := buf.String()
			if !strings.Contains(output, tt.expectMetric) {
				t.Errorf("Expected metric output to contain %q, got: %s", tt.expectMetric, output)
			}

			if tt.expectValue != "" && !strings.Contains(output, tt.expectValue) {
				t.Errorf("Expected metric value %q, got: %s", tt.expectValue, output)
			}
		})
	}
}

func TestMetricsHandler_TagTemplateRendering(t *testing.T) {
	handler := NewMetricsHandler()
	ctx := context.Background()

	type TestPayload struct {
		Event    events.Event
		Resource map[string]interface{}
		User     map[string]interface{}
	}

	tests := []struct {
		name        string
		tags        map[string]string
		payload     TestPayload
		expectTags  []string
		expectValue string
	}{
		{
			name: "template in tag value",
			tags: map[string]string{
				"event_type": "{{event.type}}",
			},
			payload: TestPayload{
				Event: events.Event{
					Type: "file.create.completion.succeeded",
				},
			},
			expectTags: []string{
				`event_type="file.create.completion.succeeded"`,
			},
		},
		{
			name: "resource path in tags",
			tags: map[string]string{
				"resource": "{{resource.path}}",
			},
			payload: TestPayload{
				Resource: map[string]interface{}{
					"path": "/data/test.json",
				},
			},
			expectTags: []string{
				`resource="/data/test.json"`,
			},
		},
		{
			name: "user info in tags",
			tags: map[string]string{
				"user": "{{user.user_id}}",
			},
			payload: TestPayload{
				User: map[string]interface{}{
					"user_id": "alice",
				},
			},
			expectTags: []string{
				`user="alice"`,
			},
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
				Type: events.HandlerTypeMetrics,
				Config: map[string]interface{}{
					"metric_name": "test_metric",
					"tags":        tt.tags,
				},
			}

			resp := handler.Handle(ctx, eventHandler, tt.payload)

			if !resp.Success {
				t.Errorf("Expected success, got error: %s", resp.Message)
			}

			output := buf.String()
			for _, expectedTag := range tt.expectTags {
				if !strings.Contains(output, expectedTag) {
					t.Errorf("Expected metric output to contain tag %q, got: %s", expectedTag, output)
				}
			}
		})
	}
}

func TestMetricsHandler_RenderTags(t *testing.T) {
	handler := NewMetricsHandler()

	type TestPayload struct {
		Event events.Event
	}

	tests := []struct {
		name     string
		tags     map[string]string
		payload  interface{}
		expected map[string]string
	}{
		{
			name: "static tags",
			tags: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			payload: TestPayload{},
			expected: map[string]string{
				"env":  "production",
				"team": "platform",
			},
		},
		{
			name: "template tags",
			tags: map[string]string{
				"event_type": "{{event.type}}",
			},
			payload: TestPayload{
				Event: events.Event{
					Type: "file.create.completion.succeeded",
				},
			},
			expected: map[string]string{
				"event_type": "file.create.completion.succeeded",
			},
		},
		{
			name: "mixed tags",
			tags: map[string]string{
				"env":        "production",
				"event_type": "{{event.type}}",
			},
			payload: TestPayload{
				Event: events.Event{Type: "file.create.completion.succeeded"},
			},
			expected: map[string]string{
				"env":        "production",
				"event_type": "file.create.completion.succeeded",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.renderTags(tt.tags, tt.payload)
			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("Tag %q: expected %q, got %q", key, expectedValue, result[key])
				}
			}
		})
	}
}

func TestMetricsHandler_GetMetricValue(t *testing.T) {
	handler := NewMetricsHandler()

	type Resource struct {
		Size int
		Name string
	}

	type Metrics struct {
		Duration float64
	}

	type TestPayload struct {
		Resource Resource
		Metrics  Metrics
	}

	tests := []struct {
		name       string
		valueField string
		payload    interface{}
		expected   float64
	}{
		{
			name:       "default value (count)",
			valueField: "",
			payload:    TestPayload{},
			expected:   1.0,
		},
		{
			name:       "extract integer",
			valueField: "resource.size",
			payload:    TestPayload{Resource: Resource{Size: 1024}},
			expected:   1024.0,
		},
		{
			name:       "extract float",
			valueField: "metrics.duration",
			payload:    TestPayload{Metrics: Metrics{Duration: 2.5}},
			expected:   2.5,
		},
		{
			name:       "missing field",
			valueField: "missing.field",
			payload:    TestPayload{},
			expected:   0.0,
		},
		{
			name:       "non-numeric value",
			valueField: "resource.name",
			payload:    TestPayload{Resource: Resource{Name: "test.json"}},
			expected:   1.0, // Falls back to 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.getMetricValue(tt.valueField, tt.payload)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestMetricsHandler_EmitMetric(t *testing.T) {
	handler := NewMetricsHandler()

	tests := []struct {
		name           string
		metricName     string
		value          float64
		tags           map[string]string
		expectContains []string
	}{
		{
			name:       "simple metric",
			metricName: "test_metric",
			value:      42.0,
			tags:       map[string]string{},
			expectContains: []string{
				"METRIC test_metric 42.000000",
			},
		},
		{
			name:       "metric with tags",
			metricName: "test_metric",
			value:      1.0,
			tags: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			expectContains: []string{
				"METRIC test_metric{",
				`env="production"`,
				`team="platform"`,
				"} 1.000000",
			},
		},
		{
			name:       "metric with special characters in tags",
			metricName: "test_metric",
			value:      1.0,
			tags: map[string]string{
				"path": `/data/file.json`,
			},
			expectContains: []string{
				"METRIC test_metric{",
				`path="/data/file.json"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			handler.emitMetric(tt.metricName, tt.value, tt.tags)

			output := buf.String()
			for _, expected := range tt.expectContains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got: %s", expected, output)
				}
			}
		})
	}
}

func TestMetricsHandler_InvalidConfig(t *testing.T) {
	handler := NewMetricsHandler()
	ctx := context.Background()

	tests := []struct {
		name        string
		config      interface{}
		expectError string
	}{
		{
			name:        "invalid config type",
			config:      "invalid config",
			expectError: "invalid metrics config",
		},
		{
			name: "missing metric name",
			config: map[string]interface{}{
				"tags": map[string]string{},
			},
			expectError: "metric_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventHandler := &events.EventHandler{
				Name:   "test-handler",
				Type:   events.HandlerTypeMetrics,
				Config: tt.config,
			}

			resp := handler.Handle(ctx, eventHandler, nil)

			if resp.Success {
				t.Error("Expected error for invalid config")
			}
			if !strings.Contains(resp.Message, tt.expectError) {
				t.Errorf("Expected error to contain %q, got: %s", tt.expectError, resp.Message)
			}
		})
	}
}

func TestMetricsHandler_PrometheusFormat(t *testing.T) {
	handler := NewMetricsHandler()

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	// Emit a metric in Prometheus format
	handler.emitMetric("http_requests_total", 150, map[string]string{
		"method": "GET",
		"status": "200",
	})

	output := buf.String()

	// Verify Prometheus-compatible format
	// Format should be: METRIC metric_name{label1="value1",label2="value2"} value
	if !strings.Contains(output, "METRIC http_requests_total{") {
		t.Errorf("Expected Prometheus metric format, got: %s", output)
	}

	if !strings.Contains(output, `method="GET"`) {
		t.Error("Expected method label in output")
	}

	if !strings.Contains(output, `status="200"`) {
		t.Error("Expected status label in output")
	}

	if !strings.Contains(output, "150.000000") {
		t.Error("Expected value 150.000000 in output")
	}
}
