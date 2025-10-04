package service

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
)

const (
	storageModeInlineJSON = "inline_json"
	storageModeBlob       = "blob"
)

type FileService struct {
	DB *gorm.DB
}

type FileDTO struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	DirectoryID  string          `json:"directory_id"`
	Path         string          `json:"path"`
	Version      int64           `json:"version"`
	OriginFileID *string         `json:"origin_file_id"`
	Checksum     *string         `json:"checksum"`
	Size         *int64          `json:"size"`
	MimeType     *string         `json:"mime_type"`
	StorageMode  string          `json:"storage_mode"`
	BlobKey      *string         `json:"blob_key,omitempty"`
	InlineJSON   json.RawMessage `json:"inline_json,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
}

type FileVersionDTO struct {
	ID          string          `json:"id"`
	Index       int             `json:"index"`
	StorageMode string          `json:"storage_mode"`
	BlobKey     *string         `json:"blob_key,omitempty"`
	JSONPayload json.RawMessage `json:"json_payload,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

type FileVersionData struct {
	StorageMode string
	BlobKey     *string
	JSONPayload []byte
	Metadata    map[string]any
	Checksum    *string
	Size        *int64
	MimeType    *string
	Actor       string
}

type CreateFileInput struct {
	DirectoryID  string
	Name         string
	OriginFileID *string
	VersionData  FileVersionData
	RequestID    string
}

type UpdateFileInput struct {
	FileID          string
	NewName         *string
	NewDirectoryID  *string
	VersionData     *FileVersionData
	ExpectedVersion *int64
	RequestID       string
}

type DeleteFileInput struct {
	FileID          string
	ExpectedVersion *int64
	RequestID       string
}

func NewFileService(db *gorm.DB) *FileService {
	return &FileService{DB: db}
}

func (s *FileService) Create(ctx context.Context, in CreateFileInput) (FileDTO, error) {
	if strings.TrimSpace(in.Name) == "" {
		return FileDTO{}, ErrInvalidRequest
	}

	var dto FileDTO
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var directory db.Directory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", in.DirectoryID).First(&directory).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrDirectoryNotFound
			}
			return err
		}

		path := buildChildPath(directory.Path, in.Name)

		var existing db.File
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("directory_id = ? AND name = ? AND deleted_at IS NULL", in.DirectoryID, in.Name).
			First(&existing).Error
		if err == nil {
			existing.Version++
			existing.OriginFileID = in.OriginFileID
			if err := s.storeFileVersion(tx, &existing, in.VersionData, true); err != nil {
				return err
			}
			if err := tx.Model(&db.File{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"origin_file_id": existing.OriginFileID,
					"path":           existing.Path,
					"version":        existing.Version,
				}).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", existing.ID).First(&existing).Error; err != nil {
				return err
			}
			version, err := s.loadVersionByID(tx, existing.CurrentVersionID)
			if err != nil {
				return err
			}
			dto = mapFile(existing, version)
			if _, err := persistEvent(ctx, tx, EventPayload{
				EventType: "file.updated",
				SubjectID: existing.ID,
				RequestID: in.RequestID,
				Data:      dto,
				Scopes: ScopeSet{
					DirectoryIDs: []string{existing.DirectoryID},
					FileIDs:      []string{existing.ID},
				},
			}); err != nil {
				return err
			}
			return nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		file := db.File{
			ID:           uuid.NewString(),
			DirectoryID:  in.DirectoryID,
			Name:         in.Name,
			Path:         path,
			Version:      1,
			OriginFileID: in.OriginFileID,
		}

		if err := tx.Create(&file).Error; err != nil {
			return err
		}

		if err := s.storeFileVersion(tx, &file, in.VersionData, false); err != nil {
			return err
		}

		if err := tx.Where("id = ?", file.ID).First(&file).Error; err != nil {
			return err
		}
		version, err := s.loadVersionByID(tx, file.CurrentVersionID)
		if err != nil {
			return err
		}
		dto = mapFile(file, version)

		if in.OriginFileID != nil {
			rel := db.FileRelation{ParentFileID: *in.OriginFileID, ChildFileID: file.ID, RelationType: "derivative"}
			if err := tx.Create(&rel).Error; err != nil {
				return err
			}
		}

		if _, err := persistEvent(ctx, tx, EventPayload{
			EventType: "file.created",
			SubjectID: file.ID,
			RequestID: in.RequestID,
			Data:      dto,
			Scopes: ScopeSet{
				DirectoryIDs: []string{file.DirectoryID},
				FileIDs:      []string{file.ID},
			},
		}); err != nil {
			return err
		}
		return nil
	})

	return dto, err
}

func (s *FileService) Update(ctx context.Context, in UpdateFileInput) (FileDTO, error) {
	var dto FileDTO
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var file db.File
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", in.FileID).First(&file).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrFileNotFound
			}
			return err
		}

		if in.ExpectedVersion != nil && file.Version != *in.ExpectedVersion {
			return ErrVersionConflict
		}

		originalDirectoryID := file.DirectoryID
		originalPath := file.Path

		if in.NewDirectoryID != nil && *in.NewDirectoryID != file.DirectoryID {
			var directory db.Directory
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", *in.NewDirectoryID).First(&directory).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return ErrDirectoryNotFound
				}
				return err
			}
			file.DirectoryID = directory.ID
			file.Path = buildChildPath(directory.Path, file.Name)
		}

		if in.NewName != nil && strings.TrimSpace(*in.NewName) != "" && *in.NewName != file.Name {
			newName := strings.TrimSpace(*in.NewName)
			var count int64
			if err := tx.Model(&db.File{}).
				Where("directory_id = ? AND name = ? AND id <> ? AND deleted_at IS NULL", file.DirectoryID, newName, file.ID).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrNameConflict
			}
			file.Name = newName
			file.Path = buildChildPath(path.Dir(originalPath), newName)
		}

		if in.VersionData != nil {
			file.Version++
			if err := s.storeFileVersion(tx, &file, *in.VersionData, true); err != nil {
				return err
			}
		}

		if err := tx.Model(&db.File{}).Where("id = ?", file.ID).
			Updates(map[string]any{
				"directory_id": file.DirectoryID,
				"name":         file.Name,
				"path":         file.Path,
				"version":      file.Version,
				"checksum":     file.Checksum,
				"size":         file.Size,
				"mime_type":    file.MimeType,
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("id = ?", file.ID).First(&file).Error; err != nil {
			return err
		}
		version, err := s.loadVersionByID(tx, file.CurrentVersionID)
		if err != nil {
			return err
		}
		dto = mapFile(file, version)

		if _, err := persistEvent(ctx, tx, EventPayload{
			EventType: "file.updated",
			SubjectID: file.ID,
			RequestID: in.RequestID,
			Data: map[string]any{
				"file":             dto,
				"old_directory_id": originalDirectoryID,
				"old_path":         originalPath,
			},
			Scopes: ScopeSet{
				DirectoryIDs: dedupeScopes(originalDirectoryID, file.DirectoryID),
				FileIDs:      []string{file.ID},
			},
		}); err != nil {
			return err
		}
		return nil
	})

	return dto, err
}

func (s *FileService) Delete(ctx context.Context, in DeleteFileInput) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var file db.File
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", in.FileID).First(&file).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrFileNotFound
			}
			return err
		}
		if in.ExpectedVersion != nil && file.Version != *in.ExpectedVersion {
			return ErrVersionConflict
		}

		if err := tx.Delete(&file).Error; err != nil {
			return err
		}

		if _, err := persistEvent(ctx, tx, EventPayload{
			EventType: "file.deleted",
			SubjectID: file.ID,
			RequestID: in.RequestID,
			Data: map[string]any{
				"file_id":      file.ID,
				"directory_id": file.DirectoryID,
			},
			Scopes: ScopeSet{
				DirectoryIDs: []string{file.DirectoryID},
				FileIDs:      []string{file.ID},
			},
		}); err != nil {
			return err
		}
		return nil
	})
}

func (s *FileService) ListVersions(ctx context.Context, fileID string) ([]FileVersionDTO, error) {
	var file db.File
	if err := s.DB.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", fileID).First(&file).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrFileNotFound
		}
		return nil, err
	}

	var versions []db.FileVersion
	if err := s.DB.WithContext(ctx).Where("file_id = ?", fileID).Order("created_at ASC").Find(&versions).Error; err != nil {
		return nil, err
	}

	result := make([]FileVersionDTO, 0, len(versions))
	for idx, v := range versions {
		var metadata map[string]any
		if len(v.MetadataJSON) > 0 {
			if err := json.Unmarshal(v.MetadataJSON, &metadata); err != nil {
				metadata = map[string]any{"_raw": string(v.MetadataJSON)}
			}
		}
		var payload json.RawMessage
		if len(v.JSONPayload) > 0 {
			payload = json.RawMessage(append([]byte(nil), v.JSONPayload...))
		}
		result = append(result, FileVersionDTO{
			ID:          v.ID,
			Index:       idx + 1,
			StorageMode: v.StorageMode,
			BlobKey:     v.BlobKey,
			JSONPayload: payload,
			Metadata:    metadata,
			CreatedBy:   v.CreatedBy,
			CreatedAt:   v.CreatedAt,
		})
	}
	return result, nil
}

func (s *FileService) GetByID(ctx context.Context, id string) (FileDTO, error) {
	var file db.File
	if err := s.DB.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&file).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return FileDTO{}, ErrFileNotFound
		}
		return FileDTO{}, err
	}
	version, err := s.loadVersionByID(s.DB, file.CurrentVersionID)
	if err != nil {
		return FileDTO{}, err
	}
	return mapFile(file, version), nil
}

func (s *FileService) ResolvePath(ctx context.Context, p string) (FileDTO, error) {
	cleaned := normalizePath(p)
	var file db.File
	if err := s.DB.WithContext(ctx).Where("path = ? AND deleted_at IS NULL", cleaned).First(&file).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return FileDTO{}, ErrFileNotFound
		}
		return FileDTO{}, err
	}
	version, err := s.loadVersionByID(s.DB, file.CurrentVersionID)
	if err != nil {
		return FileDTO{}, err
	}
	return mapFile(file, version), nil
}

func (s *FileService) storeFileVersion(tx *gorm.DB, file *db.File, data FileVersionData, increment bool) error {
	if err := validateVersionData(data); err != nil {
		return err
	}

	meta := []byte("{}")
	if data.Metadata != nil {
		encoded, err := json.Marshal(data.Metadata)
		if err != nil {
			return err
		}
		meta = encoded
	}

	version := db.FileVersion{
		ID:           uuid.NewString(),
		FileID:       file.ID,
		StorageMode:  data.StorageMode,
		MetadataJSON: datatypes.JSON(meta),
		CreatedBy:    data.Actor,
	}

	if data.StorageMode == storageModeInlineJSON {
		version.JSONPayload = datatypes.JSON(data.JSONPayload)
	} else {
		version.BlobKey = data.BlobKey
	}

	if err := tx.Create(&version).Error; err != nil {
		return err
	}

	updates := map[string]any{
		"current_version_id": version.ID,
		"checksum":           data.Checksum,
		"size":               data.Size,
		"mime_type":          data.MimeType,
		"updated_at":         time.Now(),
	}
	if increment {
		updates["version"] = file.Version
	}
	if err := tx.Model(&db.File{}).Where("id = ?", file.ID).Updates(updates).Error; err != nil {
		return err
	}
	file.CurrentVersionID = version.ID
	file.Checksum = data.Checksum
	file.Size = data.Size
	file.MimeType = data.MimeType
	return nil
}

func validateVersionData(data FileVersionData) error {
	switch data.StorageMode {
	case storageModeInlineJSON:
		if len(data.JSONPayload) == 0 {
			return errors.New("inline_json storage requires json payload")
		}
	case storageModeBlob:
		if data.BlobKey == nil || strings.TrimSpace(*data.BlobKey) == "" {
			return errors.New("blob storage requires blob key")
		}
	default:
		return errors.New("unknown storage mode")
	}
	if strings.TrimSpace(data.Actor) == "" {
		return errors.New("actor required for version")
	}
	return nil
}

func mapFile(f db.File, version *db.FileVersion) FileDTO {
	var inline json.RawMessage
	var blobKey *string
	storageMode := ""
	if version != nil {
		storageMode = version.StorageMode
		switch storageMode {
		case storageModeInlineJSON:
			if len(version.JSONPayload) > 0 {
				inline = json.RawMessage([]byte(version.JSONPayload))
			}
		case storageModeBlob:
			blobKey = version.BlobKey
		}
	}
	return FileDTO{
		ID:           f.ID,
		Name:         f.Name,
		DirectoryID:  f.DirectoryID,
		Path:         f.Path,
		Version:      f.Version,
		OriginFileID: f.OriginFileID,
		Checksum:     f.Checksum,
		Size:         f.Size,
		MimeType:     f.MimeType,
		StorageMode:  storageMode,
		BlobKey:      blobKey,
		InlineJSON:   inline,
		CreatedAt:    f.CreatedAt,
		UpdatedAt:    f.UpdatedAt,
		DeletedAt:    f.DeletedAt,
	}
}

func (s *FileService) loadVersionByID(tx *gorm.DB, id string) (*db.FileVersion, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	var version db.FileVersion
	if err := tx.Where("id = ?", id).First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &version, nil
}
