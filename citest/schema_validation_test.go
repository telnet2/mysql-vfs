package citest

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/repository/gorm"
	"github.com/telnet2/mysql-vfs/pkg/services"
)

var _ = Describe("Files Validation E2E", Ordered, func() {
	var (
		ctx         context.Context
		testDB      *fixtures.TestDatabase
		testStorage *fixtures.TestS3
		dirService  *services.DirectoryService
		fileService *services.FileService
		filesLoader *domain.FilesLoader
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Files Validation test environment...")
		testDB = fixtures.NewTestDatabase()
		testStorage = fixtures.NewTestS3()
		ctx = context.Background()

		// Create repositories
		fileRepo := gorm.NewGormFileRepository(testDB.GetDB())
		dirRepo := gorm.NewGormDirectoryRepository(testDB.GetDB())

		// Create files loader with 5-minute cache
		filesLoader = domain.NewFilesLoader(fileRepo, dirRepo, 5*time.Minute)

		// Create services
		dirService = services.NewDirectoryService(testDB.GetDB())
		fileService = services.NewFileServiceWithValidation(testDB.GetDB(), testStorage.Storage, filesLoader)

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testStorage.Cleanup()
	})

	Context("when admin creates a .files config", func() {
		It("should validate files uploaded to that directory", func() {
			// Step 1: Create directory
			dir, err := dirService.CreateDirectory(ctx, "/", "users")
			Expect(err).NotTo(HaveOccurred())
			Expect(dir.Path).To(Equal("/users"))

			// Step 2: Admin creates .files config for user data validation
			filesConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"type": "object",
							"properties": {
								"name": {
									"type": "string",
									"minLength": 1
								},
								"email": {
									"type": "string",
									"format": "email"
								},
								"age": {
									"type": "integer",
									"minimum": 0,
									"maximum": 150
								}
							},
							"required": ["name", "email"]
						}
					}
				]
			}`

			filesFile, err := fileService.CreateFile(
				ctx,
				"/users",
				".files",
				"application/json",
				int64(len(filesConfig)),
				io.NopCloser(strings.NewReader(filesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(filesFile.Name).To(Equal(".files"))

			// Step 3: User uploads valid JSON file
			validUser := `{
				"name": "Alice",
				"email": "alice@example.com",
				"age": 30
			}`

			validFile, err := fileService.CreateFile(
				ctx,
				"/users",
				"alice.json",
				"application/json",
				int64(len(validUser)),
				io.NopCloser(strings.NewReader(validUser)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(validFile.Name).To(Equal("alice.json"))

			// Step 4: User uploads invalid JSON file (missing required field)
			invalidUser := `{
				"name": "Bob"
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/users",
				"bob.json",
				"application/json",
				int64(len(invalidUser)),
				io.NopCloser(strings.NewReader(invalidUser)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("email"))

			// Step 5: User uploads invalid JSON file (wrong type)
			invalidAge := `{
				"name": "Charlie",
				"email": "charlie@example.com",
				"age": "thirty"
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/users",
				"charlie.json",
				"application/json",
				int64(len(invalidAge)),
				io.NopCloser(strings.NewReader(invalidAge)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("age"))
		})
	})

	Context("when .files inheritance is used", func() {
		It("should inherit .files config from parent directory", func() {
			// Step 1: Create parent directory with .files config
			_, err := dirService.CreateDirectory(ctx, "/", "data")
			Expect(err).NotTo(HaveOccurred())

			filesConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"type": "object",
							"properties": {
								"value": {
									"type": "number"
								}
							},
							"required": ["value"]
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/data",
				".files",
				"application/json",
				int64(len(filesConfig)),
				io.NopCloser(strings.NewReader(filesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Create child directory (no schema)
			childDir, err := dirService.CreateDirectory(ctx, "/data", "metrics")
			Expect(err).NotTo(HaveOccurred())
			Expect(childDir.Path).To(Equal("/data/metrics"))

			// Step 3: Upload to child directory - should inherit parent schema
			validData := `{"value": 42}`
			_, err = fileService.CreateFile(
				ctx,
				"/data/metrics",
				"temperature.json",
				"application/json",
				int64(len(validData)),
				io.NopCloser(strings.NewReader(validData)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 4: Upload invalid data to child - should fail with parent's schema
			invalidData := `{"count": 10}`
			_, err = fileService.CreateFile(
				ctx,
				"/data/metrics",
				"invalid.json",
				"application/json",
				int64(len(invalidData)),
				io.NopCloser(strings.NewReader(invalidData)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("value"))
		})

		It("should allow child directory to override parent .files config", func() {
			// Step 1: Create parent with lenient .files config
			_, err := dirService.CreateDirectory(ctx, "/", "config")
			Expect(err).NotTo(HaveOccurred())

			parentConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"type": "object"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/config",
				".files",
				"application/json",
				int64(len(parentConfig)),
				io.NopCloser(strings.NewReader(parentConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Create child with strict .files config
			_, err = dirService.CreateDirectory(ctx, "/config", "strict")
			Expect(err).NotTo(HaveOccurred())

			strictConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"type": "object",
							"properties": {
								"key": {
									"type": "string"
								}
							},
							"required": ["key"],
							"additionalProperties": false
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/config/strict",
				".files",
				"application/json",
				int64(len(strictConfig)),
				io.NopCloser(strings.NewReader(strictConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Upload to parent - lenient, should succeed
			_, err = fileService.CreateFile(
				ctx,
				"/config",
				"general.json",
				"application/json",
				int64(len(`{"anything": "goes", "extra": true}`)),
				io.NopCloser(strings.NewReader(`{"anything": "goes", "extra": true}`)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 4: Upload same to child - strict, should fail
			_, err = fileService.CreateFile(
				ctx,
				"/config/strict",
				"settings.json",
				"application/json",
				int64(len(`{"anything": "goes"}`)),
				io.NopCloser(strings.NewReader(`{"anything": "goes"}`)),
			)
			Expect(err).To(HaveOccurred())

			// Step 5: Upload valid to child - should succeed
			_, err = fileService.CreateFile(
				ctx,
				"/config/strict",
				"valid.json",
				"application/json",
				int64(len(`{"key": "value"}`)),
				io.NopCloser(strings.NewReader(`{"key": "value"}`)),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when validating files with patterns", func() {
		It("should only validate files matching patterns", func() {
			// Create directory with .files config for JSON only
			_, err := dirService.CreateDirectory(ctx, "/", "mixed")
			Expect(err).NotTo(HaveOccurred())

			filesConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"type": "object"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/mixed",
				".files",
				"application/json",
				int64(len(filesConfig)),
				io.NopCloser(strings.NewReader(filesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Upload text file - should not validate
			textContent := "This is plain text, not JSON"
			_, err = fileService.CreateFile(
				ctx,
				"/mixed",
				"readme.txt",
				"text/plain",
				int64(len(textContent)),
				io.NopCloser(strings.NewReader(textContent)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Upload binary file - should not validate
			binaryContent := []byte{0x00, 0x01, 0x02, 0xFF}
			_, err = fileService.CreateFile(
				ctx,
				"/mixed",
				"data.bin",
				"application/octet-stream",
				int64(len(binaryContent)),
				io.NopCloser(bytes.NewReader(binaryContent)),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when directory has no .files config", func() {
		It("should allow any content", func() {
			// Create directory without .files config
			_, err := dirService.CreateDirectory(ctx, "/", "unvalidated")
			Expect(err).NotTo(HaveOccurred())

			// Upload any JSON - should succeed
			anyJSON := `{"random": "data", "numbers": [1, 2, 3]}`
			_, err = fileService.CreateFile(
				ctx,
				"/unvalidated",
				"anything.json",
				"application/json",
				int64(len(anyJSON)),
				io.NopCloser(strings.NewReader(anyJSON)),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
