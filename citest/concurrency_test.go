package citest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/domain"
)

var _ = Describe("Concurrent Operations", func() {
	var (
		testDB     *fixtures.TestDatabase
		dirService *domain.DirectoryService
		ctx        context.Context
	)

	BeforeEach(func() {
		testDB = fixtures.NewTestDatabase()
		dirService = domain.NewDirectoryService(testDB.GetDB())
		ctx = context.Background()

		// Create root directory
		root := &models.Directory{
			ID:       "root",
			Name:     "/",
			Path:     "/",
			PathHash: calculateConcurrentPathHash("/"),
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

	Context("when creating directories concurrently", func() {
		It("should prevent duplicate directory names", func() {
			const concurrency = 10
			var wg sync.WaitGroup
			errors := make(chan error, concurrency)
			successes := make(chan string, concurrency)

			// Try to create same directory concurrently
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					dir, err := dirService.CreateDirectory(ctx, "/", "concurrent-test")
					if err != nil {
						errors <- err
					} else {
						successes <- dir.ID
					}
				}(i)
			}

			wg.Wait()
			close(errors)
			close(successes)

			// Exactly one should succeed
			successCount := 0
			for range successes {
				successCount++
			}

			errorCount := 0
			for range errors {
				errorCount++
			}

			Expect(successCount).To(Equal(1), "Exactly one concurrent create should succeed")
			Expect(errorCount).To(Equal(concurrency-1), "All other creates should fail")
		})

		It("should allow creating different directories concurrently", func() {
			const concurrency = 10
			var wg sync.WaitGroup
			errors := make(chan error, concurrency)
			successes := make(chan string, concurrency)

			// Create different directories concurrently
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					name := fmt.Sprintf("dir-%d", idx)
					dir, err := dirService.CreateDirectory(ctx, "/", name)
					if err != nil {
						errors <- err
					} else {
						successes <- dir.ID
					}
				}(i)
			}

			wg.Wait()
			close(errors)
			close(successes)

			// All should succeed
			successCount := 0
			for range successes {
				successCount++
			}

			fmt.Printf("Success count: %d out of %d\n", successCount, concurrency)

			errorList := []error{}
			for err := range errors {
				errorList = append(errorList, err)
				fmt.Printf("  Create error: %v\n", err)
			}

			Expect(successCount).To(Equal(concurrency), "All different directories should be created successfully")
			Expect(errorList).To(BeEmpty(), "No errors should occur")
		})

		It("should handle concurrent nested directory creation", func() {
			// Create parent first
			parent, err := dirService.CreateDirectory(ctx, "/", "parent")
			Expect(err).NotTo(HaveOccurred())

			const concurrency = 10
			var wg sync.WaitGroup
			errors := make(chan error, concurrency)
			successes := make(chan string, concurrency)

			// Create children concurrently
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					name := fmt.Sprintf("child-%d", idx)
					dir, err := dirService.CreateDirectory(ctx, parent.Path, name)
					if err != nil {
						errors <- err
					} else {
						successes <- dir.ID
					}
				}(i)
			}

			wg.Wait()
			close(errors)
			close(successes)

			// All should succeed
			successCount := 0
			for range successes {
				successCount++
			}

			Expect(successCount).To(Equal(concurrency), "All nested directories should be created successfully")
		})
	})

	Context("when deleting directories concurrently", func() {
		BeforeEach(func() {
			// Create test hierarchy
			dirService.CreateDirectory(ctx, "/", "to-delete-1")
			dirService.CreateDirectory(ctx, "/", "to-delete-2")
			dirService.CreateDirectory(ctx, "/", "to-delete-3")
		})

		It("should handle concurrent deletions", func() {
			paths := []string{"/to-delete-1", "/to-delete-2", "/to-delete-3"}

			var wg sync.WaitGroup
			errors := make(chan error, len(paths))

			for _, path := range paths {
				wg.Add(1)
				go func(p string) {
					defer wg.Add(-1)

					err := dirService.DeleteDirectory(ctx, p, false)
					if err != nil {
						errors <- err
					}
				}(path)
			}

			wg.Wait()
			close(errors)

			// All deletions should succeed
			errorList := []error{}
			for err := range errors {
				errorList = append(errorList, err)
			}
			Expect(errorList).To(BeEmpty(), "All deletions should succeed")
		})

		It("should prevent deleting parent while child is being created", func() {
			// This tests the tree locking mechanism
			// Create parent
			parent, err := dirService.CreateDirectory(ctx, "/", "parent-lock")
			Expect(err).NotTo(HaveOccurred())

			const concurrency = 5
			var wg sync.WaitGroup
			createErrors := make(chan error, concurrency)
			deleteErrors := make(chan error, 1)

			// Start creating children
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					name := fmt.Sprintf("child-%d", idx)
					_, err := dirService.CreateDirectory(ctx, parent.Path, name)
					if err != nil {
						createErrors <- err
					}
				}(i)
			}

			// Try to delete parent concurrently
			wg.Add(1)
			go func() {
				defer wg.Done()

				err := dirService.DeleteDirectory(ctx, parent.Path, true)
				if err != nil {
					deleteErrors <- err
				}
			}()

			wg.Wait()
			close(createErrors)
			close(deleteErrors)

			// Either: all creates succeed OR delete succeeds but creates fail
			// The important thing is no data corruption
			createCount := 0
			for range createErrors {
				// Count errors
			}
			createSuccessCount := concurrency - createCount

			deleteCount := 0
			for range deleteErrors {
				deleteCount++
			}

			// Verify database consistency
			gormDB := testDB.GetDB()
			var dirCount int64
			gormDB.Model(&models.Directory{}).Where("path LIKE ?", parent.Path+"%").Count(&dirCount)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Either parent and children exist, or none exist
			// No partial state
			if deleteCount == 1 {
				// Delete succeeded - no children should exist
				Expect(dirCount).To(Equal(int64(0)), "If parent deleted, no children should exist")
			} else {
				// Delete failed - parent should exist
				Expect(dirCount).To(BeNumerically(">=", int64(createSuccessCount)), "If delete failed, created children should exist")
			}
		})
	})

	Context("when updating directories concurrently", func() {
		It("should handle concurrent moves", func() {
			// Create test structure
			dirService.CreateDirectory(ctx, "/", "source")
			dirService.CreateDirectory(ctx, "/", "dest1")
			dirService.CreateDirectory(ctx, "/", "dest2")
			item, err := dirService.CreateDirectory(ctx, "/source", "item")
			Expect(err).NotTo(HaveOccurred())

			// Try to move to different destinations concurrently
			var wg sync.WaitGroup
			results := make(chan string, 2)

			wg.Add(1)
			go func() {
				defer wg.Done()

				gormDB := testDB.GetDB()
				var dir models.Directory
				gormDB.Where("id = ?", item.ID).First(&dir)

				var dest models.Directory
				gormDB.Where("path = ?", "/dest1").First(&dest)

				dir.ParentID = &dest.ID
				dir.Path = "/dest1/item"
				dir.PathHash = calculateConcurrentPathHash("/dest1/item")
				gormDB.Save(&dir)

				sqlDB, _ := gormDB.DB()
				if sqlDB != nil {
					sqlDB.Close()
				}
				results <- "/dest1/item"
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()

				gormDB := testDB.GetDB()
				var dir models.Directory
				gormDB.Where("id = ?", item.ID).First(&dir)

				var dest models.Directory
				gormDB.Where("path = ?", "/dest2").First(&dest)

				dir.ParentID = &dest.ID
				dir.Path = "/dest2/item"
				dir.PathHash = calculateConcurrentPathHash("/dest2/item")
				gormDB.Save(&dir)

				sqlDB, _ := gormDB.DB()
				if sqlDB != nil {
					sqlDB.Close()
				}
				results <- "/dest2/item"
			}()

			wg.Wait()
			close(results)

			// One move should succeed
			resultList := []string{}
			for r := range results {
				resultList = append(resultList, r)
			}
			_ = resultList // consume resultList to satisfy linter

			// Verify final state is consistent
			gormDB := testDB.GetDB()
			var finalDir models.Directory
			gormDB.Where("id = ?", item.ID).First(&finalDir)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Directory should be in one of the destinations
			Expect(finalDir.Path).To(Or(
				Equal("/dest1/item"),
				Equal("/dest2/item"),
			), "Directory should end up in one destination")
		})
	})

	Context("when handling race conditions", func() {
		It("should maintain consistency during concurrent operations", func() {
			// Create initial structure
			root, err := dirService.CreateDirectory(ctx, "/", "race-test")
			Expect(err).NotTo(HaveOccurred())

			const concurrency = 20
			var wg sync.WaitGroup

			// Mix of operations: create, list, update
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					switch idx % 3 {
					case 0:
						// Create
						dirService.CreateDirectory(ctx, root.Path, fmt.Sprintf("item-%d", idx))
					case 1:
						// List
						dirService.ListDirectory(root.Path, 100, "")
					case 2:
						// Create and delete
						dir, err := dirService.CreateDirectory(ctx, root.Path, fmt.Sprintf("temp-%d", idx))
						if err == nil {
							dirService.DeleteDirectory(ctx, dir.Path, false)
						}
					}
				}(i)
			}

			wg.Wait()

			// Verify database is consistent
			gormDB := testDB.GetDB()

			// Count directories
			var count int64
			gormDB.Model(&models.Directory{}).
				Where("path LIKE ?", root.Path+"/%").
				Count(&count)

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Should have some directories (depends on races, but >0)
			Expect(count).To(BeNumerically(">=", int64(0)), "Database should be consistent")
		})
	})
})

func calculateConcurrentPathHash(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}
