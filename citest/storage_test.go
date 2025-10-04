package citest

import (
	"encoding/base64"
	"encoding/json"
	"net/url"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Storage operations end-to-end", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
		directoryID    string
	)

	BeforeAll(func() {
		metadataClient = httpexpect.New(GinkgoT(7), metadataURL)
		contentClient = httpexpect.New(GinkgoT(7), contentURL)

		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "storage-test"}).
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

	It("handles inline JSON storage for small files", func() {
		smallPayload := map[string]any{
			"name":        "test-user",
			"age":         30,
			"preferences": []string{"option1", "option2"},
		}
		smallBytes, err := json.Marshal(smallPayload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "small.json", "application/json", smallBytes)

		file := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "small.json",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "storage-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := file.Value("id").String().Raw()
		file.Value("storage_mode").String().Equal("inline_json")

		// Verify content retrieval
		retrieved := metadataClient.GET("/api/v1/files/" + fileID).
			Expect().
			Status(200).
			JSON().Object()

		retrieved.Value("storage_mode").String().Equal("inline_json")
		retrieved.Value("inline_json").Object().Equal(smallPayload)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)
	})

	It("handles S3 blob storage for large files", func() {
		// Create 15MB file (exceeds inline threshold)
		largeData := make([]byte, 15*1024*1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		blobUpload := uploadContent(contentClient, "large.bin", "application/octet-stream", largeData)

		file := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "large.bin",
				"storage_mode": blobUpload.Value("storage_mode").String().Raw(),
				"blob_key":     blobUpload.Value("blob_key").String().Raw(),
				"checksum":     blobUpload.Value("checksum").String().Raw(),
				"size":         blobUpload.Value("size").Number().Raw(),
				"mime_type":    "application/octet-stream",
				"actor":        "storage-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := file.Value("id").String().Raw()
		file.Value("storage_mode").String().Equal("s3_blob")
		blobKey := file.Value("blob_key").String().Raw()
		Expect(blobKey).NotTo(BeEmpty())

		// Retrieve content via content service
		encoded := contentClient.GET("/api/v1/content/"+url.PathEscape(blobKey)).
			WithQuery("format", "base64").
			Expect().
			Status(200).
			JSON().Object()

		downloaded := encoded.Value("data").String().Raw()
		decoded, err := base64.StdEncoding.DecodeString(downloaded)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded).To(Equal(largeData))

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)
	})

	It("transitions from inline to blob storage when size increases", func() {
		// Start with small inline JSON
		smallPayload := map[string]any{"size": "small", "data": "initial"}
		smallBytes, err := json.Marshal(smallPayload)
		Expect(err).NotTo(HaveOccurred())
		smallUpload := uploadContent(contentClient, "growing.json", "application/json", smallBytes)

		file := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "growing.json",
				"storage_mode": smallUpload.Value("storage_mode").String().Raw(),
				"json_payload": smallUpload.Value("json_payload").Raw(),
				"checksum":     smallUpload.Value("checksum").String().Raw(),
				"size":         smallUpload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "storage-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := file.Value("id").String().Raw()
		file.Value("storage_mode").String().Equal("inline_json")

		// Update with large binary data
		largeData := make([]byte, 12*1024*1024) // 12MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}
		blobUpload := uploadContent(contentClient, "growing.bin", "application/octet-stream", largeData)

		updated := metadataClient.PATCH("/api/v1/files/" + fileID).
			WithJSON(map[string]any{
				"storage_mode": blobUpload.Value("storage_mode").String().Raw(),
				"blob_key":     blobUpload.Value("blob_key").String().Raw(),
				"checksum":     blobUpload.Value("checksum").String().Raw(),
				"size":         blobUpload.Value("size").Number().Raw(),
				"mime_type":    "application/octet-stream",
				"actor":        "storage-tester",
			}).
			Expect().
			Status(200).
			JSON().Object()

		updated.Value("storage_mode").String().Equal("s3_blob")
		updated.Value("version").Number().Equal(2)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)
	})

	It("handles different MIME types correctly", func() {
		testCases := []struct {
			name     string
			mimeType string
			data     []byte
		}{
			{"text.txt", "text/plain", []byte("Hello, World!")},
			{"data.xml", "application/xml", []byte("<root><item>test</item></root>")},
			{"style.css", "text/css", []byte("body { margin: 0; }")},
			{"script.js", "application/javascript", []byte("console.log('test');")},
		}

		fileIDs := make([]string, 0, len(testCases))

		for _, tc := range testCases {
			upload := uploadContent(contentClient, tc.name, tc.mimeType, tc.data)

			file := metadataClient.POST("/api/v1/files").
				WithJSON(map[string]any{
					"directory_id": directoryID,
					"name":         tc.name,
					"storage_mode": upload.Value("storage_mode").String().Raw(),
					"blob_key":     upload.Value("blob_key").String().Raw(),
					"checksum":     upload.Value("checksum").String().Raw(),
					"size":         upload.Value("size").Number().Raw(),
					"mime_type":    tc.mimeType,
					"actor":        "mime-tester",
				}).
				Expect().
				Status(201).
				JSON().Object()

			fileID := file.Value("id").String().Raw()
			file.Value("mime_type").String().Equal(tc.mimeType)
			fileIDs = append(fileIDs, fileID)
		}

		// Cleanup
		for _, fid := range fileIDs {
			metadataClient.DELETE("/api/v1/files/" + fid).
				Expect().
				Status(204)
		}
	})

	It("verifies checksum integrity", func() {
		payload := map[string]any{"integrity": "check", "value": 12345}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "checksum.json", "application/json", payloadBytes)

		file := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "checksum.json",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "checksum-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := file.Value("id").String().Raw()
		originalChecksum := file.Value("checksum").String().Raw()

		// Update with different content
		newPayload := map[string]any{"integrity": "check", "value": 67890}
		newBytes, err := json.Marshal(newPayload)
		Expect(err).NotTo(HaveOccurred())
		newUpload := uploadContent(contentClient, "checksum.json", "application/json", newBytes)

		updated := metadataClient.PATCH("/api/v1/files/" + fileID).
			WithJSON(map[string]any{
				"storage_mode": newUpload.Value("storage_mode").String().Raw(),
				"json_payload": newUpload.Value("json_payload").Raw(),
				"checksum":     newUpload.Value("checksum").String().Raw(),
				"size":         newUpload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "checksum-tester",
			}).
			Expect().
			Status(200).
			JSON().Object()

		newChecksum := updated.Value("checksum").String().Raw()
		Expect(newChecksum).NotTo(Equal(originalChecksum))

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)
	})

	It("handles binary data with various patterns", func() {
		patterns := []struct {
			name    string
			pattern []byte
		}{
			{"zeros.bin", make([]byte, 1024)},
			{"ones.bin", bytes(1024, 0xFF)},
			{"alternating.bin", alternating(1024)},
			{"random.bin", randomBytes(1024)},
		}

		fileIDs := make([]string, 0, len(patterns))

		for _, p := range patterns {
			upload := uploadContent(contentClient, p.name, "application/octet-stream", p.pattern)

			file := metadataClient.POST("/api/v1/files").
				WithJSON(map[string]any{
					"directory_id": directoryID,
					"name":         p.name,
					"storage_mode": upload.Value("storage_mode").String().Raw(),
					"blob_key":     upload.Value("blob_key").String().Raw(),
					"checksum":     upload.Value("checksum").String().Raw(),
					"size":         upload.Value("size").Number().Raw(),
					"mime_type":    "application/octet-stream",
					"actor":        "binary-tester",
				}).
				Expect().
				Status(201).
				JSON().Object()

			fileIDs = append(fileIDs, file.Value("id").String().Raw())
		}

		// Cleanup
		for _, fid := range fileIDs {
			metadataClient.DELETE("/api/v1/files/" + fid).
				Expect().
				Status(204)
		}
	})

	It("handles empty files correctly", func() {
		emptyData := []byte{}
		upload := uploadContent(contentClient, "empty.txt", "text/plain", emptyData)

		file := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "empty.txt",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"blob_key":     upload.Value("blob_key").String().Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         0,
				"mime_type":    "text/plain",
				"actor":        "empty-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := file.Value("id").String().Raw()
		file.Value("size").Number().Equal(0)

		// Cleanup
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)
	})
})

// Helper functions for binary pattern generation
func bytes(n int, value byte) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = value
	}
	return data
}

func alternating(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		if i%2 == 0 {
			data[i] = 0xAA
		} else {
			data[i] = 0x55
		}
	}
	return data
}

func randomBytes(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte((i * 7919) % 256) // Pseudo-random pattern
	}
	return data
}
