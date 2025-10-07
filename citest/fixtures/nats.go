package fixtures

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestNATS manages a NATS test container
type TestNATS struct {
	Container testcontainers.Container
	URL       string
	Conn      *nats.Conn
	ctx       context.Context
}

// NewTestNATS creates and starts a NATS container for testing
func NewTestNATS() *TestNATS {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "nats:2.10-alpine",
		Name:         fmt.Sprintf("cc-vfs-nats-%s", uuid.New().String()[:8]),
		ExposedPorts: []string{"4222/tcp", "8222/tcp"},
		Cmd:          []string{"-js", "-m", "8222"},
	}
	req.WaitingFor = wait.ForHTTP("/healthz").
		WithPort("8222").
		WithStartupTimeout(30 * time.Second)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	Expect(err).NotTo(HaveOccurred())

	host, err := container.Host(ctx)
	Expect(err).NotTo(HaveOccurred())

	mappedPort, err := container.MappedPort(ctx, "4222")
	Expect(err).NotTo(HaveOccurred())

	url := fmt.Sprintf("nats://%s:%s", host, mappedPort.Port())

	testNATS := &TestNATS{
		Container: container,
		URL:       url,
		ctx:       ctx,
	}

	// Wait for NATS to be fully ready
	time.Sleep(1 * time.Second)

	// Connect to NATS with retries
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		conn, err := nats.Connect(url,
			nats.MaxReconnects(-1),
			nats.ReconnectWait(1*time.Second),
		)
		if err == nil && conn.IsConnected() {
			testNATS.Conn = conn
			break
		}
		if conn != nil {
			conn.Close()
		}
		if i == maxRetries-1 {
			Expect(err).NotTo(HaveOccurred(), "Failed to connect to NATS after retries")
		}
		time.Sleep(1 * time.Second)
	}

	return testNATS
}

// GetConnection returns a new NATS connection
func (tn *TestNATS) GetConnection() *nats.Conn {
	conn, err := nats.Connect(tn.URL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(1*time.Second),
	)
	Expect(err).NotTo(HaveOccurred())
	return conn
}

// Cleanup terminates the test container
func (tn *TestNATS) Cleanup() {
	if tn.Conn != nil {
		tn.Conn.Close()
	}
	if tn.Container != nil {
		_ = tn.Container.Terminate(tn.ctx)
	}
}
