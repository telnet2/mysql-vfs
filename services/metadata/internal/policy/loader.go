package policy

import (
	"context"
	"errors"
)

var (
	ErrDirectoryNotFound = errors.New("policy: directory not found")
	ErrCycleDetected     = errors.New("policy: directory cycle detected")
)

type DirectoryNode struct {
	ID       string
	ParentID *string
	Path     string
}

type DirectoryRepository interface {
	GetDirectory(ctx context.Context, id string) (DirectoryNode, error)
}

type ManifestRepository interface {
	ListManifests(ctx context.Context, directoryID string) ([]Manifest, error)
}

type Loader struct {
	directories DirectoryRepository
	manifests   ManifestRepository
}

func NewLoader(dirRepo DirectoryRepository, manifestRepo ManifestRepository) *Loader {
	return &Loader{directories: dirRepo, manifests: manifestRepo}
}

func (l *Loader) Resolve(ctx context.Context, directoryID string) ([]Manifest, error) {
	if directoryID == "" {
		return nil, ErrDirectoryNotFound
	}
	typeBreak := make(map[Type]bool)
	levels := make([][]Manifest, 0, 4)
	visited := make(map[string]bool)
	current := directoryID

	for current != "" {
		if visited[current] {
			return nil, ErrCycleDetected
		}
		visited[current] = true

		manifests, err := l.manifests.ListManifests(ctx, current)
		if err != nil {
			return nil, err
		}
		filtered := make([]Manifest, 0, len(manifests))
		for _, m := range manifests {
			if typeBreak[m.Type] {
				continue
			}
			filtered = append(filtered, m)
			if m.Inheritance == InheritanceBreak {
				typeBreak[m.Type] = true
			}
		}
		if len(filtered) > 0 {
			levels = append(levels, filtered)
		}

		dir, err := l.directories.GetDirectory(ctx, current)
		if err != nil {
			return nil, err
		}
		if dir.ParentID == nil || *dir.ParentID == "" {
			break
		}
		current = *dir.ParentID
	}

	result := make([]Manifest, 0)
	for i := len(levels) - 1; i >= 0; i-- {
		result = append(result, levels[i]...)
	}
	return result, nil
}
