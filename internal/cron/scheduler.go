package cron

import (
	"context"
	"encoding/json"
	"log"
	"time"

	robocron "github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/config"
	"github.com/telnet2/mysql-vfs/internal/models"
)

// Handler is invoked for each cron task execution.
type Handler func(ctx context.Context, task models.CronTask) error

// Scheduler polls the database for cron tasks and executes them.
type Scheduler struct {
	db      *gorm.DB
	cfg     config.CronScheduler
	handler Handler
	cron    *robocron.Cron
}

// NewScheduler builds a scheduler with the provided handler.
func NewScheduler(db *gorm.DB, cfg config.CronScheduler, handler Handler) *Scheduler {
	c := robocron.New()
	return &Scheduler{db: db, cfg: cfg, handler: handler, cron: c}
}

// Start loads cron tasks and schedules them.
func (s *Scheduler) Start(ctx context.Context) error {
	var tasks []models.CronTask
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return err
	}

	for _, task := range tasks {
		task := task
		if _, err := s.cron.AddFunc(task.Expression, func() {
			if err := s.execute(context.Background(), task); err != nil {
				log.Printf("cron task failed: id=%d err=%v", task.ID, err)
			}
		}); err != nil {
			return err
		}
	}

	s.cron.Start()
	return nil
}

// Stop terminates the scheduler.
func (s *Scheduler) Stop() context.Context {
	return s.cron.Stop()
}

func (s *Scheduler) execute(ctx context.Context, task models.CronTask) error {
	if err := s.handler(ctx, task); err != nil {
		return err
	}

	now := time.Now()
	task.LastRunAt = &now
	payload, _ := json.Marshal(task)
	return s.db.WithContext(ctx).Model(&models.CronTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"last_run_at": now,
		"payload":     payload,
	}).Error
}
