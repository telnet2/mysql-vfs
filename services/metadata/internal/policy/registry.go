package policy

import (
	"context"
	"sync"
)

type Resolver interface {
	Resolve(ctx context.Context, directoryID string) ([]Manifest, error)
}

type cachedEntry struct {
	manifests []Manifest
}

type Registry struct {
	resolver Resolver
	mu       sync.RWMutex
	cache    map[string]cachedEntry
}

func NewRegistry(resolver Resolver) *Registry {
	return &Registry{
		resolver: resolver,
		cache:    make(map[string]cachedEntry),
	}
}

func (r *Registry) Resolve(ctx context.Context, directoryID string) ([]Manifest, error) {
	if directoryID == "" {
		return nil, ErrDirectoryNotFound
	}

	r.mu.RLock()
	entry, ok := r.cache[directoryID]
	r.mu.RUnlock()
	if ok {
		return cloneManifests(entry.manifests), nil
	}

	manifests, err := r.resolver.Resolve(ctx, directoryID)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[directoryID] = cachedEntry{manifests: cloneManifests(manifests)}
	r.mu.Unlock()

	return cloneManifests(manifests), nil
}

func (r *Registry) InvalidateDirectories(directoryIDs ...string) {
	if len(directoryIDs) == 0 {
		return
	}
	r.mu.Lock()
	for _, id := range directoryIDs {
		if id == "" {
			continue
		}
		delete(r.cache, id)
	}
	r.mu.Unlock()
}

func (r *Registry) InvalidateAll() {
	r.mu.Lock()
	r.cache = make(map[string]cachedEntry)
	r.mu.Unlock()
}

func cloneManifests(src []Manifest) []Manifest {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Manifest, len(src))
	copy(dst, src)
	return dst
}
