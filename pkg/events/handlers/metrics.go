package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/telnet2/mysql-vfs/pkg/events"
)

// MetricsHandler handles metrics events with template support
type MetricsHandler struct{}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

// Type returns the handler type
func (h *MetricsHandler) Type() events.HandlerType {
	return events.HandlerTypeMetrics
}

// Handle processes a metrics event
func (h *MetricsHandler) Handle(ctx context.Context, handler *events.EventHandler, payload interface{}) events.HandlerResponse {
	// Parse metrics config
	configBytes, err := json.Marshal(handler.Config)
	if err != nil {
		return events.ErrorResponse(fmt.Sprintf("invalid metrics config: %v", err))
	}

	var config events.MetricsConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return events.ErrorResponse(fmt.Sprintf("invalid metrics config: %v", err))
	}

	// Validate metric name
	if config.MetricName == "" {
		return events.ErrorResponse("metric_name is required")
	}

	// Render tags with templates
	renderedTags := h.renderTags(config.Tags, payload)

	// Get metric value
	value := h.getMetricValue(config.ValueField, payload)

	// Emit metric (log format that can be scraped by monitoring systems)
	h.emitMetric(config.MetricName, value, renderedTags)

	return events.SuccessResponse()
}

// renderTags renders tag templates with payload data
func (h *MetricsHandler) renderTags(tags map[string]string, payload interface{}) map[string]string {
	rendered := make(map[string]string)

	for key, template := range tags {
		// Render template
		value := h.renderTemplate(template, payload)
		rendered[key] = value
	}

	return rendered
}

// renderTemplate renders a template string with payload data
// Reuses same logic as LogHandler for consistency
func (h *MetricsHandler) renderTemplate(template string, payload interface{}) string {
	result := template

	// Find all template variables {{...}}
	vars := h.extractTemplateVars(template)

	for _, varName := range vars {
		// Get value from payload
		value := h.getNestedValue(payload, varName)

		// Replace {{varName}} with value
		result = strings.ReplaceAll(result, "{{"+varName+"}}", value)
	}

	return result
}

// extractTemplateVars extracts all template variables from a template string
func (h *MetricsHandler) extractTemplateVars(template string) []string {
	var vars []string
	start := 0

	for {
		// Find next {{
		idx := strings.Index(template[start:], "{{")
		if idx == -1 {
			break
		}
		start += idx + 2

		// Find closing }}
		end := strings.Index(template[start:], "}}")
		if end == -1 {
			break
		}

		varName := strings.TrimSpace(template[start : start+end])
		vars = append(vars, varName)
		start += end + 2
	}

	return vars
}

// getNestedValue gets a nested value from payload using dot notation
func (h *MetricsHandler) getNestedValue(payload interface{}, path string) string {
	parts := strings.Split(path, ".")

	current := reflect.ValueOf(payload)

	for _, part := range parts {
		// Dereference pointers
		if current.Kind() == reflect.Ptr {
			if current.IsNil() {
				return ""
			}
			current = current.Elem()
		}

		// Handle structs
		if current.Kind() == reflect.Struct {
			// Try to find field (case-insensitive)
			field := h.findField(current, part)
			if !field.IsValid() {
				return ""
			}
			current = field
			continue
		}

		// Handle maps
		if current.Kind() == reflect.Map {
			key := reflect.ValueOf(part)
			val := current.MapIndex(key)
			if !val.IsValid() {
				return ""
			}
			current = val
			continue
		}

		// Can't navigate further
		return ""
	}

	// Convert final value to string
	return h.valueToString(current)
}

// findField finds a struct field by name (case-insensitive)
func (h *MetricsHandler) findField(v reflect.Value, name string) reflect.Value {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)

		// Check field name (case-insensitive)
		if strings.EqualFold(field.Name, name) {
			return v.Field(i)
		}

		// Check json tag
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" {
			// Remove options like ",omitempty"
			jsonName := strings.Split(jsonTag, ",")[0]
			if strings.EqualFold(jsonName, name) {
				return v.Field(i)
			}
		}
	}

	return reflect.Value{}
}

// valueToString converts a reflect.Value to string
func (h *MetricsHandler) valueToString(v reflect.Value) string {
	// Dereference pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	// Handle different types
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%f", v.Float())
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Bool())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// getMetricValue gets the metric value from payload
// If valueField is empty, defaults to 1 (for counting events)
// If valueField is specified, extracts the numeric value from that field
func (h *MetricsHandler) getMetricValue(valueField string, payload interface{}) float64 {
	if valueField == "" {
		// Default: count event (value = 1)
		return 1.0
	}

	// Extract value from field
	valueStr := h.getNestedValue(payload, valueField)
	if valueStr == "" {
		return 0.0
	}

	// Try to parse as number
	var value float64
	_, err := fmt.Sscanf(valueStr, "%f", &value)
	if err != nil {
		// Not a number, return 1
		return 1.0
	}

	return value
}

// emitMetric emits a metric in a format that can be scraped by monitoring systems
// Format: METRIC metric_name{tag1="value1",tag2="value2"} value
func (h *MetricsHandler) emitMetric(name string, value float64, tags map[string]string) {
	// Build tags string
	var tagPairs []string
	for key, val := range tags {
		// Escape quotes in values
		escapedVal := strings.ReplaceAll(val, `"`, `\"`)
		tagPairs = append(tagPairs, fmt.Sprintf(`%s="%s"`, key, escapedVal))
	}

	var tagsStr string
	if len(tagPairs) > 0 {
		tagsStr = "{" + strings.Join(tagPairs, ",") + "}"
	}

	// Log in Prometheus/StatsD compatible format
	log.Printf("METRIC %s%s %f", name, tagsStr, value)
}
