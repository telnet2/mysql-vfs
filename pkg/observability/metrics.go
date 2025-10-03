package observability

import (
	"sync"
	"time"
)

// MetricsCollector collects application metrics
type MetricsCollector struct {
	mu sync.RWMutex

	// Request metrics
	requestCount      map[string]int64  // endpoint -> count
	requestDuration   map[string][]int64 // endpoint -> durations in ms
	requestErrors     map[string]int64  // endpoint -> error count

	// File operation metrics
	fileUploads       int64
	fileDownloads     int64
	fileDeletions     int64
	totalBytesUploaded   int64
	totalBytesDownloaded int64

	// Directory operation metrics
	directoryCreations int64
	directoryDeletions int64
	directoryLists     int64

	// Event metrics
	eventsCreated    int64
	eventsProcessed  int64
	eventsFailed     int64
	eventsDeadLetter int64

	// Webhook metrics
	webhooksSent      int64
	webhooksSucceeded int64
	webhooksFailed    int64
	webhooksCircuitOpen int64

	// Cron metrics
	cronJobsExecuted  int64
	cronJobsSucceeded int64
	cronJobsFailed    int64
	cronLeasesRecovered int64

	// Database metrics
	dbQueries         int64
	dbQueryDuration   []int64
	dbErrors          int64

	// Cache metrics (idempotency)
	idempotencyHits   int64
	idempotencyMisses int64
	idempotencyExpired int64

	// System metrics
	startTime time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestCount:    make(map[string]int64),
		requestDuration: make(map[string][]int64),
		requestErrors:   make(map[string]int64),
		startTime:       time.Now(),
	}
}

// Global metrics instance
var globalMetrics = NewMetricsCollector()

// GetGlobalMetrics returns the global metrics instance
func GetGlobalMetrics() *MetricsCollector {
	return globalMetrics
}

// RecordRequest records a request metric
func (m *MetricsCollector) RecordRequest(endpoint string, durationMS int64, isError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestCount[endpoint]++
	m.requestDuration[endpoint] = append(m.requestDuration[endpoint], durationMS)

	if isError {
		m.requestErrors[endpoint]++
	}
}

// RecordFileUpload records a file upload
func (m *MetricsCollector) RecordFileUpload(bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fileUploads++
	m.totalBytesUploaded += bytes
}

// RecordFileDownload records a file download
func (m *MetricsCollector) RecordFileDownload(bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fileDownloads++
	m.totalBytesDownloaded += bytes
}

// RecordFileDeletion records a file deletion
func (m *MetricsCollector) RecordFileDeletion() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fileDeletions++
}

// RecordDirectoryCreation records a directory creation
func (m *MetricsCollector) RecordDirectoryCreation() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.directoryCreations++
}

// RecordDirectoryDeletion records a directory deletion
func (m *MetricsCollector) RecordDirectoryDeletion() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.directoryDeletions++
}

// RecordDirectoryList records a directory listing
func (m *MetricsCollector) RecordDirectoryList() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.directoryLists++
}

// RecordEventCreated records an event creation
func (m *MetricsCollector) RecordEventCreated() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventsCreated++
}

// RecordEventProcessed records a successful event processing
func (m *MetricsCollector) RecordEventProcessed() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventsProcessed++
}

// RecordEventFailed records a failed event
func (m *MetricsCollector) RecordEventFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventsFailed++
}

// RecordEventDeadLetter records an event moved to dead letter queue
func (m *MetricsCollector) RecordEventDeadLetter() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventsDeadLetter++
}

// RecordWebhookSent records a webhook dispatch
func (m *MetricsCollector) RecordWebhookSent(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.webhooksSent++
	if success {
		m.webhooksSucceeded++
	} else {
		m.webhooksFailed++
	}
}

// RecordWebhookCircuitOpen records a circuit breaker opening
func (m *MetricsCollector) RecordWebhookCircuitOpen() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.webhooksCircuitOpen++
}

// RecordCronJobExecution records a cron job execution
func (m *MetricsCollector) RecordCronJobExecution(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cronJobsExecuted++
	if success {
		m.cronJobsSucceeded++
	} else {
		m.cronJobsFailed++
	}
}

// RecordCronLeaseRecovered records a recovered lease
func (m *MetricsCollector) RecordCronLeaseRecovered() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cronLeasesRecovered++
}

// RecordDBQuery records a database query
func (m *MetricsCollector) RecordDBQuery(durationMS int64, isError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dbQueries++
	m.dbQueryDuration = append(m.dbQueryDuration, durationMS)

	if isError {
		m.dbErrors++
	}
}

// RecordIdempotencyCheck records an idempotency check result
func (m *MetricsCollector) RecordIdempotencyCheck(hit bool, expired bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hit {
		m.idempotencyHits++
	} else {
		m.idempotencyMisses++
	}

	if expired {
		m.idempotencyExpired++
	}
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	Uptime    int64     `json:"uptime_seconds"`

	// Request metrics
	TotalRequests     int64            `json:"total_requests"`
	RequestsByEndpoint map[string]int64 `json:"requests_by_endpoint"`
	ErrorsByEndpoint   map[string]int64 `json:"errors_by_endpoint"`
	AvgDurationByEndpoint map[string]float64 `json:"avg_duration_ms_by_endpoint"`

	// File metrics
	FileUploads          int64 `json:"file_uploads"`
	FileDownloads        int64 `json:"file_downloads"`
	FileDeletions        int64 `json:"file_deletions"`
	TotalBytesUploaded   int64 `json:"total_bytes_uploaded"`
	TotalBytesDownloaded int64 `json:"total_bytes_downloaded"`

	// Directory metrics
	DirectoryCreations int64 `json:"directory_creations"`
	DirectoryDeletions int64 `json:"directory_deletions"`
	DirectoryLists     int64 `json:"directory_lists"`

	// Event metrics
	EventsCreated    int64   `json:"events_created"`
	EventsProcessed  int64   `json:"events_processed"`
	EventsFailed     int64   `json:"events_failed"`
	EventsDeadLetter int64   `json:"events_dead_letter"`
	EventSuccessRate float64 `json:"event_success_rate"`

	// Webhook metrics
	WebhooksSent         int64   `json:"webhooks_sent"`
	WebhooksSucceeded    int64   `json:"webhooks_succeeded"`
	WebhooksFailed       int64   `json:"webhooks_failed"`
	WebhooksCircuitOpen  int64   `json:"webhooks_circuit_open"`
	WebhookSuccessRate   float64 `json:"webhook_success_rate"`

	// Cron metrics
	CronJobsExecuted    int64   `json:"cron_jobs_executed"`
	CronJobsSucceeded   int64   `json:"cron_jobs_succeeded"`
	CronJobsFailed      int64   `json:"cron_jobs_failed"`
	CronLeasesRecovered int64   `json:"cron_leases_recovered"`
	CronSuccessRate     float64 `json:"cron_success_rate"`

	// Database metrics
	DBQueries       int64   `json:"db_queries"`
	DBErrors        int64   `json:"db_errors"`
	AvgDBQueryDuration float64 `json:"avg_db_query_duration_ms"`

	// Cache metrics
	IdempotencyHits    int64   `json:"idempotency_hits"`
	IdempotencyMisses  int64   `json:"idempotency_misses"`
	IdempotencyExpired int64   `json:"idempotency_expired"`
	IdempotencyHitRate float64 `json:"idempotency_hit_rate"`
}

// GetSnapshot returns a snapshot of current metrics
func (m *MetricsCollector) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := MetricsSnapshot{
		Timestamp: time.Now(),
		Uptime:    int64(time.Since(m.startTime).Seconds()),

		RequestsByEndpoint: make(map[string]int64),
		ErrorsByEndpoint:   make(map[string]int64),
		AvgDurationByEndpoint: make(map[string]float64),

		FileUploads:          m.fileUploads,
		FileDownloads:        m.fileDownloads,
		FileDeletions:        m.fileDeletions,
		TotalBytesUploaded:   m.totalBytesUploaded,
		TotalBytesDownloaded: m.totalBytesDownloaded,

		DirectoryCreations: m.directoryCreations,
		DirectoryDeletions: m.directoryDeletions,
		DirectoryLists:     m.directoryLists,

		EventsCreated:    m.eventsCreated,
		EventsProcessed:  m.eventsProcessed,
		EventsFailed:     m.eventsFailed,
		EventsDeadLetter: m.eventsDeadLetter,

		WebhooksSent:        m.webhooksSent,
		WebhooksSucceeded:   m.webhooksSucceeded,
		WebhooksFailed:      m.webhooksFailed,
		WebhooksCircuitOpen: m.webhooksCircuitOpen,

		CronJobsExecuted:    m.cronJobsExecuted,
		CronJobsSucceeded:   m.cronJobsSucceeded,
		CronJobsFailed:      m.cronJobsFailed,
		CronLeasesRecovered: m.cronLeasesRecovered,

		DBQueries: m.dbQueries,
		DBErrors:  m.dbErrors,

		IdempotencyHits:    m.idempotencyHits,
		IdempotencyMisses:  m.idempotencyMisses,
		IdempotencyExpired: m.idempotencyExpired,
	}

	// Calculate request metrics
	var totalRequests int64
	for endpoint, count := range m.requestCount {
		totalRequests += count
		snapshot.RequestsByEndpoint[endpoint] = count
		snapshot.ErrorsByEndpoint[endpoint] = m.requestErrors[endpoint]

		// Calculate average duration
		if durations, ok := m.requestDuration[endpoint]; ok && len(durations) > 0 {
			var sum int64
			for _, d := range durations {
				sum += d
			}
			snapshot.AvgDurationByEndpoint[endpoint] = float64(sum) / float64(len(durations))
		}
	}
	snapshot.TotalRequests = totalRequests

	// Calculate success rates
	if m.eventsProcessed+m.eventsFailed > 0 {
		snapshot.EventSuccessRate = float64(m.eventsProcessed) / float64(m.eventsProcessed+m.eventsFailed)
	}

	if m.webhooksSent > 0 {
		snapshot.WebhookSuccessRate = float64(m.webhooksSucceeded) / float64(m.webhooksSent)
	}

	if m.cronJobsExecuted > 0 {
		snapshot.CronSuccessRate = float64(m.cronJobsSucceeded) / float64(m.cronJobsExecuted)
	}

	// Calculate average DB query duration
	if len(m.dbQueryDuration) > 0 {
		var sum int64
		for _, d := range m.dbQueryDuration {
			sum += d
		}
		snapshot.AvgDBQueryDuration = float64(sum) / float64(len(m.dbQueryDuration))
	}

	// Calculate idempotency hit rate
	total := m.idempotencyHits + m.idempotencyMisses
	if total > 0 {
		snapshot.IdempotencyHitRate = float64(m.idempotencyHits) / float64(total)
	}

	return snapshot
}

// Reset clears all metrics (useful for testing)
func (m *MetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestCount = make(map[string]int64)
	m.requestDuration = make(map[string][]int64)
	m.requestErrors = make(map[string]int64)

	m.fileUploads = 0
	m.fileDownloads = 0
	m.fileDeletions = 0
	m.totalBytesUploaded = 0
	m.totalBytesDownloaded = 0

	m.directoryCreations = 0
	m.directoryDeletions = 0
	m.directoryLists = 0

	m.eventsCreated = 0
	m.eventsProcessed = 0
	m.eventsFailed = 0
	m.eventsDeadLetter = 0

	m.webhooksSent = 0
	m.webhooksSucceeded = 0
	m.webhooksFailed = 0
	m.webhooksCircuitOpen = 0

	m.cronJobsExecuted = 0
	m.cronJobsSucceeded = 0
	m.cronJobsFailed = 0
	m.cronLeasesRecovered = 0

	m.dbQueries = 0
	m.dbQueryDuration = []int64{}
	m.dbErrors = 0

	m.idempotencyHits = 0
	m.idempotencyMisses = 0
	m.idempotencyExpired = 0

	m.startTime = time.Now()
}
