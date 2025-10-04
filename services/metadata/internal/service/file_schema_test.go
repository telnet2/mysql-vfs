package service

import (
	"context"
	"testing"

	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

type trackingPolicyRegistry struct {
	resolveCalled bool
}

func (t *trackingPolicyRegistry) InvalidateDirectories(directoryIDs ...string) {}

func (t *trackingPolicyRegistry) Resolve(ctx context.Context, directoryID string) ([]policy.Manifest, error) {
	t.resolveCalled = true
	return nil, nil
}

type trackingPolicyValidator struct {
	validateCalled bool
}

func (t *trackingPolicyValidator) Validate(ctx context.Context, directoryID string, manifests []policy.Manifest, input policy.ValidationInput) error {
	t.validateCalled = true
	return nil
}

func (t *trackingPolicyValidator) Invalidate(directoryIDs ...string) {}

func TestValidateSchemaSkipsSpecialFiles(t *testing.T) {
	registry := &trackingPolicyRegistry{}
	validator := &trackingPolicyValidator{}
	svc := &FileService{policies: registry, validator: validator}

	if err := svc.validateSchema(context.Background(), "dir1", ".group", FileVersionData{StorageMode: storageModeInlineJSON}); err != nil {
		t.Fatalf("validateSchema returned error: %v", err)
	}

	if registry.resolveCalled {
		t.Fatalf("expected policy resolution to be skipped for special files")
	}

	if validator.validateCalled {
		t.Fatalf("expected schema validation to be skipped for special files")
	}
}
