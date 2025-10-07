package discovery

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// NewNATSConnection creates a NATS connection with optional Consul service discovery
// Supports both regular NATS URLs and consul+ URLs:
//   - nats://localhost:4222
//   - consul+nats://nats-service
//   - consul+nats://nats-cluster?consul.cluster=prod
func NewNATSConnection(urlStr string, opts ...nats.Option) (*nats.Conn, error) {
	// Resolve consul+ URL if needed
	resolvedURL, err := ResolveOrPassthrough(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve NATS URL: %w", err)
	}

	// Add default options for resilience
	defaultOpts := []nats.Option{
		nats.MaxReconnects(-1),            // Unlimited reconnects
		nats.ReconnectWait(2 * time.Second), // Wait 2s between reconnects
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				log.Printf("NATS disconnected: %v", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %s", nc.ConnectedUrl())
		}),
	}

	// Merge with user-provided options (user options take precedence)
	allOpts := append(defaultOpts, opts...)

	log.Printf("Connecting to NATS at %s...", resolvedURL)
	conn, err := nats.Connect(resolvedURL, allOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	log.Printf("NATS connection established successfully")
	return conn, nil
}

// MustConnectNATS is like NewNATSConnection but panics on error
func MustConnectNATS(urlStr string, opts ...nats.Option) *nats.Conn {
	conn, err := NewNATSConnection(urlStr, opts...)
	if err != nil {
		panic(fmt.Sprintf("failed to connect to NATS: %v", err))
	}
	return conn
}

// NewOptionalNATSConnection creates a NATS connection, but returns nil (without error) if URL is empty
// This is useful for optional NATS connections
func NewOptionalNATSConnection(urlStr string, opts ...nats.Option) (*nats.Conn, error) {
	if urlStr == "" {
		log.Println("NATS URL not provided, NATS connection disabled")
		return nil, nil
	}

	conn, err := NewNATSConnection(urlStr, opts...)
	if err != nil {
		log.Printf("Warning: Failed to connect to NATS (continuing without NATS): %v", err)
		return nil, nil
	}

	return conn, nil
}
