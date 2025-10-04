package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

const (
	storageModeInlineJSON = "inline_json"
	storageModeBlob       = "blob"
)

type PolicyRegistry interface {
	InvalidateDirectories(directoryIDs ...string)
	Resolve(ctx context.Context, directoryID string) ([]policy.Manifest, error)
}

type PolicyEvaluator interface {
	Evaluate(ctx context.Context, directoryID string, manifests []policy.Manifest, input policy.AuthorizationInput) (bool, error)
	Invalidate(directoryIDs ...string)
}

type PolicyValidator interface {
	Validate(ctx context.Context, directoryID string, manifests []policy.Manifest, input policy.ValidationInput) error
	Invalidate(directoryIDs ...string)
}

type FileService struct {
	DB        *gorm.DB
	policies  PolicyRegistry
	eval      PolicyEvaluator
	validator PolicyValidator
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
	Actor        string
}

type UpdateFileInput struct {
	FileID          string
	NewName         *string
	NewDirectoryID  *string
	VersionData     *FileVersionData
	ExpectedVersion *int64
	RequestID       string
	Actor           string
}

type DeleteFileInput struct {
	FileID          string
	ExpectedVersion *int64
	RequestID       string
	Actor           string
}

func NewFileService(db *gorm.DB, registry PolicyRegistry, evaluator PolicyEvaluator, validator PolicyValidator) *FileService {
	return &FileService{DB: db, policies: registry, eval: evaluator, validator: validator}
}

func (s *FileService) invalidatePolicyCaches(changes map[string]struct{}) {
	if len(changes) == 0 {
		return
	}
	ids := make([]string, 0, len(changes))
	for id := range changes {
		if strings.TrimSpace(id) == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}
	if s.policies != nil {
		s.policies.InvalidateDirectories(ids...)
	}
	if s.eval != nil {
		s.eval.Invalidate(ids...)
	}
	if s.validator != nil {
		s.validator.Invalidate(ids...)
	}
}

func (s *FileService) ensurePolicyAdmin(ctx context.Context, directoryID, actor string) error {
	actor = strings.TrimSpace(actor)
	if actor == "" || strings.EqualFold(actor, "system") || strings.EqualFold(actor, "admin") {
		return nil
	}
	if s.policies == nil {
		return ErrPolicyForbidden
	}
	manifests, err := s.policies.Resolve(ctx, directoryID)
	if err != nil {
		return err
	}
	users := make(map[string]policy.UserPrincipal)
	groups := make(map[string]policy.GroupPrincipal)
	for _, manifest := range manifests {
		if manifest.Principals == nil {
			continue
		}
		for _, user := range manifest.Principals.Users {
			id := strings.ToLower(strings.TrimSpace(user.ID))
			if id == "" {
				continue
			}
			normalized := user
			normalized.ID = strings.TrimSpace(user.ID)
			normalized.Groups = normalizeStringList(user.Groups)
			users[id] = normalized
		}
		for _, group := range manifest.Principals.Groups {
			id := strings.ToLower(strings.TrimSpace(group.ID))
			if id == "" {
				continue
			}
			normalized := group
			normalized.ID = strings.TrimSpace(group.ID)
			normalized.Members = normalizeStringList(group.Members)
			groups[id] = normalized
		}
	}
	if len(users) == 0 && len(groups) == 0 {
		return ErrPolicyForbidden
	}
	actorLower := strings.ToLower(actor)
	if user, ok := users[actorLower]; ok {
		for _, groupID := range user.Groups {
			if isAdminGroup(groupID) {
				return nil
			}
			if grp, ok := groups[strings.ToLower(groupID)]; ok && isAdminGroup(grp.ID) && groupHasMember(grp, actor) {
				return nil
			}
		}
	}
	for _, grp := range groups {
		if !isAdminGroup(grp.ID) {
			continue
		}
		if groupHasMember(grp, actor) {
			return nil
		}
	}
	return ErrPolicyForbidden
}

func (s *FileService) authorize(ctx context.Context, directoryID, fileID, fileName, filePath, actor string, action policy.Action, attributes map[string]any) error {
	if s.eval == nil {
		return nil
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return ErrPolicyForbidden
	}
	if strings.EqualFold(actor, "system") {
		return nil
	}
	if s.policies == nil {
		return ErrPolicyForbidden
	}
	if attributes == nil {
		attributes = make(map[string]any)
	}
	manifests, err := s.policies.Resolve(ctx, directoryID)
	if err != nil {
		return err
	}
	principals := aggregatePrincipals(manifests)
	allowed, err := s.eval.Evaluate(ctx, directoryID, manifests, policy.AuthorizationInput{
		Actor:       actor,
		Action:      action,
		DirectoryID: directoryID,
		FileID:      fileID,
		FileName:    fileName,
		Path:        filePath,
		Principals:  principals,
		Attributes:  attributes,
	})
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPolicyForbidden
	}
	return nil
}

func aggregatePrincipals(manifests []policy.Manifest) policy.PrincipalSet {
	principals := policy.PrincipalSet{}
	for _, manifest := range manifests {
		if manifest.Principals == nil {
			continue
		}
		if len(manifest.Principals.Users) > 0 {
			principals.Users = append(principals.Users, manifest.Principals.Users...)
		}
		if len(manifest.Principals.Groups) > 0 {
			principals.Groups = append(principals.Groups, manifest.Principals.Groups...)
		}
	}
	return principals
}

func (s *FileService) validateSchema(ctx context.Context, directoryID, fileName string, data FileVersionData) error {
	if s == nil || s.validator == nil {
		return nil
	}
	if policy.IsSpecialFile(fileName) {
		return nil
	}
	if strings.TrimSpace(directoryID) == "" {
		return ErrDirectoryNotFound
	}
	if s.policies == nil {
		return ErrInvalidRequest
	}
	manifests, err := s.policies.Resolve(ctx, directoryID)
	if err != nil {
		return err
	}
	document, err := buildSchemaDocument(fileName, data)
	if err != nil {
		return err
	}
	if err := s.validator.Validate(ctx, directoryID, manifests, policy.ValidationInput{
		FileName: fileName,
		Document: document,
	}); err != nil {
		var schemaErr *policy.SchemaValidationError
		if errors.As(err, &schemaErr) {
			return fmt.Errorf("%w: %w", ErrSchemaValidation, schemaErr)
		}
		return err
	}
	return nil
}

func buildSchemaDocument(fileName string, data FileVersionData) (map[string]any, error) {
	fileInfo := map[string]any{
		"name":         strings.TrimSpace(fileName),
		"storage_mode": data.StorageMode,
	}
	if data.BlobKey != nil {
		fileInfo["blob_key"] = strings.TrimSpace(*data.BlobKey)
	}
	if data.Checksum != nil {
		fileInfo["checksum"] = strings.TrimSpace(*data.Checksum)
	}
	if data.Size != nil {
		fileInfo["size"] = *data.Size
	}
	if data.MimeType != nil {
		fileInfo["mime_type"] = strings.TrimSpace(*data.MimeType)
	}

	document := map[string]any{
		"file": fileInfo,
	}
	if data.Metadata != nil {
		document["metadata"] = data.Metadata
	}
	if len(data.JSONPayload) > 0 {
		var payload any
		if err := json.Unmarshal(data.JSONPayload, &payload); err != nil {
			return nil, fmt.Errorf("%w: invalid json payload: %v", ErrInvalidRequest, err)
		}
		document["payload"] = payload
	}
	return document, nil
}

func buildCreateAttributes(in CreateFileInput) map[string]any {
	attrs := map[string]any{
		"request_id":   in.RequestID,
		"version_data": buildVersionAttributes(in.VersionData),
	}
	if in.OriginFileID != nil {
		attrs["origin_file_id"] = *in.OriginFileID
	}
	return attrs
}

func buildUpdateAttributes(in UpdateFileInput, file db.File, originalDirectoryID, originalName, originalPath string) map[string]any {
	attrs := map[string]any{
		"request_id":            in.RequestID,
		"original_directory_id": originalDirectoryID,
		"original_name":         originalName,
		"original_path":         originalPath,
		"current_version":       file.Version,
	}
	if in.NewDirectoryID != nil {
		attrs["new_directory_id"] = *in.NewDirectoryID
	}
	if in.NewName != nil {
		attrs["new_name"] = strings.TrimSpace(*in.NewName)
	}
	if in.ExpectedVersion != nil {
		attrs["expected_version"] = *in.ExpectedVersion
	}
	if in.VersionData != nil {
		attrs["version_data"] = buildVersionAttributes(*in.VersionData)
	}
	return attrs
}

func buildDeleteAttributes(in DeleteFileInput, file db.File) map[string]any {
	attrs := map[string]any{
		"request_id":   in.RequestID,
		"name":         file.Name,
		"version":      file.Version,
		"directory_id": file.DirectoryID,
	}
	if in.ExpectedVersion != nil {
		attrs["expected_version"] = *in.ExpectedVersion
	}
	return attrs
}

func buildVersionAttributes(data FileVersionData) map[string]any {
	attrs := map[string]any{
		"storage_mode": data.StorageMode,
		"actor":        data.Actor,
	}
	if data.BlobKey != nil {
		attrs["blob_key"] = *data.BlobKey
	}
	if len(data.JSONPayload) > 0 {
		attrs["json_bytes"] = len(data.JSONPayload)
	}
	if data.Metadata != nil {
		attrs["metadata"] = data.Metadata
	}
	if data.Checksum != nil {
		attrs["checksum"] = *data.Checksum
	}
	if data.Size != nil {
		attrs["size"] = *data.Size
	}
	if data.MimeType != nil {
		attrs["mime_type"] = *data.MimeType
	}
	return attrs
}

func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func isAdminGroup(id string) bool {
	return strings.EqualFold(id, "admin") || strings.EqualFold(id, "admins")
}

func groupHasMember(group policy.GroupPrincipal, actor string) bool {
	for _, member := range group.Members {
		if strings.EqualFold(strings.TrimSpace(member), actor) {
			return true
		}
	}
	return false
}

func (s *FileService) Create(ctx context.Context, in CreateFileInput) (FileDTO, error) {
	if strings.TrimSpace(in.Name) == "" {
		return FileDTO{}, ErrInvalidRequest
	}
	if policy.IsSpecialFile(in.Name) {
		if err := s.ensurePolicyAdmin(ctx, in.DirectoryID, in.Actor); err != nil {
			return FileDTO{}, err
		}
	}

	var dto FileDTO
	invalidateDirs := make(map[string]struct{})
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var directory db.Directory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", in.DirectoryID).First(&directory).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrDirectoryNotFound
			}
			return err
		}

		path := buildChildPath(directory.Path, in.Name)

		if err := s.authorize(ctx, directory.ID, "", in.Name, path, in.Actor, policy.ActionCreate, buildCreateAttributes(in)); err != nil {
			return err
		}

		if err := s.validateSchema(ctx, directory.ID, in.Name, in.VersionData); err != nil {
			return err
		}

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
			if policy.IsSpecialFile(existing.Name) {
				invalidateDirs[existing.DirectoryID] = struct{}{}
			}
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
		if policy.IsSpecialFile(file.Name) {
			invalidateDirs[file.DirectoryID] = struct{}{}
		}

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

	if err == nil {
		s.invalidatePolicyCaches(invalidateDirs)
	}
	return dto, err
}

func (s *FileService) Update(ctx context.Context, in UpdateFileInput) (FileDTO, error) {
	var dto FileDTO
	invalidateDirs := make(map[string]struct{})
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
		originalName := file.Name
		originalPath := file.Path
		if policy.IsSpecialFile(originalName) {
			if err := s.ensurePolicyAdmin(ctx, originalDirectoryID, in.Actor); err != nil {
				return err
			}
		}

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
			if err := s.validateSchema(ctx, file.DirectoryID, file.Name, *in.VersionData); err != nil {
				return err
			}
			file.Version++
			if err := s.storeFileVersion(tx, &file, *in.VersionData, true); err != nil {
				return err
			}
		}

		if policy.IsSpecialFile(file.Name) {
			if err := s.ensurePolicyAdmin(ctx, file.DirectoryID, in.Actor); err != nil {
				return err
			}
		}

		if err := s.authorize(ctx, file.DirectoryID, file.ID, file.Name, file.Path, in.Actor, policy.ActionUpdate, buildUpdateAttributes(in, file, originalDirectoryID, originalName, originalPath)); err != nil {
			return err
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
		if policy.IsSpecialFile(originalName) {
			invalidateDirs[originalDirectoryID] = struct{}{}
		}
		if policy.IsSpecialFile(file.Name) {
			invalidateDirs[file.DirectoryID] = struct{}{}
		}

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

	if err == nil {
		s.invalidatePolicyCaches(invalidateDirs)
	}
	return dto, err
}

func (s *FileService) Delete(ctx context.Context, in DeleteFileInput) error {
	invalidateDirs := make(map[string]struct{})
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

		if policy.IsSpecialFile(file.Name) {
			if err := s.ensurePolicyAdmin(ctx, file.DirectoryID, in.Actor); err != nil {
				return err
			}
			invalidateDirs[file.DirectoryID] = struct{}{}
		}

		if err := s.authorize(ctx, file.DirectoryID, file.ID, file.Name, file.Path, in.Actor, policy.ActionDelete, buildDeleteAttributes(in, file)); err != nil {
			return err
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

	if err == nil {
		s.invalidatePolicyCaches(invalidateDirs)
	}
	return err
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
