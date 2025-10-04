package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/db"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

func enqueueTriggerActions(ctx context.Context, tx *gorm.DB, matches []policy.TriggerMatch, input policy.TriggerContext) ([]db.Event, error) {
	if len(matches) == 0 {
		return nil, nil
	}
	events := make([]db.Event, 0)
	for _, match := range matches {
		if match.Trigger == nil {
			continue
		}
		for idx, action := range match.Trigger.Actions {
			payload, err := buildActionPayload(match, action, idx, input)
			if err != nil {
				return nil, err
			}
			event, err := persistEvent(ctx, tx, payload)
			if err != nil {
				return nil, err
			}
			events = append(events, event)
		}
	}
	return events, nil
}

func buildActionPayload(match policy.TriggerMatch, action policy.EventAction, actionIdx int, input policy.TriggerContext) (EventPayload, error) {
	actionType := strings.ToLower(strings.TrimSpace(action.Type))
	if actionType == "" {
		return EventPayload{}, fmt.Errorf("policy: trigger action missing type")
	}

	contextDirectory := strings.TrimSpace(input.DirectoryID)
	manifestDirectory := strings.TrimSpace(match.Manifest.DirectoryID)
	subjectID := strings.TrimSpace(input.FileID)
	if subjectID == "" {
		subjectID = contextDirectory
	}
	if subjectID == "" {
		subjectID = manifestDirectory
	}

	scopes := ScopeSet{
		DirectoryIDs: []string{contextDirectory, manifestDirectory},
		FileIDs:      []string{strings.TrimSpace(input.FileID)},
	}

	manifestInfo := map[string]any{
		"name":         match.Manifest.Name,
		"path":         match.Manifest.SourcePath,
		"directory_id": manifestDirectory,
		"type":         match.Manifest.Type,
		"scope":        match.Manifest.Scope,
		"inheritance":  match.Manifest.Inheritance,
	}

	triggerInfo := map[string]any{
		"name":  match.Trigger.Name,
		"on":    match.Trigger.On,
		"match": copyMatchMap(match.Trigger.Match),
		"scope": match.Trigger.Scope,
		"index": actionIdx,
	}

	contextInfo := map[string]any{
		"request_id":   input.RequestID,
		"actor":        input.Actor,
		"scope":        input.Scope,
		"directory_id": contextDirectory,
		"file_id":      strings.TrimSpace(input.FileID),
		"file_name":    input.FileName,
		"file_path":    input.FilePath,
		"storage_mode": input.StorageMode,
		"metadata":     copyAnyMap(input.Metadata),
		"attributes":   copyAnyMap(input.Attributes),
		"payload":      input.Payload,
	}
	if strings.TrimSpace(input.EventType) != "" {
		contextInfo["event_type"] = strings.TrimSpace(input.EventType)
	}

	actionInfo := map[string]any{
		"type":             strings.TrimSpace(action.Type),
		"metadata":         copyAnyMap(action.Metadata),
		"payload_template": strings.TrimSpace(action.PayloadTemplate),
	}
	if action.Retry != nil {
		actionInfo["retry"] = map[string]any{
			"max_attempts":    action.Retry.MaxAttempts,
			"backoff_seconds": action.Retry.BackoffSeconds,
		}
	}

	var eventType string
	switch actionType {
	case "emit_event":
		eventType = strings.TrimSpace(action.EventType)
		if eventType == "" {
			return EventPayload{}, fmt.Errorf("policy: emit_event action missing event_type")
		}
	case "invoke_workflow":
		eventType = "ext.workflow.triggered"
		actionInfo["workflow"] = strings.TrimSpace(action.Workflow)
	case "call_webhook":
		eventType = "ext.webhook.triggered"
		actionInfo["webhook"] = strings.TrimSpace(action.Webhook)
	default:
		return EventPayload{}, fmt.Errorf("policy: unknown trigger action type %q", action.Type)
	}
	actionInfo["event_type"] = eventType

	key := computeActionKey(input, match, action, actionIdx, eventType)
	data := map[string]any{
		"manifest": manifestInfo,
		"trigger":  triggerInfo,
		"context":  contextInfo,
		"action":   actionInfo,
	}

	return EventPayload{
		EventType:      eventType,
		SubjectID:      subjectID,
		RequestID:      strings.TrimSpace(input.RequestID),
		IdempotencyKey: key,
		Source:         "extension",
		Data:           data,
		Scopes:         scopes,
	}, nil
}

func computeActionKey(input policy.TriggerContext, match policy.TriggerMatch, action policy.EventAction, actionIdx int, eventType string) string {
	parts := []string{
		strings.TrimSpace(input.RequestID),
		strings.TrimSpace(eventType),
		strings.TrimSpace(match.Manifest.SourcePath),
		strings.TrimSpace(match.Manifest.DirectoryID),
		strings.TrimSpace(match.Trigger.Name),
		strings.TrimSpace(match.Trigger.On),
		fmt.Sprintf("%d", actionIdx),
		strings.TrimSpace(action.Type),
		strings.TrimSpace(action.EventType),
		strings.TrimSpace(action.Workflow),
		strings.TrimSpace(action.Webhook),
		strings.TrimSpace(input.DirectoryID),
		strings.TrimSpace(input.FileID),
	}
	base := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}

func copyMatchMap(src policy.EventTriggerMatch) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(src))
	for key, values := range src {
		dup := make([]string, len(values))
		copy(dup, values)
		clone[key] = dup
	}
	return clone
}

func copyAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	clone := make(map[string]any, len(src))
	for key, value := range src {
		clone[key] = value
	}
	return clone
}
