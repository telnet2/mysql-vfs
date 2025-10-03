package citest

import (
	"context"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/services"
)

var _ = Describe("VFS File Operations", func() {
	var (
		testDB      *fixtures.TestDatabase
		testS3      *fixtures.TestS3
		dirService  *services.DirectoryService
		fileService *services.FileService
		ctx         context.Context
	)

	BeforeEach(func() {
		testDB = fixtures.NewTestDatabase()
		testS3 = fixtures.NewTestS3()
		dirService = services.NewDirectoryService(testDB.GetDB())

		// Use in-memory storage
		fileService = services.NewFileService(testDB.GetDB(), testS3.Storage)
		ctx = context.Background()

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
	})

	AfterEach(func() {
		testDB.Cleanup()
		testS3.Cleanup()
	})

	Context("when creating files", func() {
		It("should create a new file in root directory", func() {
			content := "Hello, World!"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"hello.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(file).NotTo(BeNil())
			Expect(file.Name).To(Equal("hello.txt"))
			Expect(file.SizeBytes).To(Equal(int64(len(content))))
			Expect(file.Version).To(Equal(int64(1)))
			Expect(file.StorageType).To(Equal("s3"))
			Expect(file.ChecksumSHA256).NotTo(BeEmpty())
		})

		It("should store file content in S3", func() {
			content := "Test content for S3"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"test.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(file.S3Key).NotTo(BeEmpty())

			// Verify S3 key was stored
			Expect(file.S3Key).To(HavePrefix("files/"))
		})

		It("should reject duplicate file names in same directory", func() {
			content := "Content"
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"duplicate.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Try to create duplicate
			_, err = fileService.CreateFile(
				ctx,
				"/",
				"duplicate.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})

		It("should allow same filename in different directories", func() {
			content := "Content"

			// Create file in root
			_, err := fileService.CreateFile(
				ctx,
				"/",
				"readme.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create directory
			dirService.CreateDirectory(ctx, "/", "docs", nil)

			// Create file with same name in /docs
			file2, err := fileService.CreateFile(
				ctx,
				"/docs",
				"readme.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			gormDB := testDB.GetDB()
			var dir models.Directory
			gormDB.Where("path = ?", "/docs").First(&dir)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(file2.DirectoryID).To(Equal(dir.ID))
		})
	})

	Context("when updating files (versioning)", func() {
		var fileID string

		BeforeEach(func() {
			content := "Version 1"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"versioned.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())
			fileID = file.ID
		})

		It("should create new version when updating file", func() {
			newContent := "Version 2"
			updated, err := fileService.UpdateFile(
				ctx,
				"/versioned.txt",
				"text/plain",
				int64(len(newContent)),
				io.NopCloser(strings.NewReader(newContent)),
				1, // expected version
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Version).To(Equal(int64(2)))
			Expect(updated.SizeBytes).To(Equal(int64(len(newContent))))
		})

		It("should reject update with wrong expected version (optimistic locking)", func() {
			newContent := "Version 2"
			_, err := fileService.UpdateFile(
				ctx,
				"/versioned.txt",
				"text/plain",
				int64(len(newContent)),
				io.NopCloser(strings.NewReader(newContent)),
				999, // wrong expected version
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("version mismatch"))
		})

		It("should maintain version history", func() {
			// Create multiple versions
			for i := 2; i <= 5; i++ {
				content := "Version " + string(rune('0'+i))
				_, err := fileService.UpdateFile(
					ctx,
					"/versioned.txt",
					"text/plain",
					int64(len(content)),
					io.NopCloser(strings.NewReader(content)),
					int64(i-1),
				)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify version count
			gormDB := testDB.GetDB()
			var count int64
			gormDB.Model(&models.FileVersion{}).
				Where("file_id = ?", fileID).
				Count(&count)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(count).To(Equal(int64(5)))
		})
	})

	Context("when retrieving files", func() {
		It("should download file content", func() {
			originalContent := "Download test content"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"download.txt",
				"text/plain",
				int64(len(originalContent)),
				io.NopCloser(strings.NewReader(originalContent)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Download file
			_, reader, err := fileService.GetFile(ctx, "/download.txt")
			Expect(err).NotTo(HaveOccurred())
			defer reader.Close()

			downloaded, err := io.ReadAll(reader)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(downloaded)).To(Equal(originalContent))

			_ = file
		})

		It("should return error for non-existent file", func() {
			_, _, err := fileService.GetFile(ctx, "/nonexistent.txt")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("when deleting files", func() {
		It("should soft delete file", func() {
			content := "To be deleted"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"delete-me.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Delete file
			err = fileService.DeleteFile(ctx, "/delete-me.txt")
			Expect(err).NotTo(HaveOccurred())

			// Verify soft deleted
			gormDB := testDB.GetDB()
			var deletedFile models.File
			result := gormDB.Where("id = ?", file.ID).First(&deletedFile)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(result.Error).To(HaveOccurred()) // Not found (soft deleted)
		})

		It("should maintain file versions after delete", func() {
			content := "Versioned delete test"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"versioned-delete.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create version 2
			newContent := "Version 2"
			fileService.UpdateFile(
				ctx,
				"/versioned-delete.txt",
				"text/plain",
				int64(len(newContent)),
				io.NopCloser(strings.NewReader(newContent)),
				1,
			)

			// Delete file
			fileService.DeleteFile(ctx, "/versioned-delete.txt")

			// Verify versions still exist (for audit trail)
			gormDB := testDB.GetDB()
			var count int64
			gormDB.Model(&models.FileVersion{}).
				Where("file_id = ?", file.ID).
				Count(&count)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(count).To(Equal(int64(2)))
		})
	})

	Context("when handling concurrent file updates", func() {
		It("should handle race conditions with optimistic locking", func() {
			content := "Initial content"
			file, err := fileService.CreateFile(
				ctx,
				"/",
				"concurrent.txt",
				"text/plain",
				int64(len(content)),
				io.NopCloser(strings.NewReader(content)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Simulate two concurrent updates with same expected version
			update1 := func() error {
				_, err := fileService.UpdateFile(
					ctx,
					"/concurrent.txt",
					"text/plain",
					10,
					io.NopCloser(strings.NewReader("Update 1")),
					1, // expected version
				)
				return err
			}

			update2 := func() error {
				time.Sleep(50 * time.Millisecond) // Slight delay
				_, err := fileService.UpdateFile(
					ctx,
					"/concurrent.txt",
					"text/plain",
					10,
					io.NopCloser(strings.NewReader("Update 2")),
					1, // same expected version
				)
				return err
			}

			// Run concurrently
			done1 := make(chan error, 1)
			done2 := make(chan error, 1)

			go func() { done1 <- update1() }()
			go func() { done2 <- update2() }()

			err1 := <-done1
			err2 := <-done2

			// One should succeed, one should fail
			success := (err1 == nil && err2 != nil) || (err1 != nil && err2 == nil)
			Expect(success).To(BeTrue(), "One update should succeed, one should fail due to version mismatch")

			_ = file
		})
	})
})
