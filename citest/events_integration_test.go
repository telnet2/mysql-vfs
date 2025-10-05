package citest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/events/handlers"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// ============================================================================
// Stub Repositories for Event Trigger Tests
// ============================================================================

type stubFileRepository struct {
	files map[string]map[string]*models.File
}

func (r *stubFileRepository) Create(ctx context.Context, file *models.File) error {
	return fmt.Errorf("not implemented")
}

func (r *stubFileRepository) CreateFile(ctx context.Context, file *models.File, content []byte) error {
	return fmt.Errorf("not implemented")
}

func (r *stubFileRepository) FindByID(ctx context.Context, id string) (*models.File, error) {
	return nil, db.ErrNotFound
}

func (r *stubFileRepository) FindByDirectoryAndName(ctx context.Context, dirID, name string) (*models.File, error) {
	if dirFiles, ok := r.files[dirID]; ok {
		if file, ok := dirFiles[name]; ok {
			return file, nil
		}
	}
	return nil, db.ErrNotFound
}

func (r *stubFileRepository) FindByDirectoryID(ctx context.Context, dirID string, limit int, cursor string) ([]*models.File, string, error) {
	return nil, "", db.ErrNotFound
}

func (r *stubFileRepository) Update(ctx context.Context, file *models.File) error {
	return fmt.Errorf("not implemented")
}

func (r *stubFileRepository) UpdateFile(ctx context.Context, file *models.File, content []byte) error {
	return fmt.Errorf("not implemented")
}

func (r *stubFileRepository) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *stubFileRepository) SoftDelete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *stubFileRepository) CreateVersion(ctx context.Context, version *models.FileVersion) error {
	return fmt.Errorf("not implemented")
}

func (r *stubFileRepository) GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error) {
	return nil, db.ErrNotFound
}

func (r *stubFileRepository) GetVersion(ctx context.Context, fileID string, version int) (*models.FileVersion, error) {
	return nil, db.ErrNotFound
}

func (r *stubFileRepository) ListVersions(ctx context.Context, fileID string) ([]*models.FileVersion, error) {
	return nil, db.ErrNotFound
}

func (r *stubFileRepository) GetFileContent(ctx context.Context, file *models.File) ([]byte, error) {
	return nil, db.ErrNotFound
}

func (r *stubFileRepository) Exists(ctx context.Context, dirID, name string) (bool, error) {
	return false, nil
}

type stubDirectoryRepository struct {
	byID   map[string]*models.Directory
	byPath map[string]*models.Directory
	mu     sync.RWMutex
}

func (r *stubDirectoryRepository) Create(ctx context.Context, dir *models.Directory) error {
	return fmt.Errorf("not implemented")
}

func (r *stubDirectoryRepository) FindByID(ctx context.Context, id string) (*models.Directory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if dir, ok := r.byID[id]; ok {
		return dir, nil
	}
	return nil, db.ErrNotFound
}

func (r *stubDirectoryRepository) FindByPath(ctx context.Context, path string) (*models.Directory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if dir, ok := r.byPath[path]; ok {
		return dir, nil
	}
	return nil, db.ErrNotFound
}

func (r *stubDirectoryRepository) FindByParentID(ctx context.Context, parentID string, limit int, cursor string) ([]*models.Directory, string, error) {
	return nil, "", nil
}

func (r *stubDirectoryRepository) Update(ctx context.Context, dir *models.Directory) error {
	return fmt.Errorf("not implemented")
}

func (r *stubDirectoryRepository) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *stubDirectoryRepository) SoftDelete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *stubDirectoryRepository) LockPaths(ctx context.Context, tx db.Transaction, paths []string) error {
	return fmt.Errorf("not implemented")
}

func (r *stubDirectoryRepository) Exists(ctx context.Context, path string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byPath[path]
	return ok, nil
}

// ============================================================================
// Events Configuration E2E Tests
// ============================================================================

var _ = Describe("Events Configuration E2E", Ordered, func() {
	var (
		ctx         context.Context
		testDB      *fixtures.TestDatabase
		testStorage *fixtures.TestS3
		dirService  *domain.DirectoryService
		fileService *domain.FileService
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Events Configuration test environment...")
		testDB = fixtures.NewTestDatabase()
		testStorage = fixtures.NewTestS3()
		ctx = context.Background()

		// Create services
		dirService = domain.NewDirectoryService(testDB.GetDB())
		fileService = domain.NewFileService(testDB.GetDB(), testStorage.Storage)

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testStorage.Cleanup()
	})

	Context("when creating .events file with valid patterns", func() {
		It("should accept valid lifecycle event patterns", func() {
			// Create directory
			dir, err := dirService.CreateDirectory(ctx, "/", "webhooks")
			Expect(err).NotTo(HaveOccurred())
			Expect(dir.Path).To(Equal("/webhooks"))

			// Create .events with valid lifecycle patterns
			eventsConfig := `{
				"handlers": [
					{
						"name": "file-create-webhook",
						"events": ["file.create.completion.succeeded"],
						"type": "webhook",
						"config": {
							"url": "https://example.com/webhook"
						}
					}
				]
			}`

			eventsFile, err := fileService.CreateFile(
				ctx,
				"/webhooks",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(eventsFile.Name).To(Equal(".events"))
		})

		It("should accept wildcard patterns", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "monitoring")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with wildcard patterns
			eventsConfig := `{
				"handlers": [
					{
						"name": "all-creates",
						"events": ["file.create.*"],
						"type": "log",
						"config": {
							"level": "info",
							"message": "File created"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/monitoring",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept multi-wildcard patterns", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "audit")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with multi-wildcard patterns
			eventsConfig := `{
				"handlers": [
					{
						"name": "all-auth-events",
						"events": ["*.*.authorization.*"],
						"type": "metrics",
						"config": {
							"metric_name": "auth_checks"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/audit",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept brace expansion patterns", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "notifications")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with brace expansion
			eventsConfig := `{
				"handlers": [
					{
						"name": "create-update-webhook",
						"events": ["file.{create,update}.completion.succeeded"],
						"type": "webhook",
						"config": {
							"url": "https://example.com/events"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/notifications",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept multiple valid patterns in single handler", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "multi-events")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with multiple patterns
			eventsConfig := `{
				"handlers": [
					{
						"name": "multi-pattern-handler",
						"events": [
							"file.create.authorization.started",
							"file.create.validation.succeeded",
							"file.create.completion.*"
						],
						"type": "webhook",
						"config": {
							"url": "https://example.com/multi"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/multi-events",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when creating .events file with invalid patterns", func() {
		It("should reject empty event patterns", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-empty")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with empty pattern
			eventsConfig := `{
				"handlers": [
					{
						"name": "bad-handler",
						"events": [""],
						"type": "webhook",
						"config": {
							"url": "https://example.com"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-empty",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty"))
		})

		It("should reject missing events array", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-no-events")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with no events
			eventsConfig := `{
				"handlers": [
					{
						"name": "no-events-handler",
						"events": [],
						"type": "webhook",
						"config": {
							"url": "https://example.com"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-no-events",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one event"))
		})

		It("should reject invalid handler type", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-type")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with invalid handler type
			eventsConfig := `{
				"handlers": [
					{
						"name": "bad-type",
						"events": ["file.create.*"],
						"type": "invalid_type",
						"config": {}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-type",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid handler type"))
		})

		It("should reject missing handler name", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-no-name")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with no handler name
			eventsConfig := `{
				"handlers": [
					{
						"events": ["file.create.*"],
						"type": "webhook",
						"config": {
							"url": "https://example.com"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-no-name",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name is required"))
		})

		It("should reject duplicate handler names", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-duplicate")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with duplicate names
			eventsConfig := `{
				"handlers": [
					{
						"name": "webhook",
						"events": ["file.create.*"],
						"type": "webhook",
						"config": {
							"url": "https://example.com"
						}
					},
					{
						"name": "webhook",
						"events": ["file.update.*"],
						"type": "webhook",
						"config": {
							"url": "https://example.com/other"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-duplicate",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate handler name"))
		})

		It("should reject missing config", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-no-config")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with no config
			eventsConfig := `{
				"handlers": [
					{
						"name": "no-config",
						"events": ["file.create.*"],
						"type": "webhook"
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-no-config",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config is required"))
		})

		It("should reject empty handlers array", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-no-handlers")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with no handlers
			eventsConfig := `{
				"handlers": []
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-no-handlers",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one handler"))
		})

		It("should reject invalid JSON", func() {
			// Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "invalid-json")
			Expect(err).NotTo(HaveOccurred())

			// Create .events with invalid JSON
			eventsConfig := `{
				"handlers": [
					{
						"name": "broken"
						"events": ["file.create.*"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/invalid-json",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})
	})

	Context("when testing different handler types", func() {
		It("should accept webhook handler with valid config", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "webhook-test")
			Expect(err).NotTo(HaveOccurred())

			eventsConfig := `{
				"handlers": [
					{
						"name": "webhook-handler",
						"events": ["file.create.completion.succeeded"],
						"type": "webhook",
						"config": {
							"url": "https://example.com/webhook",
							"method": "POST",
							"headers": {
								"Authorization": "Bearer token123"
							}
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/webhook-test",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept log handler with valid config", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "log-test")
			Expect(err).NotTo(HaveOccurred())

			eventsConfig := `{
				"handlers": [
					{
						"name": "log-handler",
						"events": ["*.*.validation.*"],
						"type": "log",
						"config": {
							"level": "info",
							"message": "Validation event: {{event.type}}"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/log-test",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept metrics handler with valid config", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "metrics-test")
			Expect(err).NotTo(HaveOccurred())

			eventsConfig := `{
				"handlers": [
					{
						"name": "metrics-handler",
						"events": ["file.{create,update,delete}.completion.succeeded"],
						"type": "metrics",
						"config": {
							"metric_name": "vfs_operations_total",
							"tags": {
								"operation": "{{event.type}}"
							}
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/metrics-test",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when testing advanced event patterns", func() {
		It("should accept complex nested wildcard patterns", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "complex-patterns")
			Expect(err).NotTo(HaveOccurred())

			eventsConfig := `{
				"handlers": [
					{
						"name": "complex-handler",
						"events": [
							"file.create.authorization.*",
							"file.create.validation.schema.*",
							"directory.*.completion.succeeded"
						],
						"type": "log",
						"config": {
							"level": "debug"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/complex-patterns",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept patterns matching all failure events", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "failure-tracking")
			Expect(err).NotTo(HaveOccurred())

			eventsConfig := `{
				"handlers": [
					{
						"name": "failure-alerts",
						"events": ["*.*.*.failed"],
						"type": "webhook",
						"config": {
							"url": "https://alerts.example.com/failures"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/failure-tracking",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept patterns for specific substages", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "substage-events")
			Expect(err).NotTo(HaveOccurred())

			eventsConfig := `{
				"handlers": [
					{
						"name": "schema-validation-tracker",
						"events": ["file.*.validation.schema.checking"],
						"type": "metrics",
						"config": {
							"metric_name": "schema_validations"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/substage-events",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when testing real-world scenarios", func() {
		It("should support comprehensive monitoring setup", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "production")
			Expect(err).NotTo(HaveOccurred())

			eventsConfig := `{
				"handlers": [
					{
						"name": "auth-failures",
						"events": ["*.*.authorization.failed"],
						"type": "webhook",
						"config": {
							"url": "https://security.example.com/auth-failures"
						}
					},
					{
						"name": "operation-metrics",
						"events": ["*.*.completion.succeeded"],
						"type": "metrics",
						"config": {
							"metric_name": "vfs_operations_total"
						}
					},
					{
						"name": "audit-log",
						"events": ["file.{create,update,delete}.completion.*"],
						"type": "log",
						"config": {
							"level": "info",
							"message": "File operation: {{event.type}} on {{resource.path}}"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/production",
				".events",
				"application/json",
				int64(len(eventsConfig)),
				io.NopCloser(strings.NewReader(eventsConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// ============================================================================
// Event Trigger Async Behavior Tests
// ============================================================================

var _ = Describe("Event Trigger Async Behavior", func() {
	It("keeps async handlers alive after request context cancellation", func() {
		tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("webhook-%d", time.Now().UnixNano()))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = os.WriteFile(tmpFile, []byte("ok"), 0o644)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		defer os.Remove(tmpFile)

		dirRepo := &stubDirectoryRepository{
			byID:   map[string]*models.Directory{},
			byPath: map[string]*models.Directory{},
		}

		rootID := "root"
		dirRepo.byID[rootID] = &models.Directory{ID: rootID, Path: "/", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		childID := "dir-1"
		dirRepo.byID[childID] = &models.Directory{ID: childID, Path: "/projects", ParentID: &rootID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		dirRepo.byPath["/"] = dirRepo.byID[rootID]
		dirRepo.byPath["/projects"] = dirRepo.byID[childID]

		handlerConfig := events.EventsFile{Handlers: []events.EventHandler{
			{
				Name:   "test-webhook",
				Events: []events.EventType{"file.create.completion.succeeded"},
				Type:   events.HandlerTypeWebhook,
				Config: events.WebhookConfig{URL: server.URL, Method: http.MethodPost},
			},
		}}
		configBytes, err := json.Marshal(handlerConfig)
		Expect(err).NotTo(HaveOccurred())
		content := string(configBytes)

		fileRepo := &stubFileRepository{
			files: map[string]map[string]*models.File{
				childID: {
					".events": {
						ID:          "events-file",
						DirectoryID: childID,
						Name:        ".events",
						JSONContent: &content,
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					},
				},
			},
		}

		loader := domain.NewEventsLoader(fileRepo, dirRepo, time.Minute)
		registry := handlers.NewRegistry()
		registry.Register(handlers.NewWebhookHandler())

		trigger := domain.NewLifecycleEventTrigger(loader, registry, domain.EventTriggerConfig{
			MaxConcurrentHandlers: 1,
			AsyncHandlerTimeout:   5 * time.Second,
		})

		payload := &events.CompletionEventPayload{
			Event: events.LifecycleEvent{
				ID:          "event-id",
				Category:    events.CategoryFile,
				Operation:   events.OperationCreate,
				Stage:       events.StageCompletion,
				Timestamp:   time.Now(),
				OperationID: "op-1",
			},
			Resource: events.FileResource{
				Path: "/projects/test.txt",
			},
			Metadata: events.EventMetadata{
				RequestID: "req-123",
			},
			OperationContext: &events.OperationContext{
				OperationID:  "op-1",
				Category:     events.CategoryFile,
				Operation:    events.OperationCreate,
				ResourcePath: "/projects/test.txt",
			},
		}

		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "requestID", "req-123"))
		trigger.Emit(ctx, "file.create.completion.succeeded", payload)
		cancel()

		Eventually(func() bool {
			data, err := os.ReadFile(tmpFile)
			if err != nil {
				return false
			}
			return string(data) == "ok"
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

		Expect(trigger.Shutdown(context.Background())).To(Succeed())
	})
})
