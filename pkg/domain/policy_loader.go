package domain

import (
	"context"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/repository"
)

// PolicyLoader loads and caches OPA Rego policies from .rego files
type PolicyLoader struct {
	*GenericLoader
}

// NewPolicyLoader creates a new policy loader with caching and inheritance
func NewPolicyLoader(
	fileRepo repository.FileRepository,
	dirRepo repository.DirectoryRepository,
	cacheTTL time.Duration,
) *PolicyLoader {
	return &PolicyLoader{
		GenericLoader: NewGenericLoader(
			fileRepo,
			dirRepo,
			SpecialFileTypePolicy,
			cacheTTL,
		),
	}
}

// LoadPolicy loads a Rego policy for a directory
func (l *PolicyLoader) LoadPolicy(ctx context.Context, directoryPath string) (string, error) {
	content, err := l.Load(ctx, directoryPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// LoadPolicyBytes loads a Rego policy as bytes
func (l *PolicyLoader) LoadPolicyBytes(ctx context.Context, directoryPath string) ([]byte, error) {
	return l.Load(ctx, directoryPath)
}
