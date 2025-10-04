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

var _ = Describe("Idempotency System", Ordered, func() {
	var (
		testDB             *fixtures.TestDatabase
		idempotencyService *idempotency.Service
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Idempotency System test environment (this may take a few seconds)...")
		GinkgoWriter.Println("   - Starting MySQL test container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")

		idempotencyService = idempotency.NewService(testDB.GetDB())
		GinkgoWriter.Println("✅ Test environment ready - running tests...")
	})

	AfterAll(func() {
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
		It("should set correct expiration time with default TTL", func() {
			requestID := uuid.New().String()
			response := map[string]interface{}{"status": "success"}

			beforeCache := time.Now()
			err := idempotencyService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())
			afterCache := time.Now()

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())

			expectedEarliest := beforeCache.Add(idempotency.DefaultIdempotencyTTL)
			expectedLatest := afterCache.Add(idempotency.DefaultIdempotencyTTL)

			Expect(record.ExpiresAt.After(expectedEarliest.Add(-1 * time.Second))).To(BeTrue())
			Expect(record.ExpiresAt.Before(expectedLatest.Add(1 * time.Second))).To(BeTrue())
		})

		It("should use 24-hour default TTL", func() {
			Expect(idempotency.DefaultIdempotencyTTL).To(Equal(24 * time.Hour))
		})

		It("should support custom TTL for testing", func() {
			customTTL := 500 * time.Millisecond
			testService := idempotency.NewServiceWithTTL(testDB.GetDB(), customTTL)

			requestID := uuid.New().String()
			response := map[string]interface{}{"test": "custom-ttl"}

			beforeCache := time.Now()
			err := testService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())
			afterCache := time.Now()

			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())

			expectedEarliest := beforeCache.Add(customTTL)
			expectedLatest := afterCache.Add(customTTL)

			Expect(record.ExpiresAt.After(expectedEarliest.Add(-10 * time.Millisecond))).To(BeTrue())
			Expect(record.ExpiresAt.Before(expectedLatest.Add(10 * time.Millisecond))).To(BeTrue())
		})

		It("should expire records after TTL with realistic timing (150ms)", func() {
			// Use a short TTL for realistic testing
			shortTTL := 150 * time.Millisecond
			testService := idempotency.NewServiceWithTTL(testDB.GetDB(), shortTTL)

			requestID := uuid.New().String()
			response := map[string]interface{}{"test": "expiration"}

			// Cache the response
			err := testService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())

			// Record should exist immediately
			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())
			Expect(record.RequestID).To(Equal(requestID))

			// Wait for expiration
			time.Sleep(200 * time.Millisecond)

			// Cleanup should remove the expired record
			err = testService.CleanupExpired()
			Expect(err).NotTo(HaveOccurred())

			// Record should be deleted
			gormDB := testDB.GetDB()
			var count int64
			gormDB.Model(&models.IdempotencyRecord{}).Where("request_id = ?", requestID).Count(&count)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
			Expect(count).To(Equal(int64(0)))
		})

		It("should not expire records before TTL (100ms test)", func() {
			shortTTL := 200 * time.Millisecond
			testService := idempotency.NewServiceWithTTL(testDB.GetDB(), shortTTL)

			requestID := uuid.New().String()
			response := map[string]interface{}{"test": "not-expired"}

			err := testService.CacheResponse(requestID, response)
			Expect(err).NotTo(HaveOccurred())

			// Wait less than TTL
			time.Sleep(100 * time.Millisecond)

			// Cleanup should NOT remove the record
			err = testService.CleanupExpired()
			Expect(err).NotTo(HaveOccurred())

			// Record should still exist
			record := fixtures.GetIdempotencyRecord(testDB, requestID)
			Expect(record).NotTo(BeNil())
		})
	})

	Context("when cleaning up expired records", Ordered, func() {
		var expiredRecordID string
		var validRecordID string

		BeforeAll(func() {
			gormDB := testDB.GetDB()

			// Create expired record
			expiredRecordID = uuid.New().String()
			expiredRecord := &models.IdempotencyRecord{
				RequestID:    expiredRecordID,
				ResponseHash: "abc123",
				ResponseBody: `{"expired":true}`,
				ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
				CreatedAt:    time.Now().Add(-25 * time.Hour),
			}
			gormDB.Create(expiredRecord)

			// Create valid record
			validRecordID = uuid.New().String()
			validRecord := &models.IdempotencyRecord{
				RequestID:    validRecordID,
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
			// Count before cleanup (with Ordered tests, includes records from previous tests)
			gormDB := testDB.GetDB()
			var countBefore int64
			gormDB.Model(&models.IdempotencyRecord{}).Count(&countBefore)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			Expect(countBefore).To(BeNumerically(">=", 2)) // At least our 2 records

			// Run cleanup
			err := idempotencyService.CleanupExpired()
			Expect(err).NotTo(HaveOccurred())

			// Verify expired record was deleted
			gormDB = testDB.GetDB()
			var expiredRecord models.IdempotencyRecord
			err = gormDB.Where("request_id = ?", expiredRecordID).First(&expiredRecord).Error
			Expect(err).To(HaveOccurred()) // Should not be found

			// Verify valid record still exists
			var validRecord models.IdempotencyRecord
			err = gormDB.Where("request_id = ?", validRecordID).First(&validRecord).Error
			Expect(err).NotTo(HaveOccurred()) // Should be found
			Expect(validRecord.RequestID).To(Equal(validRecordID))

			sqlDB, _ = gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
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
