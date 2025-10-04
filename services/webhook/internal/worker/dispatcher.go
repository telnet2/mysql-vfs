package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
	"github.com/telnet2/mysql-vfs/services/webhook/internal/service"
)

type Dispatcher struct {
	DB       *gorm.DB
	Client   *http.Client
	Interval time.Duration
}

func NewDispatcher(db *gorm.DB) *Dispatcher {
	return &Dispatcher{
		DB:       db,
		Client:   &http.Client{Timeout: 10 * time.Second},
		Interval: 2 * time.Second,
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	jobService := service.NewJobService(d.DB)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, cfg, err := d.fetchJob(ctx)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				time.Sleep(d.Interval)
				continue
			}
			time.Sleep(d.Interval)
			continue
		}

		if err := d.deliver(ctx, jobService, job, cfg); err != nil {
			// delivery function handles state updates
		}
	}
}

func (d *Dispatcher) fetchJob(ctx context.Context) (*db.WebhookJob, db.WebhookConfig, error) {
	var job db.WebhookJob
	var cfg db.WebhookConfig
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_attempt_at <= ?", []string{"pending", "retry"}, time.Now()).
			Order("next_attempt_at ASC").
			First(&job).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", job.ConfigID).First(&cfg).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.WebhookJob{}).
			Where("id = ?", job.ID).
			Updates(map[string]any{
				"status":     "in_progress",
				"updated_at": time.Now(),
			}).Error; err != nil {
			return err
		}
		job.Status = "in_progress"
		return nil
	})
	if err != nil {
		return nil, db.WebhookConfig{}, err
	}
	return &job, cfg, nil
}

func (d *Dispatcher) deliver(ctx context.Context, jobService *service.JobService, job *db.WebhookJob, cfg db.WebhookConfig) error {
	backoff := computeBackoff(job.RetryCount)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TargetURL, bytes.NewReader(job.Payload))
	if err != nil {
		_ = jobService.MarkFailed(ctx, job.ID, true, err.Error(), backoff)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Delivery-ID", job.ID)
	req.Header.Set("X-Webhook-Event-ID", job.EventID)
	req.Header.Set("X-Webhook-Idempotency-Key", job.IdempotencyKey)
	if cfg.Secret != "" {
		req.Header.Set("X-Webhook-Signature", signPayload(job.Payload, cfg.Secret))
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		_ = jobService.MarkFailed(ctx, job.ID, true, err.Error(), backoff)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return jobService.MarkDelivered(ctx, job.ID)
	}

	_ = jobService.MarkFailed(ctx, job.ID, true, resp.Status, backoff)
	return nil
}

func signPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func computeBackoff(retry int) time.Duration {
	if retry < 0 {
		retry = 0
	}
	if retry > 6 {
		retry = 6
	}
	return time.Duration(1<<retry) * time.Second
}
