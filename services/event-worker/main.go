package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/config"
	"github.com/telnet2/mysql-vfs/pkg/models"
	persistencedb "github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	DefaultWorkerCount   = 5
	DefaultPollInterval  = 1 * time.Second
	DefaultBatchSize     = 10
	MaxRetries           = 3
	ProcessingTimeout    = 30 * time.Second
)

type EventWorker struct {
	db           *gorm.DB
	workerID     int
	pollInterval time.Duration
	batchSize    int
}

func main() {
	// Parse command-line flags
	configFile := flag.String("conf", "", "Path to configuration file (optional, uses env vars if not specified)")
	flag.Parse()

	// Load configuration (supports both config file and env vars)
	var cfg *config.Config
	var err error
	if *configFile != "" {
		log.Printf("Loading configuration from file: %s", *configFile)
		cfg, err = config.LoadConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config file: %v", err)
		}
	} else {
		log.Println("Loading configuration from environment variables")
		cfg, err = config.LoadConfigWithEnv()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}
	}

	// Extract worker-specific configuration
	dsn := cfg.DatabaseDSN
	tablePrefix := cfg.TablePrefix
	workerCount := getEnvInt("WORKER_COUNT", DefaultWorkerCount)
	pollInterval := getEnvDuration("POLL_INTERVAL", DefaultPollInterval)
	batchSize := getEnvInt("BATCH_SIZE", DefaultBatchSize)

	// Connect to database
	log.Println("Event Worker starting...")
	log.Printf("Worker count: %d, Poll interval: %v, Batch size: %d", workerCount, pollInterval, batchSize)

	database, err := persistencedb.Connect(persistencedb.Config{
		DSN:         dsn,
		TablePrefix: tablePrefix,
		LogLevel:    logger.Info,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Create worker pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < workerCount; i++ {
		worker := &EventWorker{
			db:           database,
			workerID:     i + 1,
			pollInterval: pollInterval,
			batchSize:    batchSize,
		}
		go worker.Start(ctx)
	}

	log.Printf("Started %d event workers", workerCount)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down event workers...")
	cancel()
	time.Sleep(2 * time.Second) // Grace period for workers to finish
	log.Println("Event workers stopped")
}

// Start begins the worker event loop
func (w *EventWorker) Start(ctx context.Context) {
	log.Printf("Worker %d started", w.workerID)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopping", w.workerID)
			return
		case <-ticker.C:
			w.processEvents(ctx)
		}
	}
}

// processEvents fetches and processes a batch of pending events
func (w *EventWorker) processEvents(ctx context.Context) {
	// Fetch pending events
	var events []models.Event
	err := w.db.Where("status = ? AND visible_at <= ?", models.EventStatusPending, time.Now()).
		Order("created_at ASC").
		Limit(w.batchSize).
		Find(&events).Error

	if err != nil {
		log.Printf("Worker %d: Failed to fetch events: %v", w.workerID, err)
		return
	}

	if len(events) == 0 {
		return
	}

	log.Printf("Worker %d: Processing %d events", w.workerID, len(events))

	for _, event := range events {
		if err := w.processEvent(ctx, &event); err != nil {
			log.Printf("Worker %d: Failed to process event %s: %v", w.workerID, event.ID, err)
		}
	}
}

// processEvent processes a single event with state machine transitions
func (w *EventWorker) processEvent(ctx context.Context, event *models.Event) error {
	// Try to claim the event by updating status to processing
	result := w.db.Model(&models.Event{}).
		Where("id = ? AND status = ?", event.ID, models.EventStatusPending).
		Updates(map[string]interface{}{
			"status":                 models.EventStatusProcessing,
			"processing_started_at":  time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to claim event: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		// Event was already claimed by another worker
		return nil
	}

	log.Printf("Worker %d: Processing event %s (type: %s, aggregate: %s)",
		w.workerID, event.ID, event.EventType, event.AggregateID)

	// Create processing context with timeout
	processCtx, cancel := context.WithTimeout(ctx, ProcessingTimeout)
	defer cancel()

	// Process the event
	err := w.handleEvent(processCtx, event)

	// Update event status based on result
	if err != nil {
		event.RetryCount++
		event.ErrorMessage = ptr(err.Error())

		if event.RetryCount >= MaxRetries {
			// Move to dead letter queue
			event.Status = models.EventStatusDeadLetter
			log.Printf("Worker %d: Event %s moved to dead letter queue after %d retries",
				w.workerID, event.ID, event.RetryCount)
		} else {
			// Retry with exponential backoff
			event.Status = models.EventStatusPending
			backoff := time.Duration(event.RetryCount*event.RetryCount) * 5 * time.Second
			event.VisibleAt = time.Now().Add(backoff)
			log.Printf("Worker %d: Event %s will retry in %v (attempt %d/%d)",
				w.workerID, event.ID, backoff, event.RetryCount, MaxRetries)
		}
	} else {
		// Success
		event.Status = models.EventStatusCompleted
		now := time.Now()
		event.CompletedAt = &now
		log.Printf("Worker %d: Event %s completed successfully", w.workerID, event.ID)
	}

	// Save event state
	if err := w.db.Save(event).Error; err != nil {
		return fmt.Errorf("failed to update event status: %w", err)
	}

	return nil
}

// handleEvent contains the actual event processing logic
func (w *EventWorker) handleEvent(ctx context.Context, event *models.Event) error {
	// Fetch webhooks subscribed to this event type
	var webhookConfigs []models.WebhookConfig
	err := w.db.Where("event_type = ? AND is_active = true AND circuit_state != ? AND deleted_at IS NULL",
		event.EventType, models.CircuitStateOpen).Find(&webhookConfigs).Error
	if err != nil {
		return fmt.Errorf("failed to fetch webhook configs: %w", err)
	}

	if len(webhookConfigs) == 0 {
		// No webhooks subscribed to this event type
		log.Printf("Worker %d: No webhooks for event type %s", w.workerID, event.EventType)
		return nil
	}

	log.Printf("Worker %d: Creating %d webhook jobs for event %s",
		w.workerID, len(webhookConfigs), event.ID)

	// Create webhook jobs for each subscribed webhook
	for _, webhookConfig := range webhookConfigs {
		nextRetry := time.Now()
		job := &models.WebhookJob{
			ID:              fmt.Sprintf("job-%s-%s", event.ID[:8], webhookConfig.ID[:8]),
			EventID:         event.ID,
			WebhookConfigID: webhookConfig.ID,
			IdempotencyKey:  fmt.Sprintf("%s-%s", event.ID, webhookConfig.ID),
			Status:          models.WebhookJobStatusPending,
			AttemptCount:    0,
			NextRetryAt:     &nextRetry,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		if err := w.db.Create(job).Error; err != nil {
			log.Printf("Worker %d: Failed to create webhook job: %v", w.workerID, err)
			continue
		}
	}

	return nil
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
