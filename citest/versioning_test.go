package citest

import (
	"encoding/json"
	"fmt"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("File versioning end-to-end", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
		directoryID    string
		fileID         string
	)

	BeforeAll(func() {
		metadataClient = httpexpect.New(GinkgoT(7), metadataURL)
		contentClient = httpexpect.New(GinkgoT(8), contentURL)

		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "versioning-test"}).
			Expect().
			Status(201).
			JSON().Object()
		directoryID = dir.Value("id").String().Raw()
	})

	AfterAll(func() {
		if fileID != "" {
			metadataClient.DELETE("/api/v1/files/" + fileID).
				Expect().
				Status(204)
		}
		metadataClient.DELETE("/api/v1/directories/" + directoryID).
			Expect().
			Status(204)
	})

	It("tracks file version history correctly", func() {
		// Create initial version
		v1Payload := map[string]any{"version": 1, "data": "initial"}
		v1Bytes, err := json.Marshal(v1Payload)
		Expect(err).NotTo(HaveOccurred())
		v1Upload := uploadContent(contentClient, "doc.json", "application/json", v1Bytes)

		created := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "doc.json",
				"storage_mode": v1Upload.Value("storage_mode").String().Raw(),
				"json_payload": v1Upload.Value("json_payload").Raw(),
				"checksum":     v1Upload.Value("checksum").String().Raw(),
				"size":         v1Upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "version-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID = created.Value("id").String().Raw()
		created.Value("version").Number().Equal(1)
		v1Checksum := created.Value("checksum").String().Raw()

		// Update to version 2
		v2Payload := map[string]any{"version": 2, "data": "updated"}
		v2Bytes, err := json.Marshal(v2Payload)
		Expect(err).NotTo(HaveOccurred())
		v2Upload := uploadContent(contentClient, "doc.json", "application/json", v2Bytes)

		v2 := metadataClient.PATCH("/api/v1/files/" + fileID).
			WithJSON(map[string]any{
				"storage_mode": v2Upload.Value("storage_mode").String().Raw(),
				"json_payload": v2Upload.Value("json_payload").Raw(),
				"checksum":     v2Upload.Value("checksum").String().Raw(),
				"size":         v2Upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "version-tester",
			}).
			Expect().
			Status(200).
			JSON().Object()

		v2.Value("version").Number().Equal(2)
		v2Checksum := v2.Value("checksum").String().Raw()
		Expect(v2Checksum).NotTo(Equal(v1Checksum))

		// Update to version 3
		v3Payload := map[string]any{"version": 3, "data": "final"}
		v3Bytes, err := json.Marshal(v3Payload)
		Expect(err).NotTo(HaveOccurred())
		v3Upload := uploadContent(contentClient, "doc.json", "application/json", v3Bytes)

		v3 := metadataClient.PATCH("/api/v1/files/" + fileID).
			WithJSON(map[string]any{
				"storage_mode": v3Upload.Value("storage_mode").String().Raw(),
				"json_payload": v3Upload.Value("json_payload").Raw(),
				"checksum":     v3Upload.Value("checksum").String().Raw(),
				"size":         v3Upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "version-tester",
			}).
			Expect().
			Status(200).
			JSON().Object()

		v3.Value("version").Number().Equal(3)

		// Verify current state shows latest version
		current := metadataClient.GET("/api/v1/files/" + fileID).
			Expect().
			Status(200).
			JSON().Object()

		current.Value("version").Number().Equal(3)
		current.Value("checksum").String().Equal(v3.Value("checksum").String().Raw())
	})

	It("handles rapid version updates correctly", func() {
		rapidPayload := map[string]any{"test": "rapid"}
		rapidBytes, err := json.Marshal(rapidPayload)
		Expect(err).NotTo(HaveOccurred())
		rapidUpload := uploadContent(contentClient, "rapid.json", "application/json", rapidBytes)

		rapid := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "rapid.json",
				"storage_mode": rapidUpload.Value("storage_mode").String().Raw(),
				"json_payload": rapidUpload.Value("json_payload").Raw(),
				"checksum":     rapidUpload.Value("checksum").String().Raw(),
				"size":         rapidUpload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "rapid-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		rapidID := rapid.Value("id").String().Raw()
		rapid.Value("version").Number().Equal(1)

		// Perform 10 rapid updates
		for i := 2; i <= 10; i++ {
			updatePayload := map[string]any{"test": "rapid", "iteration": i}
			updateBytes, err := json.Marshal(updatePayload)
			Expect(err).NotTo(HaveOccurred())
			updateUpload := uploadContent(contentClient, "rapid.json", "application/json", updateBytes)

			updated := metadataClient.PATCH("/api/v1/files/" + rapidID).
				WithJSON(map[string]any{
					"storage_mode": updateUpload.Value("storage_mode").String().Raw(),
					"json_payload": updateUpload.Value("json_payload").Raw(),
					"checksum":     updateUpload.Value("checksum").String().Raw(),
					"size":         updateUpload.Value("size").Number().Raw(),
					"mime_type":    "application/json",
					"actor":        fmt.Sprintf("rapid-tester-%d", i),
				}).
				Expect().
				Status(200).
				JSON().Object()

			updated.Value("version").Number().Equal(float64(i))
		}

		// Verify final version
		final := metadataClient.GET("/api/v1/files/" + rapidID).
			Expect().
			Status(200).
			JSON().Object()

		final.Value("version").Number().Equal(10)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + rapidID).
			Expect().
			Status(204)
	})

	It("preserves version integrity during storage mode transitions", func() {
		// Start with inline storage
		smallPayload := map[string]any{"size": "small"}
		smallBytes, err := json.Marshal(smallPayload)
		Expect(err).NotTo(HaveOccurred())
		smallUpload := uploadContent(contentClient, "transition.json", "application/json", smallBytes)

		transition := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "transition.json",
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
		transition.Value("storage_mode").String().Equal("inline_json")
		transition.Value("version").Number().Equal(1)

		// Transition to blob storage with large payload
		largeData := make([]byte, 20*1024*1024) // 20MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}
		blobUpload := uploadContent(contentClient, "transition.json", "application/octet-stream", largeData)

		updated := metadataClient.PATCH("/api/v1/files/" + transitionID).
			WithJSON(map[string]any{
				"storage_mode": blobUpload.Value("storage_mode").String().Raw(),
				"blob_key":     blobUpload.Value("blob_key").String().Raw(),
				"checksum":     blobUpload.Value("checksum").String().Raw(),
				"size":         blobUpload.Value("size").Number().Raw(),
				"mime_type":    "application/octet-stream",
				"actor":        "transition-tester",
			}).
			Expect().
			Status(200).
			JSON().Object()

		updated.Value("storage_mode").String().Equal("s3_blob")
		updated.Value("version").Number().Equal(2)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + transitionID).
			Expect().
			Status(204)
	})
})
