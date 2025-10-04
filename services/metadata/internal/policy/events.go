package policy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EventsConfig represents the parsed contents of a `.events` manifest.
type EventsConfig struct {
	Triggers []EventTrigger `json:"triggers"`
}

// EventTrigger describes when an action pipeline should execute.
type EventTrigger struct {
	Name       string                `json:"name,omitempty"`
	On         string                `json:"on"`
	Scope      *Scope                `json:"-"`
	Match      EventTriggerMatch     `json:"match,omitempty"`
	Conditions EventTriggerCondition `json:"conditions,omitempty"`
	Actions    []EventAction         `json:"actions"`
}

// EventTriggerMatch stores attribute filters (each value can be a single string or list).
type EventTriggerMatch map[string][]string

// EventTriggerCondition holds optional expressions to evaluate before firing.
type EventTriggerCondition struct {
	Rego string `json:"rego,omitempty"`
}

// EventAction defines a single handler invocation triggered by a match.
type EventAction struct {
	Type            string            `json:"type"`
	EventType       string            `json:"event_type,omitempty"`
	PayloadTemplate string            `json:"payload_template,omitempty"`
	Workflow        string            `json:"workflow,omitempty"`
	Webhook         string            `json:"webhook,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	Retry           *EventActionRetry `json:"retry,omitempty"`
}

// EventActionRetry configures retry semantics for a handler.
type EventActionRetry struct {
	MaxAttempts    int `json:"max_attempts,omitempty"`
	BackoffSeconds int `json:"backoff_seconds,omitempty"`
}

type eventsDocument struct {
	Scope       string         `json:"scope,omitempty"`
	Inheritance string         `json:"inheritance,omitempty"`
	Triggers    []EventTrigger `json:"triggers"`
}

// ParseEventsManifest converts manifest payload into EventsConfig and inheritance overrides.
func ParseEventsManifest(payload []byte) (EventsConfig, ManifestOverrides, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return EventsConfig{}, ManifestOverrides{}, fmt.Errorf("policy: events manifest payload required")
	}
	var doc eventsDocument
	if err := json.Unmarshal(payload, &doc); err != nil {
		return EventsConfig{}, ManifestOverrides{}, fmt.Errorf("policy: invalid events manifest: %w", err)
	}
	if len(doc.Triggers) == 0 {
		return EventsConfig{}, ManifestOverrides{}, fmt.Errorf("policy: events manifest must define at least one trigger")
	}
	for idx := range doc.Triggers {
		if err := doc.Triggers[idx].validate(idx); err != nil {
			return EventsConfig{}, ManifestOverrides{}, err
		}
	}

	overrides := ManifestOverrides{}
	if scope, ok := parseScope(doc.Scope); ok {
		overrides.Scope = &scope
	}
	if inh, ok := parseInheritance(doc.Inheritance); ok {
		overrides.Inheritance = &inh
	}

	return EventsConfig{Triggers: doc.Triggers}, overrides, nil
}

func (t *EventTrigger) UnmarshalJSON(data []byte) error {
	type alias struct {
		Name       string                `json:"name,omitempty"`
		On         string                `json:"on"`
		Scope      string                `json:"scope,omitempty"`
		Match      EventTriggerMatch     `json:"match,omitempty"`
		Conditions EventTriggerCondition `json:"conditions,omitempty"`
		Actions    []EventAction         `json:"actions"`
	}
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*t = EventTrigger{
		Name:       strings.TrimSpace(raw.Name),
		On:         strings.TrimSpace(raw.On),
		Match:      raw.Match,
		Conditions: raw.Conditions,
		Actions:    raw.Actions,
	}
	if raw.Scope != "" {
		if scope, ok := parseScope(raw.Scope); ok {
			s := scope
			t.Scope = &s
		} else {
			return fmt.Errorf("policy: invalid trigger scope %q", raw.Scope)
		}
	}
	return nil
}

func (m *EventTriggerMatch) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*m = nil
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	result := make(EventTriggerMatch, len(raw))
	for key, value := range raw {
		list, err := parseStringList(value)
		if err != nil {
			return fmt.Errorf("policy: invalid match value for %s: %w", key, err)
		}
		if len(list) == 0 {
			continue
		}
		result[key] = list
	}
	if len(result) == 0 {
		*m = nil
		return nil
	}
	*m = result
	return nil
}

func (t *EventTrigger) validate(index int) error {
	if strings.TrimSpace(t.On) == "" {
		return fmt.Errorf("policy: events trigger %d missing 'on' field", index)
	}
	if len(t.Actions) == 0 {
		return fmt.Errorf("policy: events trigger %d must define at least one action", index)
	}
	for actionIdx, action := range t.Actions {
		if err := action.validate(index, actionIdx); err != nil {
			return err
		}
	}
	return nil
}

func (a EventAction) validate(triggerIdx, actionIdx int) error {
	typeName := strings.TrimSpace(a.Type)
	if typeName == "" {
		return fmt.Errorf("policy: events trigger %d action %d missing type", triggerIdx, actionIdx)
	}
	switch typeName {
	case "emit_event":
		if strings.TrimSpace(a.EventType) == "" {
			return fmt.Errorf("policy: events trigger %d action %d missing event_type", triggerIdx, actionIdx)
		}
	case "invoke_workflow":
		if strings.TrimSpace(a.Workflow) == "" {
			return fmt.Errorf("policy: events trigger %d action %d missing workflow", triggerIdx, actionIdx)
		}
	case "call_webhook":
		if strings.TrimSpace(a.Webhook) == "" {
			return fmt.Errorf("policy: events trigger %d action %d missing webhook", triggerIdx, actionIdx)
		}
	}
	return nil
}

func parseStringList(value json.RawMessage) ([]string, error) {
	if len(value) == 0 {
		return nil, nil
	}
	var single string
	if err := json.Unmarshal(value, &single); err == nil {
		trimmed := strings.TrimSpace(single)
		if trimmed == "" {
			return nil, nil
		}
		return []string{trimmed}, nil
	}
	var list []string
	if err := json.Unmarshal(value, &list); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(list))
	for _, v := range list {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result, nil
}
