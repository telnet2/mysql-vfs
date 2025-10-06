package citest

import (
	"context"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
)

var _ = Describe("Schema Protocol ($ref with schema://) Validation E2E", Ordered, func() {
	var (
		ctx         context.Context
		testDB      *fixtures.TestDatabase
		testStorage *fixtures.TestS3
		dirService  *domain.DirectoryService
		fileService *domain.FileService
		filesLoader *domain.FilesLoader
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Schema Protocol Validation test environment...")
		testDB = fixtures.NewTestDatabase()
		testStorage = fixtures.NewTestS3()
		ctx = context.Background()

		// Create repositories
		fileRepo := mysql.NewGormFileRepository(testDB.GetDB(), testStorage.Storage)
		dirRepo := mysql.NewGormDirectoryRepository(testDB.GetDB())

		// Create files loader with 5-minute cache
		filesLoader = domain.NewFilesLoader(fileRepo, dirRepo, 5*time.Minute)

		// Create services
		dirService = domain.NewDirectoryService(testDB.GetDB())
		fileService = domain.NewFileServiceWithValidation(testDB.GetDB(), testStorage.Storage, filesLoader)

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testStorage.Cleanup()
	})

	Context("External Schema Files using $ref with schema:// protocol", func() {
		It("should validate files using $ref to VFS schemas", func() {
			// Step 1: Create /schemas directory for storing reusable schemas
			schemasDir, err := dirService.CreateDirectory(ctx, "/", "schemas")
			Expect(err).NotTo(HaveOccurred())
			Expect(schemasDir.Path).To(Equal("/schemas"))

			// Step 2: Create a reusable user schema in /schemas/user.json
			userSchema := `{
				"type": "object",
				"properties": {
					"name": {
						"type": "string"
					},
					"email": {
						"type": "string"
					}
				},
				"required": ["name", "email"]
			}`

			schemaFile, err := fileService.CreateFile(
				ctx,
				"/schemas",
				"user.json",
				"application/json",
				int64(len(userSchema)),
				io.NopCloser(strings.NewReader(userSchema)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(schemaFile.Name).To(Equal("user.json"))

			// Step 3: Create /data directory
			dataDir, err := dirService.CreateDirectory(ctx, "/", "data")
			Expect(err).NotTo(HaveOccurred())
			Expect(dataDir.Path).To(Equal("/data"))

			// Step 4: Create .files config that uses $ref with schema:// protocol
			filesConfig := `{
				"rules": [
					{
						"pattern": "user-*.json",
						"type": "glob",
						"schema": {
							"$ref": "schema:///schemas/user.json"
						}
					}
				],
				"default_action": "allow"
			}`

			filesFile, err := fileService.CreateFile(
				ctx,
				"/data",
				".files",
				"application/json",
				int64(len(filesConfig)),
				io.NopCloser(strings.NewReader(filesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(filesFile.Name).To(Equal(".files"))

			// Step 5: Upload valid user file - should succeed
			validUser := `{
				"name": "Alice Smith",
				"email": "alice@example.com",
				"age": 30
			}`

			validFile, err := fileService.CreateFile(
				ctx,
				"/data",
				"user-alice.json",
				"application/json",
				int64(len(validUser)),
				io.NopCloser(strings.NewReader(validUser)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(validFile.Name).To(Equal("user-alice.json"))

			// Step 6: Upload invalid user file - should fail
			invalidUser := `{
				"name": "Bob",
				"age": 25
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/data",
				"user-bob.json",
				"application/json",
				int64(len(invalidUser)),
				io.NopCloser(strings.NewReader(invalidUser)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("content validation failed"))
			Expect(err.Error()).To(ContainSubstring("email"))

			// Step 7: Upload file that doesn't match pattern - should allow
			otherFile := `{"random": "data"}`
			allowedFile, err := fileService.CreateFile(
				ctx,
				"/data",
				"config.json",
				"application/json",
				int64(len(otherFile)),
				io.NopCloser(strings.NewReader(otherFile)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowedFile.Name).To(Equal("config.json"))
		})
	})

	Context("Schema Caching", func() {
		It("should cache externally referenced schemas", func() {
			// Step 1: Create another directory to test schema caching
			_, err := dirService.CreateDirectory(ctx, "/", "products")
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Create a product schema
			productSchema := `{
				"type": "object",
				"properties": {
					"id": {"type": "string"},
					"name": {"type": "string"},
					"price": {"type": "number"}
				},
				"required": ["id", "name", "price"]
			}`

			// Create schema in /schemas directory (already exists from previous test)
			schemaFile, err := fileService.CreateFile(
				ctx,
				"/schemas",
				"product.json",
				"application/json",
				int64(len(productSchema)),
				io.NopCloser(strings.NewReader(productSchema)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(schemaFile.Name).To(Equal("product.json"))

			// Step 3: Create .files config with $ref to product schema
			filesConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"$ref": "schema:///schemas/product.json"
						}
					}
				]
			}`

			filesFile, err := fileService.CreateFile(
				ctx,
				"/products",
				".files",
				"application/json",
				int64(len(filesConfig)),
				io.NopCloser(strings.NewReader(filesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(filesFile.Name).To(Equal(".files"))

			// Step 4: Upload multiple files - schema should be loaded once and cached
			validProduct1 := `{"id": "p1", "name": "Widget", "price": 9.99}`
			validProduct2 := `{"id": "p2", "name": "Gadget", "price": 19.99}`

			file1, err := fileService.CreateFile(
				ctx,
				"/products",
				"product1.json",
				"application/json",
				int64(len(validProduct1)),
				io.NopCloser(strings.NewReader(validProduct1)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(file1.Name).To(Equal("product1.json"))

			file2, err := fileService.CreateFile(
				ctx,
				"/products",
				"product2.json",
				"application/json",
				int64(len(validProduct2)),
				io.NopCloser(strings.NewReader(validProduct2)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(file2.Name).To(Equal("product2.json"))

			// Both files should validate successfully using cached schema
		})
	})

	Context("Nested $ref Support", func() {
		It("should support $ref within schemas", func() {
			// Ensure /schemas directory exists (may already exist from previous tests)
			// Try to create it, ignore error if it already exists
			dirService.CreateDirectory(ctx, "/", "schemas")

			// Create address schema
			addressSchema := `{
				"type": "object",
				"properties": {
					"street": {"type": "string"},
					"city": {"type": "string"},
					"zip": {"type": "string"}
				},
				"required": ["street", "city"]
			}`

			_, err := fileService.CreateFile(
				ctx,
				"/schemas",
				"address.json",
				"application/json",
				int64(len(addressSchema)),
				io.NopCloser(strings.NewReader(addressSchema)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create person schema that references address
			personSchema := `{
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"address": {"$ref": "schema:///schemas/address.json"}
				},
				"required": ["name", "address"]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/schemas",
				"person.json",
				"application/json",
				int64(len(personSchema)),
				io.NopCloser(strings.NewReader(personSchema)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create directory for testing
			_, err = dirService.CreateDirectory(ctx, "/", "people")
			Expect(err).NotTo(HaveOccurred())

			// Create .files that references person schema
			filesConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"$ref": "schema:///schemas/person.json"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/people",
				".files",
				"application/json",
				int64(len(filesConfig)),
				io.NopCloser(strings.NewReader(filesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Upload valid person with address
			validPerson := `{
				"name": "Alice",
				"address": {
					"street": "123 Main St",
					"city": "Boston",
					"zip": "02101"
				}
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/people",
				"alice.json",
				"application/json",
				int64(len(validPerson)),
				io.NopCloser(strings.NewReader(validPerson)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Upload invalid person (missing city in address)
			invalidPerson := `{
				"name": "Bob",
				"address": {
					"street": "456 Oak Ave"
				}
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/people",
				"bob.json",
				"application/json",
				int64(len(invalidPerson)),
				io.NopCloser(strings.NewReader(invalidPerson)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("content validation failed"))
		})
	})

	Context("Missing Schema File", func() {
		It("should fail validation when referenced schema does not exist", func() {
			// Create test directory
			_, err := dirService.CreateDirectory(ctx, "/", "missing-schema-test")
			Expect(err).NotTo(HaveOccurred())

			// Create .files config with $ref to non-existent schema
			filesConfig := `{
				"rules": [
					{
						"pattern": "*.json",
						"type": "glob",
						"schema": {
							"$ref": "schema:///schemas/nonexistent.json"
						}
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/missing-schema-test",
				".files",
				"application/json",
				int64(len(filesConfig)),
				io.NopCloser(strings.NewReader(filesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Try to upload a file - should fail because schema doesn't exist
			testData := `{"test": "data"}`
			_, err = fileService.CreateFile(
				ctx,
				"/missing-schema-test",
				"test.json",
				"application/json",
				int64(len(testData)),
				io.NopCloser(strings.NewReader(testData)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("schema file not found"))
		})
	})
})
