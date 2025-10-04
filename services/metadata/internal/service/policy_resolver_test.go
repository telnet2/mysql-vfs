package service

import (
	"context"
	"testing"

	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

type fakeRegistry struct {
	manifests map[string][]policy.Manifest
	Err       error
}

func (f *fakeRegistry) Resolve(ctx context.Context, directoryID string) ([]policy.Manifest, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	items := f.manifests[directoryID]
	res := make([]policy.Manifest, len(items))
	copy(res, items)
	return res, nil
}

type fakeDirectoryService struct {
	paths map[string]DirectoryDTO
}

func (f *fakeDirectoryService) ResolvePath(ctx context.Context, path string) (DirectoryDTO, error) {
	if dto, ok := f.paths[path]; ok {
		return dto, nil
	}
	return DirectoryDTO{}, ErrDirectoryNotFound
}

type fakeFileService struct {
	paths map[string]FileDTO
	Err   error
}

func (f *fakeFileService) ResolvePath(ctx context.Context, path string) (FileDTO, error) {
	if f.Err != nil {
		return FileDTO{}, f.Err
	}
	if dto, ok := f.paths[path]; ok {
		return dto, nil
	}
	return FileDTO{}, ErrFileNotFound
}

func TestPolicyResolverByDirectoryPath(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string][]policy.Manifest{
		"dir1": {
			{Type: policy.TypeRego, Name: ".rego"},
		},
	}}
	resolver := NewPolicyResolver(reg, &fakeDirectoryService{paths: map[string]DirectoryDTO{
		"/docs": {ID: "dir1"},
	}}, &fakeFileService{})
	result, err := resolver.Resolve(context.Background(), "", "/docs", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if result.DirectoryID != "dir1" {
		t.Fatalf("unexpected directory id %s", result.DirectoryID)
	}
	if len(result.Manifests) != 1 || result.Manifests[0].Type != policy.TypeRego {
		t.Fatalf("unexpected manifests: %+v", result.Manifests)
	}
}

func TestPolicyResolverFallsBackToFilePath(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string][]policy.Manifest{
		"dir1": {{Type: policy.TypeWorkflow}},
	}}
	resolver := NewPolicyResolver(reg, &fakeDirectoryService{}, &fakeFileService{paths: map[string]FileDTO{
		"/docs/report.json": {DirectoryID: "dir1"},
	}})
	result, err := resolver.Resolve(context.Background(), "", "/docs/report.json", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if result.DirectoryID != "dir1" {
		t.Fatalf("expected dir1, got %s", result.DirectoryID)
	}
}

func TestPolicyResolverTypeFilter(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string][]policy.Manifest{
		"dir1": {
			{Type: policy.TypeRego},
			{Type: policy.TypeWorkflow},
		},
	}}
	resolver := NewPolicyResolver(reg, &fakeDirectoryService{paths: map[string]DirectoryDTO{
		"/docs": {ID: "dir1"},
	}}, &fakeFileService{})
	filter := policy.TypeWorkflow
	result, err := resolver.Resolve(context.Background(), "", "/docs", &filter)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(result.Manifests) != 1 || result.Manifests[0].Type != policy.TypeWorkflow {
		t.Fatalf("unexpected filtered manifests: %+v", result.Manifests)
	}
}

func TestPolicyResolverRequiresTarget(t *testing.T) {
	resolver := NewPolicyResolver(&fakeRegistry{}, &fakeDirectoryService{}, &fakeFileService{})
	if _, err := resolver.Resolve(context.Background(), "", "", nil); err != ErrInvalidRequest {
		t.Fatalf("expected invalid request, got %v", err)
	}
}

func TestPolicyResolverPropagatesErrors(t *testing.T) {
	resolver := NewPolicyResolver(&fakeRegistry{}, &fakeDirectoryService{paths: map[string]DirectoryDTO{}}, &fakeFileService{Err: ErrInvalidRequest})
	_, err := resolver.Resolve(context.Background(), "", "/unknown", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestPolicyResolverAggregatesPrincipals(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string][]policy.Manifest{
		"dir1": {
			{Type: policy.TypeUser, Principals: &policy.PrincipalSet{Users: []policy.UserPrincipal{{ID: "alice"}}}},
			{Type: policy.TypeGroup, Principals: &policy.PrincipalSet{Groups: []policy.GroupPrincipal{{ID: "admins"}}}},
		},
	}}
	resolver := NewPolicyResolver(reg, &fakeDirectoryService{paths: map[string]DirectoryDTO{"/docs": {ID: "dir1"}}}, &fakeFileService{})
	result, err := resolver.Resolve(context.Background(), "", "/docs", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(result.Principals.Users) != 1 || result.Principals.Users[0].ID != "alice" {
		t.Fatalf("expected aggregated user alice, got %#v", result.Principals.Users)
	}
	if len(result.Principals.Groups) != 1 || result.Principals.Groups[0].ID != "admins" {
		t.Fatalf("expected aggregated group admins, got %#v", result.Principals.Groups)
	}
}
