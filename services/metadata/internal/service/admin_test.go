package service

import (
	"context"
	"testing"

	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

type fakePolicyRegistry struct {
	manifests map[string][]policy.Manifest
}

func (f *fakePolicyRegistry) InvalidateDirectories(directoryIDs ...string) {}

func (f *fakePolicyRegistry) Resolve(ctx context.Context, directoryID string) ([]policy.Manifest, error) {
	if f == nil {
		return nil, nil
	}
	return f.manifests[directoryID], nil
}

func TestEnsurePolicyAdmin_AllowsAdminUser(t *testing.T) {
	registry := &fakePolicyRegistry{manifests: map[string][]policy.Manifest{
		"dir1": {
			{Principals: &policy.PrincipalSet{Users: []policy.UserPrincipal{{ID: "alice", Groups: []string{"admin"}}}}},
		},
	}}
	svc := NewFileService(nil, registry, nil, nil, nil)
	if err := svc.ensurePolicyAdmin(context.Background(), "dir1", "alice"); err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
}

func TestEnsurePolicyAdmin_BlocksNonAdmin(t *testing.T) {
	registry := &fakePolicyRegistry{manifests: map[string][]policy.Manifest{
		"dir1": {
			{Principals: &policy.PrincipalSet{Users: []policy.UserPrincipal{{ID: "bob", Groups: []string{"viewer"}}}}},
		},
	}}
	svc := NewFileService(nil, registry, nil, nil, nil)
	if err := svc.ensurePolicyAdmin(context.Background(), "dir1", "bob"); err != ErrPolicyForbidden {
		t.Fatalf("expected ErrPolicyForbidden, got %v", err)
	}
}

func TestEnsurePolicyAdmin_SystemBypasses(t *testing.T) {
	svc := NewFileService(nil, nil, nil, nil, nil)
	if err := svc.ensurePolicyAdmin(context.Background(), "dir1", "system"); err != nil {
		t.Fatalf("system actor should bypass checks, got %v", err)
	}
}
