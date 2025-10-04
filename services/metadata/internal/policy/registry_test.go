package policy

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type fakeResolver struct {
	count int32
	data  map[string][]Manifest
	Err   error
}

func (f *fakeResolver) Resolve(ctx context.Context, directoryID string) ([]Manifest, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	atomic.AddInt32(&f.count, 1)
	manifests := f.data[directoryID]
	res := make([]Manifest, len(manifests))
	copy(res, manifests)
	return res, nil
}

func TestRegistryCachesResults(t *testing.T) {
	resolver := &fakeResolver{data: map[string][]Manifest{
		"dir": {DefaultManifest(".rego", "/dir/.rego", "dir", TypeRego)},
	}}
	registry := NewRegistry(resolver)

	ctx := context.Background()
	first, err := registry.Resolve(ctx, "dir")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	second, err := registry.Resolve(ctx, "dir")
	if err != nil {
		t.Fatalf("resolve second: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("expected cached result with same length")
	}
	if resolver.count != 1 {
		t.Fatalf("expected resolver to be called once, got %d", resolver.count)
	}
	if &first[0] == &second[0] {
		t.Fatalf("expected distinct slices to avoid mutation side-effects")
	}
}

func TestRegistryInvalidateDirectories(t *testing.T) {
	resolver := &fakeResolver{data: map[string][]Manifest{
		"dir": {DefaultManifest(".rego", "/dir/.rego", "dir", TypeRego)},
	}}
	registry := NewRegistry(resolver)
	ctx := context.Background()
	if _, err := registry.Resolve(ctx, "dir"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	registry.InvalidateDirectories("dir")
	if _, err := registry.Resolve(ctx, "dir"); err != nil {
		t.Fatalf("resolve after invalidation: %v", err)
	}
	if resolver.count != 2 {
		t.Fatalf("expected resolver to run twice, got %d", resolver.count)
	}
}

func TestRegistryInvalidateAll(t *testing.T) {
	resolver := &fakeResolver{data: map[string][]Manifest{
		"dir": {DefaultManifest(".rego", "/dir/.rego", "dir", TypeRego)},
	}}
	registry := NewRegistry(resolver)
	ctx := context.Background()
	if _, err := registry.Resolve(ctx, "dir"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	registry.InvalidateAll()
	if _, err := registry.Resolve(ctx, "dir"); err != nil {
		t.Fatalf("resolve after invalidate all: %v", err)
	}
	if resolver.count != 2 {
		t.Fatalf("expected resolver to run twice, got %d", resolver.count)
	}
}

func TestRegistryPropagatesErrors(t *testing.T) {
	expectedErr := errors.New("boom")
	resolver := &fakeResolver{Err: expectedErr}
	registry := NewRegistry(resolver)
	_, err := registry.Resolve(context.Background(), "dir")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
