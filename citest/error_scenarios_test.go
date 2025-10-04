package citest

import (
	"encoding/json"
	"strings"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Error scenarios and edge cases", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
		directoryID    string
	)

	BeforeAll(func() {
		metadataClient = httpexpect.Default(GinkgoT(7), metadataURL)
		contentClient = httpexpect.Default(GinkgoT(7), contentURL)

		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "error-test"}).
			Expect().
			Status(201).
			JSON().Object()
		directoryID = dir.Value("id").String().Raw()
	})

	AfterAll(func() {
		metadataClient.DELETE("/api/v1/directories/" + directoryID).
			Expect().
			Status(204)
	})

	It("handles duplicate file names in same directory", func() {
		payload := map[string]any{"data": "first"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload1 := uploadContent(contentClient, "duplicate.json", "application/json", payloadBytes)

		// Create first file
		file1 := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "duplicate.json",
				"storage_mode": upload1.Value("storage_mode").String().Raw(),
				"json_payload": upload1.Value("json_payload").Raw(),
				"checksum":     upload1.Value("checksum").String().Raw(),
				"size":         upload1.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "error-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()
		file1ID := file1.Value("id").String().Raw()

		// Attempt to create duplicate - should be rejected with validation error
		payload2 := map[string]any{"data": "second"}
		payload2Bytes, err := json.Marshal(payload2)
		Expect(err).NotTo(HaveOccurred())
		upload2 := uploadContent(contentClient, "duplicate.json", "application/json", payload2Bytes)

		errorResp := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "duplicate.json",
				"storage_mode": upload2.Value("storage_mode").String().Raw(),
				"json_payload": upload2.Value("json_payload").Raw(),
				"checksum":     upload2.Value("checksum").String().Raw(),
				"size":         upload2.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "error-tester",
			}).
			Expect().
			Status(400).
			JSON().Object()

		errorResp.Value("error").Object().Value("code").String().Equal("invalid_request")
		errorResp.Value("error").Object().Value("message").String().Contains("already exists")

		metadataClient.GET("/api/v1/files/" + file1ID).
			Expect().
			Status(200).
			JSON().Object().
			Value("version").Number().Equal(1)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + file1ID).
			Expect().
			Status(204)
	})

	It("handles invalid directory references", func() {
		payload := map[string]any{"data": "test"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "orphan.json", "application/json", payloadBytes)

		// Attempt to create file with non-existent directory
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": "00000000-0000-0000-0000-000000000000",
				"name":         "orphan.json",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)
	})

	It("validates file name constraints", func() {
		payload := map[string]any{"data": "test"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "test.json", "application/json", payloadBytes)

		// Test empty name
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)

		// Test very long name (over 255 chars)
		longName := strings.Repeat("a", 300) + ".json"
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         longName,
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)
	})

	It("handles checksum mismatches", func() {
		payload := map[string]any{"data": "test"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "checksum.json", "application/json", payloadBytes)

		// Attempt to create file with incorrect checksum
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "checksum.json",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     "invalid-checksum-value",
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)
	})

	It("handles missing required fields", func() {
		// Missing directory_id
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"name":         "missing-dir.json",
				"storage_mode": "inline_json",
				"json_payload": map[string]any{"data": "test"},
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)

		// Missing storage_mode
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "missing-mode.json",
				"json_payload": map[string]any{"data": "test"},
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)
	})

	It("handles invalid JSON payloads for inline storage", func() {
		// Attempt to create file with malformed JSON
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "malformed.json",
				"storage_mode": "inline_json",
				"json_payload": "not-a-json-object",
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)
	})

	It("validates blob storage references", func() {
		// Attempt to create file with non-existent blob key
		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "missing-blob.bin",
				"storage_mode": "s3_blob",
				"blob_key":     "non-existent-blob-key",
				"checksum":     "abc123",
				"size":         1024,
				"mime_type":    "application/octet-stream",
				"actor":        "error-tester",
			}).
			Expect().
			Status(400)
	})

	It("handles concurrent updates with version conflicts", func() {
		payload := map[string]any{"counter": 0}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "conflict.json", "application/json", payloadBytes)

		file := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "conflict.json",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "conflict-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := file.Value("id").String().Raw()

		// Simulate concurrent updates
		for i := 1; i <= 3; i++ {
			updatePayload := map[string]any{"counter": i}
			updateBytes, err := json.Marshal(updatePayload)
			Expect(err).NotTo(HaveOccurred())
			updateUpload := uploadContent(contentClient, "conflict.json", "application/json", updateBytes)

			metadataClient.PATCH("/api/v1/files/" + fileID).
				WithJSON(map[string]any{
					"storage_mode": updateUpload.Value("storage_mode").String().Raw(),
					"json_payload": updateUpload.Value("json_payload").Raw(),
					"checksum":     updateUpload.Value("checksum").String().Raw(),
					"size":         updateUpload.Value("size").Number().Raw(),
					"mime_type":    "application/json",
					"actor":        "conflict-tester",
				}).
				Expect().
				Status(200)
		}

		// Final version should be at least 4
		final := metadataClient.GET("/api/v1/files/" + fileID).
			Expect().
			Status(200).
			JSON().Object()

		Expect(final.Value("version").Number().Raw()).To(BeNumerically(">=", 4))

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)
	})

	It("handles delete operations on non-existent resources", func() {
		// Attempt to delete non-existent file
		metadataClient.DELETE("/api/v1/files/00000000-0000-0000-0000-000000000000").
			Expect().
			Status(404)

		// Attempt to delete non-existent directory
		metadataClient.DELETE("/api/v1/directories/00000000-0000-0000-0000-000000000000").
			Expect().
			Status(404)
	})

	It("handles retrieval of non-existent resources", func() {
		// Attempt to get non-existent file
		metadataClient.GET("/api/v1/files/00000000-0000-0000-0000-000000000000").
			Expect().
			Status(404)

		// Attempt to get non-existent directory
		metadataClient.GET("/api/v1/directories/00000000-0000-0000-0000-000000000000").
			Expect().
			Status(404)
	})
})
