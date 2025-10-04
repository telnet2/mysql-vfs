package service

import (
	"context"
	"strings"

	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

type registry interface {
	Resolve(ctx context.Context, directoryID string) ([]policy.Manifest, error)
}

type directoryResolver interface {
	ResolvePath(ctx context.Context, path string) (DirectoryDTO, error)
}

type fileResolver interface {
	ResolvePath(ctx context.Context, path string) (FileDTO, error)
}

type PolicyResolutionResult struct {
	DirectoryID string
	Manifests   []policy.Manifest
	Principals  policy.PrincipalSet
}

type PolicyResolver struct {
	registry    registry
	directories directoryResolver
	files       fileResolver
}

func NewPolicyResolver(reg registry, dirs directoryResolver, files fileResolver) *PolicyResolver {
	return &PolicyResolver{registry: reg, directories: dirs, files: files}
}

func (r *PolicyResolver) Resolve(ctx context.Context, directoryID, path string, filter *policy.Type) (PolicyResolutionResult, error) {
	resolvedID := strings.TrimSpace(directoryID)
	if resolvedID == "" {
		trimmedPath := strings.TrimSpace(path)
		if trimmedPath == "" {
			return PolicyResolutionResult{}, ErrInvalidRequest
		}
		dto, err := r.directories.ResolvePath(ctx, trimmedPath)
		if err == nil {
			resolvedID = dto.ID
		} else if err == ErrDirectoryNotFound {
			fileDTO, fileErr := r.files.ResolvePath(ctx, trimmedPath)
			if fileErr != nil {
				if fileErr == ErrFileNotFound {
					return PolicyResolutionResult{}, ErrDirectoryNotFound
				}
				return PolicyResolutionResult{}, fileErr
			}
			resolvedID = fileDTO.DirectoryID
		} else {
			return PolicyResolutionResult{}, err
		}
	}

	manifests, err := r.registry.Resolve(ctx, resolvedID)
	if err != nil {
		return PolicyResolutionResult{}, err
	}

	if filter != nil {
		filtered := make([]policy.Manifest, 0, len(manifests))
		for _, m := range manifests {
			if m.Type == *filter {
				filtered = append(filtered, m)
			}
		}
		manifests = filtered
	}

	aggregated := policy.PrincipalSet{}
	for _, manifest := range manifests {
		if manifest.Principals == nil {
			continue
		}
		aggregated.Users = append(aggregated.Users, manifest.Principals.Users...)
		aggregated.Groups = append(aggregated.Groups, manifest.Principals.Groups...)
	}

	return PolicyResolutionResult{DirectoryID: resolvedID, Manifests: manifests, Principals: aggregated}, nil
}
