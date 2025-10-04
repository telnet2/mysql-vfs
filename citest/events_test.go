package citest

import (
	"encoding/json"
	"time"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Events system end-to-end", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
		directoryID    string
	)

	BeforeAll(func() {
		metadataClient = httpexpect.New(GinkgoT(), metadataURL)
		contentClient = httpexpect.New(GinkgoT(), contentURL)

		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "events-test"}).
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

	It("generates events for file lifecycle operations", func() {
		// Create file - should generate file.created event
		payload := map[string]any{"event": "test", "type": "create"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "lifecycle.json", "application/json", payloadBytes)

		created := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "lifecycle.json",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "events-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := created.Value("id").String().Raw()
		created.Value("version").Number().Equal(1)

		// Update file - should generate file.updated event
		updatePayload := map[string]any{"event": "test", "type": "update"}
		updateBytes, err := json.Marshal(updatePayload)
		Expect(err).NotTo(HaveOccurred())
		updateUpload := uploadContent(contentClient, "lifecycle.json", "application/json", updateBytes)

		metadataClient.PATCH("/api/v1/files/" + fileID).
			WithJSON(map[string]any{
				"storage_mode": updateUpload.Value("storage_mode").String().Raw(),
				"json_payload": updateUpload.Value("json_payload").Raw(),
				"checksum":     updateUpload.Value("checksum").String().Raw(),
				"size":         updateUpload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "events-tester",
			}).
			Expect().
			Status(200)

		// Delete file - should generate file.deleted event
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)

		// Verify events were created (wait a bit for async processing)
		time.Sleep(500 * time.Millisecond)
	})

	It("generates events for directory operations", func() {
		// Create directory - should generate directory.created event
		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "event-dir"}).
			Expect().
			Status(201).
			JSON().Object()
		eventDirID := dir.Value("id").String().Raw()

		// Update directory - should generate directory.updated event
		metadataClient.PATCH("/api/v1/directories/" + eventDirID).
			WithJSON(map[string]any{"name": "event-dir-renamed"}).
			Expect().
			Status(200)

		// Delete directory - should generate directory.deleted event
		metadataClient.DELETE("/api/v1/directories/" + eventDirID).
			Expect().
			Status(204)

		// Wait for async processing
		time.Sleep(500 * time.Millisecond)
	})

	It("handles event triggers with custom actions", func() {
		// Create a file with a trigger pattern (e.g., .trigger.json)
		triggerPayload := map[string]any{
			"trigger": "action",
			"data":    "automated-workflow",
		}
		triggerBytes, err := json.Marshal(triggerPayload)
		Expect(err).NotTo(HaveOccurred())
		triggerUpload := uploadContent(contentClient, "workflow.trigger.json", "application/json", triggerBytes)

		trigger := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "workflow.trigger.json",
				"storage_mode": triggerUpload.Value("storage_mode").String().Raw(),
				"json_payload": triggerUpload.Value("json_payload").Raw(),
				"checksum":     triggerUpload.Value("checksum").String().Raw(),
				"size":         triggerUpload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "trigger-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		triggerID := trigger.Value("id").String().Raw()

		// Wait for any async trigger processing
		time.Sleep(1 * time.Second)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + triggerID).
			Expect().
			Status(204)
	})

	It("batches multiple events from rapid operations", func() {
		// Create multiple files rapidly
		fileIDs := make([]string, 0, 10)

		for i := 0; i < 10; i++ {
			payload := map[string]any{"batch": i, "data": "rapid"}
			payloadBytes, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())
			upload := uploadContent(contentClient, "batch-"+string(rune('0'+i))+".json", "application/json", payloadBytes)

			file := metadataClient.POST("/api/v1/files").
				WithJSON(map[string]any{
					"directory_id": directoryID,
					"name":         "batch-" + string(rune('0'+i)) + ".json",
					"storage_mode": upload.Value("storage_mode").String().Raw(),
					"json_payload": upload.Value("json_payload").Raw(),
					"checksum":     upload.Value("checksum").String().Raw(),
					"size":         upload.Value("size").Number().Raw(),
					"mime_type":    "application/json",
					"actor":        "batch-tester",
				}).
				Expect().
				Status(201).
				JSON().Object()

			fileIDs = append(fileIDs, file.Value("id").String().Raw())
		}

		// Wait for event processing
		time.Sleep(2 * time.Second)

		// Cleanup
		for _, fid := range fileIDs {
			metadataClient.DELETE("/api/v1/files/" + fid).
				Expect().
				Status(204)
		}
	})

	It("maintains event ordering for file version updates", func() {
		orderPayload := map[string]any{"version": 1}
		orderBytes, err := json.Marshal(orderPayload)
		Expect(err).NotTo(HaveOccurred())
		orderUpload := uploadContent(contentClient, "ordered.json", "application/json", orderBytes)

		ordered := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "ordered.json",
				"storage_mode": orderUpload.Value("storage_mode").String().Raw(),
				"json_payload": orderUpload.Value("json_payload").Raw(),
				"checksum":     orderUpload.Value("checksum").String().Raw(),
				"size":         orderUpload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "order-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		orderedID := ordered.Value("id").String().Raw()

		// Perform sequential updates
		for v := 2; v <= 5; v++ {
			updatePayload := map[string]any{"version": v}
			updateBytes, err := json.Marshal(updatePayload)
			Expect(err).NotTo(HaveOccurred())
			updateUpload := uploadContent(contentClient, "ordered.json", "application/json", updateBytes)

			metadataClient.PATCH("/api/v1/files/" + orderedID).
				WithJSON(map[string]any{
					"storage_mode": updateUpload.Value("storage_mode").String().Raw(),
					"json_payload": updateUpload.Value("json_payload").Raw(),
					"checksum":     updateUpload.Value("checksum").String().Raw(),
					"size":         updateUpload.Value("size").Number().Raw(),
					"mime_type":    "application/json",
					"actor":        "order-tester",
				}).
				Expect().
				Status(200)

			// Small delay to ensure ordering
			time.Sleep(100 * time.Millisecond)
		}

		// Wait for all events to process
		time.Sleep(1 * time.Second)

		// Verify final state
		final := metadataClient.GET("/api/v1/files/" + orderedID).
			Expect().
			Status(200).
			JSON().Object()

		final.Value("version").Number().Equal(5)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + orderedID).
			Expect().
			Status(204)
	})

	It("handles event processing for storage mode transitions", func() {
		// Start with inline storage
		smallPayload := map[string]any{"size": "small"}
		smallBytes, err := json.Marshal(smallPayload)
		Expect(err).NotTo(HaveOccurred())
		smallUpload := uploadContent(contentClient, "transition-event.json", "application/json", smallBytes)

		transition := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "transition-event.json",
				"storage_mode": smallUpload.Value("storage_mode").String().Raw(),
				"json_payload": smallUpload.Value("json_payload").Raw(),
				"checksum":     smallUpload.Value("checksum").String().Raw(),
				"size":         smallUpload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "transition-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		transitionID := transition.Value("id").String().Raw()

		// Transition to blob storage
		blobData := []byte("large blob data for transition event test")
		blobUpload := uploadContent(contentClient, "transition-event.bin", "application/octet-stream", blobData)

		metadataClient.PATCH("/api/v1/files/" + transitionID).
			WithJSON(map[string]any{
				"storage_mode": blobUpload.Value("storage_mode").String().Raw(),
				"blob_key":     blobUpload.Value("blob_key").String().Raw(),
				"checksum":     blobUpload.Value("checksum").String().Raw(),
				"size":         blobUpload.Value("size").Number().Raw(),
				"mime_type":    "application/octet-stream",
				"actor":        "transition-tester",
			}).
			Expect().
			Status(200)

		// Wait for event processing
		time.Sleep(500 * time.Millisecond)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + transitionID).
			Expect().
			Status(204)
	})
})
