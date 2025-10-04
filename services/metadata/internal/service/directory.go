package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type DirectoryService struct {
	DB *gorm.DB
}

type CreateDirectoryInput struct {
	Name      string
	ParentID  *string
	RequestID string
}

type UpdateDirectoryInput struct {
	DirectoryID     string
	NewName         *string
	NewParentID     *string
	ExpectedVersion *int64
	RequestID       string
}

type DeleteDirectoryInput struct {
	DirectoryID     string
	ExpectedVersion *int64
	RequestID       string
	Force           bool
}

type DirectoryDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id"`
	Path      string    `json:"path"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ListDirectoryInput struct {
	ParentID  *string
	Recursive bool
}

type ListDirectoryOutput struct {
	Directories []DirectoryDTO `json:"directories"`
	Files       []FileDTO      `json:"files"`
}

func NewDirectoryService(db *gorm.DB) *DirectoryService {
	return &DirectoryService{DB: db}
}

func (s *DirectoryService) Create(ctx context.Context, in CreateDirectoryInput) (DirectoryDTO, error) {
	if strings.TrimSpace(in.Name) == "" {
		return DirectoryDTO{}, ErrInvalidRequest
	}

	var dto DirectoryDTO
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent *db.Directory
		var parentPath string
		if in.ParentID != nil {
			parent = &db.Directory{}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", *in.ParentID).First(parent).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return ErrDirectoryNotFound
				}
				return err
			}
			parentPath = parent.Path
		}

		path := buildChildPath(parentPath, in.Name)
		pathHash := hashPath(path)

		var count int64
		query := tx.Model(&db.Directory{}).Where("name = ? AND deleted_at IS NULL", in.Name)
		if in.ParentID != nil {
			query = query.Where("parent_id = ?", *in.ParentID)
		} else {
			query = query.Where("parent_id IS NULL")
		}
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrNameConflict
		}

		directory := db.Directory{
			ID:       uuid.NewString(),
			ParentID: in.ParentID,
			Name:     in.Name,
			Path:     path,
			PathHash: pathHash,
			Version:  1,
		}

		if err := tx.Create(&directory).Error; err != nil {
			return err
		}

		dto = mapDirectory(directory)

		if _, err := persistEvent(ctx, tx, EventPayload{
			EventType: "directory.created",
			SubjectID: directory.ID,
			RequestID: in.RequestID,
			Data:      dto,
			Scopes: ScopeSet{
				DirectoryIDs: append(scopeFromParent(parent), directory.ID),
			},
		}); err != nil {
			return err
		}
		return nil
	})

	return dto, err
}

func (s *DirectoryService) List(ctx context.Context, in ListDirectoryInput) (ListDirectoryOutput, error) {
	tx := s.DB.WithContext(ctx)

	var directories []db.Directory
	if in.Recursive && in.ParentID != nil {
		var parent db.Directory
		if err := tx.Where("id = ? AND deleted_at IS NULL", *in.ParentID).First(&parent).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ListDirectoryOutput{}, ErrDirectoryNotFound
			}
			return ListDirectoryOutput{}, err
		}
		prefix := ensureTrailingSlash(parent.Path)
		if err := tx.Where("path LIKE ? AND deleted_at IS NULL", prefix+"%").Order("path ASC").Find(&directories).Error; err != nil {
			return ListDirectoryOutput{}, err
		}
	} else {
		query := tx.Where("deleted_at IS NULL")
		if in.ParentID != nil {
			query = query.Where("parent_id = ?", *in.ParentID)
		} else {
			query = query.Where("parent_id IS NULL")
		}
		if err := query.Order("name ASC").Find(&directories).Error; err != nil {
			return ListDirectoryOutput{}, err
		}
	}

	dirDTOs := make([]DirectoryDTO, 0, len(directories))
	directoryIDs := make([]string, 0, len(directories))
	for _, d := range directories {
		dirDTOs = append(dirDTOs, mapDirectory(d))
		directoryIDs = append(directoryIDs, d.ID)
	}

	files, err := fetchFiles(ctx, tx, in, directories)
	if err != nil {
		return ListDirectoryOutput{}, err
	}

	return ListDirectoryOutput{Directories: dirDTOs, Files: files}, nil
}

func (s *DirectoryService) GetByID(ctx context.Context, id string) (DirectoryDTO, error) {
	var directory db.Directory
	if err := s.DB.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&directory).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return DirectoryDTO{}, ErrDirectoryNotFound
		}
		return DirectoryDTO{}, err
	}
	return mapDirectory(directory), nil
}

func (s *DirectoryService) ResolvePath(ctx context.Context, path string) (DirectoryDTO, error) {
	cleaned := normalizePath(path)
	var directory db.Directory
	if err := s.DB.WithContext(ctx).Where("path_hash = ? AND deleted_at IS NULL", hashPath(cleaned)).First(&directory).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return DirectoryDTO{}, ErrDirectoryNotFound
		}
		return DirectoryDTO{}, err
	}
	return mapDirectory(directory), nil
}

func (s *DirectoryService) Update(ctx context.Context, in UpdateDirectoryInput) (DirectoryDTO, error) {
	var dto DirectoryDTO
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var directory db.Directory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", in.DirectoryID).First(&directory).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrDirectoryNotFound
			}
			return err
		}
		if in.ExpectedVersion != nil && directory.Version != *in.ExpectedVersion {
			return ErrVersionConflict
		}

		oldParentID := directory.ParentID
		var parentPath string
		newParentID := directory.ParentID
		if in.NewParentID != nil {
			if *in.NewParentID == "" {
				newParentID = nil
			} else {
				var parent db.Directory
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", *in.NewParentID).First(&parent).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						return ErrDirectoryNotFound
					}
					return err
				}
				newParentID = &parent.ID
				parentPath = parent.Path
			}
		}

		newName := directory.Name
		if in.NewName != nil && strings.TrimSpace(*in.NewName) != "" {
			newName = strings.TrimSpace(*in.NewName)
		}

		if parentPath == "" && newParentID != nil {
			var parent db.Directory
			if err := tx.Where("id = ?", *newParentID).First(&parent).Error; err == nil {
				parentPath = parent.Path
			}
		}

		if err := ensureDirectoryNameUnique(tx, newParentID, newName, directory.ID); err != nil {
			return err
		}

		oldPath := directory.Path
		newPath := buildChildPath(parentPath, newName)

		if err := updateDescendantPaths(tx, directory.ID, oldPath, newPath); err != nil {
			return err
		}

		directory.Name = newName
		directory.ParentID = newParentID
		directory.Path = newPath
		directory.PathHash = hashPath(newPath)
		directory.Version++

		if err := tx.Model(&db.Directory{}).Where("id = ?", directory.ID).Updates(map[string]any{
			"name":      directory.Name,
			"parent_id": directory.ParentID,
			"path":      directory.Path,
			"path_hash": directory.PathHash,
			"version":   directory.Version,
		}).Error; err != nil {
			return err
		}

		dto = mapDirectory(directory)

		if _, err := persistEvent(ctx, tx, EventPayload{
			EventType: "directory.updated",
			SubjectID: directory.ID,
			RequestID: in.RequestID,
			Data: map[string]any{
				"directory":     dto,
				"old_path":      oldPath,
				"new_path":      newPath,
				"old_parent_id": cloneStringPointer(oldParentID),
				"new_parent_id": cloneStringPointer(newParentID),
			},
			Scopes: ScopeSet{
				DirectoryIDs: dedupeScopes(directory.ID, stringPtrValue(oldParentID), stringPtrValue(newParentID)),
			},
		}); err != nil {
			return err
		}
		return nil
	})
	return dto, err
}

func (s *DirectoryService) Delete(ctx context.Context, in DeleteDirectoryInput) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var directory db.Directory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", in.DirectoryID).First(&directory).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrDirectoryNotFound
			}
			return err
		}
		if in.ExpectedVersion != nil && directory.Version != *in.ExpectedVersion {
			return ErrVersionConflict
		}

		if !in.Force {
			var count int64
			if err := tx.Model(&db.Directory{}).Where("parent_id = ? AND deleted_at IS NULL", directory.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrInvalidRequest
			}
			if err := tx.Model(&db.File{}).Where("directory_id = ? AND deleted_at IS NULL", directory.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrInvalidRequest
			}
		}

		if err := tx.Delete(&directory).Error; err != nil {
			return err
		}

		if _, err := persistEvent(ctx, tx, EventPayload{
			EventType: "directory.deleted",
			SubjectID: directory.ID,
			RequestID: in.RequestID,
			Data: map[string]any{
				"directory_id": directory.ID,
			},
			Scopes: ScopeSet{
				DirectoryIDs: dedupeScopes(directory.ID, stringPtrValue(directory.ParentID)),
			},
		}); err != nil {
			return err
		}
		return nil
	})
}

func mapDirectory(d db.Directory) DirectoryDTO {
	return DirectoryDTO{
		ID:        d.ID,
		Name:      d.Name,
		ParentID:  d.ParentID,
		Path:      d.Path,
		Version:   d.Version,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}

func fetchFiles(ctx context.Context, tx *gorm.DB, in ListDirectoryInput, dirs []db.Directory) ([]FileDTO, error) {
	if len(dirs) == 0 && in.ParentID == nil {
		return []FileDTO{}, nil
	}

	query := tx.Where("deleted_at IS NULL")
	if in.Recursive && in.ParentID != nil {
		var parent db.Directory
		if err := tx.Where("id = ? AND deleted_at IS NULL", *in.ParentID).First(&parent).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, ErrDirectoryNotFound
			}
			return nil, err
		}
		prefix := ensureTrailingSlash(parent.Path)
		query = query.Where("path LIKE ?", prefix+"%")
	} else if in.ParentID != nil {
		query = query.Where("directory_id = ?", *in.ParentID)
	} else {
		// root level files (parent_id IS NULL directories) fetch by directory ids
		ids := make([]string, 0, len(dirs))
		for _, d := range dirs {
			ids = append(ids, d.ID)
		}
		if len(ids) == 0 {
			return []FileDTO{}, nil
		}
		query = query.Where("directory_id IN ?", ids)
	}

	var files []db.File
	if err := query.Order("path ASC").Find(&files).Error; err != nil {
		return nil, err
	}

	versionMap, err := loadVersionsForFiles(tx, files)
	if err != nil {
		return nil, err
	}

	result := make([]FileDTO, 0, len(files))
	for _, f := range files {
		result = append(result, mapFile(f, versionMap[f.CurrentVersionID]))
	}
	return result, nil
}

func loadVersionsForFiles(tx *gorm.DB, files []db.File) (map[string]*db.FileVersion, error) {
	result := make(map[string]*db.FileVersion, len(files))
	ids := make([]string, 0, len(files))
	for _, f := range files {
		if id := strings.TrimSpace(f.CurrentVersionID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	var versions []db.FileVersion
	if err := tx.Where("id IN ?", ids).Find(&versions).Error; err != nil {
		return nil, err
	}
	for i := range versions {
		v := versions[i]
		result[v.ID] = &versions[i]
	}
	return result, nil
}

func buildChildPath(parentPath, name string) string {
	if parentPath == "" {
		return "/" + strings.TrimLeft(name, "/")
	}
	return ensureTrailingSlash(parentPath) + strings.TrimLeft(name, "/")
}

func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

func hashPath(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func normalizePath(p string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(p))
	if cleaned == "." {
		return "/"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func scopeFromParent(parent *db.Directory) []string {
	if parent == nil {
		return nil
	}
	return []string{parent.ID}
}

func ensureDirectoryNameUnique(tx *gorm.DB, parentID *string, name string, excludeID string) error {
	query := tx.Model(&db.Directory{}).Where("name = ? AND deleted_at IS NULL", name)
	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}
	if excludeID != "" {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrNameConflict
	}
	return nil
}

func updateDescendantPaths(tx *gorm.DB, directoryID string, oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	oldPrefix := ensureTrailingSlash(oldPath)
	newPrefix := ensureTrailingSlash(newPath)

	var subDirs []db.Directory
	if err := tx.Where("path LIKE ? AND deleted_at IS NULL", oldPrefix+"%").Find(&subDirs).Error; err != nil {
		return err
	}
	for _, dir := range subDirs {
		updatedPath := strings.Replace(dir.Path, oldPrefix, newPrefix, 1)
		if err := tx.Model(&db.Directory{}).Where("id = ?", dir.ID).Updates(map[string]any{
			"path":      updatedPath,
			"path_hash": hashPath(updatedPath),
		}).Error; err != nil {
			return err
		}
	}

	var files []db.File
	if err := tx.Where("path LIKE ? AND deleted_at IS NULL", oldPrefix+"%").Find(&files).Error; err != nil {
		return err
	}
	for _, file := range files {
		updatedPath := strings.Replace(file.Path, oldPrefix, newPrefix, 1)
		updates := map[string]any{
			"path": updatedPath,
		}
		if file.DirectoryID == directoryID {
			updates["directory_id"] = directoryID
		}
		if err := tx.Model(&db.File{}).Where("id = ?", file.ID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func cloneStringPointer(v *string) *string {
	if v == nil {
		return nil
	}
	value := *v
	return &value
}
