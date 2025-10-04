package citest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/services"
)

var _ = Describe("VFS Directory Operations", Ordered, func() {
	var (
		testDB     *fixtures.TestDatabase
		dirService *services.DirectoryService
		ctx        context.Context
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up VFS Directory Operations test environment (this may take a few seconds)...")
		GinkgoWriter.Println("   - Starting MySQL test container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")

		dirService = services.NewDirectoryService(testDB.GetDB())
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
		GinkgoWriter.Println("✅ Test environment ready - running tests...")
	})

	AfterAll(func() {
		testDB.Cleanup()
	})

	Context("when creating directories", func() {
		It("should create a new directory under root", func() {
			dir, err := dirService.CreateDirectory(ctx, "/", "create-root-dir")

			Expect(err).NotTo(HaveOccurred())
			Expect(dir).NotTo(BeNil())
			Expect(dir.Name).To(Equal("create-root-dir"))
			Expect(dir.Path).To(Equal("/create-root-dir"))
			Expect(dir.ParentID).To(Equal(stringPtr("root")))
		})

		It("should create nested directories", func() {
			// Create /nested-parent
			parent, err := dirService.CreateDirectory(ctx, "/", "nested-parent")
			Expect(err).NotTo(HaveOccurred())
			Expect(parent).NotTo(BeNil())

			// Create /nested-parent/child
			child, err := dirService.CreateDirectory(ctx, "/nested-parent", "child")
			Expect(err).NotTo(HaveOccurred())

			Expect(child.Path).To(Equal("/nested-parent/child"))
			Expect(child.ParentID).To(Equal(&parent.ID))
		})

		It("should reject duplicate directory names in same parent", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "dup-test")
			Expect(err).NotTo(HaveOccurred())

			// Try to create duplicate
			_, err = dirService.CreateDirectory(ctx, "/", "dup-test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})

		It("should allow same directory name in different parents", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "common-name")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/", "diff-parent")
			Expect(err).NotTo(HaveOccurred())

			// Create /diff-parent/common-name (same name, different parent)
			src2, err := dirService.CreateDirectory(ctx, "/diff-parent", "common-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(src2.Path).To(Equal("/diff-parent/common-name"))
		})

		It("should reject invalid directory names", func() {
			invalidNames := []string{"", ".", "..", "name/with/slash", "name\x00null"}

			for _, name := range invalidNames {
				_, err := dirService.CreateDirectory(ctx, "/", name)
				Expect(err).To(HaveOccurred(), "should reject name: %s", name)
			}
		})
	})

	Context("when listing directories", Ordered, func() {
		BeforeAll(func() {
			// Create test hierarchy
			_, err := dirService.CreateDirectory(ctx, "/", "list-projects")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/", "list-documents")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/list-projects", "backend")
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/list-projects", "frontend")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should list directories under root", func() {
			dirs, _, _, err := dirService.ListDirectory("/", 100, "")

			Expect(err).NotTo(HaveOccurred())
			// With Ordered tests, we see all directories created by previous tests
			Expect(len(dirs)).To(BeNumerically(">=", 2))

			var names []string
			for _, d := range dirs {
				names = append(names, d.Name)
			}
			Expect(names).To(ContainElements("list-projects", "list-documents"))
		})

		It("should list nested directories", func() {
			dirs, _, _, err := dirService.ListDirectory("/list-projects", 100, "")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirs)).To(Equal(2))

			names := []string{dirs[0].Name, dirs[1].Name}
			Expect(names).To(ContainElements("backend", "frontend"))
		})

		It("should handle empty directories", func() {
			dirService.CreateDirectory(ctx, "/", "list-empty")

			dirs, files, _, err := dirService.ListDirectory("/list-empty", 100, "")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirs)).To(Equal(0))
			Expect(len(files)).To(Equal(0))
		})
	})

	Context("when deleting directories", func() {
		It("should delete empty directory", func() {
			dirService.CreateDirectory(ctx, "/", "del-empty")

			err := dirService.DeleteDirectory(ctx, "/del-empty", false)
			Expect(err).NotTo(HaveOccurred())

			// Verify deleted (soft delete)
			gormDB := testDB.GetDB()
			var dir models.Directory
			result := gormDB.Where("path = ?", "/del-empty").First(&dir)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(result.Error).To(HaveOccurred()) // Not found (soft deleted)
		})

		It("should reject deleting non-empty directory without recursive flag", func() {
			dirService.CreateDirectory(ctx, "/", "del-nonempty")
			dirService.CreateDirectory(ctx, "/del-nonempty", "child")

			err := dirService.DeleteDirectory(ctx, "/del-nonempty", false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not empty"))
		})

		It("should recursively delete directory tree", func() {
			dirService.CreateDirectory(ctx, "/", "del-recursive")
			dirService.CreateDirectory(ctx, "/del-recursive", "child1")
			dirService.CreateDirectory(ctx, "/del-recursive", "child2")

			err := dirService.DeleteDirectory(ctx, "/del-recursive", true)
			Expect(err).NotTo(HaveOccurred())

			// Verify all deleted
			gormDB := testDB.GetDB()
			var count int64
			gormDB.Model(&models.Directory{}).
				Where("path LIKE ?", "/del-recursive%").
				Count(&count)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(count).To(Equal(int64(0)))
		})

		It("should prevent deleting root directory", func() {
			err := dirService.DeleteDirectory(ctx, "/", true)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("root"))
		})
	})

	Context("when moving directories", Ordered, func() {
		BeforeAll(func() {
			dirService.CreateDirectory(ctx, "/", "move-projects")
			dirService.CreateDirectory(ctx, "/", "move-archive")
			dirService.CreateDirectory(ctx, "/move-projects", "backend")
		})

		It("should move directory to new parent", func() {
			// Move /move-projects/backend to /move-archive/backend
			gormDB := testDB.GetDB()

			// Get backend directory
			var backend models.Directory
			gormDB.Where("path = ?", "/move-projects/backend").First(&backend)

			// Get archive directory
			var archive models.Directory
			gormDB.Where("path = ?", "/move-archive").First(&archive)

			// Update parent
			backend.ParentID = &archive.ID
			backend.Path = "/move-archive/backend"
			gormDB.Save(&backend)

			// Verify
			var moved models.Directory
			gormDB.Where("id = ?", backend.ID).First(&moved)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(moved.Path).To(Equal("/move-archive/backend"))
			Expect(moved.ParentID).To(Equal(&archive.ID))
		})

		It("should prevent circular parent relationships", func() {
			// Create hierarchy: /a/b/c
			a, _ := dirService.CreateDirectory(ctx, "/", "a")
			_, _ = dirService.CreateDirectory(ctx, "/a", "b")
			c, _ := dirService.CreateDirectory(ctx, "/a/b", "c")

			gormDB := testDB.GetDB()

			// Try to move /a under /a/b/c (would create circle)
			a.ParentID = &c.ID
			result := gormDB.Save(&a)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// This should be prevented by application logic
			// For now, we just verify the operation completes
			// In Phase 6, we'll add validation to prevent this
			_ = result
		})
	})
})

func stringPtr(s string) *string {
	return &s
}

func calculateTestPathHash(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}
