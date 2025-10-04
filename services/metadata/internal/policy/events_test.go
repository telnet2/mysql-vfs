package policy

import "testing"

func TestParseEventsManifest(t *testing.T) {
	payload := []byte(`{
		"scope": "directory",
		"inheritance": "override",
		"triggers": [
			{
				"name": "audit-create",
				"on": "file.created",
				"scope": "file",
				"match": {
					"file_name": "*.json",
					"storage_mode": ["inline_json", "blob"]
				},
				"conditions": {"rego": "input.actor == \"admin\""},
				"actions": [
					{
						"type": "emit_event",
						"event_type": "ext.audit",
						"payload_template": "templates/audit.json",
						"metadata": {"source": "policy"},
						"retry": {"max_attempts": 3, "backoff_seconds": 10}
					}
				]
			}
		]
	}`)

	config, overrides, err := ParseEventsManifest(payload)
	if err != nil {
		t.Fatalf("parse events manifest: %v", err)
	}
	if len(config.Triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(config.Triggers))
	}
	trigger := config.Triggers[0]
	if trigger.Name != "audit-create" {
		t.Fatalf("unexpected trigger name: %s", trigger.Name)
	}
	if trigger.On != "file.created" {
		t.Fatalf("unexpected trigger on: %s", trigger.On)
	}
	if trigger.Scope == nil || *trigger.Scope != ScopeFile {
		t.Fatalf("expected trigger scope file, got %+v", trigger.Scope)
	}
	if trigger.Match == nil {
		t.Fatalf("expected match to be parsed")
	}
	if values := trigger.Match["file_name"]; len(values) != 1 || values[0] != "*.json" {
		t.Fatalf("unexpected match file_name: %+v", values)
	}
	if values := trigger.Match["storage_mode"]; len(values) != 2 || values[0] != "inline_json" || values[1] != "blob" {
		t.Fatalf("unexpected storage_mode: %+v", values)
	}
	if trigger.Conditions.Rego != "input.actor == \"admin\"" {
		t.Fatalf("unexpected rego condition: %s", trigger.Conditions.Rego)
	}
	if len(trigger.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(trigger.Actions))
	}
	action := trigger.Actions[0]
	if action.Type != "emit_event" {
		t.Fatalf("unexpected action type: %s", action.Type)
	}
	if action.EventType != "ext.audit" {
		t.Fatalf("unexpected event type: %s", action.EventType)
	}
	if action.PayloadTemplate != "templates/audit.json" {
		t.Fatalf("unexpected payload template: %s", action.PayloadTemplate)
	}
	if action.Metadata == nil || action.Metadata["source"] != "policy" {
		t.Fatalf("expected metadata with source policy, got %+v", action.Metadata)
	}
	if action.Retry == nil || action.Retry.MaxAttempts != 3 || action.Retry.BackoffSeconds != 10 {
		t.Fatalf("unexpected retry config: %+v", action.Retry)
	}
	if overrides.Scope == nil || *overrides.Scope != ScopeDirectory {
		t.Fatalf("expected override scope directory, got %+v", overrides.Scope)
	}
	if overrides.Inheritance == nil || *overrides.Inheritance != InheritanceOverride {
		t.Fatalf("expected override inheritance override, got %+v", overrides.Inheritance)
	}
}

func TestParseEventsManifestErrors(t *testing.T) {
	if _, _, err := ParseEventsManifest([]byte("")); err == nil {
		t.Fatalf("expected error for empty manifest")
	}
	payload := []byte(`{"triggers": []}`)
	if _, _, err := ParseEventsManifest(payload); err == nil {
		t.Fatalf("expected error for missing triggers")
	}
	invalid := []byte(`{"triggers": [{"on": "file.created", "actions": [{"type": "emit_event"}]}]}`)
	if _, _, err := ParseEventsManifest(invalid); err == nil {
		t.Fatalf("expected error for missing action data")
	}
}
