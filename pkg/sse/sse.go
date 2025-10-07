package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"github.com/hertz-contrib/sse"
)

// AuthMiddleware interface for authentication
type AuthMiddleware interface {
	ValidateToken(c *app.RequestContext) (interface{}, error)
	CheckPermission(claims interface{}, filter string) bool
}

// Metrics interface for metrics recording
type Metrics interface {
	RecordFailedAuth()
	RecordConnectionOpened()
	RecordConnectionClosed()
	RecordEventPublished()
	RecordEventReceived(eventType string)
	RecordEventDropped()
	UpdateBufferSize(size int)
	SetNATSConnected(connected bool)
}

// SSEClient represents a connected SSE client
type SSEClient struct {
	ID            string
	EventChan     chan []byte
	Filter        string // Event filter pattern (e.g., "file.*", "directory.create")
	ConnectedAt   time.Time
	LastEventSent time.Time
}

// SSEServer manages SSE connections and event broadcasting
type SSEServer struct {
	clients        map[string]*SSEClient
	clientsMutex   sync.RWMutex
	maxConnections int
	authMiddleware AuthMiddleware
	metrics        Metrics
}

// NewSSEServer creates a new SSE server
func NewSSEServer(maxConnections int, authMiddleware AuthMiddleware, metrics Metrics) *SSEServer {
	return &SSEServer{
		clients:        make(map[string]*SSEClient),
		maxConnections: maxConnections,
		authMiddleware: authMiddleware,
		metrics:        metrics,
	}
}

// HandleSSE handles SSE connection requests
func (s *SSEServer) HandleSSE(ctx context.Context, c *app.RequestContext) {
	// Authenticate user
	claims, err := s.authMiddleware.ValidateToken(c)
	if err != nil {
		s.metrics.RecordFailedAuth()
		c.JSON(401, map[string]string{
			"error": fmt.Sprintf("authentication failed: %v", err),
		})
		return
	}

	claimsMap, ok := claims.(map[string]interface{})
	if !ok {
		s.metrics.RecordFailedAuth()
		c.JSON(401, map[string]string{
			"error": "invalid claims format",
		})
		return
	}

	// Check connection limit
	s.clientsMutex.RLock()
	currentCount := len(s.clients)
	s.clientsMutex.RUnlock()

	if currentCount >= s.maxConnections {
		c.JSON(503, map[string]string{
			"error": "maximum connections reached",
		})
		return
	}

	// Get filter parameter (optional)
	filter := string(c.Query("filter"))

	// Check permission to access events with this filter
	if !s.authMiddleware.CheckPermission(claimsMap, filter) {
		c.JSON(403, map[string]string{
			"error": "insufficient permissions for requested filter",
		})
		return
	}

	// Set CORS headers (allow browser access)
	c.Response.Header.Set("Access-Control-Allow-Origin", "*")
	c.Response.Header.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Set SSE headers
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")

	// Set HTTP status code (must be done before writing body)
	c.Response.SetStatusCode(200)

	// Create client
	client := &SSEClient{
		ID:          uuid.New().String(),
		EventChan:   make(chan []byte, 100),
		Filter:      filter,
		ConnectedAt: time.Now(),
	}

	// Register client
	s.clientsMutex.Lock()
	s.clients[client.ID] = client
	s.clientsMutex.Unlock()

	// Record connection metrics
	s.metrics.RecordConnectionOpened()

	log.Printf("SSE client connected: %s (filter: %s, total: %d)", client.ID, filter, len(s.clients))

	// Create SSE stream
	stream := sse.NewStream(c)

	// Send initial connection event
	dataJSON, err := json.Marshal(map[string]interface{}{
		"client_id":    client.ID,
		"connected_at": client.ConnectedAt.Format(time.RFC3339),
		"filter":       filter,
	})
	if err != nil {
		log.Printf("Error marshaling connected event: %v", err)
		s.removeClient(client.ID)
		return
	}
	if err := stream.Publish(&sse.Event{Event: "connected", Data: dataJSON}); err != nil {
		log.Printf("Error sending connected event to client %s: %v", client.ID, err)
		s.removeClient(client.ID)
		return
	}

	// Flush to ensure client receives the connection event
	c.Flush()

	// Start keepalive ticker
	keepaliveTicker := time.NewTicker(30 * time.Second)
	defer keepaliveTicker.Stop()

	// Stream events to client
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			s.removeClient(client.ID)
			return

		case eventData, ok := <-client.EventChan:
			if !ok {
				// Channel closed, client being removed
				return
			}

			// Send event to client
			if err := stream.Publish(&sse.Event{Event: "vfs-event", Data: eventData}); err != nil {
				log.Printf("Error sending event to client %s: %v", client.ID, err)
				s.removeClient(client.ID)
				return
			}

			client.LastEventSent = time.Now()

			// Flush to ensure immediate delivery
			c.Flush()

		case <-keepaliveTicker.C:
			// Send keepalive comment
			if err := stream.Publish(&sse.Event{Data: []byte(": keepalive\n")}); err != nil {
				log.Printf("Error sending keepalive to client %s: %v", client.ID, err)
				s.removeClient(client.ID)
				return
			}
			c.Flush()
		}
	}
}

// Broadcast sends an event to all connected clients (respecting filters)
func (s *SSEServer) Broadcast(eventData []byte) {
	// Parse event to extract event type for filtering
	var event map[string]interface{}
	if err := json.Unmarshal(eventData, &event); err != nil {
		log.Printf("Invalid event data for broadcast: %v", err)
		return
	}

	eventType, ok := event["_event_type"].(string)
	if !ok {
		log.Printf("Event missing _event_type field")
		return
	}

	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	publishedCount := 0
	for _, client := range s.clients {
		// Apply filter if specified
		if client.Filter != "" && !matchFilter(client.Filter, eventType) {
			continue
		}

		// Send to client (non-blocking)
		select {
		case client.EventChan <- eventData:
			// Event queued successfully
			publishedCount++
		default:
			// Client's buffer is full, skip this event
			log.Printf("Client %s buffer full, dropping event", client.ID)
		}
	}

	// Record metrics
	for i := 0; i < publishedCount; i++ {
		s.metrics.RecordEventPublished()
	}

	log.Printf("Broadcasted event %s to %d clients", eventType, publishedCount)
}

// removeClient removes a client from the server
func (s *SSEServer) removeClient(clientID string) {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()

	if client, exists := s.clients[clientID]; exists {
		close(client.EventChan)
		delete(s.clients, clientID)
		s.metrics.RecordConnectionClosed()
		log.Printf("SSE client disconnected: %s (total: %d)", clientID, len(s.clients))
	}
}

// GetConnectionCount returns the number of connected clients
func (s *SSEServer) GetConnectionCount() int {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	return len(s.clients)
}

// Shutdown closes all client connections
func (s *SSEServer) Shutdown(ctx context.Context) {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()

	log.Printf("Shutting down SSE server, disconnecting %d clients", len(s.clients))

	for _, client := range s.clients {
		close(client.EventChan)
	}

	s.clients = make(map[string]*SSEClient)
}

// matchFilter checks if an event type matches a filter pattern
// Supports wildcards: "file.*" matches "file.create", "file.delete", etc.
func matchFilter(filter, eventType string) bool {
	// Exact match
	if filter == eventType {
		return true
	}

	// Wildcard match
	// Split both filter and eventType by dots
	filterParts := strings.Split(filter, ".")
	eventParts := strings.Split(eventType, ".")

	// If filter has more parts than event, it can't match
	if len(filterParts) > len(eventParts) {
		return false
	}

	// Check each part
	for i, filterPart := range filterParts {
		if filterPart == "*" {
			// Wildcard matches anything, but only for one level
			continue
		}

		if filterPart == "**" {
			// Double wildcard matches everything from this point on
			return true
		}

		if filterPart != eventParts[i] {
			return false
		}
	}

	// If filter is shorter and doesn't end with wildcard, it's a prefix match
	// e.g., filter "file" matches "file.create.completion.succeeded"
	return true
}
