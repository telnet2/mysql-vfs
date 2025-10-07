package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the event publisher
type Metrics struct {
	// Event metrics
	eventsReceived  prometheus.Counter
	eventsPublished prometheus.Counter
	eventsDropped   prometheus.Counter
	eventsByType    *prometheus.CounterVec

	// Connection metrics
	activeConnections prometheus.Gauge
	totalConnections  prometheus.Counter
	failedAuth        prometheus.Counter

	// Buffer metrics
	bufferSize     prometheus.Gauge
	bufferCapacity prometheus.Gauge

	// NATS metrics
	natsConnected prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		eventsReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "event_publisher_events_received_total",
			Help: "Total number of events received from NATS",
		}),
		eventsPublished: promauto.NewCounter(prometheus.CounterOpts{
			Name: "event_publisher_events_published_total",
			Help: "Total number of events published to SSE clients",
		}),
		eventsDropped: promauto.NewCounter(prometheus.CounterOpts{
			Name: "event_publisher_events_dropped_total",
			Help: "Total number of events dropped due to buffer overflow",
		}),
		eventsByType: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "event_publisher_events_by_type_total",
				Help: "Total number of events by type",
			},
			[]string{"event_type"},
		),
		activeConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "event_publisher_active_connections",
			Help: "Current number of active SSE connections",
		}),
		totalConnections: promauto.NewCounter(prometheus.CounterOpts{
			Name: "event_publisher_connections_total",
			Help: "Total number of SSE connections established",
		}),
		failedAuth: promauto.NewCounter(prometheus.CounterOpts{
			Name: "event_publisher_failed_auth_total",
			Help: "Total number of failed authentication attempts",
		}),
		bufferSize: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "event_publisher_buffer_size",
			Help: "Current number of events in buffer",
		}),
		bufferCapacity: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "event_publisher_buffer_capacity",
			Help: "Total capacity of event buffer",
		}),
		natsConnected: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "event_publisher_nats_connected",
			Help: "NATS connection status (1 = connected, 0 = disconnected)",
		}),
	}
}

// RecordEventReceived increments the events received counter
func (m *Metrics) RecordEventReceived(eventType string) {
	m.eventsReceived.Inc()
	m.eventsByType.WithLabelValues(eventType).Inc()
}

// RecordEventPublished increments the events published counter
func (m *Metrics) RecordEventPublished() {
	m.eventsPublished.Inc()
}

// RecordEventDropped increments the events dropped counter
func (m *Metrics) RecordEventDropped() {
	m.eventsDropped.Inc()
}

// RecordConnectionOpened records a new SSE connection
func (m *Metrics) RecordConnectionOpened() {
	m.totalConnections.Inc()
	m.activeConnections.Inc()
}

// RecordConnectionClosed records an SSE connection closure
func (m *Metrics) RecordConnectionClosed() {
	m.activeConnections.Dec()
}

// RecordFailedAuth increments the failed authentication counter
func (m *Metrics) RecordFailedAuth() {
	m.failedAuth.Inc()
}

// UpdateBufferSize updates the current buffer size gauge
func (m *Metrics) UpdateBufferSize(size int) {
	m.bufferSize.Set(float64(size))
}

// SetBufferCapacity sets the buffer capacity gauge
func (m *Metrics) SetBufferCapacity(capacity int) {
	m.bufferCapacity.Set(float64(capacity))
}

// SetNATSConnected sets the NATS connection status
func (m *Metrics) SetNATSConnected(connected bool) {
	if connected {
		m.natsConnected.Set(1)
	} else {
		m.natsConnected.Set(0)
	}
}
