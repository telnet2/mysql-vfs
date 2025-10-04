package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/repository"
)

// QuotaLoader loads and caches resource quotas from .quota files
type QuotaLoader struct {
	*GenericLoader
}

// NewQuotaLoader creates a new quota loader with caching and inheritance
func NewQuotaLoader(
	fileRepo repository.FileRepository,
	dirRepo repository.DirectoryRepository,
	cacheTTL time.Duration,
) *QuotaLoader {
	return &QuotaLoader{
		GenericLoader: NewGenericLoader(
			fileRepo,
			dirRepo,
			SpecialFileTypeQuota,
			cacheTTL,
		),
	}
}

// LoadQuota loads the quota configuration for a directory
func (l *QuotaLoader) LoadQuota(ctx context.Context, directoryPath string) (*QuotaConfig, error) {
	content, err := l.Load(ctx, directoryPath)
	if err != nil {
		return nil, err
	}

	var quota QuotaConfig
	if err := json.Unmarshal(content, &quota); err != nil {
		return nil, err
	}

	return &quota, nil
}

// CheckQuota checks if an operation would exceed quota limits
func (l *QuotaLoader) CheckQuota(ctx context.Context, directoryPath string, additionalFiles int, additionalBytes int64) error {
	quota, err := l.LoadQuota(ctx, directoryPath)
	if err != nil {
		if err == ErrNotFound {
			// No quota = unlimited
			return nil
		}
		return err
	}

	// TODO: Query current usage from repository
	// For now, this is a placeholder - full implementation would:
	// 1. Get current file count in directory
	// 2. Get current total size
	// 3. Compare with quota limits

	// Example checks (implement fully later):
	if quota.MaxFiles > 0 {
		// currentFiles + additionalFiles > quota.MaxFiles?
	}

	if quota.MaxSizeBytes > 0 {
		// currentBytes + additionalBytes > quota.MaxSizeBytes?
	}

	if quota.MaxFileSize > 0 && additionalBytes > quota.MaxFileSize {
		return ErrQuotaExceeded
	}

	return nil
}
