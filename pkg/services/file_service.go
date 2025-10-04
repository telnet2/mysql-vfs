package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MaxFileSize    = 104857600 // 100MB
	JSONThreshold  = 16777216  // 16MB
	MaxVersions    = 10         // Keep last 10 versions
)

// FileService handles file operations
type FileService struct {
	db           *gorm.DB
	storage      storage.Storage
	filesLoader  *domain.FilesLoader
	eventTrigger domain.EventTrigger // For lifecycle events
}

// NewFileService creates a new file service
func NewFileService(db *gorm.DB, storage storage.Storage) *FileService {
	return &FileService{
		db:      db,
		storage: storage,
	}
}

// NewFileServiceWithValidation creates a new file service with .files validation
func NewFileServiceWithValidation(db *gorm.DB, storage storage.Storage, filesLoader *domain.FilesLoader) *FileService {
	return &FileService{
		db:          db,
		storage:     storage,
		filesLoader: filesLoader,
	}
}

// NewFileServiceWithLifecycle creates a new file service with lifecycle events
func NewFileServiceWithLifecycle(db *gorm.DB, storage storage.Storage, filesLoader *domain.FilesLoader, eventTrigger domain.EventTrigger) *FileService {
	return &FileService{
		db:           db,
		storage:      storage,
		filesLoader:  filesLoader,
		eventTrigger: eventTrigger,
	}
}

// emitEvent creates an event in the same transaction
func (s *FileService) emitEvent(ctx context.Context, tx *gorm.DB, eventType, aggregateID string, payload interface{}) error {
	// Get request ID from context
	requestID := ctx.Value("requestID")
	if requestID == nil {
		// If no request ID, generate one (for non-API calls)
		requestID = uuid.New().String()
	}

	// Marshal payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	// Create event
	event := &models.Event{
		ID:          uuid.New().String(),
		EventType:   eventType,
		AggregateID: aggregateID,
		Payload:     string(payloadJSON),
		RequestID:   requestID.(string),
		Status:      models.EventStatusPending,
		CreatedAt:   time.Now(),
	}

	// Insert event (VisibleAt will be set by BeforeCreate hook)
	if err := tx.Create(event).Error; err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	return nil
}

// getUserContext extracts user context from request context
func (s *FileService) getUserContext(ctx context.Context) events.UserContext {
	// TODO: Extract from actual auth context
	// For now, return a default user
	return events.UserContext{
		UserID: "system",
		Role:   "admin",
		Groups: []string{},
	}
}

// getRequestID gets or generates a request ID
func (s *FileService) getRequestID(ctx context.Context) string {
	requestID := ctx.Value("requestID")
	if requestID == nil {
		return uuid.New().String()
	}
	if rid, ok := requestID.(string); ok {
		return rid
	}
	return uuid.New().String()
}

// CreateFile creates a new file with lifecycle event tracking
func (s *FileService) CreateFile(ctx context.Context, directoryPath, name, contentType string, size int64, content io.Reader) (*models.File, error) {
	// Read content into buffer for checksum and storage
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, content); err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	contentBytes := buf.Bytes()

	// Calculate checksum
	hash := sha256.Sum256(contentBytes)
	checksum := hex.EncodeToString(hash[:])

	// Get user context
	user := s.getUserContext(ctx)
	requestID := s.getRequestID(ctx)

	// Create operation context
	opCtx := &events.OperationContext{
		OperationID:  uuid.New().String(),
		Category:     events.CategoryFile,
		Operation:    events.OperationCreate,
		ResourcePath: directoryPath + "/" + name,
		UserID:       user.UserID,
		StartedAt:    time.Now(),
		CurrentStage: events.StageAuthorization,
		Status:       "in_progress",
	}

	// ========== AUTHORIZATION STAGE ==========
	if s.eventTrigger != nil {
		authStartTime := time.Now()
		opCtx.AuthorizationStartedAt = &authStartTime

		// Emit authorization.started
		authPayload := s.buildAuthPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionStarted, events.OutcomeSucceeded)
		s.eventTrigger.Emit(ctx, "file.create.authorization.started", authPayload)

		// TODO: Actual authorization check would go here
		// For now, we assume authorization succeeds

		authEndTime := time.Now()
		opCtx.AuthorizationEndedAt = &authEndTime

		// Emit authorization.succeeded
		authSuccessPayload := s.buildAuthPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeSucceeded)
		authSuccessPayload.Decision = "allow"
		authSuccessPayload.Reason = "default allow policy"
		s.eventTrigger.Emit(ctx, "file.create.authorization.succeeded", authSuccessPayload)
	}

	// ========== VALIDATION STAGE ==========
	opCtx.CurrentStage = events.StageValidation

	// Basic validation
	if size > MaxFileSize {
		if s.eventTrigger != nil {
			valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeFailed)
			valPayload.ValidationType = "size"
			valPayload.Violations = []events.Violation{{
				Field:   "size",
				Message: fmt.Sprintf("file size %d exceeds maximum %d bytes", size, MaxFileSize),
				Code:    "max_size_exceeded",
			}}
			s.eventTrigger.Emit(ctx, "file.create.validation.size.failed", valPayload)
		}
		return nil, fmt.Errorf("file size %d exceeds maximum %d bytes", size, MaxFileSize)
	}

	// Validate name
	if name == "" || name == "." || name == ".." {
		if s.eventTrigger != nil {
			valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeFailed)
			valPayload.ValidationType = "name"
			valPayload.Violations = []events.Violation{{
				Field:   "name",
				Message: "invalid file name",
				Code:    "invalid_name",
			}}
			s.eventTrigger.Emit(ctx, "file.create.validation.failed", valPayload)
		}
		return nil, fmt.Errorf("invalid file name")
	}

	// Reject path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		if s.eventTrigger != nil {
			valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeFailed)
			valPayload.ValidationType = "name"
			valPayload.Violations = []events.Violation{{
				Field:   "name",
				Message: "file name cannot contain path separators",
				Code:    "invalid_name",
			}}
			s.eventTrigger.Emit(ctx, "file.create.validation.failed", valPayload)
		}
		return nil, fmt.Errorf("invalid file name")
	}

	// Reject control characters
	for _, r := range name {
		if r < 32 || r == 127 {
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeFailed)
				valPayload.ValidationType = "name"
				valPayload.Violations = []events.Violation{{
					Field:   "name",
					Message: "file name contains control characters",
					Code:    "invalid_name",
				}}
				s.eventTrigger.Emit(ctx, "file.create.validation.failed", valPayload)
			}
			return nil, fmt.Errorf("invalid file name")
		}
	}

	// ========== EXECUTION STAGE ==========
	opCtx.CurrentStage = events.StageExecution

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		execStartTime := time.Now()
		opCtx.ExecutionStartedAt = &execStartTime

		// Find directory
		var dir models.Directory
		if err := tx.Where("path = ? AND deleted_at IS NULL", directoryPath).First(&dir).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("directory not found: %s", directoryPath)
			}
			return err
		}

		// Check if file already exists
		var existing models.File
		err := tx.Where("directory_id = ? AND name = ? AND deleted_at IS NULL", dir.ID, name).First(&existing).Error
		if err == nil {
			return fmt.Errorf("file already exists: %s", name)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// Validate content against .files rules (pattern + schema)
		// Skip validation for special files (they don't validate against themselves)
		isSpecialFile := strings.HasPrefix(name, ".")
		if s.filesLoader != nil && !isSpecialFile {
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecking, events.OutcomeSucceeded)
				valPayload.ValidationType = "schema"
				s.eventTrigger.Emit(ctx, "file.create.validation.schema.checking", valPayload)
			}

			if validationErr := s.filesLoader.ValidateFile(ctx, dir.ID, name, contentBytes); validationErr != nil {
				// Schema validation failed
				if s.eventTrigger != nil {
					valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeFailed)
					valPayload.ValidationType = "schema"
					valPayload.Violations = []events.Violation{{
						Field:   "content",
						Message: validationErr.Error(),
						Code:    "schema_validation_failed",
					}}
					s.eventTrigger.Emit(ctx, "file.create.validation.schema.failed", valPayload)
				}
				return validationErr
			}

			// Schema validation succeeded
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeSucceeded)
				valPayload.ValidationType = "schema"
				s.eventTrigger.Emit(ctx, "file.create.validation.schema.succeeded", valPayload)
			}
		}

		// All validation passed
		if s.eventTrigger != nil {
			valEndTime := time.Now()
			opCtx.ValidationStartedAt = &execStartTime
			opCtx.ValidationEndedAt = &valEndTime

			valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeSucceeded)
			valPayload.ValidationType = "all"
			s.eventTrigger.Emit(ctx, "file.create.validation.succeeded", valPayload)
		}

		// Determine storage type
		var storageType models.StorageType
		var jsonContent *string
		var s3Key *string

		if size < JSONThreshold && s.isJSON(contentBytes) {
			// Store as JSON in MySQL
			storageType = models.StorageTypeJSON
			contentStr := string(contentBytes)
			jsonContent = &contentStr
		} else {
			// Store in S3
			storageType = models.StorageTypeS3
			key := fmt.Sprintf("files/%s/%s", dir.ID, uuid.New().String())

			if err := s.storage.Put(ctx, key, bytes.NewReader(contentBytes)); err != nil {
				return fmt.Errorf("failed to upload to S3: %w", err)
			}
			s3Key = &key
		}

		// Create file record
		file = &models.File{
			ID:             uuid.New().String(),
			DirectoryID:    dir.ID,
			Name:           name,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			Version:        1,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if err := tx.Create(file).Error; err != nil {
			// Rollback S3 upload if DB insert fails
			if s3Key != nil {
				s.storage.Delete(ctx, *s3Key)
			}
			return fmt.Errorf("failed to create file record: %w", err)
		}

		// Create initial version
		version := &models.FileVersion{
			ID:             uuid.New().String(),
			FileID:         file.ID,
			VersionNumber:  1,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			CreatedAt:      time.Now(),
		}

		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create file version: %w", err)
		}

		// Emit file.created event
		if err := s.emitEvent(ctx, tx, "file.created", file.ID, map[string]interface{}{
			"file_id":       file.ID,
			"name":          file.Name,
			"directory_id":  file.DirectoryID,
			"content_type":  file.ContentType,
			"size_bytes":    file.SizeBytes,
			"storage_type":  file.StorageType,
			"checksum":      file.ChecksumSHA256,
			"version":       file.Version,
		}); err != nil {
			return err
		}

		return nil
	})

	// ========== COMPLETION STAGE ==========
	if err != nil {
		// Operation failed
		if s.eventTrigger != nil {
			opCtx.Status = "failed"
			opCtx.ErrorMessage = err.Error()
			completedAt := time.Now()
			opCtx.CompletedAt = &completedAt

			completionPayload := s.buildCompletionPayload(opCtx, user, requestID, file, false, err.Error())
			s.eventTrigger.Emit(ctx, "file.create.completion.failed", completionPayload)
		}
		return nil, err
	}

	// Operation succeeded
	if s.eventTrigger != nil {
		opCtx.Status = "succeeded"
		completedAt := time.Now()
		opCtx.CompletedAt = &completedAt

		completionPayload := s.buildCompletionPayload(opCtx, user, requestID, file, true, "")
		s.eventTrigger.Emit(ctx, "file.create.completion.succeeded", completionPayload)

		// Also emit legacy file.created event for backward compatibility
		s.eventTrigger.Emit(ctx, "file.created", completionPayload)
	}

	return file, nil
}

// GetFile retrieves file metadata and content
func (s *FileService) GetFile(ctx context.Context, filePath string) (*models.File, io.ReadCloser, error) {
	// Parse path
	dirPath, fileName := s.parsePath(filePath)

	// Find file
	var file models.File
	err := s.db.Joins("JOIN directories ON directories.id = files.directory_id").
		Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
		First(&file).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fmt.Errorf("file not found: %s", filePath)
		}
		return nil, nil, err
	}

	// Get content
	var reader io.ReadCloser
	if file.StorageType == models.StorageTypeJSON {
		reader = io.NopCloser(strings.NewReader(*file.JSONContent))
	} else {
		r, err := s.storage.Get(ctx, *file.S3Key)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to retrieve from S3: %w", err)
		}
		reader = r
	}

	return &file, reader, nil
}

// UpdateFile updates an existing file
func (s *FileService) UpdateFile(ctx context.Context, filePath, contentType string, size int64, content io.Reader, expectedVersion int64) (*models.File, error) {
	if size > MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d bytes", size, MaxFileSize)
	}

	// Read content
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, content); err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	contentBytes := buf.Bytes()

	// Calculate checksum
	hash := sha256.Sum256(contentBytes)
	checksum := hex.EncodeToString(hash[:])

	dirPath, fileName := s.parsePath(filePath)

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Find and lock file
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&file).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("file not found: %s", filePath)
			}
			return err
		}

		// Check version for optimistic locking
		if file.Version != expectedVersion {
			return fmt.Errorf("version conflict: expected %d, got %d", expectedVersion, file.Version)
		}

		// Determine storage type
		var storageType models.StorageType
		var jsonContent *string
		var s3Key *string
		var oldS3Key *string

		if size < JSONThreshold && s.isJSON(contentBytes) {
			storageType = models.StorageTypeJSON
			contentStr := string(contentBytes)
			jsonContent = &contentStr

			// If old version was S3, mark for cleanup
			if file.StorageType == models.StorageTypeS3 {
				oldS3Key = file.S3Key
			}
		} else {
			storageType = models.StorageTypeS3
			key := fmt.Sprintf("files/%s/%s", file.DirectoryID, uuid.New().String())

			if err := s.storage.Put(ctx, key, bytes.NewReader(contentBytes)); err != nil {
				return fmt.Errorf("failed to upload to S3: %w", err)
			}
			s3Key = &key

			// Mark old S3 key for cleanup
			if file.StorageType == models.StorageTypeS3 {
				oldS3Key = file.S3Key
			}
		}

		// Update file
		file.ContentType = contentType
		file.SizeBytes = size
		file.StorageType = storageType
		file.JSONContent = jsonContent
		file.S3Key = s3Key
		file.ChecksumSHA256 = checksum
		file.Version++
		file.UpdatedAt = time.Now()

		if err := tx.Save(file).Error; err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}

		// Create new version
		version := &models.FileVersion{
			ID:             uuid.New().String(),
			FileID:         file.ID,
			VersionNumber:  file.Version,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			CreatedAt:      time.Now(),
		}

		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create file version: %w", err)
		}

		// Cleanup old versions (keep last MaxVersions)
		if err := s.cleanupOldVersions(tx, file.ID); err != nil {
			return err
		}

		// Schedule old S3 key for deletion
		if oldS3Key != nil {
			// In a real system, this would be done asynchronously
			go s.storage.Delete(ctx, *oldS3Key)
		}

		// Emit file.updated event
		if err := s.emitEvent(ctx, tx, "file.updated", file.ID, map[string]interface{}{
			"file_id":           file.ID,
			"name":              file.Name,
			"directory_id":      file.DirectoryID,
			"content_type":      file.ContentType,
			"size_bytes":        file.SizeBytes,
			"storage_type":      file.StorageType,
			"checksum":          file.ChecksumSHA256,
			"version":           file.Version,
			"previous_version":  expectedVersion,
		}); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return file, nil
}

// DeleteFile deletes a file
func (s *FileService) DeleteFile(ctx context.Context, filePath string) error {
	dirPath, fileName := s.parsePath(filePath)

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Find and lock file
		var file models.File
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&file).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("file not found: %s", filePath)
			}
			return err
		}

		// Soft delete
		if err := tx.Delete(&file).Error; err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}

		// Emit file.deleted event
		if err := s.emitEvent(ctx, tx, "file.deleted", file.ID, map[string]interface{}{
			"file_id":      file.ID,
			"name":         file.Name,
			"directory_id": file.DirectoryID,
			"size_bytes":   file.SizeBytes,
		}); err != nil {
			return err
		}

		// Schedule S3 cleanup (async in real system)
		if file.StorageType == models.StorageTypeS3 && file.S3Key != nil {
			go s.storage.Delete(ctx, *file.S3Key)
		}

		return nil
	})
}

// MoveFile moves a file to a different directory or renames it
func (s *FileService) MoveFile(ctx context.Context, sourcePath, destPath string) (*models.File, error) {
	srcDir, srcName := s.parsePath(sourcePath)
	dstDir, dstName := s.parsePath(destPath)

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Find source file
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", srcDir, srcName).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&file).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("source file not found: %s", sourcePath)
			}
			return err
		}

		// Find destination directory
		var destDir models.Directory
		if err := tx.Where("path = ? AND deleted_at IS NULL", dstDir).First(&destDir).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("destination directory not found: %s", dstDir)
			}
			return err
		}

		// Check if destination file already exists
		var existing models.File
		err = tx.Where("directory_id = ? AND name = ? AND deleted_at IS NULL", destDir.ID, dstName).First(&existing).Error
		if err == nil {
			return fmt.Errorf("destination file already exists: %s", destPath)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// Update file
		oldDirectoryID := file.DirectoryID
		oldName := file.Name

		file.DirectoryID = destDir.ID
		file.Name = dstName
		file.UpdatedAt = time.Now()

		if err := tx.Save(file).Error; err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}

		// Emit file.moved event
		if err := s.emitEvent(ctx, tx, "file.moved", file.ID, map[string]interface{}{
			"file_id":            file.ID,
			"old_name":           oldName,
			"new_name":           dstName,
			"old_directory_id":   oldDirectoryID,
			"new_directory_id":   destDir.ID,
			"source_path":        sourcePath,
			"destination_path":   destPath,
		}); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return file, nil
}

// cleanupOldVersions keeps only the last MaxVersions versions
func (s *FileService) cleanupOldVersions(tx *gorm.DB, fileID string) error {
	var versions []models.FileVersion
	if err := tx.Where("file_id = ?", fileID).Order("version_number DESC").Find(&versions).Error; err != nil {
		return err
	}

	if len(versions) <= MaxVersions {
		return nil
	}

	// Delete old versions
	for i := MaxVersions; i < len(versions); i++ {
		if err := tx.Delete(&versions[i]).Error; err != nil {
			return err
		}

		// Schedule S3 cleanup for old versions
		if versions[i].StorageType == models.StorageTypeS3 && versions[i].S3Key != nil {
			// Async cleanup
			go s.storage.Delete(context.Background(), *versions[i].S3Key)
		}
	}

	return nil
}

// isJSON checks if content is valid JSON
func (s *FileService) isJSON(content []byte) bool {
	var js json.RawMessage
	return json.Unmarshal(content, &js) == nil
}

// parsePath splits a file path into directory and filename
func (s *FileService) parsePath(filePath string) (string, string) {
	lastSlash := strings.LastIndex(filePath, "/")
	if lastSlash == -1 {
		return "/", filePath
	}
	if lastSlash == 0 {
		return "/", filePath[1:]
	}
	return filePath[:lastSlash], filePath[lastSlash+1:]
}

// buildAuthPayload builds an authorization event payload
func (s *FileService) buildAuthPayload(
	opCtx *events.OperationContext,
	user events.UserContext,
	requestID string,
	name string,
	contentType string,
	size int64,
	checksum string,
	action events.Action,
	outcome events.Outcome,
) *events.AuthorizationEventPayload {
	now := time.Now()
	return &events.AuthorizationEventPayload{
		Event: events.LifecycleEvent{
			ID:          uuid.New().String(),
			Category:    events.CategoryFile,
			Operation:   events.OperationCreate,
			Stage:       events.StageAuthorization,
			Action:      action,
			Outcome:     outcome,
			Timestamp:   now,
			OperationID: opCtx.OperationID,
		},
		Resource: events.FileResource{
			Type:        events.ResourceTypeFile,
			ID:          "",
			Name:        name,
			Path:        opCtx.ResourcePath,
			SizeBytes:   size,
			ContentType: contentType,
			ChecksumSHA256: checksum,
		},
		User: events.UserContext{
			UserID: user.UserID,
			Role:   user.Role,
			Groups: user.Groups,
		},
		Metadata: events.EventMetadata{
			RequestID: requestID,
		},
	}
}

// buildValidationPayload builds a validation event payload
func (s *FileService) buildValidationPayload(
	opCtx *events.OperationContext,
	user events.UserContext,
	requestID string,
	name string,
	contentType string,
	size int64,
	checksum string,
	action events.Action,
	outcome events.Outcome,
) *events.ValidationEventPayload {
	now := time.Now()
	return &events.ValidationEventPayload{
		Event: events.LifecycleEvent{
			ID:          uuid.New().String(),
			Category:    events.CategoryFile,
			Operation:   events.OperationCreate,
			Stage:       events.StageValidation,
			Action:      action,
			Outcome:     outcome,
			Timestamp:   now,
			OperationID: opCtx.OperationID,
		},
		Resource: events.FileResource{
			Type:        events.ResourceTypeFile,
			ID:          "",
			Name:        name,
			Path:        opCtx.ResourcePath,
			SizeBytes:   size,
			ContentType: contentType,
			ChecksumSHA256: checksum,
		},
		User: events.UserContext{
			UserID: user.UserID,
			Role:   user.Role,
			Groups: user.Groups,
		},
		Metadata: events.EventMetadata{
			RequestID: requestID,
		},
	}
}

// buildCompletionPayload builds a completion event payload
func (s *FileService) buildCompletionPayload(
	opCtx *events.OperationContext,
	user events.UserContext,
	requestID string,
	file *models.File,
	success bool,
	errorMessage string,
) *events.CompletionEventPayload {
	now := time.Now()

	var resource events.FileResource
	if file != nil {
		resource = events.FileResource{
			Type:           events.ResourceTypeFile,
			ID:             file.ID,
			Name:           file.Name,
			Path:           opCtx.ResourcePath,
			SizeBytes:      file.SizeBytes,
			ContentType:    file.ContentType,
			Version:        file.Version,
			ChecksumSHA256: file.ChecksumSHA256,
			CreatedAt:      file.CreatedAt,
			UpdatedAt:      file.UpdatedAt,
		}
	}

	totalDuration := int64(0)
	if opCtx.CompletedAt != nil {
		totalDuration = opCtx.CompletedAt.Sub(opCtx.StartedAt).Milliseconds()
	}

	return &events.CompletionEventPayload{
		Event: events.LifecycleEvent{
			ID:          uuid.New().String(),
			Category:    events.CategoryFile,
			Operation:   events.OperationCreate,
			Stage:       events.StageCompletion,
			Timestamp:   now,
			OperationID: opCtx.OperationID,
		},
		Resource:           resource,
		User: events.UserContext{
			UserID: user.UserID,
			Role:   user.Role,
			Groups: user.Groups,
		},
		Metadata: events.EventMetadata{
			RequestID: requestID,
		},
		OperationContext:  opCtx,
		Success:           success,
		TotalDurationMs:   totalDuration,
		ErrorMessage:      errorMessage,
		RollbackPerformed: false,
	}
}
