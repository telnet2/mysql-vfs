package citest

import (
	"encoding/json"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Directory operations end-to-end", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
	)

	BeforeAll(func() {
		metadataClient = httpexpect.New(GinkgoT(), metadataURL)
		contentClient = httpexpect.New(GinkgoT(), contentURL)
	})

	It("handles hierarchical directory structures", func() {
		// Create root directory
		root := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "root"}).
			Expect().
			Status(201).
			JSON().Object()
		rootID := root.Value("id").String().Raw()

		// Create nested directories
		level1 := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{
				"name":      "level1",
				"parent_id": rootID,
			}).
			Expect().
			Status(201).
			JSON().Object()
		level1ID := level1.Value("id").String().Raw()

		level2 := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{
				"name":      "level2",
				"parent_id": level1ID,
			}).
			Expect().
			Status(201).
			JSON().Object()
		level2ID := level2.Value("id").String().Raw()

		// Add files at different levels
		payload := map[string]any{"data": "test"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())

		rootFile := uploadContent(contentClient, "root.json", "application/json", payloadBytes)
		rootCreated := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": rootID,
				"name":         "root.json",
				"storage_mode": rootFile.Value("storage_mode").String().Raw(),
				"json_payload": rootFile.Value("json_payload").Raw(),
				"checksum":     rootFile.Value("checksum").String().Raw(),
				"size":         rootFile.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "dir-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()
		rootFileID := rootCreated.Value("id").String().Raw()

		level2File := uploadContent(contentClient, "deep.json", "application/json", payloadBytes)
		created := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": level2ID,
				"name":         "deep.json",
				"storage_mode": level2File.Value("storage_mode").String().Raw(),
				"json_payload": level2File.Value("json_payload").Raw(),
				"checksum":     level2File.Value("checksum").String().Raw(),
				"size":         level2File.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "dir-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()
		level2FileID := created.Value("id").String().Raw()

		// Verify hierarchy
		metadataClient.GET("/api/v1/directories/" + rootID).
			Expect().
			Status(200).
			JSON().Object().
			Value("name").String().Equal("root")

		metadataClient.GET("/api/v1/directories/" + level2ID).
			Expect().
			Status(200).
			JSON().Object().
			Value("parent_id").String().Equal(level1ID)

		// Cleanup (bottom-up to avoid constraint violations)
		metadataClient.DELETE("/api/v1/files/" + level2FileID).
			Expect().
			Status(204)

		metadataClient.DELETE("/api/v1/directories/" + level2ID).
			Expect().
			Status(204)

		metadataClient.DELETE("/api/v1/directories/" + level1ID).
			Expect().
			Status(204)

		metadataClient.DELETE("/api/v1/files/" + rootFileID).
			Expect().
			Status(204)

		metadataClient.DELETE("/api/v1/directories/" + rootID).
			Expect().
			Status(204)
	})

	It("prevents deletion of non-empty directories", func() {
		// Create directory with file
		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "non-empty"}).
			Expect().
			Status(201).
			JSON().Object()
		dirID := dir.Value("id").String().Raw()

		payload := map[string]any{"data": "content"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, "file.json", "application/json", payloadBytes)

		file := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": dirID,
				"name":         "file.json",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    "application/json",
				"actor":        "dir-tester",
			}).
			Expect().
			Status(201).
			JSON().Object()
		fileID := file.Value("id").String().Raw()

		// Attempt to delete directory with files should fail
		metadataClient.DELETE("/api/v1/directories/" + dirID).
			Expect().
			Status(400)

		// Clean up file first, then directory
		metadataClient.DELETE("/api/v1/files/" + fileID).
			Expect().
			Status(204)

		metadataClient.DELETE("/api/v1/directories/" + dirID).
			Expect().
			Status(204)
	})

	It("handles concurrent directory operations", func() {
		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "concurrent-dir"}).
			Expect().
			Status(201).
			JSON().Object()
		dirID := dir.Value("id").String().Raw()

		// Create multiple files concurrently in same directory
		type fileSpec struct {
			name string
			data string
		}

		specs := []fileSpec{
			{"file1.json", "data1"},
			{"file2.json", "data2"},
			{"file3.json", "data3"},
			{"file4.json", "data4"},
			{"file5.json", "data5"},
		}

		fileIDs := make([]string, len(specs))

		for i, spec := range specs {
			payload := map[string]any{"data": spec.data}
			payloadBytes, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())
			upload := uploadContent(contentClient, spec.name, "application/json", payloadBytes)

			created := metadataClient.POST("/api/v1/files").
				WithJSON(map[string]any{
					"directory_id": dirID,
					"name":         spec.name,
					"storage_mode": upload.Value("storage_mode").String().Raw(),
					"json_payload": upload.Value("json_payload").Raw(),
					"checksum":     upload.Value("checksum").String().Raw(),
					"size":         upload.Value("size").Number().Raw(),
					"mime_type":    "application/json",
					"actor":        "concurrent-tester",
				}).
				Expect().
				Status(201).
				JSON().Object()

			fileIDs[i] = created.Value("id").String().Raw()
		}

		// Verify all files exist
		for _, fid := range fileIDs {
			metadataClient.GET("/api/v1/files/" + fid).
				Expect().
				Status(200)
		}

		// Cleanup
		for _, fid := range fileIDs {
			metadataClient.DELETE("/api/v1/files/" + fid).
				Expect().
				Status(204)
		}

		metadataClient.DELETE("/api/v1/directories/" + dirID).
			Expect().
			Status(204)
	})

	It("handles directory name updates", func() {
		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "old-name"}).
			Expect().
			Status(201).
			JSON().Object()
		dirID := dir.Value("id").String().Raw()

		// Update directory name
		updated := metadataClient.PATCH("/api/v1/directories/" + dirID).
			WithJSON(map[string]any{"name": "new-name"}).
			Expect().
			Status(200).
			JSON().Object()

		updated.Value("name").String().Equal("new-name")
		updated.Value("id").String().Equal(dirID)

		// Verify update persisted
		fetched := metadataClient.GET("/api/v1/directories/" + dirID).
			Expect().
			Status(200).
			JSON().Object()

		fetched.Value("name").String().Equal("new-name")

		// Cleanup
		metadataClient.DELETE("/api/v1/directories/" + dirID).
			Expect().
			Status(204)
	})
})
