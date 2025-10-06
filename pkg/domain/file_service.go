package domain

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
	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	JSONThreshold = 16777216 // 16MB
	MaxVersions   = 10       // Keep last 10 versions
)

// FileService handles file operations
type FileService struct {
	db           *gorm.DB
	storage      storage.Storage
	filesLoader  *FilesLoader
	groupLoader  *GroupLoader // For .owner validation
	eventTrigger EventTrigger // For lifecycle events
}

// NewFileService creates a new file service
func NewFileService(db *gorm.DB, storage storage.Storage) *FileService {
	return &FileService{
		db:      db,
		storage: storage,
	}
}

// NewFileServiceWithValidation creates a new file service with .files validation
func NewFileServiceWithValidation(db *gorm.DB, storage storage.Storage, filesLoader *FilesLoader) *FileService {
	return &FileService{
		db:          db,
		storage:     storage,
		filesLoader: filesLoader,
	}
}

// NewFileServiceWithLifecycle creates a new file service with lifecycle events
func NewFileServiceWithLifecycle(db *gorm.DB, storage storage.Storage, filesLoader *FilesLoader, eventTrigger EventTrigger) *FileService {
	return &FileService{
		db:           db,
		storage:      storage,
		filesLoader:  filesLoader,
		eventTrigger: eventTrigger,
	}
}

// NewFileServiceWithGroupValidation creates a new file service with group validation for .owner files
func NewFileServiceWithGroupValidation(db *gorm.DB, storage storage.Storage, filesLoader *FilesLoader, groupLoader *GroupLoader) *FileService {
	return &FileService{
		db:          db,
		storage:     storage,
		filesLoader: filesLoader,
		groupLoader: groupLoader,
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
	// Extract from middleware auth context
	authCtx := s.getAuthContext(ctx)
	return events.UserContext{
		UserID: authCtx.UserID,
		Groups: authCtx.Groups,
	}
}

// getAuthContext extracts AuthContext from request context
func (s *FileService) getAuthContext(ctx context.Context) *AuthContext {
	// Try to extract from context
	if authCtx, ok := ctx.Value("authContext").(*AuthContext); ok && authCtx != nil {
		return authCtx
	}

	// Fallback for non-HTTP calls (tests, internal operations)
	return &AuthContext{
		UserID: "system",
		Groups: []string{"system-admin"},
	}
}

// buildMetadata builds metadata JSON for a new file
func (s *FileService) buildMetadata(authCtx *AuthContext, custom map[string]interface{}) map[string]interface{} {
	metadata := map[string]interface{}{
		"owner":      authCtx.GetOwner(),
		"creator":    authCtx.GetCreator(),
		"system":     false,
		"created_at": time.Now().Format(time.RFC3339),
	}

	// Add delegation info if present
	if authCtx.IsDelegated() {
		metadata["delegated"] = true
		if authCtx.DelegationReason != "" {
			metadata["delegation_reason"] = authCtx.DelegationReason
		}
	}

	// Add custom fields if provided
	if custom != nil {
		metadata["custom"] = custom
	}

	return metadata
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
	// Check if path is system-protected
	if IsSystemProtectedPath(directoryPath) {
		return nil, ErrProtectedSystemDirectory
	}

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

		// Emit authorization.started (synchronous - can veto)
		authPayload := s.buildAuthPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionStarted, events.OutcomeSucceeded)
		if err := s.eventTrigger.EmitSync(ctx, "file.create.authorization.started", authPayload); err != nil {
			return nil, fmt.Errorf("authorization failed: %w", err)
		}

		// TODO: Actual authorization check would go here
		// For now, we assume authorization succeeds

		authEndTime := time.Now()
		opCtx.AuthorizationEndedAt = &authEndTime

		// Emit authorization.succeeded (synchronous - can veto)
		authSuccessPayload := s.buildAuthPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeSucceeded)
		authSuccessPayload.Decision = "allow"
		authSuccessPayload.Reason = "default allow policy"
		if err := s.eventTrigger.EmitSync(ctx, "file.create.authorization.succeeded", authSuccessPayload); err != nil {
			return nil, fmt.Errorf("authorization failed: %w", err)
		}
	}

	// ========== VALIDATION STAGE ==========
	opCtx.CurrentStage = events.StageValidation

	var err error

	// Basic validation
	if size > MaxFileSize {
		err = fmt.Errorf("file size %d exceeds maximum %d bytes", size, MaxFileSize)
		if s.eventTrigger != nil {
			valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeFailed)
			valPayload.ValidationType = "size"
			valPayload.Violations = []events.Violation{{
				Field:   "size",
				Message: err.Error(),
				Code:    "max_size_exceeded",
			}}
			s.eventTrigger.Emit(ctx, "file.create.validation.size.failed", valPayload)

			// Emit completion.failed for early validation failure
			opCtx.Status = "failed"
			opCtx.ErrorMessage = err.Error()
			completedAt := time.Now()
			opCtx.CompletedAt = &completedAt
			completionPayload := s.buildCompletionPayload(opCtx, user, requestID, nil, false, err.Error())
			s.eventTrigger.Emit(ctx, "file.create.completion.failed", completionPayload)
		}
		return nil, err
	}

	// Validate and normalize name
	var normalizedName string
	normalizedName, err = ValidateAndNormalizeName(name)
	if err != nil {
		if s.eventTrigger != nil {
			valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeFailed)
			valPayload.ValidationType = "name"
			valPayload.Violations = []events.Violation{{
				Field:   "name",
				Message: err.Error(),
				Code:    "invalid_name",
			}}
			s.eventTrigger.Emit(ctx, "file.create.validation.failed", valPayload)

			// Emit completion.failed for early validation failure
			opCtx.Status = "failed"
			opCtx.ErrorMessage = err.Error()
			completedAt := time.Now()
			opCtx.CompletedAt = &completedAt
			completionPayload := s.buildCompletionPayload(opCtx, user, requestID, nil, false, err.Error())
			s.eventTrigger.Emit(ctx, "file.create.completion.failed", completionPayload)
		}
		return nil, err
	}
	name = normalizedName

	// ========== EXECUTION STAGE ==========
	opCtx.CurrentStage = events.StageExecution

	var file *models.File
	err = s.db.Transaction(func(tx *gorm.DB) error {
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

		// Validate special files using domain validation
		if IsSpecialFile(name) {
			if err := ValidateSpecialFileContent(name, contentBytes); err != nil {
				return err
			}

			// Additional validation for .owner: check if all groups exist
			if name == ".owner" && s.groupLoader != nil {
				var ownerConfig OwnerConfig
				if err := json.Unmarshal(contentBytes, &ownerConfig); err == nil {
					for _, ownerGroup := range ownerConfig.Owners {
						exists, err := s.groupLoader.GroupExists(ctx, ownerGroup)
						if err != nil {
							return fmt.Errorf("failed to validate owner group: %w", err)
						}
						if !exists {
							return fmt.Errorf("owner group '%s' does not exist in /.group file", ownerGroup)
						}
					}
				}
			}
		}

		// Validate content against .files rules (pattern + schema)
		// Built-in rules (like .group/.user only at root) apply to all files
		// Schema validation is skipped for special files (they don't validate against themselves)
		if s.filesLoader != nil {
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecking, events.OutcomeSucceeded)
				valPayload.ValidationType = "schema"
				// Use EmitSync - handlers can veto at this stage
				if err := s.eventTrigger.EmitSync(ctx, "file.create.validation.schema.checking", valPayload); err != nil {
					return fmt.Errorf("schema validation vetoed: %w", err)
				}
			}

			if validationErr := s.filesLoader.ValidateFile(ctx, dir.ID, name, contentBytes); validationErr != nil {
				// Validation failed (could be built-in rules or schema)
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

			// Validation succeeded
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeSucceeded)
				valPayload.ValidationType = "schema"
				// Use EmitSync - handlers can veto even after successful validation
				if err := s.eventTrigger.EmitSync(ctx, "file.create.validation.schema.succeeded", valPayload); err != nil {
					return fmt.Errorf("schema validation vetoed: %w", err)
				}
			}
		}

		// All validation passed
		if s.eventTrigger != nil {
			valEndTime := time.Now()
			opCtx.ValidationStartedAt = &execStartTime
			opCtx.ValidationEndedAt = &valEndTime

			valPayload := s.buildValidationPayload(opCtx, user, requestID, name, contentType, size, checksum, events.ActionChecked, events.OutcomeSucceeded)
			valPayload.ValidationType = "all"
			// Use EmitSync - final validation veto point
			if err := s.eventTrigger.EmitSync(ctx, "file.create.validation.succeeded", valPayload); err != nil {
				return fmt.Errorf("validation vetoed: %w", err)
			}
		}

		// Determine storage type
		var storageType models.StorageType
		var jsonContent *string
		var textContent *string
		var s3Key *string

		// Text/JSON content: store in MySQL if < 100MB and valid UTF-8/JSON
		// Otherwise: store in S3

		if size < JSONThreshold && s.isJSON(contentBytes) {
			// Store as JSON in MySQL
			storageType = models.StorageTypeJSON
			contentStr := string(contentBytes)
			jsonContent = &contentStr
		} else if size < JSONThreshold && s.isValidUTF8(contentBytes) {
			// Store as text in MySQL (for .rego, plain text files, etc.)
			// Only if content is valid UTF-8 (not binary)
			storageType = models.StorageTypeText
			contentStr := string(contentBytes)
			textContent = &contentStr
		} else {
			// Store in S3 (binary files, large files, etc.)
			storageType = models.StorageTypeS3
			key := fmt.Sprintf("files/%s/%s", dir.ID, uuid.New().String())

			if err := s.storage.Put(ctx, key, bytes.NewReader(contentBytes)); err != nil {
				return fmt.Errorf("failed to upload to S3: %w", err)
			}
			s3Key = &key
		}

		// Build metadata
		authCtx := s.getAuthContext(ctx)
		metadata := s.buildMetadata(authCtx, nil)
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataStr := string(metadataJSON)

		// Create file record with metadata
		file = &models.File{
			ID:             uuid.New().String(),
			DirectoryID:    dir.ID,
			Name:           name,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			TextContent:    textContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			Version:        1,
			Metadata:       &metadataStr,
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

		// Create initial version with metadata
		version := &models.FileVersion{
			ID:             uuid.New().String(),
			FileID:         file.ID,
			VersionNumber:  1,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			TextContent:    textContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			Metadata:       &metadataStr,
			CreatedAt:      time.Now(),
		}

		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create file version: %w", err)
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
	}

	return file, nil
}

// GetFile retrieves file metadata and content
func (s *FileService) GetFile(ctx context.Context, filePath string, version int64) (*models.File, io.ReadCloser, error) {
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

	var fileData *models.File
	var reader io.ReadCloser

	if version == 0 || version == file.Version {
		// Get current version
		fileData = &file
		if file.StorageType == models.StorageTypeJSON {
			reader = io.NopCloser(strings.NewReader(*file.JSONContent))
		} else if file.StorageType == models.StorageTypeText {
			reader = io.NopCloser(strings.NewReader(*file.TextContent))
		} else {
			r, err := s.storage.Get(ctx, *file.S3Key)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to retrieve from S3: %w", err)
			}
			reader = r
		}
	} else {
		// Get specific version
		var fileVersion models.FileVersion
		err := s.db.Where("file_id = ? AND version_number = ?", file.ID, version).First(&fileVersion).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, nil, fmt.Errorf("version %d not found for file: %s", version, filePath)
			}
			return nil, nil, err
		}

		// Create a file struct from the version data
		fileData = &models.File{
			ID:             file.ID,
			Name:           file.Name,
			ContentType:    fileVersion.ContentType,
			SizeBytes:      fileVersion.SizeBytes,
			StorageType:    fileVersion.StorageType,
			ChecksumSHA256: fileVersion.ChecksumSHA256,
			Version:        fileVersion.VersionNumber,
			DirectoryID:    file.DirectoryID,
		}

		// Get content from version
		if fileVersion.StorageType == models.StorageTypeJSON {
			reader = io.NopCloser(strings.NewReader(*fileVersion.JSONContent))
		} else if fileVersion.StorageType == models.StorageTypeText {
			reader = io.NopCloser(strings.NewReader(*fileVersion.TextContent))
		} else {
			r, err := s.storage.Get(ctx, *fileVersion.S3Key)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to retrieve from S3: %w", err)
			}
			reader = r
		}
	}

	return fileData, reader, nil
}

// GetFileMetadata retrieves file metadata without content
func (s *FileService) GetFileMetadata(ctx context.Context, filePath string, version int64) (*models.File, error) {
	// Parse path
	dirPath, fileName := s.parsePath(filePath)

	// Find file
	var file models.File
	err := s.db.Joins("JOIN directories ON directories.id = files.directory_id").
		Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
		First(&file).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return nil, err
	}

	if version == 0 || version == file.Version {
		// Return current version metadata
		return &file, nil
	} else {
		// Get specific version metadata
		var fileVersion models.FileVersion
		err := s.db.Where("file_id = ? AND version_number = ?", file.ID, version).First(&fileVersion).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, fmt.Errorf("version %d not found for file: %s", version, filePath)
			}
			return nil, err
		}

		// Create a file struct from the version data (metadata only)
		fileData := &models.File{
			ID:             file.ID,
			Name:           file.Name,
			ContentType:    fileVersion.ContentType,
			SizeBytes:      fileVersion.SizeBytes,
			StorageType:    fileVersion.StorageType,
			ChecksumSHA256: fileVersion.ChecksumSHA256,
			Version:        fileVersion.VersionNumber,
			DirectoryID:    file.DirectoryID,
			Metadata:       fileVersion.Metadata,
			CreatedAt:      fileVersion.CreatedAt,
			UpdatedAt:      file.CreatedAt, // Use file's updated_at for versions
		}

		return fileData, nil
	}
}

// UpdateFile updates an existing file with lifecycle event tracking
func (s *FileService) UpdateFile(ctx context.Context, filePath, contentType string, size int64, content io.Reader, expectedVersion int64) (*models.File, error) {
	// Parse path first to check protection
	dirPath, fileName := s.parsePath(filePath)

	// Check if path is system-protected
	if IsSystemProtectedPath(dirPath) {
		return nil, ErrProtectedSystemDirectory
	}

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
		Operation:    events.OperationUpdate,
		ResourcePath: filePath,
		UserID:       user.UserID,
		StartedAt:    time.Now(),
		CurrentStage: events.StageAuthorization,
		Status:       "in_progress",
	}

	// ========== AUTHORIZATION STAGE ==========
	if s.eventTrigger != nil {
		authStartTime := time.Now()
		opCtx.AuthorizationStartedAt = &authStartTime

		// Emit authorization.started (synchronous - can veto)
		authPayload := s.buildAuthPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionStarted, events.OutcomeSucceeded)
		if err := s.eventTrigger.EmitSync(ctx, "file.update.authorization.started", authPayload); err != nil {
			return nil, fmt.Errorf("authorization failed: %w", err)
		}

		// TODO: Actual authorization check would go here
		// For now, we assume authorization succeeds

		authEndTime := time.Now()
		opCtx.AuthorizationEndedAt = &authEndTime

		// Emit authorization.succeeded (synchronous - can veto)
		authSuccessPayload := s.buildAuthPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionChecked, events.OutcomeSucceeded)
		authSuccessPayload.Decision = "allow"
		authSuccessPayload.Reason = "default allow policy"
		if err := s.eventTrigger.EmitSync(ctx, "file.update.authorization.succeeded", authSuccessPayload); err != nil {
			return nil, fmt.Errorf("authorization failed: %w", err)
		}
	}

	// ========== VALIDATION STAGE ==========
	opCtx.CurrentStage = events.StageValidation

	// Validate size
	if size > MaxFileSize {
		if s.eventTrigger != nil {
			valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionChecked, events.OutcomeFailed)
			valPayload.ValidationType = "size"
			valPayload.Violations = []events.Violation{{
				Field:   "size",
				Message: fmt.Sprintf("file size %d exceeds maximum %d bytes", size, MaxFileSize),
				Code:    "max_size_exceeded",
			}}
			s.eventTrigger.Emit(ctx, "file.update.validation.size.failed", valPayload)
		}
		return nil, fmt.Errorf("file size %d exceeds maximum %d bytes", size, MaxFileSize)
	}

	// ========== EXECUTION STAGE ==========
	opCtx.CurrentStage = events.StageExecution

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		execStartTime := time.Now()
		opCtx.ExecutionStartedAt = &execStartTime

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

		// Check version for optimistic locking (0 = skip version check)
		if expectedVersion != 0 && file.Version != expectedVersion {
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionChecked, events.OutcomeFailed)
				valPayload.ValidationType = "version"
				valPayload.Violations = []events.Violation{{
					Field:   "version",
					Message: fmt.Sprintf("version conflict: expected %d, got %d", expectedVersion, file.Version),
					Code:    "version_conflict",
				}}
				s.eventTrigger.Emit(ctx, "file.update.validation.version.failed", valPayload)
			}
			return fmt.Errorf("version conflict: expected %d, got %d", expectedVersion, file.Version)
		}

		// Validate special files using domain validation
		if IsSpecialFile(fileName) {
			if err := ValidateSpecialFileContent(fileName, contentBytes); err != nil {
				return err
			}

			// Additional validation for .owner: check if all groups exist
			if fileName == ".owner" && s.groupLoader != nil {
				var ownerConfig OwnerConfig
				if err := json.Unmarshal(contentBytes, &ownerConfig); err == nil {
					for _, ownerGroup := range ownerConfig.Owners {
						exists, err := s.groupLoader.GroupExists(ctx, ownerGroup)
						if err != nil {
							return fmt.Errorf("failed to validate owner group: %w", err)
						}
						if !exists {
							return fmt.Errorf("owner group '%s' does not exist in /.group file", ownerGroup)
						}
					}
				}
			}
		}

		// Validate content against .files rules (pattern + schema)
		// Built-in rules (like .group/.user only at root) apply to all files
		// Schema validation is skipped for special files (they don't validate against themselves)
		if s.filesLoader != nil {
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionChecking, events.OutcomeSucceeded)
				valPayload.ValidationType = "schema"
				// Use EmitSync - handlers can veto
				if err := s.eventTrigger.EmitSync(ctx, "file.update.validation.schema.checking", valPayload); err != nil {
					return fmt.Errorf("schema validation vetoed: %w", err)
				}
			}

			if validationErr := s.filesLoader.ValidateFile(ctx, file.DirectoryID, fileName, contentBytes); validationErr != nil {
				if s.eventTrigger != nil {
					valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionChecked, events.OutcomeFailed)
					valPayload.ValidationType = "schema"
					valPayload.Violations = []events.Violation{{
						Field:   "content",
						Message: validationErr.Error(),
						Code:    "schema_validation_failed",
					}}
					s.eventTrigger.Emit(ctx, "file.update.validation.schema.failed", valPayload)
				}
				return validationErr
			}

			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionChecked, events.OutcomeSucceeded)
				valPayload.ValidationType = "schema"
				// Use EmitSync - handlers can veto
				if err := s.eventTrigger.EmitSync(ctx, "file.update.validation.schema.succeeded", valPayload); err != nil {
					return fmt.Errorf("schema validation vetoed: %w", err)
				}
			}
		}

		// All validation passed
		if s.eventTrigger != nil {
			valEndTime := time.Now()
			opCtx.ValidationStartedAt = &execStartTime
			opCtx.ValidationEndedAt = &valEndTime

			valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, contentType, size, checksum, events.OperationUpdate, events.ActionChecked, events.OutcomeSucceeded)
			valPayload.ValidationType = "all"
			// Use EmitSync - final validation veto point
			if err := s.eventTrigger.EmitSync(ctx, "file.update.validation.succeeded", valPayload); err != nil {
				return fmt.Errorf("validation vetoed: %w", err)
			}
		}

		// Determine storage type
		var storageType models.StorageType
		var jsonContent *string
		var textContent *string
		var s3Key *string
		var oldS3Key *string

		if size < JSONThreshold && s.isJSON(contentBytes) {
			// Store as JSON in MySQL
			storageType = models.StorageTypeJSON
			contentStr := string(contentBytes)
			jsonContent = &contentStr

			// If old version was S3, mark for cleanup
			if file.StorageType == models.StorageTypeS3 {
				oldS3Key = file.S3Key
			}
		} else if size < JSONThreshold && s.isValidUTF8(contentBytes) {
			// Store as text in MySQL
			storageType = models.StorageTypeText
			contentStr := string(contentBytes)
			textContent = &contentStr

			// If old version was S3, mark for cleanup
			if file.StorageType == models.StorageTypeS3 {
				oldS3Key = file.S3Key
			}
		} else {
			// Store in S3 (binary files, large files, etc.)
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

		// Update metadata to track modification
		authCtx := s.getAuthContext(ctx)
		var existingMetadata map[string]interface{}
		if file.Metadata != nil {
			json.Unmarshal([]byte(*file.Metadata), &existingMetadata)
		} else {
			existingMetadata = make(map[string]interface{})
		}

		// Add update tracking
		existingMetadata["updated_at"] = time.Now().Format(time.RFC3339)
		existingMetadata["updated_by"] = authCtx.GetCreator()

		metadataJSON, _ := json.Marshal(existingMetadata)
		metadataStr := string(metadataJSON)

		// Update file
		file.ContentType = contentType
		file.SizeBytes = size
		file.StorageType = storageType
		file.JSONContent = jsonContent
		file.TextContent = textContent
		file.S3Key = s3Key
		file.ChecksumSHA256 = checksum
		file.Version++
		file.Metadata = &metadataStr
		file.UpdatedAt = time.Now()

		if err := tx.Save(file).Error; err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}

		// Create new version with metadata
		version := &models.FileVersion{
			ID:             uuid.New().String(),
			FileID:         file.ID,
			VersionNumber:  file.Version,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			TextContent:    textContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			Metadata:       &metadataStr,
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
			go s.storage.Delete(ctx, *oldS3Key)
		}

		// Emit legacy file.updated event
		if err := s.emitEvent(ctx, tx, "file.updated", file.ID, map[string]interface{}{
			"file_id":          file.ID,
			"name":             file.Name,
			"directory_id":     file.DirectoryID,
			"content_type":     file.ContentType,
			"size_bytes":       file.SizeBytes,
			"storage_type":     file.StorageType,
			"checksum":         file.ChecksumSHA256,
			"version":          file.Version,
			"previous_version": expectedVersion,
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

			completionPayload := s.buildCompletionPayloadForOp(opCtx, user, requestID, file, events.OperationUpdate, false, err.Error())
			s.eventTrigger.Emit(ctx, "file.update.completion.failed", completionPayload)
		}
		return nil, err
	}

	// Operation succeeded
	if s.eventTrigger != nil {
		opCtx.Status = "succeeded"
		completedAt := time.Now()
		opCtx.CompletedAt = &completedAt

		completionPayload := s.buildCompletionPayloadForOp(opCtx, user, requestID, file, events.OperationUpdate, true, "")
		s.eventTrigger.Emit(ctx, "file.update.completion.succeeded", completionPayload)
	}

	return file, nil
}

// UpdateFileMetadata updates only the metadata of a file without changing content
func (s *FileService) UpdateFileMetadata(ctx context.Context, filePath, contentType string) (*models.File, error) {
	// Parse path first to check protection
	dirPath, fileName := s.parsePath(filePath)

	// Check if path is system-protected
	if IsSystemProtectedPath(dirPath) {
		return nil, ErrProtectedSystemDirectory
	}

	// Find and update the file
	var file models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Find the file
		if err := tx.Where("directory_id = (SELECT id FROM directories WHERE path = ?) AND name = ?",
			dirPath, fileName).First(&file).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("file not found: %s", filePath)
			}
			return fmt.Errorf("failed to find file: %w", err)
		}

		// Update only the content type
		file.ContentType = contentType
		file.UpdatedAt = time.Now()

		if err := tx.Save(&file).Error; err != nil {
			return fmt.Errorf("failed to update file metadata: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &file, nil
}

// ListVersions lists all versions of a file (latest first)
func (s *FileService) ListVersions(ctx context.Context, filePath string) ([]*models.FileVersion, error) {
	// Parse path
	directoryPath, name := s.parsePath(filePath)

	// Find file
	var file models.File
	err := s.db.Joins("JOIN directories ON directories.id = files.directory_id").
		Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", directoryPath, name).
		First(&file).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return nil, err
	}

	// List versions
	var versions []*models.FileVersion
	err = s.db.Where("file_id = ?", file.ID).
		Order("version_number DESC").
		Find(&versions).Error
	if err != nil {
		return nil, err
	}

	return versions, nil
}

// DeleteFile deletes a file with lifecycle event tracking
func (s *FileService) DeleteFile(ctx context.Context, filePath string) error {
	// Parse path first to check protection
	dirPath, fileName := s.parsePath(filePath)

	// Check if path is system-protected
	if IsSystemProtectedPath(dirPath) {
		return ErrProtectedSystemDirectory
	}

	// Get user context
	user := s.getUserContext(ctx)
	requestID := s.getRequestID(ctx)

	// Create operation context
	opCtx := &events.OperationContext{
		OperationID:  uuid.New().String(),
		Category:     events.CategoryFile,
		Operation:    events.OperationDelete,
		ResourcePath: filePath,
		UserID:       user.UserID,
		StartedAt:    time.Now(),
		CurrentStage: events.StageAuthorization,
		Status:       "in_progress",
	}

	// ========== AUTHORIZATION STAGE ==========
	if s.eventTrigger != nil {
		authStartTime := time.Now()
		opCtx.AuthorizationStartedAt = &authStartTime

		// Emit authorization.started (synchronous - can veto)
		authPayload := s.buildAuthPayloadForOp(opCtx, user, requestID, fileName, "", 0, "", events.OperationDelete, events.ActionStarted, events.OutcomeSucceeded)
		if err := s.eventTrigger.EmitSync(ctx, "file.delete.authorization.started", authPayload); err != nil {
			return fmt.Errorf("authorization failed: %w", err)
		}

		// TODO: Actual authorization check would go here
		// For now, we assume authorization succeeds

		authEndTime := time.Now()
		opCtx.AuthorizationEndedAt = &authEndTime

		// Emit authorization.succeeded (synchronous - can veto)
		authSuccessPayload := s.buildAuthPayloadForOp(opCtx, user, requestID, fileName, "", 0, "", events.OperationDelete, events.ActionChecked, events.OutcomeSucceeded)
		authSuccessPayload.Decision = "allow"
		authSuccessPayload.Reason = "default allow policy"
		if err := s.eventTrigger.EmitSync(ctx, "file.delete.authorization.succeeded", authSuccessPayload); err != nil {
			return fmt.Errorf("authorization failed: %w", err)
		}
	}

	// ========== VALIDATION STAGE ==========
	opCtx.CurrentStage = events.StageValidation

	// Validation: Check file exists (done in execution stage as part of DB query)
	if s.eventTrigger != nil {
		valStartTime := time.Now()
		opCtx.ValidationStartedAt = &valStartTime
	}

	// ========== EXECUTION STAGE ==========
	opCtx.CurrentStage = events.StageExecution

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		execStartTime := time.Now()
		opCtx.ExecutionStartedAt = &execStartTime

		// Find and lock file
		var f models.File
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&f).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// File not found - validation failure
				if s.eventTrigger != nil {
					valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, "", 0, "", events.OperationDelete, events.ActionChecked, events.OutcomeFailed)
					valPayload.ValidationType = "existence"
					valPayload.Violations = []events.Violation{{
						Field:   "file",
						Message: fmt.Sprintf("file not found: %s", filePath),
						Code:    "not_found",
					}}
					s.eventTrigger.Emit(ctx, "file.delete.validation.existence.failed", valPayload)
				}
				return fmt.Errorf("file not found: %s", filePath)
			}
			return err
		}
		file = &f

		// File exists - validation succeeded
		if s.eventTrigger != nil {
			valEndTime := time.Now()
			opCtx.ValidationEndedAt = &valEndTime

			valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, fileName, file.ContentType, file.SizeBytes, file.ChecksumSHA256, events.OperationDelete, events.ActionChecked, events.OutcomeSucceeded)
			valPayload.ValidationType = "existence"
			// Use EmitSync - handlers can veto
			if err := s.eventTrigger.EmitSync(ctx, "file.delete.validation.existence.succeeded", valPayload); err != nil {
				return fmt.Errorf("validation vetoed: %w", err)
			}
			if err := s.eventTrigger.EmitSync(ctx, "file.delete.validation.succeeded", valPayload); err != nil {
				return fmt.Errorf("validation vetoed: %w", err)
			}
		}

		// Soft delete
		if err := tx.Delete(&file).Error; err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}

		// Emit legacy file.deleted event
		if err := s.emitEvent(ctx, tx, "file.deleted", file.ID, map[string]interface{}{
			"file_id":      file.ID,
			"name":         file.Name,
			"directory_id": file.DirectoryID,
			"size_bytes":   file.SizeBytes,
		}); err != nil {
			return err
		}

		// Schedule S3 cleanup (async)
		if file.StorageType == models.StorageTypeS3 && file.S3Key != nil {
			go s.storage.Delete(ctx, *file.S3Key)
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

			completionPayload := s.buildCompletionPayloadForOp(opCtx, user, requestID, file, events.OperationDelete, false, err.Error())
			s.eventTrigger.Emit(ctx, "file.delete.completion.failed", completionPayload)
		}
		return err
	}

	// Operation succeeded
	if s.eventTrigger != nil {
		opCtx.Status = "succeeded"
		completedAt := time.Now()
		opCtx.CompletedAt = &completedAt

		completionPayload := s.buildCompletionPayloadForOp(opCtx, user, requestID, file, events.OperationDelete, true, "")
		s.eventTrigger.Emit(ctx, "file.delete.completion.succeeded", completionPayload)
	}

	return nil
}

// MoveFile moves a file to a different directory or renames it with lifecycle event tracking
func (s *FileService) MoveFile(ctx context.Context, sourcePath, destPath string) (*models.File, error) {
	// Get user context
	user := s.getUserContext(ctx)
	requestID := s.getRequestID(ctx)

	srcDir, srcName := s.parsePath(sourcePath)
	dstDir, dstName := s.parsePath(destPath)

	// Validate and normalize destination name
	normalizedDstName, nameErr := ValidateAndNormalizeName(dstName)
	if nameErr != nil {
		return nil, nameErr
	}
	dstName = normalizedDstName

	// Create operation context
	opCtx := &events.OperationContext{
		OperationID:  uuid.New().String(),
		Category:     events.CategoryFile,
		Operation:    events.OperationMove,
		ResourcePath: sourcePath + " -> " + destPath,
		UserID:       user.UserID,
		StartedAt:    time.Now(),
		CurrentStage: events.StageAuthorization,
		Status:       "in_progress",
	}

	// ========== AUTHORIZATION STAGE ==========
	if s.eventTrigger != nil {
		authStartTime := time.Now()
		opCtx.AuthorizationStartedAt = &authStartTime

		// Emit authorization.started (synchronous - can veto)
		authPayload := s.buildAuthPayloadForOp(opCtx, user, requestID, srcName, "", 0, "", events.OperationMove, events.ActionStarted, events.OutcomeSucceeded)
		if err := s.eventTrigger.EmitSync(ctx, "file.move.authorization.started", authPayload); err != nil {
			return nil, fmt.Errorf("authorization failed: %w", err)
		}

		// TODO: Actual authorization check would go here
		// For now, we assume authorization succeeds

		authEndTime := time.Now()
		opCtx.AuthorizationEndedAt = &authEndTime

		// Emit authorization.succeeded (synchronous - can veto)
		authSuccessPayload := s.buildAuthPayloadForOp(opCtx, user, requestID, srcName, "", 0, "", events.OperationMove, events.ActionChecked, events.OutcomeSucceeded)
		authSuccessPayload.Decision = "allow"
		authSuccessPayload.Reason = "default allow policy"
		if err := s.eventTrigger.EmitSync(ctx, "file.move.authorization.succeeded", authSuccessPayload); err != nil {
			return nil, fmt.Errorf("authorization failed: %w", err)
		}
	}

	// ========== VALIDATION STAGE ==========
	opCtx.CurrentStage = events.StageValidation

	if s.eventTrigger != nil {
		valStartTime := time.Now()
		opCtx.ValidationStartedAt = &valStartTime
	}

	// ========== EXECUTION STAGE ==========
	opCtx.CurrentStage = events.StageExecution

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		execStartTime := time.Now()
		opCtx.ExecutionStartedAt = &execStartTime

		// Find source file
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", srcDir, srcName).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&file).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// Source file not found - validation failure
				if s.eventTrigger != nil {
					valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, srcName, "", 0, "", events.OperationMove, events.ActionChecked, events.OutcomeFailed)
					valPayload.ValidationType = "source_existence"
					valPayload.Violations = []events.Violation{{
						Field:   "source",
						Message: fmt.Sprintf("source file not found: %s", sourcePath),
						Code:    "source_not_found",
					}}
					s.eventTrigger.Emit(ctx, "file.move.validation.source.failed", valPayload)
				}
				return fmt.Errorf("source file not found: %s", sourcePath)
			}
			return err
		}

		// Source file exists
		if s.eventTrigger != nil {
			valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, srcName, file.ContentType, file.SizeBytes, file.ChecksumSHA256, events.OperationMove, events.ActionChecked, events.OutcomeSucceeded)
			valPayload.ValidationType = "source_existence"
			// Use EmitSync - handlers can veto
			if err := s.eventTrigger.EmitSync(ctx, "file.move.validation.source.succeeded", valPayload); err != nil {
				return fmt.Errorf("validation vetoed: %w", err)
			}
		}

		// Find destination directory
		var destDirectory models.Directory
		if err := tx.Where("path = ? AND deleted_at IS NULL", dstDir).First(&destDirectory).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Destination directory not found - validation failure
				if s.eventTrigger != nil {
					valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, srcName, file.ContentType, file.SizeBytes, file.ChecksumSHA256, events.OperationMove, events.ActionChecked, events.OutcomeFailed)
					valPayload.ValidationType = "destination_directory"
					valPayload.Violations = []events.Violation{{
						Field:   "destination_directory",
						Message: fmt.Sprintf("destination directory not found: %s", dstDir),
						Code:    "destination_directory_not_found",
					}}
					s.eventTrigger.Emit(ctx, "file.move.validation.destination.failed", valPayload)
				}
				return fmt.Errorf("destination directory not found: %s", dstDir)
			}
			return err
		}

		// Check if destination file already exists
		var existing models.File
		err = tx.Where("directory_id = ? AND name = ? AND deleted_at IS NULL", destDirectory.ID, dstName).First(&existing).Error
		if err == nil {
			// Destination file already exists - validation failure
			if s.eventTrigger != nil {
				valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, srcName, file.ContentType, file.SizeBytes, file.ChecksumSHA256, events.OperationMove, events.ActionChecked, events.OutcomeFailed)
				valPayload.ValidationType = "destination_conflict"
				valPayload.Violations = []events.Violation{{
					Field:   "destination",
					Message: fmt.Sprintf("destination file already exists: %s", destPath),
					Code:    "destination_already_exists",
				}}
				s.eventTrigger.Emit(ctx, "file.move.validation.destination.failed", valPayload)
			}
			return fmt.Errorf("destination file already exists: %s", destPath)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// All validation passed
		if s.eventTrigger != nil {
			valEndTime := time.Now()
			opCtx.ValidationEndedAt = &valEndTime

			valPayload := s.buildValidationPayloadForOp(opCtx, user, requestID, srcName, file.ContentType, file.SizeBytes, file.ChecksumSHA256, events.OperationMove, events.ActionChecked, events.OutcomeSucceeded)
			valPayload.ValidationType = "all"
			// Use EmitSync - final validation veto point
			if err := s.eventTrigger.EmitSync(ctx, "file.move.validation.succeeded", valPayload); err != nil {
				return fmt.Errorf("validation vetoed: %w", err)
			}
		}

		// Update file
		oldDirectoryID := file.DirectoryID
		oldName := file.Name

		file.DirectoryID = destDirectory.ID
		file.Name = dstName
		file.UpdatedAt = time.Now()

		if err := tx.Save(file).Error; err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}

		// Emit legacy file.moved event
		if err := s.emitEvent(ctx, tx, "file.moved", file.ID, map[string]interface{}{
			"file_id":          file.ID,
			"old_name":         oldName,
			"new_name":         dstName,
			"old_directory_id": oldDirectoryID,
			"new_directory_id": destDirectory.ID,
			"source_path":      sourcePath,
			"destination_path": destPath,
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

			completionPayload := s.buildCompletionPayloadForOp(opCtx, user, requestID, file, events.OperationMove, false, err.Error())
			s.eventTrigger.Emit(ctx, "file.move.completion.failed", completionPayload)
		}
		return nil, err
	}

	// Operation succeeded
	if s.eventTrigger != nil {
		opCtx.Status = "succeeded"
		completedAt := time.Now()
		opCtx.CompletedAt = &completedAt

		completionPayload := s.buildCompletionPayloadForOp(opCtx, user, requestID, file, events.OperationMove, true, "")
		s.eventTrigger.Emit(ctx, "file.move.completion.succeeded", completionPayload)

		// Also emit legacy file.moved event for backward compatibility
		s.eventTrigger.Emit(ctx, "file.moved", completionPayload)
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

// isValidUTF8 checks if content is valid UTF-8 text (not binary)
func (s *FileService) isValidUTF8(content []byte) bool {
	return strings.ToValidUTF8(string(content), "") == string(content)
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

// buildAuthPayloadForOp builds an authorization event payload for any operation
func (s *FileService) buildAuthPayloadForOp(
	opCtx *events.OperationContext,
	user events.UserContext,
	requestID string,
	name string,
	contentType string,
	size int64,
	checksum string,
	operation events.Operation,
	action events.Action,
	outcome events.Outcome,
) *events.AuthorizationEventPayload {
	now := time.Now()
	return &events.AuthorizationEventPayload{
		Event: events.LifecycleEvent{
			ID:          uuid.New().String(),
			Category:    events.CategoryFile,
			Operation:   operation,
			Stage:       events.StageAuthorization,
			Action:      action,
			Outcome:     outcome,
			Timestamp:   now,
			OperationID: opCtx.OperationID,
		},
		Resource: events.FileResource{
			Type:           events.ResourceTypeFile,
			ID:             "",
			Name:           name,
			Path:           opCtx.ResourcePath,
			SizeBytes:      size,
			ContentType:    contentType,
			ChecksumSHA256: checksum,
		},
		User: events.UserContext{
			UserID: user.UserID,
			Groups: user.Groups,
		},
		Metadata: events.EventMetadata{
			RequestID: requestID,
		},
	}
}

// buildAuthPayload builds an authorization event payload (for Create operation - backward compat)
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
	return s.buildAuthPayloadForOp(opCtx, user, requestID, name, contentType, size, checksum, events.OperationCreate, action, outcome)
}

// buildValidationPayloadForOp builds a validation event payload for any operation
func (s *FileService) buildValidationPayloadForOp(
	opCtx *events.OperationContext,
	user events.UserContext,
	requestID string,
	name string,
	contentType string,
	size int64,
	checksum string,
	operation events.Operation,
	action events.Action,
	outcome events.Outcome,
) *events.ValidationEventPayload {
	now := time.Now()
	return &events.ValidationEventPayload{
		Event: events.LifecycleEvent{
			ID:          uuid.New().String(),
			Category:    events.CategoryFile,
			Operation:   operation,
			Stage:       events.StageValidation,
			Action:      action,
			Outcome:     outcome,
			Timestamp:   now,
			OperationID: opCtx.OperationID,
		},
		Resource: events.FileResource{
			Type:           events.ResourceTypeFile,
			ID:             "",
			Name:           name,
			Path:           opCtx.ResourcePath,
			SizeBytes:      size,
			ContentType:    contentType,
			ChecksumSHA256: checksum,
		},
		User: events.UserContext{
			UserID: user.UserID,
			Groups: user.Groups,
		},
		Metadata: events.EventMetadata{
			RequestID: requestID,
		},
	}
}

// buildValidationPayload builds a validation event payload (for Create operation - backward compat)
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
	return s.buildValidationPayloadForOp(opCtx, user, requestID, name, contentType, size, checksum, events.OperationCreate, action, outcome)
}

// buildCompletionPayloadForOp builds a completion event payload for any operation
func (s *FileService) buildCompletionPayloadForOp(
	opCtx *events.OperationContext,
	user events.UserContext,
	requestID string,
	file *models.File,
	operation events.Operation,
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
			Operation:   operation,
			Stage:       events.StageCompletion,
			Timestamp:   now,
			OperationID: opCtx.OperationID,
		},
		Resource: resource,
		User: events.UserContext{
			UserID: user.UserID,
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

// buildCompletionPayload builds a completion event payload (for Create operation - backward compat)
func (s *FileService) buildCompletionPayload(
	opCtx *events.OperationContext,
	user events.UserContext,
	requestID string,
	file *models.File,
	success bool,
	errorMessage string,
) *events.CompletionEventPayload {
	return s.buildCompletionPayloadForOp(opCtx, user, requestID, file, events.OperationCreate, success, errorMessage)
}
