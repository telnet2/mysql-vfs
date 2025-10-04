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

// LogHandler handles log events with template support
type LogHandler struct{}

// NewLogHandler creates a new log handler
func NewLogHandler() *LogHandler {
	return &LogHandler{}
}

// Type returns the handler type
func (h *LogHandler) Type() events.HandlerType {
	return events.HandlerTypeLog
}

// Handle processes a log event
func (h *LogHandler) Handle(ctx context.Context, handler *events.EventHandler, payload interface{}) error {
	// Parse log config
	configBytes, err := json.Marshal(handler.Config)
	if err != nil {
		return fmt.Errorf("invalid log config: %w", err)
	}

	var config events.LogConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return fmt.Errorf("invalid log config: %w", err)
	}

	// Render message template
	message := h.renderTemplate(config.Message, payload)

	// Log at appropriate level
	h.logMessage(config.Level, handler.Name, message)

	return nil
}

// renderTemplate renders a template string with payload data
// Supports variables like {{event.type}}, {{resource.path}}, {{user.user_id}}
func (h *LogHandler) renderTemplate(template string, payload interface{}) string {
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
func (h *LogHandler) extractTemplateVars(template string) []string {
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
// e.g., "event.type" returns payload.Event.Type
func (h *LogHandler) getNestedValue(payload interface{}, path string) string {
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
func (h *LogHandler) findField(v reflect.Value, name string) reflect.Value {
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
func (h *LogHandler) valueToString(v reflect.Value) string {
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
	case reflect.Slice, reflect.Array:
		// Convert to JSON for arrays/slices
		if v.Len() == 0 {
			return "[]"
		}
		elements := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			elements[i] = h.valueToString(v.Index(i))
		}
		return "[" + strings.Join(elements, ", ") + "]"
	case reflect.Struct:
		// Return JSON representation for structs
		bytes, _ := json.Marshal(v.Interface())
		return string(bytes)
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// logMessage logs a message at the appropriate level
func (h *LogHandler) logMessage(level events.LogLevel, handlerName, message string) {
	prefix := fmt.Sprintf("[%s] %s:", strings.ToUpper(string(level)), handlerName)

	switch level {
	case events.LogLevelDebug:
		log.Printf("%s %s", prefix, message)
	case events.LogLevelInfo:
		log.Printf("%s %s", prefix, message)
	case events.LogLevelWarn:
		log.Printf("%s %s", prefix, message)
	case events.LogLevelError:
		log.Printf("%s %s", prefix, message)
	default:
		log.Printf("%s %s", prefix, message)
	}
}
