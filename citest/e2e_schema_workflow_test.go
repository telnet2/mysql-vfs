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
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
)

var _ = Describe("End-to-End Schema Validation Workflow", Ordered, func() {
	var (
		testDB      *fixtures.TestDatabase
		testS3      *fixtures.TestS3
		dirService  *domain.DirectoryService
		fileService *domain.FileService
		filesLoader *domain.FilesLoader
		ctx         context.Context
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up End-to-End Schema Workflow test environment...")
		GinkgoWriter.Println("   - Starting MySQL test container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")

		GinkgoWriter.Println("   - Starting S3 test storage...")
		testS3 = fixtures.NewTestS3()
		GinkgoWriter.Println("   ✓ S3 ready")

		ctx = context.Background()

		// Create repositories
		fileRepo := mysql.NewGormFileRepository(testDB.GetDB(), testS3.Storage)
		dirRepo := mysql.NewGormDirectoryRepository(testDB.GetDB())

		// Create files loader with 5-minute cache
		filesLoader = domain.NewFilesLoader(fileRepo, dirRepo, 5*time.Minute)

		// Create services with validation
		dirService = domain.NewDirectoryService(testDB.GetDB())
		fileService = domain.NewFileServiceWithValidation(testDB.GetDB(), testS3.Storage, filesLoader)

		// Create root directory
		root := &models.Directory{
			ID:       "root",
			Name:     "/",
			Path:     "/",
			PathHash: calculateTestPathHash("/"),
		}
		gormDB := testDB.GetDB()
		gormDB.FirstOrCreate(root, "id = ?", "root")
		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testS3.Cleanup()
	})

	Describe("Complete Schema Validation Workflow", func() {
		It("should handle a complete real-world workflow with schemas, validation, and file operations", func() {
			By("Step 1: Setting up schema library in /schemas directory")

			// Create /schemas directory for centralized schema storage
			schemasDir, err := dirService.CreateDirectory(ctx, "/", "schemas")
			Expect(err).NotTo(HaveOccurred())
			Expect(schemasDir.Path).To(Equal("/schemas"))
			GinkgoWriter.Println("   ✓ Created /schemas directory")

			// Create base schemas for reuse

			// 1. Address schema (reusable component)
			addressSchema := `{
  "type": "object",
  "properties": {
    "street": {
      "type": "string",
      "minLength": 1
    },
    "city": {
      "type": "string",
      "minLength": 1
    },
    "state": {
      "type": "string",
      "pattern": "^[A-Z]{2}$"
    },
    "zipcode": {
      "type": "string",
      "pattern": "^[0-9]{5}$"
    }
  },
  "required": ["street", "city", "state", "zipcode"]
}`
			_, err = fileService.CreateFile(
				ctx,
				"/schemas",
				"address.json",
				"application/json",
				int64(len(addressSchema)),
				io.NopCloser(strings.NewReader(addressSchema)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /schemas/address.json")

			// 2. Contact info schema
			contactSchema := `{
  "type": "object",
  "properties": {
    "email": {
      "type": "string",
      "format": "email"
    },
    "phone": {
      "type": "string",
      "pattern": "^\\+?[0-9]{10,15}$"
    }
  },
  "required": ["email"]
}`
			_, err = fileService.CreateFile(
				ctx,
				"/schemas",
				"contact.json",
				"application/json",
				int64(len(contactSchema)),
				io.NopCloser(strings.NewReader(contactSchema)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /schemas/contact.json")

			// 3. Customer schema (references address and contact)
			customerSchema := `{
  "type": "object",
  "properties": {
    "customer_id": {
      "type": "string",
      "pattern": "^CUST-[0-9]{6}$"
    },
    "name": {
      "type": "string",
      "minLength": 1
    },
    "contact": {
      "$ref": "schema:///schemas/contact.json"
    },
    "billing_address": {
      "$ref": "schema:///schemas/address.json"
    },
    "shipping_address": {
      "$ref": "schema:///schemas/address.json"
    }
  },
  "required": ["customer_id", "name", "contact", "billing_address"]
}`
			_, err = fileService.CreateFile(
				ctx,
				"/schemas",
				"customer.json",
				"application/json",
				int64(len(customerSchema)),
				io.NopCloser(strings.NewReader(customerSchema)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /schemas/customer.json (with nested $ref)")

			// 4. Product schema
			productSchema := `{
  "type": "object",
  "properties": {
    "product_id": {
      "type": "string",
      "pattern": "^PROD-[0-9]{6}$"
    },
    "name": {
      "type": "string",
      "minLength": 1
    },
    "price": {
      "type": "number",
      "minimum": 0
    },
    "category": {
      "type": "string",
      "enum": ["electronics", "clothing", "food", "books"]
    }
  },
  "required": ["product_id", "name", "price", "category"]
}`
			_, err = fileService.CreateFile(
				ctx,
				"/schemas",
				"product.json",
				"application/json",
				int64(len(productSchema)),
				io.NopCloser(strings.NewReader(productSchema)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /schemas/product.json")

			By("Step 2: Creating application directories with validation rules")

			// Create /customers directory
			_, err = dirService.CreateDirectory(ctx, "/", "customers")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /customers directory")

			// Create .files config for customers (references customer schema)
			customersFilesConfig := `{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "$ref": "schema:///schemas/customer.json"
      },
      "description": "All customer files must match customer schema"
    }
  ],
  "default_action": "deny"
}`
			_, err = fileService.CreateFile(
				ctx,
				"/customers",
				".files",
				"application/json",
				int64(len(customersFilesConfig)),
				io.NopCloser(strings.NewReader(customersFilesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /customers/.files with schema validation")

			// Create /products directory
			_, err = dirService.CreateDirectory(ctx, "/", "products")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /products directory")

			// Create .files config for products
			productsFilesConfig := `{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "$ref": "schema:///schemas/product.json"
      },
      "description": "All product files must match product schema"
    }
  ],
  "default_action": "deny"
}`
			_, err = fileService.CreateFile(
				ctx,
				"/products",
				".files",
				"application/json",
				int64(len(productsFilesConfig)),
				io.NopCloser(strings.NewReader(productsFilesConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Created /products/.files with schema validation")

			By("Step 3: Uploading valid customer data")

			// Valid customer 1
			customer1 := `{
  "customer_id": "CUST-000001",
  "name": "Alice Johnson",
  "contact": {
    "email": "alice@example.com",
    "phone": "+15551234567"
  },
  "billing_address": {
    "street": "123 Main St",
    "city": "Boston",
    "state": "MA",
    "zipcode": "02101"
  },
  "shipping_address": {
    "street": "456 Oak Ave",
    "city": "Cambridge",
    "state": "MA",
    "zipcode": "02138"
  }
}`
			customer1File, err := fileService.CreateFile(
				ctx,
				"/customers",
				"alice.json",
				"application/json",
				int64(len(customer1)),
				io.NopCloser(strings.NewReader(customer1)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(customer1File.Name).To(Equal("alice.json"))
			GinkgoWriter.Println("   ✓ Uploaded valid customer: alice.json")

			// Valid customer 2 (without optional shipping_address)
			customer2 := `{
  "customer_id": "CUST-000002",
  "name": "Bob Smith",
  "contact": {
    "email": "bob@example.com"
  },
  "billing_address": {
    "street": "789 Elm St",
    "city": "New York",
    "state": "NY",
    "zipcode": "10001"
  }
}`
			customer2File, err := fileService.CreateFile(
				ctx,
				"/customers",
				"bob.json",
				"application/json",
				int64(len(customer2)),
				io.NopCloser(strings.NewReader(customer2)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(customer2File.Name).To(Equal("bob.json"))
			GinkgoWriter.Println("   ✓ Uploaded valid customer: bob.json")

			By("Step 4: Attempting to upload invalid customer data (should fail)")

			// Invalid customer - missing required billing_address
			invalidCustomer1 := `{
  "customer_id": "CUST-000003",
  "name": "Charlie Brown",
  "contact": {
    "email": "charlie@example.com"
  }
}`
			_, err = fileService.CreateFile(
				ctx,
				"/customers",
				"charlie.json",
				"application/json",
				int64(len(invalidCustomer1)),
				io.NopCloser(strings.NewReader(invalidCustomer1)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			GinkgoWriter.Println("   ✓ Correctly rejected invalid customer (missing billing_address)")

			// Invalid customer - wrong customer_id format
			invalidCustomer2 := `{
  "customer_id": "INVALID-ID",
  "name": "David Wilson",
  "contact": {
    "email": "david@example.com"
  },
  "billing_address": {
    "street": "111 Pine St",
    "city": "Seattle",
    "state": "WA",
    "zipcode": "98101"
  }
}`
			_, err = fileService.CreateFile(
				ctx,
				"/customers",
				"david.json",
				"application/json",
				int64(len(invalidCustomer2)),
				io.NopCloser(strings.NewReader(invalidCustomer2)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			GinkgoWriter.Println("   ✓ Correctly rejected invalid customer (wrong ID format)")

			// Invalid customer - invalid email in nested contact
			invalidCustomer3 := `{
  "customer_id": "CUST-000004",
  "name": "Eve Davis",
  "contact": {
    "email": "not-an-email"
  },
  "billing_address": {
    "street": "222 Maple Dr",
    "city": "Portland",
    "state": "OR",
    "zipcode": "97201"
  }
}`
			_, err = fileService.CreateFile(
				ctx,
				"/customers",
				"eve.json",
				"application/json",
				int64(len(invalidCustomer3)),
				io.NopCloser(strings.NewReader(invalidCustomer3)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			GinkgoWriter.Println("   ✓ Correctly rejected invalid customer (invalid email)")

			// Invalid customer - invalid zipcode in nested address
			invalidCustomer4 := `{
  "customer_id": "CUST-000005",
  "name": "Frank Miller",
  "contact": {
    "email": "frank@example.com"
  },
  "billing_address": {
    "street": "333 Cedar Ln",
    "city": "Austin",
    "state": "TX",
    "zipcode": "ABCDE"
  }
}`
			_, err = fileService.CreateFile(
				ctx,
				"/customers",
				"frank.json",
				"application/json",
				int64(len(invalidCustomer4)),
				io.NopCloser(strings.NewReader(invalidCustomer4)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			GinkgoWriter.Println("   ✓ Correctly rejected invalid customer (invalid zipcode)")

			By("Step 5: Uploading valid product data")

			product1 := `{
  "product_id": "PROD-000001",
  "name": "Laptop Computer",
  "price": 999.99,
  "category": "electronics"
}`
			product1File, err := fileService.CreateFile(
				ctx,
				"/products",
				"laptop.json",
				"application/json",
				int64(len(product1)),
				io.NopCloser(strings.NewReader(product1)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(product1File.Name).To(Equal("laptop.json"))
			GinkgoWriter.Println("   ✓ Uploaded valid product: laptop.json")

			product2 := `{
  "product_id": "PROD-000002",
  "name": "T-Shirt",
  "price": 19.99,
  "category": "clothing"
}`
			_, err = fileService.CreateFile(
				ctx,
				"/products",
				"tshirt.json",
				"application/json",
				int64(len(product2)),
				io.NopCloser(strings.NewReader(product2)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Uploaded valid product: tshirt.json")

			By("Step 6: Attempting to upload invalid product data (should fail)")

			// Invalid product - negative price
			invalidProduct1 := `{
  "product_id": "PROD-000003",
  "name": "Bad Product",
  "price": -10.00,
  "category": "electronics"
}`
			_, err = fileService.CreateFile(
				ctx,
				"/products",
				"badproduct.json",
				"application/json",
				int64(len(invalidProduct1)),
				io.NopCloser(strings.NewReader(invalidProduct1)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			GinkgoWriter.Println("   ✓ Correctly rejected invalid product (negative price)")

			// Invalid product - wrong category
			invalidProduct2 := `{
  "product_id": "PROD-000004",
  "name": "Widget",
  "price": 50.00,
  "category": "invalid_category"
}`
			_, err = fileService.CreateFile(
				ctx,
				"/products",
				"widget.json",
				"application/json",
				int64(len(invalidProduct2)),
				io.NopCloser(strings.NewReader(invalidProduct2)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			GinkgoWriter.Println("   ✓ Correctly rejected invalid product (invalid category)")

			By("Step 7: Updating customer data with validation")

			// Update alice.json with new shipping address
			updatedCustomer1 := `{
  "customer_id": "CUST-000001",
  "name": "Alice Johnson",
  "contact": {
    "email": "alice.johnson@example.com",
    "phone": "+15559876543"
  },
  "billing_address": {
    "street": "123 Main St",
    "city": "Boston",
    "state": "MA",
    "zipcode": "02101"
  },
  "shipping_address": {
    "street": "999 Tech Blvd",
    "city": "San Francisco",
    "state": "CA",
    "zipcode": "94102"
  }
}`
			updatedFile, err := fileService.UpdateFile(
				ctx,
				"/customers/alice.json",
				"application/json",
				int64(len(updatedCustomer1)),
				io.NopCloser(strings.NewReader(updatedCustomer1)),
				1,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedFile.Version).To(Equal(int64(2)))
			GinkgoWriter.Println("   ✓ Updated alice.json successfully")

			// Try to update with invalid data (should fail)
			invalidUpdate := `{
  "customer_id": "CUST-000001",
  "name": "Alice Johnson",
  "contact": {
    "email": "invalid-email-format"
  },
  "billing_address": {
    "street": "123 Main St",
    "city": "Boston",
    "state": "MA",
    "zipcode": "02101"
  }
}`
			_, err = fileService.UpdateFile(
				ctx,
				"/customers/alice.json",
				"application/json",
				int64(len(invalidUpdate)),
				io.NopCloser(strings.NewReader(invalidUpdate)),
				2,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			GinkgoWriter.Println("   ✓ Correctly rejected invalid update")

			// Verify alice.json is still at version 2 (invalid update didn't go through)
			aliceFile, _, err := fileService.GetFile(ctx, "/customers/alice.json", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceFile.Version).To(Equal(int64(2)))

			By("Step 8: Verifying file listings")

			// List customers
			_, customerFiles, _, err := dirService.ListDirectory("/customers", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(customerFiles).To(HaveLen(3)) // alice.json, bob.json, .files
			GinkgoWriter.Println("   ✓ Found 3 files in /customers (2 customers + .files)")

			// List products
			_, productFiles, _, err := dirService.ListDirectory("/products", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(productFiles).To(HaveLen(3)) // laptop.json, tshirt.json, .files
			GinkgoWriter.Println("   ✓ Found 3 files in /products (2 products + .files)")

			By("Step 9: Testing that non-JSON files are rejected (default_action: deny)")

			// Try to upload a text file to customers (should fail - not matching pattern)
			textFile := "This is just text"
			_, err = fileService.CreateFile(
				ctx,
				"/customers",
				"notes.txt",
				"text/plain",
				int64(len(textFile)),
				io.NopCloser(strings.NewReader(textFile)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not match any allowed pattern"))
			GinkgoWriter.Println("   ✓ Correctly rejected non-matching file (notes.txt)")

			By("Step 10: Deleting files and cleanup")

			// Delete a customer
			err = fileService.DeleteFile(ctx, "/customers/bob.json")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("   ✓ Deleted bob.json")

			// Verify deletion
			_, bobFiles, _, err := dirService.ListDirectory("/customers", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(bobFiles).To(HaveLen(2)) // alice.json, .files

			// Cleanup entire structure
			err = dirService.DeleteDirectory(ctx, "/customers", true)
			Expect(err).NotTo(HaveOccurred())

			err = dirService.DeleteDirectory(ctx, "/products", true)
			Expect(err).NotTo(HaveOccurred())

			err = dirService.DeleteDirectory(ctx, "/schemas", true)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Println("   ✓ Cleaned up all test directories")

			By("Test completed successfully! ✅")
			GinkgoWriter.Println("\n🎉 Complete end-to-end workflow with nested schema validation passed!")
		})
	})
})
