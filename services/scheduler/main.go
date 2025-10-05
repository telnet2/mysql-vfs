package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/telnet2/mysql-vfs/pkg/models"
	persistencedb "github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	DefaultPollInterval    = 10 * time.Second
	DefaultLeaseDuration   = 5 * time.Minute
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultReaperInterval  = 1 * time.Minute
)

type Scheduler struct {
	db               *gorm.DB
	schedulerID      string
	pollInterval     time.Duration
	leaseDuration    time.Duration
	heartbeatInterval time.Duration
	cronParser       cron.Parser
}

func main() {
	// Load configuration
	dsn := getEnv("DB_DSN", "root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local")
	schedulerID := getEnv("SCHEDULER_ID", fmt.Sprintf("scheduler-%d", time.Now().Unix()))
	pollInterval := getEnvDuration("POLL_INTERVAL", DefaultPollInterval)
	leaseDuration := getEnvDuration("LEASE_DURATION", DefaultLeaseDuration)
	heartbeatInterval := getEnvDuration("HEARTBEAT_INTERVAL", DefaultHeartbeatInterval)

	// Connect to database
	log.Println("Scheduler starting...")
	log.Printf("Scheduler ID: %s", schedulerID)
	log.Printf("Poll interval: %v, Lease duration: %v, Heartbeat interval: %v",
		pollInterval, leaseDuration, heartbeatInterval)

	database, err := persistencedb.Connect(persistencedb.Config{
		DSN:      dsn,
		LogLevel: logger.Info,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Create scheduler instance
	scheduler := &Scheduler{
		db:               database,
		schedulerID:      schedulerID,
		pollInterval:     pollInterval,
		leaseDuration:    leaseDuration,
		heartbeatInterval: heartbeatInterval,
		cronParser:       cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start scheduler loop
	go scheduler.Start(ctx)

	// Start lease reaper
	go scheduler.StartLeaseReaper(ctx)

	log.Printf("Scheduler %s started", schedulerID)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down scheduler...")
	cancel()
	time.Sleep(2 * time.Second) // Grace period
	log.Println("Scheduler stopped")
}

// Start begins the scheduler event loop
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Scheduler %s stopping", s.schedulerID)
			return
		case <-ticker.C:
			s.processJobs(ctx)
		}
	}
}

// processJobs finds and processes pending cron executions
func (s *Scheduler) processJobs(ctx context.Context) {
	now := time.Now()

	// Find active cron jobs
	var cronJobs []models.CronJob
	if err := s.db.Where("is_active = true AND deleted_at IS NULL").Find(&cronJobs).Error; err != nil {
		log.Printf("Failed to fetch cron jobs: %v", err)
		return
	}

	for _, cronJob := range cronJobs {
		// Parse cron expression
		schedule, err := s.cronParser.Parse(cronJob.CronExpression)
		if err != nil {
			log.Printf("Invalid cron expression for job %s: %v", cronJob.Name, err)
			continue
		}

		// Find next scheduled time
		nextRun := schedule.Next(now.Add(-1 * time.Minute)) // Look back 1 minute

		// Check if we should run this job
		if nextRun.After(now) {
			continue // Not time yet
		}

		// Generate execution key (unique per scheduled time)
		executionKey := fmt.Sprintf("%s-%d", cronJob.ID, nextRun.Unix())

		// Try to claim this execution
		if err := s.claimExecution(ctx, &cronJob, executionKey, nextRun); err != nil {
			// Already claimed by another scheduler or error
			continue
		}
	}
}

// claimExecution attempts to claim and execute a cron job
func (s *Scheduler) claimExecution(ctx context.Context, cronJob *models.CronJob, executionKey string, scheduledAt time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Check if execution already exists
		var existing models.CronExecution
		err := tx.Where("execution_key = ?", executionKey).First(&existing).Error
		if err == nil {
			// Already exists, skip
			return fmt.Errorf("execution already exists")
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// Create pending execution
		leaseExpires := time.Now().Add(s.leaseDuration)
		execution := &models.CronExecution{
			ID:             fmt.Sprintf("exec-%s", executionKey),
			CronJobID:      cronJob.ID,
			ExecutionKey:   executionKey,
			ScheduledAt:    scheduledAt,
			LeaseHolderID:  &s.schedulerID,
			LeaseExpiresAt: &leaseExpires,
			Status:         models.CronExecutionStatusPending,
			CreatedAt:      time.Now(),
		}

		if err := tx.Create(execution).Error; err != nil {
			// Likely duplicate key, another scheduler claimed it
			return err
		}

		// Successfully claimed, execute in background
		go s.executeJob(context.Background(), cronJob, execution)

		return nil
	})
}

// executeJob executes a cron job with heartbeat support
func (s *Scheduler) executeJob(ctx context.Context, cronJob *models.CronJob, execution *models.CronExecution) {
	log.Printf("Executing job %s (execution: %s)", cronJob.Name, execution.ID)

	// Update status to running
	now := time.Now()
	execution.Status = models.CronExecutionStatusRunning
	execution.StartedAt = &now
	s.db.Save(execution)

	// Start heartbeat ticker
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()

	go s.sendHeartbeats(heartbeatCtx, execution)

	// Execute job handler
	err := s.executeHandler(ctx, cronJob, execution)

	// Update execution status
	completedAt := time.Now()
	execution.CompletedAt = &completedAt

	if err != nil {
		execution.Status = models.CronExecutionStatusFailed
		errMsg := err.Error()
		execution.ErrorMessage = &errMsg
		log.Printf("Job %s failed: %v", cronJob.Name, err)
	} else {
		execution.Status = models.CronExecutionStatusCompleted
		log.Printf("Job %s completed successfully", cronJob.Name)
	}

	s.db.Save(execution)
}

// sendHeartbeats periodically updates the heartbeat timestamp
func (s *Scheduler) sendHeartbeats(ctx context.Context, execution *models.CronExecution) {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			leaseExpires := now.Add(s.leaseDuration)

			s.db.Model(&models.CronExecution{}).
				Where("id = ?", execution.ID).
				Updates(map[string]interface{}{
					"heartbeat_at":      now,
					"lease_expires_at":  leaseExpires,
				})

			log.Printf("Heartbeat sent for execution %s", execution.ID)
		}
	}
}

// executeHandler executes the job's handler based on handler_type
func (s *Scheduler) executeHandler(ctx context.Context, cronJob *models.CronJob, execution *models.CronExecution) error {
	switch cronJob.HandlerType {
	case "cleanup_idempotency":
		return s.cleanupIdempotency(ctx)
	case "cleanup_events":
		return s.cleanupEvents(ctx)
	case "vacuum_s3":
		return s.vacuumS3(ctx)
	default:
		return fmt.Errorf("unknown handler type: %s", cronJob.HandlerType)
	}
}

// cleanupIdempotency removes expired idempotency records
func (s *Scheduler) cleanupIdempotency(ctx context.Context) error {
	result := s.db.Where("expires_at < ?", time.Now()).Delete(&models.IdempotencyRecord{})
	if result.Error != nil {
		return result.Error
	}
	log.Printf("Cleaned up %d expired idempotency records", result.RowsAffected)
	return nil
}

// cleanupEvents removes old completed events
func (s *Scheduler) cleanupEvents(ctx context.Context) error {
	// Delete completed events older than 30 days
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	result := s.db.Where("status = ? AND completed_at < ?",
		models.EventStatusCompleted, cutoff).Delete(&models.Event{})
	if result.Error != nil {
		return result.Error
	}
	log.Printf("Cleaned up %d old completed events", result.RowsAffected)
	return nil
}

// vacuumS3 placeholder for S3 orphaned object cleanup
func (s *Scheduler) vacuumS3(ctx context.Context) error {
	// TODO: Implement S3 garbage collection
	// - List all S3 keys
	// - Compare with file.s3_key and file_versions.s3_key
	// - Delete orphaned keys
	log.Println("S3 vacuum not yet implemented (placeholder)")
	return nil
}

// StartLeaseReaper recovers stale leases
func (s *Scheduler) StartLeaseReaper(ctx context.Context) {
	ticker := time.NewTicker(DefaultReaperInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reapStaleLeases(ctx)
		}
	}
}

// reapStaleLeases finds and recovers executions with expired leases
func (s *Scheduler) reapStaleLeases(ctx context.Context) {
	// Find running executions with expired leases (no heartbeat)
	var staleExecutions []models.CronExecution
	err := s.db.Where("status = ? AND lease_expires_at < ?",
		models.CronExecutionStatusRunning, time.Now()).Find(&staleExecutions).Error

	if err != nil {
		log.Printf("Failed to fetch stale executions: %v", err)
		return
	}

	if len(staleExecutions) == 0 {
		return
	}

	log.Printf("Found %d stale executions, marking as recovered", len(staleExecutions))

	for _, execution := range staleExecutions {
		execution.Status = models.CronExecutionStatusRecovered
		errMsg := "Lease expired without heartbeat"
		execution.ErrorMessage = &errMsg
		completedAt := time.Now()
		execution.CompletedAt = &completedAt

		if err := s.db.Save(&execution).Error; err != nil {
			log.Printf("Failed to mark execution %s as recovered: %v", execution.ID, err)
		}
	}
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
