package domain

import (
	"crypto/sha256"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
)

func createTestDirectory(t *testing.T, db *gorm.DB, parent *models.Directory, name string) *models.Directory {
	t.Helper()
	pathValue := path.Join(parent.Path, name)
	if parent.Path == "/" {
		pathValue = "/" + name
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(pathValue)))
	parentID := parent.ID
	dir := &models.Directory{
		ID:        uuid.NewString(),
		ParentID:  &parentID,
		Name:      name,
		Path:      pathValue,
		PathHash:  hash,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(dir).Error)
	return dir
}

func createWorkflowFile(t *testing.T, db *gorm.DB, dir *models.Directory, content string) *models.File {
	return createTestFile(t, db, dir, string(SpecialFileTypeWorkflow), content, "application/x-yaml")
}

func createTestFile(t *testing.T, db *gorm.DB, dir *models.Directory, name, content, contentType string) *models.File {
	t.Helper()
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	now := time.Now()
	file := &models.File{
		ID:             uuid.NewString(),
		DirectoryID:    dir.ID,
		Name:           name,
		ContentType:    contentType,
		SizeBytes:      int64(len(content)),
		StorageType:    models.StorageTypeText,
		TextContent:    &content,
		ChecksumSHA256: checksum,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, db.Create(file).Error)

	version := &models.FileVersion{
		ID:             uuid.NewString(),
		FileID:         file.ID,
		VersionNumber:  1,
		ContentType:    file.ContentType,
		SizeBytes:      file.SizeBytes,
		StorageType:    models.StorageTypeText,
		TextContent:    &content,
		ChecksumSHA256: checksum,
		CreatedAt:      now,
	}
	require.NoError(t, db.Create(version).Error)

	return file
}

func updateWorkflowFile(t *testing.T, db *gorm.DB, file *models.File, content string) {
	t.Helper()
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	now := time.Now()

	updates := map[string]interface{}{
		"version":         file.Version + 1,
		"size_bytes":      int64(len(content)),
		"text_content":    content,
		"checksum_sha256": checksum,
		"updated_at":      now,
	}

	require.NoError(t, db.Model(&models.File{}).
		Where("id = ?", file.ID).
		Updates(updates).Error)

	file.Version++
	file.SizeBytes = int64(len(content))
	file.TextContent = &content
	file.ChecksumSHA256 = checksum
	file.UpdatedAt = now

	version := &models.FileVersion{
		ID:             uuid.NewString(),
		FileID:         file.ID,
		VersionNumber:  file.Version,
		ContentType:    file.ContentType,
		SizeBytes:      file.SizeBytes,
		StorageType:    models.StorageTypeText,
		TextContent:    &content,
		ChecksumSHA256: checksum,
		CreatedAt:      now,
	}
	require.NoError(t, db.Create(version).Error)
}
