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

var _ = Describe("Schema Validation E2E", Ordered, func() {
	var (
		ctx          context.Context
		testDB       *fixtures.TestDatabase
		testStorage  *fixtures.TestS3
		dirService   *services.DirectoryService
		fileService  *services.FileService
		schemaLoader *domain.SchemaLoader
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Schema Validation test environment...")
		testDB = fixtures.NewTestDatabase()
		testStorage = fixtures.NewTestS3()
		ctx = context.Background()

		// Create repositories
		fileRepo := gorm.NewGormFileRepository(testDB.GetDB())
		dirRepo := gorm.NewGormDirectoryRepository(testDB.GetDB())

		// Create schema loader with 5-minute cache
		schemaLoader = domain.NewSchemaLoader(fileRepo, dirRepo, 5*time.Minute)

		// Create services
		dirService = services.NewDirectoryService(testDB.GetDB())
		fileService = services.NewFileServiceWithValidation(testDB.GetDB(), testStorage.Storage, schemaLoader)

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testStorage.Cleanup()
	})

	Context("when admin creates a schema", func() {
		It("should validate files uploaded to that directory", func() {
			// Step 1: Create directory
			dir, err := dirService.CreateDirectory(ctx, "/", "users")
			Expect(err).NotTo(HaveOccurred())
			Expect(dir.Path).To(Equal("/users"))

			// Step 2: Admin creates schema for user data
			schema := `{
				"$schema": "http://json-schema.org/draft-07/schema#",
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
			}`

			schemaFile, err := fileService.CreateFile(
				ctx,
				"/users",
				".jsonschema",
				"application/json",
				int64(len(schema)),
				io.NopCloser(strings.NewReader(schema)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(schemaFile.Name).To(Equal(".jsonschema"))

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

	Context("when schema inheritance is used", func() {
		It("should inherit schema from parent directory", func() {
			// Step 1: Create parent directory with schema
			_, err := dirService.CreateDirectory(ctx, "/", "data")
			Expect(err).NotTo(HaveOccurred())

			schema := `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"value": {
						"type": "number"
					}
				},
				"required": ["value"]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/data",
				".jsonschema",
				"application/json",
				int64(len(schema)),
				io.NopCloser(strings.NewReader(schema)),
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

		It("should allow child directory to override parent schema", func() {
			// Step 1: Create parent with lenient schema
			_, err := dirService.CreateDirectory(ctx, "/", "config")
			Expect(err).NotTo(HaveOccurred())

			parentSchema := `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object"
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/config",
				".jsonschema",
				"application/json",
				int64(len(parentSchema)),
				io.NopCloser(strings.NewReader(parentSchema)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Create child with strict schema
			_, err = dirService.CreateDirectory(ctx, "/config", "strict")
			Expect(err).NotTo(HaveOccurred())

			strictSchema := `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"key": {
						"type": "string"
					}
				},
				"required": ["key"],
				"additionalProperties": false
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/config/strict",
				".jsonschema",
				"application/json",
				int64(len(strictSchema)),
				io.NopCloser(strings.NewReader(strictSchema)),
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

	Context("when validating non-JSON files", func() {
		It("should skip validation for non-JSON content types", func() {
			// Create directory with JSON schema
			_, err := dirService.CreateDirectory(ctx, "/", "mixed")
			Expect(err).NotTo(HaveOccurred())

			schema := `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object"
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/mixed",
				".jsonschema",
				"application/json",
				int64(len(schema)),
				io.NopCloser(strings.NewReader(schema)),
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

	Context("when directory has no schema", func() {
		It("should allow any content", func() {
			// Create directory without schema
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
