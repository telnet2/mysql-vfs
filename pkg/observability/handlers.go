package observability

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// MetricsHandler handles metrics endpoint
func MetricsHandler(ctx context.Context, c *app.RequestContext) {
	metrics := GetGlobalMetrics()
	snapshot := metrics.GetSnapshot()

	c.JSON(consts.StatusOK, snapshot)
}

// AuditLogsHandler handles audit log queries
func AuditLogsHandler(auditLogger *AuditLogger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// Parse query parameters
		opts := QueryOptions{
			UserID:       c.Query("user_id"),
			ResourceType: c.Query("resource_type"),
			ResourceID:   c.Query("resource_id"),
			Limit:        100,
		}

		if action := c.Query("action"); action != "" {
			opts.Action = AuditAction(action)
		}

		if status := c.Query("status"); status != "" {
			opts.Status = AuditStatus(status)
		}

		if startTime := c.Query("start_time"); startTime != "" {
			if t, err := time.Parse(time.RFC3339, startTime); err == nil {
				opts.StartTime = &t
			}
		}

		if endTime := c.Query("end_time"); endTime != "" {
			if t, err := time.Parse(time.RFC3339, endTime); err == nil {
				opts.EndTime = &t
			}
		}

		// Query audit logs
		logs, err := auditLogger.Query(ctx, opts)
		if err != nil {
			c.JSON(consts.StatusInternalServerError, map[string]string{
				"error": "Failed to query audit logs",
			})
			return
		}

		c.JSON(consts.StatusOK, map[string]interface{}{
			"logs":  logs,
			"count": len(logs),
		})
	}
}

// AuditStatsHandler handles audit statistics endpoint
func AuditStatsHandler(auditLogger *AuditLogger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// Default: last 24 hours
		endTime := time.Now()
		startTime := endTime.Add(-24 * time.Hour)

		if st := c.Query("start_time"); st != "" {
			if t, err := time.Parse(time.RFC3339, st); err == nil {
				startTime = t
			}
		}

		if et := c.Query("end_time"); et != "" {
			if t, err := time.Parse(time.RFC3339, et); err == nil {
				endTime = t
			}
		}

		stats, err := auditLogger.GetStats(ctx, startTime, endTime)
		if err != nil {
			c.JSON(consts.StatusInternalServerError, map[string]string{
				"error": "Failed to get audit statistics",
			})
			return
		}

		c.JSON(consts.StatusOK, stats)
	}
}

// HealthCheckResponse represents detailed health check
type HealthCheckResponse struct {
	Status  string                 `json:"status"`
	Uptime  int64                  `json:"uptime_seconds"`
	Metrics map[string]interface{} `json:"metrics"`
	Checks  map[string]string      `json:"checks"`
}

// DetailedHealthHandler provides detailed health information
func DetailedHealthHandler(checks map[string]func() error) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		metrics := GetGlobalMetrics()
		snapshot := metrics.GetSnapshot()

		checkResults := make(map[string]string)
		overallStatus := "ok"

		// Run health checks
		for name, checkFn := range checks {
			if err := checkFn(); err != nil {
				checkResults[name] = "unhealthy: " + err.Error()
				overallStatus = "degraded"
			} else {
				checkResults[name] = "ok"
			}
		}

		response := HealthCheckResponse{
			Status: overallStatus,
			Uptime: snapshot.Uptime,
			Metrics: map[string]interface{}{
				"total_requests":      snapshot.TotalRequests,
				"file_uploads":        snapshot.FileUploads,
				"file_downloads":      snapshot.FileDownloads,
				"events_processed":    snapshot.EventsProcessed,
				"webhook_success_rate": snapshot.WebhookSuccessRate,
			},
			Checks: checkResults,
		}

		statusCode := consts.StatusOK
		if overallStatus == "degraded" {
			statusCode = consts.StatusServiceUnavailable
		}

		c.JSON(statusCode, response)
	}
}

// PrometheusMetricsHandler exports metrics in Prometheus format
func PrometheusMetricsHandler(ctx context.Context, c *app.RequestContext) {
	metrics := GetGlobalMetrics()
	snapshot := metrics.GetSnapshot()

	// Build Prometheus format output
	var output string

	// Uptime
	output += "# HELP vfs_uptime_seconds System uptime in seconds\n"
	output += "# TYPE vfs_uptime_seconds gauge\n"
	output += formatPrometheusMetric("vfs_uptime_seconds", snapshot.Uptime) + "\n"

	// Requests
	output += "# HELP vfs_requests_total Total number of requests\n"
	output += "# TYPE vfs_requests_total counter\n"
	output += formatPrometheusMetric("vfs_requests_total", snapshot.TotalRequests) + "\n"

	// File operations
	output += "# HELP vfs_file_uploads_total Total file uploads\n"
	output += "# TYPE vfs_file_uploads_total counter\n"
	output += formatPrometheusMetric("vfs_file_uploads_total", snapshot.FileUploads) + "\n"

	output += "# HELP vfs_file_downloads_total Total file downloads\n"
	output += "# TYPE vfs_file_downloads_total counter\n"
	output += formatPrometheusMetric("vfs_file_downloads_total", snapshot.FileDownloads) + "\n"

	output += "# HELP vfs_bytes_uploaded_total Total bytes uploaded\n"
	output += "# TYPE vfs_bytes_uploaded_total counter\n"
	output += formatPrometheusMetric("vfs_bytes_uploaded_total", snapshot.TotalBytesUploaded) + "\n"

	output += "# HELP vfs_bytes_downloaded_total Total bytes downloaded\n"
	output += "# TYPE vfs_bytes_downloaded_total counter\n"
	output += formatPrometheusMetric("vfs_bytes_downloaded_total", snapshot.TotalBytesDownloaded) + "\n"

	// Directory operations
	output += "# HELP vfs_directory_creations_total Total directory creations\n"
	output += "# TYPE vfs_directory_creations_total counter\n"
	output += formatPrometheusMetric("vfs_directory_creations_total", snapshot.DirectoryCreations) + "\n"

	// Events
	output += "# HELP vfs_events_processed_total Total events processed\n"
	output += "# TYPE vfs_events_processed_total counter\n"
	output += formatPrometheusMetric("vfs_events_processed_total", snapshot.EventsProcessed) + "\n"

	output += "# HELP vfs_events_failed_total Total events failed\n"
	output += "# TYPE vfs_events_failed_total counter\n"
	output += formatPrometheusMetric("vfs_events_failed_total", snapshot.EventsFailed) + "\n"

	output += "# HELP vfs_event_success_rate Event success rate\n"
	output += "# TYPE vfs_event_success_rate gauge\n"
	output += formatPrometheusMetric("vfs_event_success_rate", snapshot.EventSuccessRate) + "\n"

	// Webhooks
	output += "# HELP vfs_webhooks_sent_total Total webhooks sent\n"
	output += "# TYPE vfs_webhooks_sent_total counter\n"
	output += formatPrometheusMetric("vfs_webhooks_sent_total", snapshot.WebhooksSent) + "\n"

	output += "# HELP vfs_webhook_success_rate Webhook success rate\n"
	output += "# TYPE vfs_webhook_success_rate gauge\n"
	output += formatPrometheusMetric("vfs_webhook_success_rate", snapshot.WebhookSuccessRate) + "\n"

	// Cron
	output += "# HELP vfs_cron_jobs_executed_total Total cron jobs executed\n"
	output += "# TYPE vfs_cron_jobs_executed_total counter\n"
	output += formatPrometheusMetric("vfs_cron_jobs_executed_total", snapshot.CronJobsExecuted) + "\n"

	output += "# HELP vfs_cron_success_rate Cron job success rate\n"
	output += "# TYPE vfs_cron_success_rate gauge\n"
	output += formatPrometheusMetric("vfs_cron_success_rate", snapshot.CronSuccessRate) + "\n"

	// Idempotency
	output += "# HELP vfs_idempotency_hit_rate Idempotency cache hit rate\n"
	output += "# TYPE vfs_idempotency_hit_rate gauge\n"
	output += formatPrometheusMetric("vfs_idempotency_hit_rate", snapshot.IdempotencyHitRate) + "\n"

	c.Data(consts.StatusOK, "text/plain; version=0.0.4", []byte(output))
}

func formatPrometheusMetric(name string, value interface{}) string {
	switch v := value.(type) {
	case int64:
		return name + " " + string(rune(v))
	case float64:
		data, _ := json.Marshal(v)
		return name + " " + string(data)
	default:
		data, _ := json.Marshal(v)
		return name + " " + string(data)
	}
}
