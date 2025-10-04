package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type CronService struct {
	DB *gorm.DB
}

type RegisterInput struct {
	DirectoryID string
	CronExpr    string
	Payload     string
	Timezone    *string
	RequestID   string
}

type CronJobDTO struct {
	ID          string     `json:"id"`
	DirectoryID string     `json:"directory_id"`
	CronExpr    string     `json:"cron_expr"`
	Payload     string     `json:"payload"`
	Timezone    *string    `json:"timezone"`
	Status      string     `json:"status"`
	LastRunAt   *time.Time `json:"last_run_at"`
	NextRunAt   *time.Time `json:"next_run_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TriggerInput struct {
	CronJobID    string
	ExecutionKey string
	RequestID    string
}

func NewCronService(db *gorm.DB) *CronService {
	return &CronService{DB: db}
}

func (s *CronService) Register(ctx context.Context, in RegisterInput) (CronJobDTO, error) {
	var dto CronJobDTO
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		schedule, err := cron.ParseStandard(in.CronExpr)
		if err != nil {
			return err
		}
		next := schedule.Next(time.Now())
		job := db.CronJob{
			ID:          uuid.NewString(),
			DirectoryID: in.DirectoryID,
			CronExpr:    in.CronExpr,
			Payload:     in.Payload,
			Timezone:    in.Timezone,
			RequestID:   in.RequestID,
			Status:      "scheduled",
			NextRunAt:   &next,
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		dto = mapCronJob(job)
		return s.enqueueEvent(ctx, tx, job, "cron.registered", map[string]any{"cron_job_id": job.ID})
	})
	return dto, err
}

func (s *CronService) List(ctx context.Context, directoryID string) ([]CronJobDTO, error) {
	query := s.DB.WithContext(ctx).Model(&db.CronJob{})
	if directoryID != "" {
		query = query.Where("directory_id = ?", directoryID)
	}
	var jobs []db.CronJob
	if err := query.Order("created_at ASC").Find(&jobs).Error; err != nil {
		return nil, err
	}
	result := make([]CronJobDTO, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, mapCronJob(job))
	}
	return result, nil
}

func (s *CronService) Trigger(ctx context.Context, in TriggerInput) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job db.CronJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", in.CronJobID).First(&job).Error; err != nil {
			return err
		}
		exec := db.CronExecution{
			ID:            uuid.NewString(),
			CronJobID:     job.ID,
			ExecutionKey:  in.ExecutionKey,
			Status:        "pending",
			ResultPayload: []byte("{}"),
		}
		if err := tx.Create(&exec).Error; err != nil {
			return err
		}
		return s.enqueueEvent(ctx, tx, job, "cron.triggered", map[string]any{
			"cron_job_id":  job.ID,
			"execution_id": exec.ID,
		})
	})
}

func (s *CronService) enqueueEvent(ctx context.Context, tx *gorm.DB, job db.CronJob, eventType string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	event := db.Event{
		ID:        uuid.NewString(),
		Type:      eventType,
		SubjectID: job.ID,
		Payload:   body,
		Status:    "pending",
		RequestID: job.RequestID,
	}
	return tx.Create(&event).Error
}

func mapCronJob(job db.CronJob) CronJobDTO {
	return CronJobDTO{
		ID:          job.ID,
		DirectoryID: job.DirectoryID,
		CronExpr:    job.CronExpr,
		Payload:     job.Payload,
		Timezone:    job.Timezone,
		Status:      job.Status,
		LastRunAt:   job.LastRunAt,
		NextRunAt:   job.NextRunAt,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
	}
}
