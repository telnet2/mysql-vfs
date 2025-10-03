package integrity

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/pkg/models"
)

// RepairService handles fixing referential integrity violations
type RepairService struct {
	db        *gorm.DB
	validator *Validator
}

// NewRepairService creates a new repair service
func NewRepairService(db *gorm.DB) *RepairService {
	return &RepairService{
		db:        db,
		validator: NewValidator(db),
	}
}

// RepairResult represents the result of a repair operation
type RepairResult struct {
	ViolationType string
	RecordID      string
	Action        string
	Success       bool
	Error         error
}

// RepairAll attempts to fix all detectable integrity violations
func (r *RepairService) RepairAll(ctx context.Context, dryRun bool) ([]RepairResult, error) {
	log.Printf("Starting integrity repair (dry run: %v)...\n", dryRun)

	var results []RepairResult

	// Repair orphaned directories
	dirResults, err := r.RepairOrphanedDirectories(ctx, dryRun)
	if err != nil {
		log.Printf("Error repairing directories: %v\n", err)
	}
	results = append(results, dirResults...)

	// Repair orphaned files
	fileResults, err := r.RepairOrphanedFiles(ctx, dryRun)
	if err != nil {
		log.Printf("Error repairing files: %v\n", err)
	}
	results = append(results, fileResults...)

	// Repair orphaned file versions
	versionResults, err := r.RepairOrphanedFileVersions(ctx, dryRun)
	if err != nil {
		log.Printf("Error repairing file versions: %v\n", err)
	}
	results = append(results, versionResults...)

	// Repair orphaned file relations
	relationResults, err := r.RepairOrphanedFileRelations(ctx, dryRun)
	if err != nil {
		log.Printf("Error repairing file relations: %v\n", err)
	}
	results = append(results, relationResults...)

	// Cleanup stale cron leases
	cronResults, err := r.CleanupStaleCronLeases(ctx, dryRun)
	if err != nil {
		log.Printf("Error cleaning up cron leases: %v\n", err)
	}
	results = append(results, cronResults...)

	// Cleanup stuck events
	eventResults, err := r.CleanupStuckEvents(ctx, dryRun)
	if err != nil {
		log.Printf("Error cleaning up events: %v\n", err)
	}
	results = append(results, eventResults...)

	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	log.Printf("Repair complete. %d/%d operations successful.\n", successCount, len(results))
	return results, nil
}

// RepairOrphanedDirectories soft-deletes directories with missing parents
func (r *RepairService) RepairOrphanedDirectories(ctx context.Context, dryRun bool) ([]RepairResult, error) {
	orphaned, err := r.validator.GetOrphanedDirectories(ctx)
	if err != nil {
		return nil, err
	}

	var results []RepairResult

	for _, dir := range orphaned {
		action := fmt.Sprintf("soft-delete orphaned directory '%s' (parent_id: %v)", dir.Name, dir.ParentID)

		if dryRun {
			results = append(results, RepairResult{
				ViolationType: "orphaned_directory",
				RecordID:      dir.ID,
				Action:        action + " [DRY RUN]",
				Success:       true,
			})
			continue
		}

		// Soft delete the orphaned directory
		err := r.db.WithContext(ctx).Delete(&models.Directory{}, "id = ?", dir.ID).Error

		results = append(results, RepairResult{
			ViolationType: "orphaned_directory",
			RecordID:      dir.ID,
			Action:        action,
			Success:       err == nil,
			Error:         err,
		})

		if err != nil {
			log.Printf("Failed to repair orphaned directory %s: %v\n", dir.ID, err)
		}
	}

	return results, nil
}

// RepairOrphanedFiles soft-deletes files with missing directories
func (r *RepairService) RepairOrphanedFiles(ctx context.Context, dryRun bool) ([]RepairResult, error) {
	orphaned, err := r.validator.GetOrphanedFiles(ctx)
	if err != nil {
		return nil, err
	}

	var results []RepairResult

	for _, file := range orphaned {
		action := fmt.Sprintf("soft-delete orphaned file '%s' (directory_id: %s)", file.Name, file.DirectoryID)

		if dryRun {
			results = append(results, RepairResult{
				ViolationType: "orphaned_file",
				RecordID:      file.ID,
				Action:        action + " [DRY RUN]",
				Success:       true,
			})
			continue
		}

		// Soft delete the orphaned file
		err := r.db.WithContext(ctx).Delete(&models.File{}, "id = ?", file.ID).Error

		results = append(results, RepairResult{
			ViolationType: "orphaned_file",
			RecordID:      file.ID,
			Action:        action,
			Success:       err == nil,
			Error:         err,
		})

		if err != nil {
			log.Printf("Failed to repair orphaned file %s: %v\n", file.ID, err)
		}
	}

	return results, nil
}

// RepairOrphanedFileVersions deletes file versions with missing files
func (r *RepairService) RepairOrphanedFileVersions(ctx context.Context, dryRun bool) ([]RepairResult, error) {
	var orphanedVersions []struct {
		ID            string
		FileID        string
		VersionNumber int64
	}

	err := r.db.WithContext(ctx).
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

	var results []RepairResult

	for _, version := range orphanedVersions {
		action := fmt.Sprintf("delete orphaned file version %d (file_id: %s)", version.VersionNumber, version.FileID)

		if dryRun {
			results = append(results, RepairResult{
				ViolationType: "orphaned_file_version",
				RecordID:      version.ID,
				Action:        action + " [DRY RUN]",
				Success:       true,
			})
			continue
		}

		// Hard delete orphaned version
		err := r.db.WithContext(ctx).Exec("DELETE FROM file_versions WHERE id = ?", version.ID).Error

		results = append(results, RepairResult{
			ViolationType: "orphaned_file_version",
			RecordID:      version.ID,
			Action:        action,
			Success:       err == nil,
			Error:         err,
		})

		if err != nil {
			log.Printf("Failed to repair orphaned file version %s: %v\n", version.ID, err)
		}
	}

	return results, nil
}

// RepairOrphanedFileRelations deletes file relations with missing files
func (r *RepairService) RepairOrphanedFileRelations(ctx context.Context, dryRun bool) ([]RepairResult, error) {
	var orphanedRelations []struct {
		ID                string
		ParentFileID      string
		DerivativeFileID string
	}

	err := r.db.WithContext(ctx).
		Raw(`
			SELECT fr.id, fr.parent_file_id, fr.derivative_file_id
			FROM file_relations fr
			WHERE NOT EXISTS (
				SELECT 1 FROM files f1
				WHERE f1.id = fr.parent_file_id
				  AND f1.deleted_at IS NULL
			  )
			  OR NOT EXISTS (
				SELECT 1 FROM files f2
				WHERE f2.id = fr.derivative_file_id
				  AND f2.deleted_at IS NULL
			  )
		`).Scan(&orphanedRelations).Error

	if err != nil {
		return nil, err
	}

	var results []RepairResult

	for _, relation := range orphanedRelations {
		action := fmt.Sprintf("delete orphaned file relation (parent: %s, derivative: %s)",
			relation.ParentFileID, relation.DerivativeFileID)

		if dryRun {
			results = append(results, RepairResult{
				ViolationType: "orphaned_file_relation",
				RecordID:      relation.ID,
				Action:        action + " [DRY RUN]",
				Success:       true,
			})
			continue
		}

		// Hard delete orphaned relation
		err := r.db.WithContext(ctx).Exec("DELETE FROM file_relations WHERE id = ?", relation.ID).Error

		results = append(results, RepairResult{
			ViolationType: "orphaned_file_relation",
			RecordID:      relation.ID,
			Action:        action,
			Success:       err == nil,
			Error:         err,
		})

		if err != nil {
			log.Printf("Failed to repair orphaned file relation %s: %v\n", relation.ID, err)
		}
	}

	return results, nil
}

// CleanupStaleCronLeases recovers stale cron execution leases
func (r *RepairService) CleanupStaleCronLeases(ctx context.Context, dryRun bool) ([]RepairResult, error) {
	var staleLeases []struct {
		ID            string
		CronJobID     string
		LeaseHolderID *string
	}

	err := r.db.WithContext(ctx).
		Raw(`
			SELECT id, cron_job_id, lease_holder_id
			FROM cron_executions
			WHERE status = 'running'
			  AND heartbeat_at < DATE_SUB(NOW(), INTERVAL 5 MINUTE)
		`).Scan(&staleLeases).Error

	if err != nil {
		return nil, err
	}

	var results []RepairResult

	for _, lease := range staleLeases {
		leaseHolder := "unknown"
		if lease.LeaseHolderID != nil {
			leaseHolder = *lease.LeaseHolderID
		}

		action := fmt.Sprintf("recover stale lease for cron job %s (holder: %s)", lease.CronJobID, leaseHolder)

		if dryRun {
			results = append(results, RepairResult{
				ViolationType: "stale_cron_lease",
				RecordID:      lease.ID,
				Action:        action + " [DRY RUN]",
				Success:       true,
			})
			continue
		}

		// Update execution status to recovered
		err := r.db.WithContext(ctx).Exec(`
			UPDATE cron_executions
			SET status = 'recovered',
				lease_holder_id = NULL,
				error_message = 'Recovered from stale lease by integrity repair'
			WHERE id = ?
		`, lease.ID).Error

		results = append(results, RepairResult{
			ViolationType: "stale_cron_lease",
			RecordID:      lease.ID,
			Action:        action,
			Success:       err == nil,
			Error:         err,
		})

		if err != nil {
			log.Printf("Failed to recover stale cron lease %s: %v\n", lease.ID, err)
		}
	}

	return results, nil
}

// CleanupStuckEvents resets events stuck in processing
func (r *RepairService) CleanupStuckEvents(ctx context.Context, dryRun bool) ([]RepairResult, error) {
	var stuckEvents []struct {
		ID        string
		EventType string
	}

	err := r.db.WithContext(ctx).
		Raw(`
			SELECT id, event_type
			FROM events
			WHERE status = 'processing'
			  AND processing_started_at < DATE_SUB(NOW(), INTERVAL 1 HOUR)
		`).Scan(&stuckEvents).Error

	if err != nil {
		return nil, err
	}

	var results []RepairResult

	for _, event := range stuckEvents {
		action := fmt.Sprintf("reset stuck event %s (type: %s)", event.ID, event.EventType)

		if dryRun {
			results = append(results, RepairResult{
				ViolationType: "stuck_event",
				RecordID:      event.ID,
				Action:        action + " [DRY RUN]",
				Success:       true,
			})
			continue
		}

		// Reset event to pending for retry
		err := r.db.WithContext(ctx).Exec(`
			UPDATE events
			SET status = 'pending',
				visible_at = NOW(),
				error_message = 'Reset from stuck processing by integrity repair'
			WHERE id = ?
		`, event.ID).Error

		results = append(results, RepairResult{
			ViolationType: "stuck_event",
			RecordID:      event.ID,
			Action:        action,
			Success:       err == nil,
			Error:         err,
		})

		if err != nil {
			log.Printf("Failed to reset stuck event %s: %v\n", event.ID, err)
		}
	}

	return results, nil
}
