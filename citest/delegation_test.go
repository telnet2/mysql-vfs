package citest

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockStorage for testing
type MockStorage struct{}

func (m *MockStorage) Put(ctx context.Context, key string, content io.Reader) error {
	return nil
}

func (m *MockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *MockStorage) Close() error {
	return nil
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

var _ = Describe("On-Behalf-Of Delegation", Ordered, func() {
	var (
		db          *gorm.DB
		dirService  *domain.DirectoryService
		fileService *domain.FileService
		mockStorage *MockStorage
		ctx         context.Context
	)

	BeforeAll(func() {
		// Setup test database
		var err error
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())

		// Auto migrate
		err = db.AutoMigrate(&models.Directory{}, &models.File{}, &models.FileVersion{}, &models.Event{})
		Expect(err).NotTo(HaveOccurred())

		// Create root directory
		root := &models.Directory{
			ID:   "root",
			Name: "",
			Path: "/",
		}
		err = db.Create(root).Error
		Expect(err).NotTo(HaveOccurred())

		// Initialize services
		mockStorage = &MockStorage{}
		dirService = domain.NewDirectoryService(db)
		fileService = domain.NewFileService(db, mockStorage)

		// Base context
		ctx = context.Background()
	})

	Context("Service Account Delegation", func() {
		It("should create file with delegated metadata", func() {
			// Create auth context with delegation
			authCtx := &domain.AuthContext{
				UserID:           "service-account",
				Groups:           []string{"service-accounts"},
				PrincipalUserID:  "alice",
				DelegationReason: "automated-backup",
			}
			delegatedCtx := context.WithValue(ctx, "authContext", authCtx)

			// Create file
			content := strings.NewReader("delegated content")
			file, err := fileService.CreateFile(delegatedCtx, "/", "delegated-test.txt", "text/plain", 17, content)
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata shows delegation
			Expect(file.Metadata).NotTo(BeNil())
			var metadata map[string]interface{}
			err = json.Unmarshal([]byte(*file.Metadata), &metadata)
			Expect(err).NotTo(HaveOccurred())

			Expect(metadata["owner"]).To(Equal("alice"), "Owner should be principal")
			Expect(metadata["creator"]).To(Equal("service-account"), "Creator should be actor")
			Expect(metadata["delegated"]).To(BeTrue(), "Should mark as delegated")
			Expect(metadata["delegation_reason"]).To(Equal("automated-backup"), "Should record reason")
		})

		It("should create directory with delegated metadata", func() {
			// Create auth context with delegation
			authCtx := &domain.AuthContext{
				UserID:           "service-account",
				Groups:           []string{"service-accounts"},
				PrincipalUserID:  "alice",
				DelegationReason: "workspace-setup",
			}
			delegatedCtx := context.WithValue(ctx, "authContext", authCtx)

			// Create directory
			dir, err := dirService.CreateDirectory(delegatedCtx, "/", "alice-workspace")
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata
			Expect(dir.Metadata).NotTo(BeNil())
			var metadata map[string]interface{}
			json.Unmarshal([]byte(*dir.Metadata), &metadata)

			Expect(metadata["owner"]).To(Equal("alice"))
			Expect(metadata["creator"]).To(Equal("service-account"))
			Expect(metadata["delegated"]).To(BeTrue())
		})
	})

	Context("Regular User Without Delegation", func() {
		It("should create file with normal metadata", func() {
			// Regular user (no delegation)
			authCtx := &domain.AuthContext{
				UserID: "bob",
				Groups: []string{"user"},
			}
			regularCtx := context.WithValue(ctx, "authContext", authCtx)

			// Create file
			content := strings.NewReader("regular content")
			file, err := fileService.CreateFile(regularCtx, "/", "regular.txt", "text/plain", 15, content)
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata (no delegation)
			var metadata map[string]interface{}
			json.Unmarshal([]byte(*file.Metadata), &metadata)

			Expect(metadata["owner"]).To(Equal("bob"))
			Expect(metadata["creator"]).To(Equal("bob"))
			Expect(metadata["delegated"]).To(BeNil(), "Should not be marked as delegated")
		})
	})

	Context("File Updates with Delegation", func() {
		It("should track delegated updates in metadata", func() {
			// First create a file as alice
			authCtx1 := &domain.AuthContext{
				UserID: "alice",
				Groups: []string{"user"},
			}
			ctx1 := context.WithValue(ctx, "authContext", authCtx1)

			content1 := strings.NewReader("v1")
			file, err := fileService.CreateFile(ctx1, "/", "update-test.txt", "text/plain", 2, content1)
			Expect(err).NotTo(HaveOccurred())

			// Service account updates file on behalf of alice
			authCtx2 := &domain.AuthContext{
				UserID:           "service-account",
				Groups:           []string{"service-accounts"},
				PrincipalUserID:  "alice",
				DelegationReason: "automated-update",
			}
			ctx2 := context.WithValue(ctx, "authContext", authCtx2)

			content2 := strings.NewReader("v2")
			file, err = fileService.UpdateFile(ctx2, "/update-test.txt", "text/plain", 2, content2, 0)
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata tracks update
			var metadata map[string]interface{}
			json.Unmarshal([]byte(*file.Metadata), &metadata)

			Expect(metadata["creator"]).To(Equal("alice"), "Original creator preserved")
			Expect(metadata["updated_by"]).To(Equal("service-account"), "Updated by service account")
		})
	})
})
