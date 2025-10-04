package policy

import (
	"context"
	"testing"
)

type staticResolver struct {
	manifests map[string][]Manifest
}

func (r *staticResolver) Resolve(_ context.Context, directoryID string) ([]Manifest, error) {
	manifests, ok := r.manifests[directoryID]
	if !ok {
		return nil, ErrDirectoryNotFound
	}
	return manifests, nil
}

func TestTriggerEngineEvaluateMatches(t *testing.T) {
	manifest := DefaultManifest(".events", "/dir/.events", "dir", TypeEvents)
	triggerScope := ScopeFile
	manifest.Events = &EventsConfig{Triggers: []EventTrigger{
		{
			Name:  "profile-created",
			On:    "file.created",
			Scope: &triggerScope,
			Match: EventTriggerMatch{
				"file_name":    []string{"*.profile.json"},
				"storage_mode": []string{"inline_json"},
				"env":          []string{"prod"},
			},
			Actions: []EventAction{{Type: "emit_event", EventType: "ext.profile.created"}},
		},
	}}

	engine := NewTriggerEngine(&staticResolver{manifests: map[string][]Manifest{"dir": {manifest}}})
	ctx := TriggerContext{
		EventType:   "file.created",
		Scope:       ScopeFile,
		FileName:    "user.profile.json",
		FilePath:    "/profiles/user.profile.json",
		StorageMode: "inline_json",
		Attributes:  map[string]any{"env": "prod"},
	}

	matches, err := engine.Evaluate(context.Background(), "dir", nil, ctx)
	if err != nil {
		t.Fatalf("evaluate triggers: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	match := matches[0]
	if match.Trigger == nil {
		t.Fatalf("expected trigger reference")
	}
	if match.Trigger.On != "file.created" {
		t.Fatalf("unexpected trigger on: %s", match.Trigger.On)
	}
	if len(match.Trigger.Actions) != 1 || match.Trigger.Actions[0].EventType != "ext.profile.created" {
		t.Fatalf("unexpected trigger actions: %+v", match.Trigger.Actions)
	}
}

func TestTriggerEngineEvaluatesConditions(t *testing.T) {
	manifest := DefaultManifest(".events", "/dir/.events", "dir", TypeEvents)
	manifest.Events = &EventsConfig{Triggers: []EventTrigger{
		{
			On:         "file.updated",
			Conditions: EventTriggerCondition{Rego: "input.actor == \"admin\""},
			Actions:    []EventAction{{Type: "emit_event", EventType: "ext.audit"}},
		},
	}}

	engine := NewTriggerEngine(&staticResolver{manifests: map[string][]Manifest{"dir": {manifest}}})
	ctx := TriggerContext{EventType: "file.updated", Scope: ScopeFile, Actor: "admin"}

	matches, err := engine.Evaluate(context.Background(), "dir", nil, ctx)
	if err != nil {
		t.Fatalf("evaluate triggers: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected rego condition to allow trigger")
	}

	ctx.Actor = "user"
	matches, err = engine.Evaluate(context.Background(), "dir", nil, ctx)
	if err != nil {
		t.Fatalf("evaluate triggers: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected rego condition to block trigger")
	}
}
