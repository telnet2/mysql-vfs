package citest

import (
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/models"
)

var _ = Describe("Name Validation Integration", Ordered, func() {
	var (
		testDB      *fixtures.TestDatabase
		testS3      *fixtures.TestS3
		dirService  *domain.DirectoryService
		fileService *domain.FileService
		ctx         context.Context
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Name Validation Integration test environment (this may take a few seconds)...")
		GinkgoWriter.Println("   - Starting MySQL test container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")

		GinkgoWriter.Println("   - Starting S3 test storage...")
		testS3 = fixtures.NewTestS3()
		GinkgoWriter.Println("   ✓ S3 ready")

		dirService = domain.NewDirectoryService(testDB.GetDB())
		fileService = domain.NewFileService(testDB.GetDB(), testS3.Storage)
		ctx = context.Background()

		// Create root directory
		root := &models.Directory{
			ID:       "root",
			Name:     "/",
			Path:     "/",
			PathHash: calculateNameValidationPathHash("/"),
		}
		gormDB := testDB.GetDB()
		gormDB.FirstOrCreate(root, "id = ?", "root")
		sqlDB, _ := gormDB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		GinkgoWriter.Println("✅ Test environment ready - running tests...")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testS3.Cleanup()
	})

	Context("file creation with invalid names", func() {
		It("should reject files with invalid names", func() {
			invalidNames := []string{
				"",                     // empty
				".",                    // current directory
				"..",                   // parent directory
				"file with spaces.txt", // spaces
				"file@symbol.txt",      // special character
				"file#hash.txt",        // special character
				"file/dir.txt",         // forward slash
				"file\\dir.txt",        // backslash
				"file\x00null.txt",     // null character
				"file\nline.txt",       // newline
				"file\tab.txt",         // tab
				"file\x7f.txt",         // DEL character
			}

			for _, name := range invalidNames {
				content := "test content"
				_, err := fileService.CreateFile(
					ctx,
					"/",
					name,
					"text/plain",
					int64(len(content)),
					io.NopCloser(strings.NewReader(content)),
				)
				Expect(err).To(HaveOccurred(), "should reject invalid file name: %s", name)
				Expect(err.Error()).To(ContainSubstring("name"), "error should mention name validation")
			}
		})

		It("should accept and normalize valid file names", func() {
			validNames := []struct {
				input    string
				expected string
			}{
				{"testfile.txt", "testfile.txt"},
				{"my-file.json", "my-file.json"},
				{"file123.csv", "file123.csv"},
				{"file_with_underscore.xml", "file_with_underscore.xml"},
				{"file.with.multiple.dots.txt", "file.with.multiple.dots.txt"},
				{"MiXeDcAsE.JSON", "mixedcase.json"},
			}

			for _, test := range validNames {
				content := "test content"
				file, err := fileService.CreateFile(
					ctx,
					"/",
					test.input,
					"text/plain",
					int64(len(content)),
					io.NopCloser(strings.NewReader(content)),
				)
				Expect(err).NotTo(HaveOccurred(), "should accept valid file name: %s", test.input)
				Expect(file.Name).To(Equal(test.expected), "name should be normalized: %s -> %s", test.input, test.expected)
			}
		})
	})

	Context("directory creation with invalid names", func() {
		It("should reject directories with invalid names", func() {
			invalidNames := []string{
				"",                // empty
				".",               // current directory
				"..",              // parent directory
				"dir with spaces", // spaces
				"dir@symbol",      // special character
				"dir#hash",        // special character
				"dir/file",        // forward slash
				"dir\\file",       // backslash
				"dir\x00null",     // null character
				"dir\nline",       // newline
				"dir\tab",         // tab
				"dir\x7f",         // DEL character
			}

			for _, name := range invalidNames {
				_, err := dirService.CreateDirectory(ctx, "/", name)
				Expect(err).To(HaveOccurred(), "should reject invalid directory name: %s", name)
				Expect(err.Error()).To(ContainSubstring("name"), "error should mention name validation")
			}
		})

		It("should accept and normalize valid directory names", func() {
			validNames := []struct {
				input    string
				expected string
			}{
				{"testdir", "testdir"},
				{"my-directory", "my-directory"},
				{"dir123", "dir123"},
				{"dir_with_underscore", "dir_with_underscore"},
				{"dir.with.dots", "dir.with.dots"},
				{"MiXeDcAsE", "mixedcase"},
			}

			for _, test := range validNames {
				dir, err := dirService.CreateDirectory(ctx, "/", test.input)
				Expect(err).NotTo(HaveOccurred(), "should accept valid directory name: %s", test.input)
				Expect(dir.Name).To(Equal(test.expected), "name should be normalized: %s -> %s", test.input, test.expected)
			}
		})
	})

	Context("file move/rename operations", func() {
		BeforeAll(func() {
			// Create a source file for testing moves
			content := "test content for move"
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"source.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject moving file to invalid destination names", func() {
			// Ensure source file exists
			content := "test content for move"
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"source.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			// Ignore error if file already exists
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				Expect(err).NotTo(HaveOccurred())
			}

			invalidNames := []string{
				"file with spaces.txt",
				"file@symbol.txt",
				"file#hash.txt",
				"file\x00null.txt",
				"file\nline.txt",
				"file\tab.txt",
				"file\x7f.txt",
			}

			for _, name := range invalidNames {
				_, err := fileService.MoveFile(ctx, "/source.txt", "/"+name)
				Expect(err).To(HaveOccurred(), "should reject moving to invalid destination name: %s", name)
				Expect(err.Error()).To(ContainSubstring("name"), "error should mention name validation")
			}
		})

		It("should accept moving file to valid destination names", func() {
			// Ensure source file exists
			content := "test content for move"
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"source.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			// Ignore error if file already exists
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				Expect(err).NotTo(HaveOccurred())
			}

			validNames := []string{
				"renamed.txt",
				"my-renamed-file.json",
				"renamed_with_underscore.xml",
				"mixedcase.json",
			}

			for _, name := range validNames {
				expectedName := strings.ToLower(name)

				// Ensure destination doesn't exist
				_ = fileService.DeleteFile(ctx, "/"+expectedName) // Ignore error if file doesn't exist

				// Move to new name
				movedFile, err := fileService.MoveFile(ctx, "/source.txt", "/"+name)
				Expect(err).NotTo(HaveOccurred(), "should accept moving to valid destination name: %s", name)
				Expect(movedFile.Name).To(Equal(expectedName), "destination name should be normalized: %s -> %s", name, expectedName)

				// Move back to source.txt for next test
				_, err = fileService.MoveFile(ctx, "/"+expectedName, "/source.txt")
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

	Context("edge cases and comprehensive validation", func() {
		It("should handle various edge cases", func() {
			// Test that validation is consistent across operations

			// Ensure directory doesn't exist
			_ = dirService.DeleteDirectory(ctx, "/testdir-edgecase", true) // Ignore error if doesn't exist

			// Create a directory first
			_, err := dirService.CreateDirectory(ctx, "/", "testdir-edgecase")
			Expect(err).NotTo(HaveOccurred())

			// Try to create file with same name as directory (should work - different namespaces)
			content := "file content"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"testdir-edgecase", // same name as directory but with no extension
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(file.Name).To(Equal("testdir-edgecase"))

			// Try to create another file with same name (should fail)
			_, err = fileService.CreateFile(
				ctx,
				"/",
				"testdir-edgecase",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).To(HaveOccurred()) // Duplicate name in same directory
		})

		It("should validate names in nested paths", func() {
			// Ensure directory doesn't exist
			_ = dirService.DeleteDirectory(ctx, "/validparent-nested", true)

			// Create a valid directory first
			_, err := dirService.CreateDirectory(ctx, "/", "validparent-nested")
			Expect(err).NotTo(HaveOccurred())

			// Try to create file in valid directory with invalid name
			content := "test"
			_, err = fileService.CreateFile(
				ctx,
				"/validparent-nested",
				"invalid file.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).To(HaveOccurred(), "should reject invalid name even in nested path")
			Expect(err.Error()).To(ContainSubstring("name"))
		})
	})
})

// Helper function for path hash calculation
func calculateNameValidationPathHash(path string) string {
	// This is a simplified version - in real tests, use the actual hash function
	return path
}
