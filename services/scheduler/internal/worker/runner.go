package worker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type Runner struct {
	DB       *gorm.DB
	Interval time.Duration
}

func NewRunner(db *gorm.DB, interval time.Duration) *Runner {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Runner{DB: db, Interval: interval}
}

func (r *Runner) Start(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processDueJobs(ctx)
		}
	}
}

func (r *Runner) processDueJobs(ctx context.Context) {
	for {
		var job db.CronJob
		err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("status = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", "scheduled", time.Now()).
				Order("next_run_at ASC").
				First(&job).Error; err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return
			}
			return
		}

		r.executeJob(ctx, job)
	}
}

func (r *Runner) executeJob(ctx context.Context, job db.CronJob) {
	_ = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		schedule, err := cron.ParseStandard(job.CronExpr)
		if err != nil {
			return err
		}
		now := time.Now()
		next := schedule.Next(now)

		execution := db.CronExecution{
			ID:           uuid.NewString(),
			CronJobID:    job.ID,
			ExecutionKey: uuid.NewString(),
			Status:       "running",
		}
		if err := tx.Create(&execution).Error; err != nil {
			return err
		}

		if err := tx.Model(&db.CronJob{}).
			Where("id = ?", job.ID).
			Updates(map[string]any{
				"status":      "scheduled",
				"last_run_at": now,
				"next_run_at": next,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&db.CronExecution{}).
			Where("id = ?", execution.ID).
			Updates(map[string]any{
				"status":         "completed",
				"result_payload": []byte("{}"),
				"completed_at":   now,
			}).Error; err != nil {
			return err
		}

		event := db.Event{
			ID:        uuid.NewString(),
			Type:      "cron.execution",
			SubjectID: job.ID,
			Payload:   []byte(`{"cron_job_id":"` + job.ID + `","execution_id":"` + execution.ID + `"}`),
			Status:    "pending",
			RequestID: job.RequestID,
		}
		return tx.Create(&event).Error
	})
}
