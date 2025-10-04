package citest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/services"
)

var _ = Describe("End-to-End VFS Workflow", Ordered, func() {
	var (
		testDB      *fixtures.TestDatabase
		testS3      *fixtures.TestS3
		dirService  *services.DirectoryService
		fileService *services.FileService
		ctx         context.Context
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up End-to-End VFS Workflow test environment (this may take a few seconds)...")
		GinkgoWriter.Println("   - Starting MySQL test container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")

		GinkgoWriter.Println("   - Starting S3 test storage...")
		testS3 = fixtures.NewTestS3()
		GinkgoWriter.Println("   ✓ S3 ready")

		dirService = services.NewDirectoryService(testDB.GetDB())
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
		GinkgoWriter.Println("✅ Test environment ready - running tests...")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testS3.Cleanup()
	})

	Describe("Complete VFS Workflow", func() {
		It("should handle a complete workflow: create, update, move, and delete operations", func() {
			By("Creating nested directory structure: /projects/app/src and /projects/app/docs")

			// Create /projects
			projectsDir, err := dirService.CreateDirectory(ctx, "/", "projects", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(projectsDir.Path).To(Equal("/projects"))

			// Create /projects/app
			appDir, err := dirService.CreateDirectory(ctx, "/projects", "app", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(appDir.Path).To(Equal("/projects/app"))

			// Create /projects/app/src
			srcDir, err := dirService.CreateDirectory(ctx, "/projects/app", "src", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(srcDir.Path).To(Equal("/projects/app/src"))

			// Create /projects/app/docs
			docsDir, err := dirService.CreateDirectory(ctx, "/projects/app", "docs", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(docsDir.Path).To(Equal("/projects/app/docs"))

			// Create /projects/app/tests
			testsDir, err := dirService.CreateDirectory(ctx, "/projects/app", "tests", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(testsDir.Path).To(Equal("/projects/app/tests"))

			By("Verifying directory structure")
			dirs, files, _, err := dirService.ListDirectory("/projects/app", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(dirs).To(HaveLen(3))
			Expect(files).To(HaveLen(0))

			By("Creating files in /projects/app/src")

			// Create main.go
			mainGoContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello, VFS!")
}`
			mainGoFile, err := fileService.CreateFile(
				ctx,
				"/projects/app/src",
				"main.go",
				"text/plain",
				int64(len(mainGoContent)),
				io.NopCloser(strings.NewReader(mainGoContent)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mainGoFile.Name).To(Equal("main.go"))
			Expect(mainGoFile.Version).To(Equal(int64(1)))

			// Create config.json
			configContent := `{
	"app_name": "cc-vfs-app",
	"version": "1.0.0",
	"port": 8080
}`
			configFile, err := fileService.CreateFile(
				ctx,
				"/projects/app/src",
				"config.json",
				"application/json",
				int64(len(configContent)),
				io.NopCloser(strings.NewReader(configContent)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(configFile.Name).To(Equal("config.json"))

			// Create utils.go
			utilsContent := `package main

func Add(a, b int) int {
	return a + b
}`
			utilsFile, err := fileService.CreateFile(
				ctx,
				"/projects/app/src",
				"utils.go",
				"text/plain",
				int64(len(utilsContent)),
				io.NopCloser(strings.NewReader(utilsContent)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(utilsFile.Name).To(Equal("utils.go"))

			By("Creating files in /projects/app/docs")

			readmeContent := `# VFS Application

This is a sample application for testing the VFS system.

## Features
- File management
- Directory operations
- Version control`
			readmeFile, err := fileService.CreateFile(
				ctx,
				"/projects/app/docs",
				"README.md",
				"text/markdown",
				int64(len(readmeContent)),
				io.NopCloser(strings.NewReader(readmeContent)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(readmeFile.Name).To(Equal("README.md"))

			apiDocsContent := `# API Documentation

## Endpoints
- GET /api/files
- POST /api/files
- DELETE /api/files/:id`
			apiDocsFile, err := fileService.CreateFile(
				ctx,
				"/projects/app/docs",
				"api.md",
				"text/markdown",
				int64(len(apiDocsContent)),
				io.NopCloser(strings.NewReader(apiDocsContent)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(apiDocsFile.Name).To(Equal("api.md"))

			By("Verifying files in /projects/app/src")
			_, srcFiles, _, err := dirService.ListDirectory("/projects/app/src", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(srcFiles).To(HaveLen(3))

			By("Updating config.json with new version")
			updatedConfigContent := `{
	"app_name": "cc-vfs-app",
	"version": "2.0.0",
	"port": 8080,
	"debug": true
}`
			updatedConfigFile, err := fileService.UpdateFile(
				ctx,
				"/projects/app/src/config.json",
				"application/json",
				int64(len(updatedConfigContent)),
				io.NopCloser(strings.NewReader(updatedConfigContent)),
				1, // Expected version
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedConfigFile.Version).To(Equal(int64(2)))

			// Verify the content was updated
			retrievedFile, reader, err := fileService.GetFile(ctx, "/projects/app/src/config.json")
			Expect(err).NotTo(HaveOccurred())
			Expect(retrievedFile.Version).To(Equal(int64(2)))
			content, err := io.ReadAll(reader)
			reader.Close()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("2.0.0"))
			Expect(string(content)).To(ContainSubstring("debug"))

			By("Updating main.go multiple times to test versioning")
			for i := 2; i <= 5; i++ {
				updatedMainContent := mainGoContent + "\n// Version " + string(rune('0'+i))
				_, err := fileService.UpdateFile(
					ctx,
					"/projects/app/src/main.go",
					"text/plain",
					int64(len(updatedMainContent)),
					io.NopCloser(strings.NewReader(updatedMainContent)),
					int64(i-1),
				)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify final version
			retrievedMainFile, mainReader, err := fileService.GetFile(ctx, "/projects/app/src/main.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(retrievedMainFile.Version).To(Equal(int64(5)))
			mainReader.Close() // Close the reader to prevent resource leak

			By("Moving utils.go to /projects/app/tests")
			movedFile, err := fileService.MoveFile(
				ctx,
				"/projects/app/src/utils.go",
				"/projects/app/tests/utils.go",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(movedFile.Name).To(Equal("utils.go"))

			// Verify it's no longer in src
			_, srcFilesAfterMove, _, err := dirService.ListDirectory("/projects/app/src", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(srcFilesAfterMove).To(HaveLen(2))

			// Verify it's in tests
			_, testsFiles, _, err := dirService.ListDirectory("/projects/app/tests", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(testsFiles).To(HaveLen(1))
			Expect(testsFiles[0].Name).To(Equal("utils.go"))

			By("Deleting api.md from docs")
			err = fileService.DeleteFile(ctx, "/projects/app/docs/api.md")
			Expect(err).NotTo(HaveOccurred())

			// Verify it's deleted
			_, docsFiles, _, err := dirService.ListDirectory("/projects/app/docs", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(docsFiles).To(HaveLen(1))
			Expect(docsFiles[0].Name).To(Equal("README.md"))

			// Verify we can't access the deleted file
			_, deletedReader, err := fileService.GetFile(ctx, "/projects/app/docs/api.md")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			if deletedReader != nil {
				deletedReader.Close()
			}

			By("Creating more files in tests directory")
			testFileContent := `package main

import "testing"

func TestAdd(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}
}`
			_, err = fileService.CreateFile(
				ctx,
				"/projects/app/tests",
				"utils_test.go",
				"text/plain",
				int64(len(testFileContent)),
				io.NopCloser(strings.NewReader(testFileContent)),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Attempting to delete non-empty directory /projects/app/src without recursive flag")
			err = dirService.DeleteDirectory(ctx, "/projects/app/src", false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not empty"))

			By("Deleting /projects/app/src directory recursively")
			err = dirService.DeleteDirectory(ctx, "/projects/app/src", true)
			Expect(err).NotTo(HaveOccurred())

			// Verify directory is deleted
			_, _, _, err = dirService.ListDirectory("/projects/app/src", 100, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// Verify files in that directory are also deleted
			_, deletedMainReader, err := fileService.GetFile(ctx, "/projects/app/src/main.go")
			Expect(err).To(HaveOccurred())
			if deletedMainReader != nil {
				deletedMainReader.Close()
			}

			By("Deleting empty docs directory (after removing its files)")
			err = fileService.DeleteFile(ctx, "/projects/app/docs/README.md")
			Expect(err).NotTo(HaveOccurred())

			err = dirService.DeleteDirectory(ctx, "/projects/app/docs", false)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying final state of /projects/app")
			appDirs, appFiles, _, err := dirService.ListDirectory("/projects/app", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(appDirs).To(HaveLen(1)) // Only tests directory remains
			Expect(appDirs[0].Name).To(Equal("tests"))
			Expect(appFiles).To(HaveLen(0))

			// Verify tests directory still has its files
			_, finalTestsFiles, _, err := dirService.ListDirectory("/projects/app/tests", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(finalTestsFiles).To(HaveLen(2)) // utils.go and utils_test.go

			By("Testing concurrent operations tolerance")
			// Create some files concurrently
			done := make(chan bool, 3)
			for i := 1; i <= 3; i++ {
				go func(idx int) {
					content := "Concurrent file " + string(rune('0'+idx))
					_, err := fileService.CreateFile(
						ctx,
						"/projects/app/tests",
						"concurrent_"+string(rune('0'+idx))+".txt",
						"text/plain",
						int64(len(content)),
						io.NopCloser(strings.NewReader(content)),
					)
					Expect(err).NotTo(HaveOccurred())
					done <- true
				}(i)
			}

			// Wait for all goroutines
			for i := 0; i < 3; i++ {
				<-done
			}

			// Verify all concurrent files were created
			_, concurrentFiles, _, err := dirService.ListDirectory("/projects/app/tests", 100, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(concurrentFiles).To(HaveLen(5)) // 2 original + 3 concurrent

			By("Cleaning up entire /projects directory recursively")
			err = dirService.DeleteDirectory(ctx, "/projects", true)
			Expect(err).NotTo(HaveOccurred())

			// Verify projects directory is deleted
			_, _, _, err = dirService.ListDirectory("/projects", 100, "")
			Expect(err).To(HaveOccurred())

			// Verify root is clean (only root directory itself should remain)
			rootDirs, rootFiles, _, err := dirService.ListDirectory("/", 100, "")
			Expect(err).NotTo(HaveOccurred())
			// Root directory has no subdirectories (except potentially the root itself in the listing)
			// Filter out the root directory itself if it appears
			nonRootDirs := []models.Directory{}
			for _, dir := range rootDirs {
				if dir.Path != "/" {
					nonRootDirs = append(nonRootDirs, dir)
				}
			}
			Expect(nonRootDirs).To(HaveLen(0))
			Expect(rootFiles).To(HaveLen(0))
		})
	})

	Describe("Complex File Operations", func() {
		It("should handle large file uploads and modifications", func() {
			By("Creating a directory for large files")
			dataDir, err := dirService.CreateDirectory(ctx, "/", "data", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(dataDir.Path).To(Equal("/data"))

			By("Creating a large file (1MB)")
			largeContent := strings.Repeat("Large data content - ", 50000) // ~1MB
			largeFile, err := fileService.CreateFile(
				ctx,
				"/data",
				"large.txt",
				"text/plain",
				int64(len(largeContent)),
				io.NopCloser(strings.NewReader(largeContent)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(largeFile.SizeBytes).To(BeNumerically(">", 1000000))
			Expect(string(largeFile.StorageType)).To(Equal("s3")) // Large files go to S3

			By("Retrieving and verifying large file content")
			retrievedFile, reader, err := fileService.GetFile(ctx, "/data/large.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(retrievedFile.SizeBytes).To(Equal(largeFile.SizeBytes))

			retrievedContent, err := io.ReadAll(reader)
			reader.Close()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(retrievedContent)).To(Equal(len(largeContent)))

			// Verify checksum
			hash := sha256.Sum256([]byte(largeContent))
			expectedChecksum := hex.EncodeToString(hash[:])
			Expect(retrievedFile.ChecksumSHA256).To(Equal(expectedChecksum))

			By("Updating large file")
			updatedContent := strings.Repeat("Updated large data - ", 50000)
			updatedFile, err := fileService.UpdateFile(
				ctx,
				"/data/large.txt",
				"text/plain",
				int64(len(updatedContent)),
				io.NopCloser(strings.NewReader(updatedContent)),
				1,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedFile.Version).To(Equal(int64(2)))

			By("Cleaning up")
			err = dirService.DeleteDirectory(ctx, "/data", true)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
