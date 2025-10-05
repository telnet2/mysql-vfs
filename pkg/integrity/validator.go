package integrity

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/pkg/models"
)

// Validator performs referential integrity checks
type Validator struct {
	db *gorm.DB
}

// NewValidator creates a new integrity validator
func NewValidator(db *gorm.DB) *Validator {
	return &Validator{db: db}
}

// ValidationResult represents the result of an integrity check
type ValidationResult struct {
	TableName    string
	ViolationType string
	RecordID      string
	Description   string
	Count         int64
}

// ValidateAll runs all referential integrity checks
func (v *Validator) ValidateAll(ctx context.Context) ([]ValidationResult, error) {
	log.Println("Starting referential integrity validation...")

	var results []ValidationResult

	// Validate directories
	dirResults, err := v.ValidateDirectories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate directories: %w", err)
	}
	results = append(results, dirResults...)

	// Validate files
	fileResults, err := v.ValidateFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate files: %w", err)
	}
	results = append(results, fileResults...)

	// Validate file versions
	versionResults, err := v.ValidateFileVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate file versions: %w", err)
	}
	results = append(results, versionResults...)

	// Validate file relations
	relationResults, err := v.ValidateFileRelations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate file relations: %w", err)
	}
	results = append(results, relationResults...)

	// Validate events
	eventResults, err := v.ValidateEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate events: %w", err)
	}
	results = append(results, eventResults...)

	// Validate cron executions
	cronResults, err := v.ValidateCronExecutions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate cron executions: %w", err)
	}
	results = append(results, cronResults...)

	log.Printf("Integrity validation complete. Found %d violations.\n", len(results))
	return results, nil
}

// ValidateDirectories checks directory referential integrity
func (v *Validator) ValidateDirectories(ctx context.Context) ([]ValidationResult, error) {
	var results []ValidationResult

	// Check for orphaned directories (parent_id references non-existent directory)
	var orphanedDirs []struct {
		ID       string
		Name     string
		ParentID string
	}

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT d.id, d.name, d.parent_id
			FROM directories d
			WHERE d.parent_id IS NOT NULL
			  AND d.parent_id != 'root'
			  AND d.deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1 FROM directories p
				WHERE p.id = d.parent_id
				  AND p.deleted_at IS NULL
			  )
		`).Scan(&orphanedDirs).Error

	if err != nil {
		return nil, err
	}

	for _, dir := range orphanedDirs {
		results = append(results, ValidationResult{
			TableName:     "directories",
			ViolationType: "orphaned_parent",
			RecordID:      dir.ID,
			Description:   fmt.Sprintf("Directory '%s' references non-existent parent '%s'", dir.Name, dir.ParentID),
		})
	}

	// Check for circular references (directory is its own ancestor)
	// This is complex, so we'll check for direct self-reference first
	var selfRefs []struct {
		ID   string
		Name string
	}

	err = v.db.WithContext(ctx).
		Raw(`
			SELECT id, name
			FROM directories
			WHERE id = parent_id
			  AND deleted_at IS NULL
		`).Scan(&selfRefs).Error

	if err != nil {
		return nil, err
	}

	for _, dir := range selfRefs {
		results = append(results, ValidationResult{
			TableName:     "directories",
			ViolationType: "circular_reference",
			RecordID:      dir.ID,
			Description:   fmt.Sprintf("Directory '%s' references itself as parent", dir.Name),
		})
	}

	// Check for path/parent consistency
	var inconsistentPaths []struct {
		ID       string
		Name     string
		Path     string
		ParentID *string
	}

	err = v.db.WithContext(ctx).
		Raw(`
			SELECT d.id, d.name, d.path, d.parent_id
			FROM directories d
			LEFT JOIN directories p ON d.parent_id = p.id
			WHERE d.deleted_at IS NULL
			  AND d.id != 'root'
			  AND (
				(d.parent_id IS NULL AND d.path != CONCAT('/', d.name))
				OR (d.parent_id IS NOT NULL AND p.path != '/' AND d.path != CONCAT(p.path, '/', d.name))
				OR (d.parent_id IS NOT NULL AND p.path = '/' AND d.path != CONCAT('/', d.name))
			  )
		`).Scan(&inconsistentPaths).Error

	if err != nil {
		return nil, err
	}

	for _, dir := range inconsistentPaths {
		results = append(results, ValidationResult{
			TableName:     "directories",
			ViolationType: "path_inconsistency",
			RecordID:      dir.ID,
			Description:   fmt.Sprintf("Directory '%s' has inconsistent path '%s'", dir.Name, dir.Path),
		})
	}

	return results, nil
}

// ValidateFiles checks file referential integrity
func (v *Validator) ValidateFiles(ctx context.Context) ([]ValidationResult, error) {
	var results []ValidationResult

	// Check for orphaned files (directory_id references non-existent directory)
	var orphanedFiles []struct {
		ID          string
		Name        string
		DirectoryID string
	}

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT f.id, f.name, f.directory_id
			FROM files f
			WHERE f.deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1 FROM directories d
				WHERE d.id = f.directory_id
				  AND d.deleted_at IS NULL
			  )
		`).Scan(&orphanedFiles).Error

	if err != nil {
		return nil, err
	}

	for _, file := range orphanedFiles {
		results = append(results, ValidationResult{
			TableName:     "files",
			ViolationType: "orphaned_directory",
			RecordID:      file.ID,
			Description:   fmt.Sprintf("File '%s' references non-existent directory '%s'", file.Name, file.DirectoryID),
		})
	}

	// Check for storage inconsistencies
	var storageIssues []struct {
		ID          string
		Name        string
		StorageType string
		JSONContent *string
		TextContent *string
		S3Key       *string
	}

	err = v.db.WithContext(ctx).
		Raw(`
			SELECT id, name, storage_type, json_content, text_content, s3_key
			FROM files
			WHERE deleted_at IS NULL
			  AND (
				(storage_type = 'json' AND json_content IS NULL)
				OR (storage_type = 'text' AND text_content IS NULL)
				OR (storage_type = 's3' AND s3_key IS NULL)
				OR (storage_type = 'json' AND (s3_key IS NOT NULL OR text_content IS NOT NULL))
				OR (storage_type = 'text' AND (s3_key IS NOT NULL OR json_content IS NOT NULL))
				OR (storage_type = 's3' AND (json_content IS NOT NULL OR text_content IS NOT NULL))
			  )
		`).Scan(&storageIssues).Error

	if err != nil {
		return nil, err
	}

	for _, file := range storageIssues {
		results = append(results, ValidationResult{
			TableName:     "files",
			ViolationType: "storage_inconsistency",
			RecordID:      file.ID,
			Description:   fmt.Sprintf("File '%s' has inconsistent storage configuration (type=%s)", file.Name, file.StorageType),
		})
	}

	return results, nil
}

// ValidateFileVersions checks file version referential integrity
func (v *Validator) ValidateFileVersions(ctx context.Context) ([]ValidationResult, error) {
	var results []ValidationResult

	// Check for orphaned file versions
	var orphanedVersions []struct {
		ID            string
		FileID        string
		VersionNumber int64
	}

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT fv.id, fv.file_id, fv.version_number
			FROM file_versions fv
			WHERE NOT EXISTS (
				SELECT 1 FROM files f
				WHERE f.id = fv.file_id
			  )
		`).Scan(&orphanedVersions).Error

	if err != nil {
		return nil, err
	}

	for _, version := range orphanedVersions {
		results = append(results, ValidationResult{
			TableName:     "file_versions",
			ViolationType: "orphaned_file",
			RecordID:      version.ID,
			Description:   fmt.Sprintf("File version %d references non-existent file '%s'", version.VersionNumber, version.FileID),
		})
	}

	return results, nil
}

// ValidateFileRelations checks file relation referential integrity
func (v *Validator) ValidateFileRelations(ctx context.Context) ([]ValidationResult, error) {
	var results []ValidationResult

	// Check for orphaned parent references
	var orphanedParents []struct {
		ID           string
		ParentFileID string
	}

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT fr.id, fr.parent_file_id
			FROM file_relations fr
			WHERE NOT EXISTS (
				SELECT 1 FROM files f
				WHERE f.id = fr.parent_file_id
				  AND f.deleted_at IS NULL
			  )
		`).Scan(&orphanedParents).Error

	if err != nil {
		return nil, err
	}

	for _, rel := range orphanedParents {
		results = append(results, ValidationResult{
			TableName:     "file_relations",
			ViolationType: "orphaned_parent_file",
			RecordID:      rel.ID,
			Description:   fmt.Sprintf("File relation references non-existent parent file '%s'", rel.ParentFileID),
		})
	}

	// Check for orphaned derivative references
	var orphanedDerivatives []struct {
		ID                string
		DerivativeFileID string
	}

	err = v.db.WithContext(ctx).
		Raw(`
			SELECT fr.id, fr.derivative_file_id
			FROM file_relations fr
			WHERE NOT EXISTS (
				SELECT 1 FROM files f
				WHERE f.id = fr.derivative_file_id
				  AND f.deleted_at IS NULL
			  )
		`).Scan(&orphanedDerivatives).Error

	if err != nil {
		return nil, err
	}

	for _, rel := range orphanedDerivatives {
		results = append(results, ValidationResult{
			TableName:     "file_relations",
			ViolationType: "orphaned_derivative_file",
			RecordID:      rel.ID,
			Description:   fmt.Sprintf("File relation references non-existent derivative file '%s'", rel.DerivativeFileID),
		})
	}

	// Check for self-references
	var selfRefs []struct {
		ID           string
		ParentFileID string
	}

	err = v.db.WithContext(ctx).
		Raw(`
			SELECT id, parent_file_id
			FROM file_relations
			WHERE parent_file_id = derivative_file_id
		`).Scan(&selfRefs).Error

	if err != nil {
		return nil, err
	}

	for _, rel := range selfRefs {
		results = append(results, ValidationResult{
			TableName:     "file_relations",
			ViolationType: "self_reference",
			RecordID:      rel.ID,
			Description:   fmt.Sprintf("File relation has file '%s' as both parent and derivative", rel.ParentFileID),
		})
	}

	return results, nil
}

// ValidateEvents checks event referential integrity
func (v *Validator) ValidateEvents(ctx context.Context) ([]ValidationResult, error) {
	var results []ValidationResult

	// Check for events with invalid aggregate IDs (optional - depends on requirements)
	// For now, we'll just count orphaned events without valid aggregates
	// This would require checking if aggregate_id exists in the appropriate table

	// Check for stuck processing events (processing for too long)
	var stuckEvents []struct {
		ID               string
		EventType        string
		ProcessingStarted string
	}

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT id, event_type, processing_started_at
			FROM events
			WHERE status = 'processing'
			  AND processing_started_at < DATE_SUB(NOW(), INTERVAL 1 HOUR)
		`).Scan(&stuckEvents).Error

	if err != nil {
		return nil, err
	}

	for _, event := range stuckEvents {
		results = append(results, ValidationResult{
			TableName:     "events",
			ViolationType: "stuck_processing",
			RecordID:      event.ID,
			Description:   fmt.Sprintf("Event '%s' stuck in processing since %s", event.EventType, event.ProcessingStarted),
		})
	}

	return results, nil
}

// ValidateCronExecutions checks cron execution referential integrity
func (v *Validator) ValidateCronExecutions(ctx context.Context) ([]ValidationResult, error) {
	var results []ValidationResult

	// Check for orphaned cron executions
	var orphanedExecs []struct {
		ID        string
		CronJobID string
	}

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT ce.id, ce.cron_job_id
			FROM cron_executions ce
			WHERE NOT EXISTS (
				SELECT 1 FROM cron_jobs cj
				WHERE cj.id = ce.cron_job_id
			  )
		`).Scan(&orphanedExecs).Error

	if err != nil {
		return nil, err
	}

	for _, exec := range orphanedExecs {
		results = append(results, ValidationResult{
			TableName:     "cron_executions",
			ViolationType: "orphaned_cron_job",
			RecordID:      exec.ID,
			Description:   fmt.Sprintf("Cron execution references non-existent cron job '%s'", exec.CronJobID),
		})
	}

	// Check for stale leases (heartbeat timeout)
	var staleLeases []struct {
		ID            string
		CronJobID     string
		LeaseHolderID *string
		HeartbeatAt   *string
	}

	err = v.db.WithContext(ctx).
		Raw(`
			SELECT id, cron_job_id, lease_holder_id, heartbeat_at
			FROM cron_executions
			WHERE status = 'running'
			  AND heartbeat_at < DATE_SUB(NOW(), INTERVAL 5 MINUTE)
		`).Scan(&staleLeases).Error

	if err != nil {
		return nil, err
	}

	for _, lease := range staleLeases {
		results = append(results, ValidationResult{
			TableName:     "cron_executions",
			ViolationType: "stale_lease",
			RecordID:      lease.ID,
			Description:   fmt.Sprintf("Cron execution has stale lease (heartbeat: %v)", lease.HeartbeatAt),
		})
	}

	return results, nil
}

// GetOrphanedDirectories finds directories with missing parents
func (v *Validator) GetOrphanedDirectories(ctx context.Context) ([]*models.Directory, error) {
	var orphaned []*models.Directory

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT d.*
			FROM directories d
			WHERE d.parent_id IS NOT NULL
			  AND d.parent_id != 'root'
			  AND d.deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1 FROM directories p
				WHERE p.id = d.parent_id
				  AND p.deleted_at IS NULL
			  )
		`).Scan(&orphaned).Error

	return orphaned, err
}

// GetOrphanedFiles finds files with missing directories
func (v *Validator) GetOrphanedFiles(ctx context.Context) ([]*models.File, error) {
	var orphaned []*models.File

	err := v.db.WithContext(ctx).
		Raw(`
			SELECT f.*
			FROM files f
			WHERE f.deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1 FROM directories d
				WHERE d.id = f.directory_id
				  AND d.deleted_at IS NULL
			  )
		`).Scan(&orphaned).Error

	return orphaned, err
}
