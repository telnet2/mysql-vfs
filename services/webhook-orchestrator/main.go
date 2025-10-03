package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/db"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	DefaultWorkerCount        = 5
	DefaultPollInterval       = 1 * time.Second
	DefaultBatchSize          = 10
	MaxAttempts               = 5
	RequestTimeout            = 10 * time.Second
	CircuitBreakerThreshold   = 5
	CircuitBreakerCooldown    = 60 * time.Second
)

type WebhookOrchestrator struct {
	db           *gorm.DB
	httpClient   *http.Client
	workerID     int
	pollInterval time.Duration
	batchSize    int
}

func main() {
	// Load configuration
	dsn := getEnv("DB_DSN", "root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local")
	workerCount := getEnvInt("WORKER_COUNT", DefaultWorkerCount)
	pollInterval := getEnvDuration("POLL_INTERVAL", DefaultPollInterval)
	batchSize := getEnvInt("BATCH_SIZE", DefaultBatchSize)

	// Connect to database
	log.Println("Webhook Orchestrator starting...")
	log.Printf("Worker count: %d, Poll interval: %v, Batch size: %d", workerCount, pollInterval, batchSize)

	database, err := db.Connect(db.Config{
		DSN:      dsn,
		LogLevel: logger.Info,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: RequestTimeout,
	}

	// Create worker pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < workerCount; i++ {
		orchestrator := &WebhookOrchestrator{
			db:           database,
			httpClient:   httpClient,
			workerID:     i + 1,
			pollInterval: pollInterval,
			batchSize:    batchSize,
		}
		go orchestrator.Start(ctx)
	}

	log.Printf("Started %d webhook orchestrator workers", workerCount)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down webhook orchestrator...")
	cancel()
	time.Sleep(2 * time.Second) // Grace period
	log.Println("Webhook orchestrator stopped")
}

// Start begins the orchestrator event loop
func (o *WebhookOrchestrator) Start(ctx context.Context) {
	log.Printf("Worker %d started", o.workerID)
	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopping", o.workerID)
			return
		case <-ticker.C:
			o.processJobs(ctx)
		}
	}
}

// processJobs fetches and processes pending webhook jobs
func (o *WebhookOrchestrator) processJobs(ctx context.Context) {
	// Fetch pending jobs ready for retry
	var jobs []models.WebhookJob
	err := o.db.Where("status = ? AND next_retry_at <= ?",
		models.WebhookJobStatusPending, time.Now()).
		Order("created_at ASC").
		Limit(o.batchSize).
		Find(&jobs).Error

	if err != nil {
		log.Printf("Worker %d: Failed to fetch webhook jobs: %v", o.workerID, err)
		return
	}

	if len(jobs) == 0 {
		return
	}

	log.Printf("Worker %d: Processing %d webhook jobs", o.workerID, len(jobs))

	for _, job := range jobs {
		if err := o.processJob(ctx, &job); err != nil {
			log.Printf("Worker %d: Failed to process job %s: %v", o.workerID, job.ID, err)
		}
	}
}

// processJob processes a single webhook job
func (o *WebhookOrchestrator) processJob(ctx context.Context, job *models.WebhookJob) error {
	// Fetch webhook config and event
	var webhookConfig models.WebhookConfig
	if err := o.db.Where("id = ?", job.WebhookConfigID).First(&webhookConfig).Error; err != nil {
		return fmt.Errorf("failed to fetch webhook config: %w", err)
	}

	// Check circuit breaker
	if webhookConfig.CircuitState == models.CircuitStateOpen {
		// Check if cooldown period has passed
		if webhookConfig.CircuitOpenedAt != nil {
			if time.Since(*webhookConfig.CircuitOpenedAt) < CircuitBreakerCooldown {
				log.Printf("Worker %d: Circuit breaker open for webhook %s, skipping job %s",
					o.workerID, webhookConfig.ID, job.ID)
				return nil
			}
			// Transition to half-open
			webhookConfig.CircuitState = models.CircuitStateHalfOpen
			o.db.Save(&webhookConfig)
			log.Printf("Worker %d: Circuit breaker transitioning to half-open for webhook %s",
				o.workerID, webhookConfig.ID)
		}
	}

	// Fetch event payload
	var event models.Event
	if err := o.db.Where("id = ?", job.EventID).First(&event).Error; err != nil {
		return fmt.Errorf("failed to fetch event: %w", err)
	}

	log.Printf("Worker %d: Dispatching webhook job %s to %s (attempt %d/%d)",
		o.workerID, job.ID, webhookConfig.TargetURL, job.AttemptCount+1, MaxAttempts)

	// Send webhook
	err := o.sendWebhook(ctx, &webhookConfig, &event, job)

	// Update job status
	job.AttemptCount++
	job.UpdatedAt = time.Now()

	if err != nil {
		// Webhook failed
		job.LastError = ptr(err.Error())

		if job.AttemptCount >= MaxAttempts {
			// Max attempts reached
			job.Status = models.WebhookJobStatusFailed
			log.Printf("Worker %d: Job %s failed after %d attempts: %v",
				o.workerID, job.ID, job.AttemptCount, err)

			// Update circuit breaker
			o.recordFailure(&webhookConfig)
		} else {
			// Schedule retry with exponential backoff
			backoff := time.Duration(job.AttemptCount*job.AttemptCount) * 10 * time.Second
			nextRetry := time.Now().Add(backoff)
			job.NextRetryAt = &nextRetry
			log.Printf("Worker %d: Job %s will retry in %v (attempt %d/%d)",
				o.workerID, job.ID, backoff, job.AttemptCount, MaxAttempts)
		}
	} else {
		// Webhook succeeded
		job.Status = models.WebhookJobStatusAcknowledged
		log.Printf("Worker %d: Job %s completed successfully", o.workerID, job.ID)

		// Reset circuit breaker on success
		o.recordSuccess(&webhookConfig)
	}

	// Save job state
	if err := o.db.Save(job).Error; err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	return nil
}

// sendWebhook sends the HTTP request to the webhook endpoint
func (o *WebhookOrchestrator) sendWebhook(ctx context.Context, webhookConfig *models.WebhookConfig, event *models.Event, job *models.WebhookJob) error {
	// Create webhook payload
	payload := map[string]interface{}{
		"event_id":     event.ID,
		"event_type":   event.EventType,
		"aggregate_id": event.AggregateID,
		"payload":      json.RawMessage(event.Payload),
		"request_id":   event.RequestID,
		"timestamp":    event.CreatedAt.Format(time.RFC3339),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookConfig.TargetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", event.ID)
	req.Header.Set("X-Event-Type", event.EventType)
	req.Header.Set("X-Idempotency-Key", job.IdempotencyKey)

	// Sign payload with HMAC
	signature := o.signPayload(payloadBytes, webhookConfig.Secret)
	req.Header.Set("X-Signature", signature)

	// Send request
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, _ := io.ReadAll(resp.Body)

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// signPayload generates HMAC signature for webhook payload
func (o *WebhookOrchestrator) signPayload(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// recordFailure updates circuit breaker state on failure
func (o *WebhookOrchestrator) recordFailure(webhookConfig *models.WebhookConfig) {
	webhookConfig.ConsecutiveFailures++

	if webhookConfig.ConsecutiveFailures >= CircuitBreakerThreshold {
		// Open circuit
		webhookConfig.CircuitState = models.CircuitStateOpen
		now := time.Now()
		webhookConfig.CircuitOpenedAt = &now
		log.Printf("Circuit breaker opened for webhook %s after %d consecutive failures",
			webhookConfig.ID, webhookConfig.ConsecutiveFailures)
	}

	o.db.Save(webhookConfig)
}

// recordSuccess resets circuit breaker state on success
func (o *WebhookOrchestrator) recordSuccess(webhookConfig *models.WebhookConfig) {
	if webhookConfig.ConsecutiveFailures > 0 || webhookConfig.CircuitState != models.CircuitStateClosed {
		webhookConfig.ConsecutiveFailures = 0
		webhookConfig.CircuitState = models.CircuitStateClosed
		webhookConfig.CircuitOpenedAt = nil
		o.db.Save(webhookConfig)
		log.Printf("Circuit breaker closed for webhook %s", webhookConfig.ID)
	}
}

// Helper functions

func ptr(s string) *string {
	return &s
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
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
