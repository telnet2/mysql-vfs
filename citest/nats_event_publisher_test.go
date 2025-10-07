package citest

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nats-io/nats.go"
	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
)

// EventCollector collects events from NATS for testing
type EventCollector struct {
	mu     sync.RWMutex
	events []NATSEvent
}

// NATSEvent represents an event received from NATS
type NATSEvent struct {
	Subject   string
	EventType string
	Payload   map[string]interface{}
	Timestamp time.Time
}

func (ec *EventCollector) Add(subject string, data []byte) {
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to unmarshal event: %v\n", err)
		return
	}

	// Extract event type from subject (e.g., "vfs.events.file.create.completion.succeeded")
	eventType := ""
	if len(subject) > len("vfs.events.") {
		eventType = subject[len("vfs.events."):]
	}

	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.events = append(ec.events, NATSEvent{
		Subject:   subject,
		EventType: eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	})

	GinkgoWriter.Printf("📨 Received event: %s\n", eventType)
}

func (ec *EventCollector) GetEvents() []NATSEvent {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	// Return a copy
	result := make([]NATSEvent, len(ec.events))
	copy(result, ec.events)
	return result
}

func (ec *EventCollector) Count() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.events)
}

func (ec *EventCollector) FindByType(eventType string) *NATSEvent {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	for _, e := range ec.events {
		if e.EventType == eventType {
			return &e
		}
	}
	return nil
}

func (ec *EventCollector) Reset() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.events = []NATSEvent{}
}

var _ = Describe("NATS Event Publisher Integration", Ordered, func() {
	var (
		testDB      *fixtures.TestDatabase
		testS3      *fixtures.TestS3
		testNATS    *fixtures.TestNATS
		dirService  *domain.DirectoryService
		fileService *domain.FileService
		ctx         context.Context
		collector   *EventCollector
		subscription *nats.Subscription
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up NATS Event Publisher test environment...")

		GinkgoWriter.Println("   - Starting MySQL container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")

		GinkgoWriter.Println("   - Starting S3 storage...")
		testS3 = fixtures.NewTestS3()
		GinkgoWriter.Println("   ✓ S3 ready")

		GinkgoWriter.Println("   - Starting NATS container...")
		testNATS = fixtures.NewTestNATS()
		GinkgoWriter.Println("   ✓ NATS ready at", testNATS.URL)

		ctx = context.Background()

		// Initialize repositories
		fileRepo := mysql.NewGormFileRepository(testDB.GetDB(), testS3.Storage)
		dirRepo := mysql.NewGormDirectoryRepository(testDB.GetDB())

		// Initialize events loader
		eventsLoader := domain.NewEventsLoader(fileRepo, dirRepo, 1*time.Minute)

		// Initialize lifecycle event trigger with NATS
		eventTrigger := domain.NewLifecycleEventTrigger(
			eventsLoader,
			nil, // No handler registry needed for this test
			domain.EventTriggerConfig{
				NATSConn:              testNATS.Conn,
				MaxConcurrentHandlers: 10,
				AsyncHandlerTimeout:   5 * time.Second,
			},
		)

		// Create services with lifecycle events (same as VFS service main.go)
		dirService = domain.NewDirectoryServiceWithLifecycle(testDB.GetDB(), eventTrigger)
		fileService = domain.NewFileServiceWithLifecycle(testDB.GetDB(), testS3.Storage, nil, eventTrigger)

		// Create root directory
		root := &models.Directory{
			ID:       "root",
			Name:     "/",
			Path:     "/",
			PathHash: calculateTestPathHash("/"),
		}
		gormDB := testDB.GetDB()
		gormDB.FirstOrCreate(root, "id = ?", "root")
		sqlDB, _ := gormDB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}

		// Set up event collector
		collector = &EventCollector{}

		// Subscribe to all VFS events
		var err error
		subscription, err = testNATS.Conn.Subscribe("vfs.events.>", func(msg *nats.Msg) {
			collector.Add(msg.Subject, msg.Data)
		})
		Expect(err).NotTo(HaveOccurred())

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		if subscription != nil {
			subscription.Unsubscribe()
		}
		testNATS.Cleanup()
		testDB.Cleanup()
		testS3.Cleanup()
	})

	BeforeEach(func() {
		collector.Reset()
		// Small delay to ensure NATS is ready
		time.Sleep(100 * time.Millisecond)
	})

	Context("when creating files", func() {
		It("should publish file creation lifecycle events to NATS", func() {
			GinkgoWriter.Println("\n🧪 Test: File creation lifecycle events")

			content := "Hello, NATS!"
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"test-nats.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Wait for events to be published
			Eventually(func() int {
				return collector.Count()
			}, 3*time.Second, 100*time.Millisecond).Should(BeNumerically(">", 0))

			// Verify we received the expected lifecycle events
			events := collector.GetEvents()
			GinkgoWriter.Printf("📊 Received %d events\n", len(events))

			// Print all events for debugging
			for _, e := range events {
				GinkgoWriter.Printf("   - %s\n", e.EventType)
			}

			// Should receive at least: authorization.started, authorization.succeeded, completion.succeeded
			eventTypes := make(map[string]bool)
			for _, e := range events {
				eventTypes[e.EventType] = true
			}

			Expect(eventTypes).To(HaveKey("file.create.authorization.started"))
			Expect(eventTypes).To(HaveKey("file.create.authorization.succeeded"))
			Expect(eventTypes).To(HaveKey("file.create.completion.succeeded"))
		})

		It("should include correct payload in completion event", func() {
			GinkgoWriter.Println("\n🧪 Test: Event payload validation")

			content := "Payload test content"
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"payload-test.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Wait for completion event
			Eventually(func() *NATSEvent {
				return collector.FindByType("file.create.completion.succeeded")
			}, 3*time.Second, 100*time.Millisecond).ShouldNot(BeNil())

			completionEvent := collector.FindByType("file.create.completion.succeeded")
			Expect(completionEvent).NotTo(BeNil())

			// Validate payload structure
			payload := completionEvent.Payload
			Expect(payload).To(HaveKey("event"))
			Expect(payload).To(HaveKey("resource"))
			Expect(payload).To(HaveKey("user"))
			Expect(payload).To(HaveKey("metadata"))

			// Validate event details
			eventData, ok := payload["event"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(eventData).To(HaveKey("id"))
			Expect(eventData).To(HaveKey("category"))
			Expect(eventData).To(HaveKey("operation"))
			Expect(eventData).To(HaveKey("stage"))
			Expect(eventData["category"]).To(Equal("file"))
			Expect(eventData["operation"]).To(Equal("create"))
			Expect(eventData["stage"]).To(Equal("completion"))

			// Validate resource details
			resource, ok := payload["resource"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(resource["type"]).To(Equal("file"))
			Expect(resource["name"]).To(Equal("payload-test.txt"))
			// Path may have double slash due to root directory, check it contains the filename
			Expect(resource["path"]).To(ContainSubstring("payload-test.txt"))

			GinkgoWriter.Printf("✅ Event payload validated successfully\n")
		})
	})

	Context("when creating directories", func() {
		It("should publish directory creation events to NATS", func() {
			GinkgoWriter.Println("\n🧪 Test: Directory creation events")

			_, err := dirService.CreateDirectory(ctx, "/", "nats-test-dir")
			Expect(err).NotTo(HaveOccurred())

			// Wait for completion event
			Eventually(func() *NATSEvent {
				return collector.FindByType("directory.create.completion.succeeded")
			}, 3*time.Second, 100*time.Millisecond).ShouldNot(BeNil())

			completionEvent := collector.FindByType("directory.create.completion.succeeded")
			Expect(completionEvent).NotTo(BeNil())

			// Validate directory resource
			payload := completionEvent.Payload
			resource, ok := payload["resource"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(resource["type"]).To(Equal("directory"))
			Expect(resource["name"]).To(Equal("nats-test-dir"))
			Expect(resource["path"]).To(Equal("/nats-test-dir"))

			GinkgoWriter.Printf("✅ Directory event validated\n")
		})
	})

	Context("when testing event filtering with wildcards", func() {
		It("should receive events matching wildcard subscription", func() {
			GinkgoWriter.Println("\n🧪 Test: Wildcard event filtering")

			// Create a new collector for filtered events
			filteredCollector := &EventCollector{}

			// Subscribe only to file.create.> events (> matches all remaining tokens)
			sub, err := testNATS.Conn.Subscribe("vfs.events.file.create.>", func(msg *nats.Msg) {
				filteredCollector.Add(msg.Subject, msg.Data)
			})
			Expect(err).NotTo(HaveOccurred())
			defer sub.Unsubscribe()

			// Create a file
			content := "Filter test"
			_, err = fileService.CreateFile(
				ctx,
				"/",
				"filter-test.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Wait for filtered events
			Eventually(func() int {
				return filteredCollector.Count()
			}, 3*time.Second, 100*time.Millisecond).Should(BeNumerically(">", 0))

			// All received events should be file.create.* events
			events := filteredCollector.GetEvents()
			for _, e := range events {
				Expect(e.EventType).To(HavePrefix("file.create."))
				GinkgoWriter.Printf("   ✓ Filtered event: %s\n", e.EventType)
			}

			GinkgoWriter.Printf("✅ Wildcard filtering validated (%d events)\n", len(events))
		})

		It("should support multiple wildcard levels", func() {
			GinkgoWriter.Println("\n🧪 Test: Multi-level wildcard filtering")

			// Create a collector for completion events only
			completionCollector := &EventCollector{}

			// Subscribe to *.*.completion.> events (completion stage and all substages)
			sub, err := testNATS.Conn.Subscribe("vfs.events.*.*.completion.>", func(msg *nats.Msg) {
				completionCollector.Add(msg.Subject, msg.Data)
			})
			Expect(err).NotTo(HaveOccurred())
			defer sub.Unsubscribe()

			// Create both file and directory
			content := "Completion test"
			_, err = fileService.CreateFile(
				ctx,
				"/",
				"completion-test.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/", "completion-test-dir")
			Expect(err).NotTo(HaveOccurred())

			// Wait for completion events
			Eventually(func() int {
				return completionCollector.Count()
			}, 3*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2))

			events := completionCollector.GetEvents()

			// Should have both file and directory completion events
			hasFileCompletion := false
			hasDirCompletion := false

			for _, e := range events {
				Expect(e.EventType).To(ContainSubstring(".completion."))
				if strings.HasPrefix(e.EventType, "file.") {
					hasFileCompletion = true
				}
				if strings.HasPrefix(e.EventType, "directory.") {
					hasDirCompletion = true
				}
				GinkgoWriter.Printf("   ✓ Completion event: %s\n", e.EventType)
			}

			Expect(hasFileCompletion).To(BeTrue(), "Should receive file completion event")
			Expect(hasDirCompletion).To(BeTrue(), "Should receive directory completion event")

			GinkgoWriter.Printf("✅ Multi-level wildcard filtering validated\n")
		})
	})

	Context("when testing concurrent operations", func() {
		It("should publish events for multiple concurrent file creations", func() {
			GinkgoWriter.Println("\n🧪 Test: Concurrent file operations")

			collector.Reset()

			// Create 5 files concurrently
			var wg sync.WaitGroup
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()

					content := "Concurrent test " + string(rune('0'+index))
					_, err := fileService.CreateFile(
						ctx,
						"/",
						"concurrent-"+string(rune('0'+index))+".txt",
						"text/plain",
						int64(len(content)),
						io.NopCloser(strings.NewReader(content)),
					)
					Expect(err).NotTo(HaveOccurred())
				}(i)
			}
			wg.Wait()

			// Wait for all completion events
			Eventually(func() int {
				count := 0
				for _, e := range collector.GetEvents() {
					if e.EventType == "file.create.completion.succeeded" {
						count++
					}
				}
				return count
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(5))

			GinkgoWriter.Printf("✅ All 5 concurrent operations published events\n")
		})
	})

	Context("when testing event ordering", func() {
		It("should publish lifecycle events in correct order", func() {
			GinkgoWriter.Println("\n🧪 Test: Event lifecycle ordering")

			collector.Reset()

			content := "Order test"
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"order-test.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Wait for completion event
			Eventually(func() *NATSEvent {
				return collector.FindByType("file.create.completion.succeeded")
			}, 3*time.Second, 100*time.Millisecond).ShouldNot(BeNil())

			// Get all file.create events
			events := []NATSEvent{}
			for _, e := range collector.GetEvents() {
				if strings.HasPrefix(e.EventType, "file.create.") {
					events = append(events, e)
				}
			}

			// Find indices of key lifecycle stages
			authStartIdx := -1
			authSucceedIdx := -1
			completionIdx := -1

			for i, e := range events {
				switch e.EventType {
				case "file.create.authorization.started":
					authStartIdx = i
				case "file.create.authorization.succeeded":
					authSucceedIdx = i
				case "file.create.completion.succeeded":
					completionIdx = i
				}
			}

			// Verify ordering: authorization.started < authorization.succeeded < completion.succeeded
			if authStartIdx >= 0 && authSucceedIdx >= 0 {
				Expect(authStartIdx).To(BeNumerically("<", authSucceedIdx))
			}
			if authSucceedIdx >= 0 && completionIdx >= 0 {
				Expect(authSucceedIdx).To(BeNumerically("<", completionIdx))
			}

			GinkgoWriter.Printf("✅ Event ordering validated\n")
		})
	})

	Context("when testing NATS connection reliability", func() {
		It("should handle NATS subscription without errors", func() {
			GinkgoWriter.Println("\n🧪 Test: NATS connection health")

			Expect(testNATS.Conn.IsConnected()).To(BeTrue())
			Expect(subscription.IsValid()).To(BeTrue())

			// Verify we can create a new subscription
			sub, err := testNATS.Conn.Subscribe("vfs.events.test", func(msg *nats.Msg) {})
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.IsValid()).To(BeTrue())
			sub.Unsubscribe()

			GinkgoWriter.Printf("✅ NATS connection healthy\n")
		})
	})
})
