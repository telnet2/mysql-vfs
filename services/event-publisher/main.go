package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/telnet2/mysql-vfs/pkg/config"
	"github.com/telnet2/mysql-vfs/pkg/discovery"
	"github.com/telnet2/mysql-vfs/pkg/sse"
)

const (
	DefaultEventBufferSize = 1000
	DefaultMaxConnections  = 100
)

type EventPublisher struct {
	natsConn    *nats.Conn
	sseServer   *sse.SSEServer
	eventBuffer chan []byte
	metrics     *Metrics
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

	// Extract event-publisher-specific configuration
	natsURL := cfg.NatsURL
	if natsURL == "" {
		// Fallback to environment variable for backward compatibility
		natsURL = getEnv("NATS_URL", "")
	}
	if natsURL == "" {
		log.Fatalf("NATS URL is required for event-publisher. Set messaging.nats.url in config file or NATS_URL environment variable")
	}

	eventBufferSize := getEnvInt("EVENT_BUFFER_SIZE", DefaultEventBufferSize)
	maxConnections := getEnvInt("SSE_MAX_CONNECTIONS", DefaultMaxConnections)
	serverPort := getEnv("PORT", "8083")
	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		jwtSecret = os.Getenv("JWT_SECRET")
	}
	authEnabled := getEnvBool("AUTH_ENABLED", true)

	log.Printf("Event Publisher starting...")
	log.Printf("NATS URL: %s", natsURL)
	log.Printf("Event buffer size: %d", eventBufferSize)
	log.Printf("Max SSE connections: %d", maxConnections)
	log.Printf("Authentication: %v", authEnabled)

	// Connect to NATS with Consul support
	// Supports both regular URLs and consul+ URLs:
	//   messaging.nats.url: nats://localhost:4222
	//   messaging.nats.url: consul+nats://nats-service
	//   messaging.nats.url: consul+nats://nats-cluster?consul.cluster=prod
	log.Printf("Connecting to NATS (URL: %s)...", natsURL)
	nc, err := discovery.NewNATSConnection(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Initialize metrics
	metrics := NewMetrics()
	metrics.SetBufferCapacity(eventBufferSize)
	metrics.SetNATSConnected(nc.IsConnected())
	log.Println("Prometheus metrics initialized")

	// Initialize authentication middleware
	authMiddleware := NewAuthMiddleware(jwtSecret, authEnabled)
	if authEnabled {
		if jwtSecret == "" {
			log.Println("WARNING: AUTH_ENABLED=true but JWT_SECRET not set, authentication disabled")
			authMiddleware = NewAuthMiddleware("", false)
		} else {
			log.Println("JWT authentication enabled for SSE endpoint")
		}
	} else {
		log.Println("Authentication disabled - SSE endpoint is open to all connections")
	}

	// Create event publisher
	publisher := &EventPublisher{
		natsConn:    nc,
		eventBuffer: make(chan []byte, eventBufferSize),
		sseServer:   sse.NewSSEServer(maxConnections, authMiddleware, metrics),
		metrics:     metrics,
	}

	// Subscribe to all VFS events
	subject := "vfs.events.>"
	log.Printf("Subscribing to NATS subject: %s", subject)
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		publisher.handleNATSMessage(msg)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to NATS: %v", err)
	}
	defer sub.Unsubscribe()
	log.Printf("Subscribed to %s successfully", subject)

	// Start event broadcaster
	go publisher.broadcastEvents()

	// Create HTTP server for SSE endpoint
	h := server.Default(server.WithHostPorts(":" + serverPort))

	// SSE endpoint
	h.GET("/api/v1/publish-events", publisher.sseServer.HandleSSE)

	// Health check endpoint
	h.GET("/health", func(ctx context.Context, c *app.RequestContext) {
		status := "ok"
		natsStatus := "disconnected"
		if nc.IsConnected() {
			natsStatus = "connected"
		}

		c.JSON(200, map[string]interface{}{
			"status":      status,
			"nats":        natsStatus,
			"connections": publisher.sseServer.GetConnectionCount(),
			"buffer_size": len(publisher.eventBuffer),
			"buffer_cap":  cap(publisher.eventBuffer),
		})
	})

	// Prometheus metrics endpoint
	metricsHandler := promhttp.Handler()
	h.GET("/metrics", func(ctx context.Context, c *app.RequestContext) {
		// Update real-time metrics before serving
		publisher.metrics.UpdateBufferSize(len(publisher.eventBuffer))
		publisher.metrics.SetNATSConnected(nc.IsConnected())

		// Adapt Hertz to standard http.ResponseWriter and http.Request
		req := &http.Request{
			Method: string(c.Request.Method()),
			URL:    &url.URL{Path: string(c.Request.URI().Path())},
			Header: make(http.Header),
		}
		c.Request.Header.VisitAll(func(k, v []byte) {
			req.Header.Add(string(k), string(v))
		})

		metricsHandler.ServeHTTP(&hertzResponseWriter{c}, req)
	})

	log.Printf("Event Publisher HTTP server starting on port %s", serverPort)
	log.Printf("SSE endpoint: GET /api/v1/publish-events")

	// Start HTTP server in background
	go h.Spin()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down Event Publisher...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close SSE server (disconnect all clients)
	publisher.sseServer.Shutdown(ctx)

	// Drain NATS subscription
	if err := sub.Drain(); err != nil {
		log.Printf("Error draining NATS subscription: %v", err)
	}

	// Close event buffer
	close(publisher.eventBuffer)

	log.Println("Event Publisher stopped")
}

// handleNATSMessage receives events from NATS and forwards to SSE clients
func (p *EventPublisher) handleNATSMessage(msg *nats.Msg) {
	// Parse event to validate it's valid JSON
	var event map[string]interface{}
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		log.Printf("Invalid event JSON from NATS: %v", err)
		return
	}

	// Extract event type from subject (e.g., "vfs.events.file.create.completion.succeeded")
	eventType := msg.Subject[len("vfs.events."):]

	// Record event received metric
	p.metrics.RecordEventReceived(eventType)

	// Add event type to the event data for filtering
	event["_subject"] = msg.Subject
	event["_event_type"] = eventType

	// Re-marshal with event type
	enrichedData, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal enriched event: %v", err)
		return
	}

	// Send to event buffer (non-blocking)
	select {
	case p.eventBuffer <- enrichedData:
		log.Printf("Buffered event: %s", eventType)
		p.metrics.UpdateBufferSize(len(p.eventBuffer))
	default:
		log.Printf("Event buffer full, dropping event: %s", eventType)
		p.metrics.RecordEventDropped()
	}
}

// broadcastEvents reads from event buffer and broadcasts to all SSE clients
func (p *EventPublisher) broadcastEvents() {
	for eventData := range p.eventBuffer {
		// Broadcast to all connected SSE clients
		p.sseServer.Broadcast(eventData)
	}
}

// Helper functions

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

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

// Adapters to convert Hertz to standard http interfaces for Prometheus

type hertzResponseWriter struct {
	c *app.RequestContext
}

func (w *hertzResponseWriter) Header() http.Header {
	h := make(http.Header)
	w.c.Response.Header.VisitAll(func(k, v []byte) {
		h.Add(string(k), string(v))
	})
	return h
}

func (w *hertzResponseWriter) Write(b []byte) (int, error) {
	return w.c.Response.BodyWriter().Write(b)
}

func (w *hertzResponseWriter) WriteHeader(statusCode int) {
	w.c.Response.SetStatusCode(statusCode)
}
