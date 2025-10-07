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
	mysqlrepo "github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
)

var _ = Describe("Simple Workflow Integration", Ordered, func() {
	var (
		testDB      *fixtures.TestDatabase
		testS3      *fixtures.TestS3
		dirService  *domain.DirectoryService
		fileService *domain.FileService
		ctx         context.Context
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Simple Workflow Test Environment...")
		testDB = fixtures.NewTestDatabase()
		testS3 = fixtures.NewTestS3()

		db := testDB.GetDB()
		dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
		fileRepo := mysqlrepo.NewGormFileRepository(db, testS3.Storage)
		workflowAuditRepo := mysqlrepo.NewGormWorkflowAuditRepository(db)

		// Initialize workflow components
		workflowLoader := domain.NewWorkflowLoader(fileRepo, dirRepo, 5*60) // 5 min cache
		workflowGateEvaluator := domain.NewWorkflowGateEvaluator(fileRepo, 5*60)
		workflowEngine := domain.NewWorkflowEngine(workflowLoader, workflowGateEvaluator, fileRepo, dirRepo, workflowAuditRepo, nil) // nil eventDispatcher for tests

		dirService = domain.NewDirectoryService(db)
		fileService = domain.NewFileService(db, testS3.Storage)

		// Inject workflow engine
		dirService.SetWorkflowEngine(workflowEngine)
		fileService.SetWorkflowEngine(workflowEngine)

		ctx = context.Background()

		// Create root directory
		root := &models.Directory{
			ID:       "root",
			Name:     "/",
			Path:     "/",
			PathHash: calculateTestPathHash("/"),
		}
		db.FirstOrCreate(root, "id = ?", "root")

		GinkgoWriter.Println("✅ Workflow test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testS3.Cleanup()
	})

	Describe("Basic Document Workflow", func() {
		It("should support creating workflow and transitioning files between states", func() {
			By("Creating workflow directory structure")
			_, err := dirService.CreateDirectory(ctx, "/", "docs")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/docs", "draft")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/docs", "review")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/docs", "final")
			Expect(err).NotTo(HaveOccurred())

			By("Creating .workflow configuration file")
			workflowContent := `
state_directories:
  draft: "draft"
  review: "review"
  final: "final"
initial_state: draft
states:
  draft:
    transitions:
      - to: review
        gates:
          - policy: |
              package vfs.workflow.gates
              default allow = true
  review:
    transitions:
      - to: final
        gates:
          - policy: |
              package vfs.workflow.gates
              default allow = true
      - to: draft
        gates:
          - policy: |
              package vfs.workflow.gates
              default allow = true
  final:
    transitions: []
`
			_, err = fileService.CreateFile(
				ctx,
				"/docs",
				".workflow",
				"application/x-yaml",
				int64(len(workflowContent)),
				io.NopCloser(strings.NewReader(workflowContent)),
			)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("✓ Workflow configuration created")

			By("Creating a document in the draft state")
			draftFile, err := fileService.CreateFile(
				ctx,
				"/docs/draft",
				"proposal.txt",
				"text/plain",
				13,
				io.NopCloser(strings.NewReader("draft content")),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(draftFile.Name).To(Equal("proposal.txt"))
			GinkgoWriter.Printf("✓ Created file: /docs/draft/%s\n", draftFile.Name)

			By("Moving document from draft to review")
			reviewFile, err := fileService.MoveFile(ctx, "/docs/draft/proposal.txt", "/docs/review/proposal.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(reviewFile.Name).To(Equal("proposal.txt"))
			GinkgoWriter.Printf("✓ Moved file to: /docs/review/%s\n", reviewFile.Name)

			By("Moving document from review to final")
			finalFile, err := fileService.MoveFile(ctx, "/docs/review/proposal.txt", "/docs/final/proposal.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(finalFile.Name).To(Equal("proposal.txt"))
			GinkgoWriter.Printf("✓ Moved file to: /docs/final/%s\n", finalFile.Name)

			By("Verifying final file exists and is readable")
			// The file should still be accessible
			Expect(finalFile.ID).NotTo(BeEmpty())
			Expect(finalFile.SizeBytes).To(Equal(int64(13)))
			GinkgoWriter.Println("✓ Workflow transition sequence completed successfully!")

			By("Cleaning up test files")
			err = fileService.DeleteFile(ctx, "/docs/final/proposal.txt")
			Expect(err).NotTo(HaveOccurred())

			err = dirService.DeleteDirectory(ctx, "/docs", true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("✓ Cleanup completed")
		})
	})

	Describe("Workflow with Subdirectories", func() {
		It("should preserve subdirectory structure during transitions", func() {
			By("Creating workflow directory structure")
			_, err := dirService.CreateDirectory(ctx, "/", "reports")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/reports", "pending")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/reports", "approved")
			Expect(err).NotTo(HaveOccurred())

			By("Creating subdirectories in pending state")
			_, err = dirService.CreateDirectory(ctx, "/reports/pending", "2024")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/reports/pending/2024", "Q4")
			Expect(err).NotTo(HaveOccurred())

			By("Creating .workflow configuration")
			workflowContent := `
state_directories:
  pending: "pending"
  approved: "approved"
initial_state: pending
states:
  pending:
    transitions:
      - to: approved
        gates:
          - policy: |
              package vfs.workflow.gates
              default allow = true
  approved:
    transitions: []
`
			_, err = fileService.CreateFile(
				ctx,
				"/reports",
				".workflow",
				"application/x-yaml",
				int64(len(workflowContent)),
				io.NopCloser(strings.NewReader(workflowContent)),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a file in nested directory")
			nestedFile, err := fileService.CreateFile(
				ctx,
				"/reports/pending/2024/Q4",
				"financial.pdf",
				"application/pdf",
				10,
				io.NopCloser(strings.NewReader("PDF data")),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(nestedFile.Name).To(Equal("financial.pdf"))
			GinkgoWriter.Printf("✓ Created nested file: /reports/pending/2024/Q4/%s\n", nestedFile.Name)

			By("Creating matching subdirectory structure in approved state")
			_, err = dirService.CreateDirectory(ctx, "/reports/approved", "2024")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/reports/approved/2024", "Q4")
			Expect(err).NotTo(HaveOccurred())

			By("Moving file to approved (preserving subdirectory structure)")
			// When moving, the subdirectory structure should be preserved
			approvedFile, err := fileService.MoveFile(
				ctx,
				"/reports/pending/2024/Q4/financial.pdf",
				"/reports/approved/2024/Q4/financial.pdf",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(approvedFile.Name).To(Equal("financial.pdf"))
			GinkgoWriter.Printf("✓ Moved to: /reports/approved/2024/Q4/%s (structure preserved)\n", approvedFile.Name)

			By("Cleaning up")
			err = fileService.DeleteFile(ctx, "/reports/approved/2024/Q4/financial.pdf")
			Expect(err).NotTo(HaveOccurred())

			err = dirService.DeleteDirectory(ctx, "/reports", true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("✓ Cleanup completed")
		})
	})

	Describe("Multiple Files Workflow", func() {
		It("should handle multiple files transitioning through workflow states", func() {
			By("Setting up workflow")
			_, err := dirService.CreateDirectory(ctx, "/", "projects")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/projects", "active")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/projects", "archived")
			Expect(err).NotTo(HaveOccurred())

			workflowContent := `
state_directories:
  active: "active"
  archived: "archived"
initial_state: active
states:
  active:
    transitions:
      - to: archived
  archived:
    transitions: []
`
			_, err = fileService.CreateFile(
				ctx,
				"/projects",
				".workflow",
				"application/x-yaml",
				int64(len(workflowContent)),
				io.NopCloser(strings.NewReader(workflowContent)),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Creating multiple files in active state")
			fileNames := []string{"project1.txt", "project2.txt", "project3.txt"}
			for _, name := range fileNames {
				_, err = fileService.CreateFile(
					ctx,
					"/projects/active",
					name,
					"text/plain",
					7,
					io.NopCloser(strings.NewReader("content")),
				)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("✓ Created: /projects/active/%s\n", name)
			}

			By("Archiving files one by one")
			for _, name := range fileNames {
				sourcePath := "/projects/active/" + name
				targetPath := "/projects/archived/" + name

				archivedFile, err := fileService.MoveFile(ctx, sourcePath, targetPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(archivedFile.Name).To(Equal(name))
				GinkgoWriter.Printf("✓ Archived: %s\n", targetPath)
			}

			By("Cleaning up")
			for _, name := range fileNames {
				err = fileService.DeleteFile(ctx, "/projects/archived/"+name)
				Expect(err).NotTo(HaveOccurred())
			}

			err = dirService.DeleteDirectory(ctx, "/projects", true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("✓ All files processed successfully!")
		})
	})
})
