package citest

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/models"
)

var _ = Describe("Cron Scheduler System", Ordered, func() {
	var (
		testDB *fixtures.TestDatabase
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Cron Scheduler System test environment (this may take a few seconds)...")
		GinkgoWriter.Println("   - Starting MySQL test container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")
		GinkgoWriter.Println("✅ Test environment ready - running tests...")
	})

	AfterAll(func() {
		testDB.Cleanup()
	})

	Context("when creating cron jobs", func() {
		It("should create a valid cron job", func() {
			gormDB := testDB.GetDB()

			job := &models.CronJob{
				ID:             uuid.New().String(),
				Name:           "cron-test-cleanup",
				CronExpression: "0 2 * * *", // Daily at 2 AM
				HandlerType:    "cleanup_idempotency",
				Payload:        "{}",
				Timezone:       "UTC",
				SkipMissedRuns: true,
				IsActive:       true,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			err := gormDB.Create(job).Error
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(err).NotTo(HaveOccurred())
		})

		It("should prevent duplicate job names", func() {
			gormDB := testDB.GetDB()

			job1 := &models.CronJob{
				ID:             uuid.New().String(),
				Name:           "cron-unique-job",
				CronExpression: "* * * * *",
				HandlerType:    "cleanup_events",
				Payload:        "{}",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			gormDB.Create(job1)

			// Try duplicate name
			job2 := &models.CronJob{
				ID:             uuid.New().String(),
				Name:           "cron-unique-job", // Same name
				CronExpression: "* * * * *",
				HandlerType:    "vacuum_s3",
				Payload:        "{}",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			err := gormDB.Create(job2).Error
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(err).To(HaveOccurred()) // Unique constraint violation
		})

		It("should store cron expression correctly", func() {
			gormDB := testDB.GetDB()

			expressions := []string{
				"* * * * *",   // Every minute
				"0 * * * *",   // Every hour
				"0 0 * * *",   // Daily at midnight
				"0 0 * * 0",   // Weekly on Sunday
				"0 0 1 * *",   // Monthly on 1st
				"*/5 * * * *", // Every 5 minutes
			}

			for _, expr := range expressions {
				job := &models.CronJob{
					ID:             uuid.New().String(),
					Name:           "job-" + uuid.New().String()[:8],
					CronExpression: expr,
					HandlerType:    "cleanup_idempotency",
					Payload:        "{}",
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}
				err := gormDB.Create(job).Error
				Expect(err).NotTo(HaveOccurred(), "Failed to create job with expression: %s", expr)
			}

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		})
	})

	Context("when managing cron executions", Ordered, func() {
		var jobID string

		BeforeAll(func() {
			gormDB := testDB.GetDB()

			job := &models.CronJob{
				ID:             uuid.New().String(),
				Name:           "cron-exec-test-job",
				CronExpression: "* * * * *",
				HandlerType:    "cleanup_idempotency",
				Payload:        "{}",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			gormDB.Create(job)
			jobID = job.ID

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		})

		It("should create execution with unique execution key", func() {
			scheduledTime := time.Now().Truncate(time.Minute)
			executionKey := "test-job-" + scheduledTime.Format("20060102-1504")

			gormDB := testDB.GetDB()

			execution := &models.CronExecution{
				ID:           uuid.New().String(),
				CronJobID:    jobID,
				ExecutionKey: executionKey,
				ScheduledAt:  scheduledTime,
				Status:       models.CronExecutionStatusPending,
				CreatedAt:    time.Now(),
			}

			err := gormDB.Create(execution).Error
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(err).NotTo(HaveOccurred())
		})

		It("should prevent duplicate execution keys", func() {
			scheduledTime := time.Now().Truncate(time.Minute)
			executionKey := "duplicate-key-" + scheduledTime.Format("20060102-1504")

			gormDB := testDB.GetDB()

			// First execution
			exec1 := &models.CronExecution{
				ID:           uuid.New().String(),
				CronJobID:    jobID,
				ExecutionKey: executionKey,
				ScheduledAt:  scheduledTime,
				Status:       models.CronExecutionStatusPending,
				CreatedAt:    time.Now(),
			}
			gormDB.Create(exec1)

			// Try duplicate
			exec2 := &models.CronExecution{
				ID:           uuid.New().String(),
				CronJobID:    jobID,
				ExecutionKey: executionKey, // Same key
				ScheduledAt:  scheduledTime,
				Status:       models.CronExecutionStatusPending,
				CreatedAt:    time.Now(),
			}
			err := gormDB.Create(exec2).Error
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(err).To(HaveOccurred()) // Unique constraint violation
		})

		It("should track execution status progression", func() {
			gormDB := testDB.GetDB()

			execution := &models.CronExecution{
				ID:           uuid.New().String(),
				CronJobID:    jobID,
				ExecutionKey: "status-test-" + uuid.New().String()[:8],
				ScheduledAt:  time.Now(),
				Status:       models.CronExecutionStatusPending,
				CreatedAt:    time.Now(),
			}
			gormDB.Create(execution)

			// Update to running
			startTime := time.Now()
			execution.Status = models.CronExecutionStatusRunning
			execution.StartedAt = &startTime
			gormDB.Save(execution)

			// Update to completed
			completedTime := time.Now()
			execution.Status = models.CronExecutionStatusCompleted
			execution.CompletedAt = &completedTime
			gormDB.Save(execution)

			// Verify final state
			var final models.CronExecution
			gormDB.Where("id = ?", execution.ID).First(&final)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(final.Status).To(Equal(models.CronExecutionStatusCompleted))
			Expect(final.StartedAt).NotTo(BeNil())
			Expect(final.CompletedAt).NotTo(BeNil())
			Expect(final.CompletedAt.After(*final.StartedAt)).To(BeTrue())
		})

		It("should store error messages on failure", func() {
			gormDB := testDB.GetDB()

			execution := &models.CronExecution{
				ID:           uuid.New().String(),
				CronJobID:    jobID,
				ExecutionKey: "error-test-" + uuid.New().String()[:8],
				ScheduledAt:  time.Now(),
				Status:       models.CronExecutionStatusRunning,
				CreatedAt:    time.Now(),
			}
			gormDB.Create(execution)

			// Mark as failed with error
			errorMsg := "Database connection lost"
			execution.Status = models.CronExecutionStatusFailed
			execution.ErrorMessage = &errorMsg
			completedAt := time.Now()
			execution.CompletedAt = &completedAt
			gormDB.Save(execution)

			// Verify
			var failed models.CronExecution
			gormDB.Where("id = ?", execution.ID).First(&failed)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(failed.Status).To(Equal(models.CronExecutionStatusFailed))
			Expect(failed.ErrorMessage).NotTo(BeNil())
			Expect(*failed.ErrorMessage).To(Equal("Database connection lost"))
		})
	})

	Context("when managing leases", Ordered, func() {
		var jobID string

		BeforeAll(func() {
			gormDB := testDB.GetDB()

			job := &models.CronJob{
				ID:             uuid.New().String(),
				Name:           "cron-lease-test-job",
				CronExpression: "* * * * *",
				HandlerType:    "cleanup_idempotency",
				Payload:        "{}",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			gormDB.Create(job)
			jobID = job.ID

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		})

		It("should set lease holder and expiration", func() {
			schedulerID := "scheduler-1"
			leaseExpires := time.Now().Add(5 * time.Minute)

			gormDB := testDB.GetDB()

			execution := &models.CronExecution{
				ID:             uuid.New().String(),
				CronJobID:      jobID,
				ExecutionKey:   "lease-test-" + uuid.New().String()[:8],
				ScheduledAt:    time.Now(),
				LeaseHolderID:  &schedulerID,
				LeaseExpiresAt: &leaseExpires,
				Status:         models.CronExecutionStatusRunning,
				CreatedAt:      time.Now(),
			}

			err := gormDB.Create(execution).Error
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(execution.LeaseHolderID).NotTo(BeNil())
			Expect(*execution.LeaseHolderID).To(Equal("scheduler-1"))
		})

		It("should update lease with heartbeat", func() {
			schedulerID := "scheduler-1"
			initialLease := time.Now().Add(5 * time.Minute)

			gormDB := testDB.GetDB()

			execution := &models.CronExecution{
				ID:             uuid.New().String(),
				CronJobID:      jobID,
				ExecutionKey:   "heartbeat-test-" + uuid.New().String()[:8],
				ScheduledAt:    time.Now(),
				LeaseHolderID:  &schedulerID,
				LeaseExpiresAt: &initialLease,
				Status:         models.CronExecutionStatusRunning,
				CreatedAt:      time.Now(),
			}
			gormDB.Create(execution)

			// Simulate heartbeat
			time.Sleep(100 * time.Millisecond)
			newHeartbeat := time.Now()
			newLease := newHeartbeat.Add(5 * time.Minute)

			gormDB.Model(&models.CronExecution{}).
				Where("id = ?", execution.ID).
				Updates(map[string]interface{}{
					"heartbeat_at":     newHeartbeat,
					"lease_expires_at": newLease,
				})

			// Verify
			var updated models.CronExecution
			gormDB.Where("id = ?", execution.ID).First(&updated)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(updated.HeartbeatAt).NotTo(BeNil())
			Expect(updated.LeaseExpiresAt.After(initialLease)).To(BeTrue())
		})

		It("should identify stale leases (reaper scenario)", func() {
			schedulerID := "scheduler-crashed"
			expiredLease := time.Now().Add(-1 * time.Minute) // Already expired

			gormDB := testDB.GetDB()

			staleExecution := &models.CronExecution{
				ID:             uuid.New().String(),
				CronJobID:      jobID,
				ExecutionKey:   "stale-test-" + uuid.New().String()[:8],
				ScheduledAt:    time.Now().Add(-10 * time.Minute),
				LeaseHolderID:  &schedulerID,
				LeaseExpiresAt: &expiredLease,
				Status:         models.CronExecutionStatusRunning, // Still marked as running
				CreatedAt:      time.Now().Add(-10 * time.Minute),
			}
			gormDB.Create(staleExecution)

			// Find stale executions (what reaper would do)
			var staleExecutions []models.CronExecution
			gormDB.Where("status = ? AND lease_expires_at < ?",
				models.CronExecutionStatusRunning, time.Now()).
				Find(&staleExecutions)

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(len(staleExecutions)).To(BeNumerically(">=", 1))
			Expect(staleExecutions[0].LeaseExpiresAt.Before(time.Now())).To(BeTrue())
		})

		It("should mark stale execution as recovered", func() {
			schedulerID := "scheduler-crashed"
			expiredLease := time.Now().Add(-5 * time.Minute)

			gormDB := testDB.GetDB()

			execution := &models.CronExecution{
				ID:             uuid.New().String(),
				CronJobID:      jobID,
				ExecutionKey:   "recover-test-" + uuid.New().String()[:8],
				ScheduledAt:    time.Now().Add(-10 * time.Minute),
				LeaseHolderID:  &schedulerID,
				LeaseExpiresAt: &expiredLease,
				Status:         models.CronExecutionStatusRunning,
				CreatedAt:      time.Now().Add(-10 * time.Minute),
			}
			gormDB.Create(execution)

			// Reaper marks as recovered
			errorMsg := "Lease expired without heartbeat"
			completedAt := time.Now()
			gormDB.Model(&models.CronExecution{}).
				Where("id = ?", execution.ID).
				Updates(map[string]interface{}{
					"status":        models.CronExecutionStatusRecovered,
					"error_message": errorMsg,
					"completed_at":  completedAt,
				})

			// Verify
			var recovered models.CronExecution
			gormDB.Where("id = ?", execution.ID).First(&recovered)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(recovered.Status).To(Equal(models.CronExecutionStatusRecovered))
			Expect(recovered.ErrorMessage).NotTo(BeNil())
			Expect(*recovered.ErrorMessage).To(ContainSubstring("Lease expired"))
		})
	})

	Context("when handling multiple schedulers", Ordered, func() {
		var jobID string

		BeforeAll(func() {
			gormDB := testDB.GetDB()

			job := &models.CronJob{
				ID:             uuid.New().String(),
				Name:           "cron-multi-scheduler-job",
				CronExpression: "* * * * *",
				HandlerType:    "cleanup_idempotency",
				Payload:        "{}",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			gormDB.Create(job)
			jobID = job.ID

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		})

		It("should allow only one scheduler to claim an execution", func() {
			executionKey := "multi-claim-" + uuid.New().String()[:8]
			scheduledTime := time.Now()

			gormDB := testDB.GetDB()

			// Scheduler 1 claims
			scheduler1ID := "scheduler-1"
			lease1 := time.Now().Add(5 * time.Minute)
			exec1 := &models.CronExecution{
				ID:             uuid.New().String(),
				CronJobID:      jobID,
				ExecutionKey:   executionKey,
				ScheduledAt:    scheduledTime,
				LeaseHolderID:  &scheduler1ID,
				LeaseExpiresAt: &lease1,
				Status:         models.CronExecutionStatusPending,
				CreatedAt:      time.Now(),
			}
			err1 := gormDB.Create(exec1).Error

			// Scheduler 2 tries to claim same execution
			scheduler2ID := "scheduler-2"
			lease2 := time.Now().Add(5 * time.Minute)
			exec2 := &models.CronExecution{
				ID:             uuid.New().String(),
				CronJobID:      jobID,
				ExecutionKey:   executionKey, // Same execution key
				ScheduledAt:    scheduledTime,
				LeaseHolderID:  &scheduler2ID,
				LeaseExpiresAt: &lease2,
				Status:         models.CronExecutionStatusPending,
				CreatedAt:      time.Now(),
			}
			err2 := gormDB.Create(exec2).Error

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// First should succeed, second should fail
			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).To(HaveOccurred()) // Unique constraint on execution_key
		})
	})
})
