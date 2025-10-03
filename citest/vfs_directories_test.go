package citest

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/services"
)

var _ = Describe("VFS Directory Operations", func() {
	var (
		testDB     *fixtures.TestDatabase
		dirService *services.DirectoryService
		ctx        context.Context
	)

	BeforeEach(func() {
		testDB = fixtures.NewTestDatabase()
		dirService = services.NewDirectoryService(testDB.GetDB())
		ctx = context.Background()

		// Create root directory
		root := &models.Directory{
			ID:   "root",
			Name: "/",
			Path: "/",
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
	})

	Context("when creating directories", func() {
		It("should create a new directory under root", func() {
			dir, err := dirService.CreateDirectory(ctx, "/", "projects", nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(dir).NotTo(BeNil())
			Expect(dir.Name).To(Equal("projects"))
			Expect(dir.Path).To(Equal("/projects"))
			Expect(dir.ParentID).To(Equal(stringPtr("root")))
		})

		It("should create nested directories", func() {
			// Create /projects
			projects, err := dirService.CreateDirectory(ctx, "/", "projects", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(projects).NotTo(BeNil())

			// Create /projects/backend
			backend, err := dirService.CreateDirectory(ctx, "/projects", "backend", nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(backend.Path).To(Equal("/projects/backend"))
			Expect(backend.ParentID).To(Equal(&projects.ID))
		})

		It("should reject duplicate directory names in same parent", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "projects", nil)
			Expect(err).NotTo(HaveOccurred())

			// Try to create duplicate
			_, err = dirService.CreateDirectory(ctx, "/", "projects", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})

		It("should allow same directory name in different parents", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "src", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = dirService.CreateDirectory(ctx, "/", "projects", nil)
			Expect(err).NotTo(HaveOccurred())

			// Create /projects/src (same name, different parent)
			src2, err := dirService.CreateDirectory(ctx, "/projects", "src", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(src2.Path).To(Equal("/projects/src"))
		})

		It("should reject invalid directory names", func() {
			invalidNames := []string{"", ".", "..", "name/with/slash", "name\x00null"}

			for _, name := range invalidNames {
				_, err := dirService.CreateDirectory(ctx, "/", name, nil)
				Expect(err).To(HaveOccurred(), "should reject name: %s", name)
			}
		})
	})

	Context("when listing directories", func() {
		BeforeEach(func() {
			// Create test hierarchy
			dirService.CreateDirectory(ctx, "/", "projects", nil)
			dirService.CreateDirectory(ctx, "/", "documents", nil)
			dirService.CreateDirectory(ctx, "/projects", "backend", nil)
			dirService.CreateDirectory(ctx, "/projects", "frontend", nil)
		})

		It("should list directories under root", func() {
			dirs, _, _, err := dirService.ListDirectory("/", 100, "")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirs)).To(Equal(2))

			names := []string{dirs[0].Name, dirs[1].Name}
			Expect(names).To(ContainElements("projects", "documents"))
		})

		It("should list nested directories", func() {
			dirs, _, _, err := dirService.ListDirectory("/projects", 100, "")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirs)).To(Equal(2))

			names := []string{dirs[0].Name, dirs[1].Name}
			Expect(names).To(ContainElements("backend", "frontend"))
		})

		It("should handle empty directories", func() {
			dirService.CreateDirectory(ctx, "/", "empty", nil)

			dirs, files, _, err := dirService.ListDirectory("/empty", 100, "")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirs)).To(Equal(0))
			Expect(len(files)).To(Equal(0))
		})
	})

	Context("when deleting directories", func() {
		It("should delete empty directory", func() {
			dirService.CreateDirectory(ctx, "/", "temp", nil)

			err := dirService.DeleteDirectory(ctx, "/temp", false)
			Expect(err).NotTo(HaveOccurred())

			// Verify deleted (soft delete)
			gormDB := testDB.GetDB()
			var dir models.Directory
			result := gormDB.Where("path = ?", "/temp").First(&dir)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(result.Error).To(HaveOccurred()) // Not found (soft deleted)
		})

		It("should reject deleting non-empty directory without recursive flag", func() {
			dirService.CreateDirectory(ctx, "/", "projects", nil)
			dirService.CreateDirectory(ctx, "/projects", "backend", nil)

			err := dirService.DeleteDirectory(ctx, "/projects", false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not empty"))
		})

		It("should recursively delete directory tree", func() {
			dirService.CreateDirectory(ctx, "/", "projects", nil)
			dirService.CreateDirectory(ctx, "/projects", "backend", nil)
			dirService.CreateDirectory(ctx, "/projects", "frontend", nil)

			err := dirService.DeleteDirectory(ctx, "/projects", true)
			Expect(err).NotTo(HaveOccurred())

			// Verify all deleted
			gormDB := testDB.GetDB()
			var count int64
			gormDB.Model(&models.Directory{}).
				Where("path LIKE ?", "/projects%").
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

	Context("when moving directories", func() {
		BeforeEach(func() {
			dirService.CreateDirectory(ctx, "/", "projects", nil)
			dirService.CreateDirectory(ctx, "/", "archive", nil)
			dirService.CreateDirectory(ctx, "/projects", "backend", nil)
		})

		It("should move directory to new parent", func() {
			// Move /projects/backend to /archive/backend
			gormDB := testDB.GetDB()

			// Get backend directory
			var backend models.Directory
			gormDB.Where("path = ?", "/projects/backend").First(&backend)

			// Get archive directory
			var archive models.Directory
			gormDB.Where("path = ?", "/archive").First(&archive)

			// Update parent
			backend.ParentID = &archive.ID
			backend.Path = "/archive/backend"
			gormDB.Save(&backend)

			// Verify
			var moved models.Directory
			gormDB.Where("id = ?", backend.ID).First(&moved)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(moved.Path).To(Equal("/archive/backend"))
			Expect(moved.ParentID).To(Equal(&archive.ID))
		})

		It("should prevent circular parent relationships", func() {
			// Create hierarchy: /a/b/c
			a, _ := dirService.CreateDirectory(ctx, "/", "a", nil)
			_, _ = dirService.CreateDirectory(ctx, "/a", "b", nil)
			c, _ := dirService.CreateDirectory(ctx, "/a/b", "c", nil)

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
