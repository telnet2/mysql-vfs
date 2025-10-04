package policy

import (
	"context"
	"errors"
	"testing"
)

type fakeDirRepo struct {
	nodes map[string]DirectoryNode
}

func (f *fakeDirRepo) GetDirectory(ctx context.Context, id string) (DirectoryNode, error) {
	n, ok := f.nodes[id]
	if !ok {
		return DirectoryNode{}, ErrDirectoryNotFound
	}
	return n, nil
}

type fakeManifestRepo struct {
	data map[string][]Manifest
}

func (f *fakeManifestRepo) ListManifests(ctx context.Context, directoryID string) ([]Manifest, error) {
	if f.data == nil {
		return nil, nil
	}
	items := f.data[directoryID]
	// return a copy to avoid callers mutating shared slice
	res := make([]Manifest, len(items))
	copy(res, items)
	return res, nil
}

func strPtr(s string) *string { return &s }

func TestLoaderResolveOrder(t *testing.T) {
	rootID := "root"
	childID := "child"
	leafID := "leaf"
	dirs := &fakeDirRepo{nodes: map[string]DirectoryNode{
		rootID:  {ID: rootID, ParentID: nil},
		childID: {ID: childID, ParentID: strPtr(rootID)},
		leafID:  {ID: leafID, ParentID: strPtr(childID)},
	}}
	manifests := &fakeManifestRepo{data: map[string][]Manifest{
		rootID: {DefaultManifest(".rego", "/root/.rego", rootID, TypeRego)},
		childID: {
			DefaultManifest(".workflow", "/child/.workflow", childID, TypeWorkflow),
		},
		leafID: {DefaultManifest(".jsonschema", "/leaf/.jsonschema", leafID, TypeJSONSchema)},
	}}
	loader := NewLoader(dirs, manifests)
	resolved, err := loader.Resolve(context.Background(), leafID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	expectedOrder := []Type{TypeRego, TypeWorkflow, TypeJSONSchema}
	if len(resolved) != len(expectedOrder) {
		t.Fatalf("expected %d manifests, got %d", len(expectedOrder), len(resolved))
	}
	for i, typ := range expectedOrder {
		if resolved[i].Type != typ {
			t.Fatalf("expected type[%d]=%s, got %s", i, typ, resolved[i].Type)
		}
	}
}

func TestLoaderInheritanceBreak(t *testing.T) {
	rootID := "root"
	childID := "child"
	dirs := &fakeDirRepo{nodes: map[string]DirectoryNode{
		rootID:  {ID: rootID, ParentID: nil},
		childID: {ID: childID, ParentID: strPtr(rootID)},
	}}
	breakManifest := DefaultManifest(".rego", "/child/.rego", childID, TypeRego).WithInheritance(InheritanceBreak)
	manifests := &fakeManifestRepo{data: map[string][]Manifest{
		rootID:  {DefaultManifest(".rego", "/root/.rego", rootID, TypeRego)},
		childID: {breakManifest},
	}}
	loader := NewLoader(dirs, manifests)
	resolved, err := loader.Resolve(context.Background(), childID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected only child manifest, got %d", len(resolved))
	}
	if resolved[0].DirectoryID != childID {
		t.Fatalf("expected manifest from child directory")
	}
}

func TestLoaderCycleDetection(t *testing.T) {
	a := "a"
	b := "b"
	dirs := &fakeDirRepo{nodes: map[string]DirectoryNode{
		a: {ID: a, ParentID: strPtr(b)},
		b: {ID: b, ParentID: strPtr(a)},
	}}
	loader := NewLoader(dirs, &fakeManifestRepo{})
	_, err := loader.Resolve(context.Background(), a)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestLoaderMissingDirectory(t *testing.T) {
	loader := NewLoader(&fakeDirRepo{nodes: map[string]DirectoryNode{}}, &fakeManifestRepo{})
	_, err := loader.Resolve(context.Background(), "unknown")
	if !errors.Is(err, ErrDirectoryNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestLoaderFiltersBreakPerType(t *testing.T) {
	rootID := "root"
	childID := "child"
	dirs := &fakeDirRepo{nodes: map[string]DirectoryNode{
		rootID:  {ID: rootID, ParentID: nil},
		childID: {ID: childID, ParentID: strPtr(rootID)},
	}}
	childRego := DefaultManifest(".rego", "/child/.rego", childID, TypeRego).WithInheritance(InheritanceBreak)
	rootWorkflow := DefaultManifest(".workflow", "/root/.workflow", rootID, TypeWorkflow)
	manifests := &fakeManifestRepo{data: map[string][]Manifest{
		rootID: {
			DefaultManifest(".rego", "/root/.rego", rootID, TypeRego),
			rootWorkflow,
		},
		childID: {childRego},
	}}
	loader := NewLoader(dirs, manifests)
	resolved, err := loader.Resolve(context.Background(), childID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	expected := []Manifest{rootWorkflow, childRego}
	if len(resolved) != len(expected) {
		t.Fatalf("expected %d manifests, got %d", len(expected), len(resolved))
	}
	for i := range expected {
		if resolved[i].Type != expected[i].Type || resolved[i].DirectoryID != expected[i].DirectoryID {
			t.Fatalf("unexpected manifest at %d: %+v", i, resolved[i])
		}
	}
}
