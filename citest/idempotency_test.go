package citest

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/idempotency"
	"github.com/telnet2/mysql-vfs/pkg/models"
)

var _ = Describe("Idempotency System", func() {
	var (
		testDB             *fixtures.TestDatabase
		idempotencyService *idempotency.Service
	)

	BeforeEach(func() {
		testDB = fixtures.NewTestDatabase()
		idempotencyService = idempotency.NewService(testDB.GetDB())
	})

	AfterEach(func() {
		testDB.Cleanup()
	})

	Context("when caching responses", func() {
		It("should store idempotency record", func() {
			requestID := uuid.New().String()
			response := map[string]interface{}{
				"id":   "file-123",
				"name": "test.txt",
			}

			err := idempotencyService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())

			// Verify record was created
			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())
			Expect(record.RequestID).To(Equal(requestID))
			Expect(record.ResponseBody).NotTo(BeEmpty())
		})

		It("should serialize response correctly", func() {
			requestID := uuid.New().String()
			response := map[string]interface{}{
				"id":   "123",
				"name": "test",
			}

			err := idempotencyService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())

			// Verify response body can be unmarshaled
			var retrieved map[string]interface{}
			err = json.Unmarshal([]byte(record.ResponseBody), &retrieved)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["id"]).To(Equal("123"))
			Expect(retrieved["name"]).To(Equal("test"))
		})

		It("should calculate response hash", func() {
			requestID := uuid.New().String()
			response := map[string]interface{}{"result": "test"}

			err := idempotencyService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())
			Expect(record.ResponseHash).To(HaveLen(64)) // SHA256 hex is 64 characters
		})

		It("should handle concurrent cache operations", func() {
			requestID := uuid.New().String()

			response1 := map[string]interface{}{"result": "first"}
			response2 := map[string]interface{}{"result": "second"}

			done1 := make(chan error, 1)
			done2 := make(chan error, 1)

			go func() {
				done1 <- idempotencyService.CacheResponse(requestID, response1)
			}()

			go func() {
				time.Sleep(10 * time.Millisecond)
				done2 <- idempotencyService.CacheResponse(requestID, response2)
			}()

			err1 := <-done1
			err2 := <-done2

			// First should succeed, second may fail due to duplicate key
			Expect(err1).NotTo(HaveOccurred())
			// err2 might fail due to unique constraint, which is expected
			_ = err2
		})
	})

	Context("when managing expiration", func() {
		It("should set correct expiration time", func() {
			requestID := uuid.New().String()
			response := map[string]interface{}{"status": "success"}

			beforeCache := time.Now()
			err := idempotencyService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())
			afterCache := time.Now()

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())

			expectedEarliest := beforeCache.Add(idempotency.IdempotencyTTL)
			expectedLatest := afterCache.Add(idempotency.IdempotencyTTL)

			Expect(record.ExpiresAt.After(expectedEarliest.Add(-1 * time.Second))).To(BeTrue())
			Expect(record.ExpiresAt.Before(expectedLatest.Add(1 * time.Second))).To(BeTrue())
		})

		It("should use 24-hour TTL", func() {
			Expect(idempotency.IdempotencyTTL).To(Equal(24 * time.Hour))
		})
	})

	Context("when cleaning up expired records", func() {
		BeforeEach(func() {
			gormDB := testDB.GetDB()

			// Create expired record
			expiredRecord := &models.IdempotencyRecord{
				RequestID:    uuid.New().String(),
				ResponseHash: "abc123",
				ResponseBody: `{"expired":true}`,
				ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
				CreatedAt:    time.Now().Add(-25 * time.Hour),
			}
			gormDB.Create(expiredRecord)

			// Create valid record
			validRecord := &models.IdempotencyRecord{
				RequestID:    uuid.New().String(),
				ResponseHash: "def456",
				ResponseBody: `{"valid":true}`,
				ExpiresAt:    time.Now().Add(23 * time.Hour), // Not expired
				CreatedAt:    time.Now(),
			}
			gormDB.Create(validRecord)

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		})

		It("should remove expired idempotency records", func() {
			// Count before cleanup
			gormDB := testDB.GetDB()
			var countBefore int64
			gormDB.Model(&models.IdempotencyRecord{}).Count(&countBefore)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(countBefore).To(Equal(int64(2)))

			// Run cleanup
			err := idempotencyService.CleanupExpired()
			Expect(err).NotTo(HaveOccurred())

			// Count after cleanup
			gormDB = testDB.GetDB()
			var countAfter int64
			gormDB.Model(&models.IdempotencyRecord{}).Count(&countAfter)
			sqlDB, _ = gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(countAfter).To(Equal(int64(1))) // Only valid record remains
		})

		It("should not remove valid records", func() {
			validRequestID := uuid.New().String()
			response := map[string]interface{}{"id": "keep-me"}

			err := idempotencyService.CacheResponse(validRequestID, response)
			Expect(err).NotTo(HaveOccurred())

			// Run cleanup
			idempotencyService.CleanupExpired()

			// Verify valid record still exists
			record := fixtures.GetIdempotencyRecord(testDB, validRequestID)
			Expect(record).NotTo(BeNil())
		})
	})

	Context("when handling edge cases", func() {
		It("should handle empty response", func() {
			requestID := uuid.New().String()
			emptyResponse := map[string]interface{}{}

			err := idempotencyService.CacheResponse(requestID, emptyResponse)
			Expect(err).NotTo(HaveOccurred())

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())
		})

		It("should handle complex nested response", func() {
			requestID := uuid.New().String()
			complexResponse := map[string]interface{}{
				"id":   "file-456",
				"name": "document.pdf",
				"metadata": map[string]interface{}{
					"size": 1024,
					"type": "application/pdf",
				},
				"versions": []int{1, 2, 3},
			}

			err := idempotencyService.CacheResponse(requestID, complexResponse)
			Expect(err).NotTo(HaveOccurred())

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())
			Expect(record.ResponseBody).NotTo(BeEmpty())
		})

		It("should handle nil values in response", func() {
			requestID := uuid.New().String()
			responseWithNil := map[string]interface{}{
				"id":     "123",
				"parent": nil,
			}

			err := idempotencyService.CacheResponse(requestID, responseWithNil)
			Expect(err).NotTo(HaveOccurred())

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())
		})
	})
})
