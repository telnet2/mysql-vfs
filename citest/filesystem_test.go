package citest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Virtual filesystem end-to-end", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
	)

	BeforeAll(func() {
		metadataClient = httpexpect.New(GinkgoT(), metadataURL)
		contentClient = httpexpect.New(GinkgoT(), contentURL)
	})

	It("handles end-to-end file workflows", func() {
		metadataClient.GET("/ping").Expect().Status(200)
		contentClient.GET("/ping").Expect().Status(200)

		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "docs"}).
			Expect().
			Status(201).
			JSON().Object()
		dirID := dir.Value("id").String().Raw()

		inlinePayload := []byte(`{"hello":"world"}`)
		inlineUpload := uploadContent(contentClient, "inline.json", "application/json", inlinePayload)

		inlineFile := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": dirID,
				"name":         "inline.json",
				"storage_mode": inlineUpload.Value("storage_mode").String().Raw(),
				"json_payload": inlineUpload.Value("json_payload").Raw(),
				"checksum":     inlineUpload.Value("checksum").String().Raw(),
				"size":         inlineUpload.Value("size").Number().Raw(),
				"mime_type":    inlineUpload.Value("mime_type").String().Raw(),
				"actor":        "citest",
			}).
			Expect().
			Status(201).
			JSON().Object()

		fileID := inlineFile.Value("id").String().Raw()
		inlineFile.Value("storage_mode").String().Equal("inline_json")

		var wg sync.WaitGroup
		types := []struct {
			name string
			data string
		}{
			{"inline.json", "concurrent-version-1"},
			{"inline.json", "concurrent-version-2"},
		}
		wg.Add(len(types))
		for idx, item := range types {
			i := idx
			t := item
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				payload := map[string]any{
					"chunk": i,
					"value": t.data,
				}
				jsonBytes, err := json.Marshal(payload)
				Expect(err).NotTo(HaveOccurred())
				upload := uploadContent(contentClient, t.name, "application/json", jsonBytes)
				metadataClient.PATCH("/api/v1/files/" + fileID).
					WithJSON(map[string]any{
						"storage_mode": upload.Value("storage_mode").String().Raw(),
						"json_payload": upload.Value("json_payload").Raw(),
						"checksum":     upload.Value("checksum").String().Raw(),
						"size":         upload.Value("size").Number().Raw(),
						"mime_type":    upload.Value("mime_type").String().Raw(),
						"actor":        fmt.Sprintf("citest-%d", i),
					}).
					Expect().Status(200)
			}()
		}
		wg.Wait()

		fileState := metadataClient.GET("/api/v1/files/" + fileID).Expect().Status(200).JSON().Object()
		fileState.Value("storage_mode").String().Equal("inline_json")
		Expect(fileState.Value("version").Number().Raw()).To(BeNumerically(">=", 3))

		blobData := []byte(strings.Repeat("blob-data", 2048))
		blobUpload := uploadContent(contentClient, "binary.bin", "application/octet-stream", blobData)

		blobFile := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": dirID,
				"name":         "binary.bin",
				"storage_mode": blobUpload.Value("storage_mode").String().Raw(),
				"blob_key":     blobUpload.Value("blob_key").String().Raw(),
				"checksum":     blobUpload.Value("checksum").String().Raw(),
				"size":         blobUpload.Value("size").Number().Raw(),
				"mime_type":    "application/octet-stream",
				"actor":        "citest",
			}).
			Expect().
			Status(201).
			JSON().Object()
		blobFileID := blobFile.Value("id").String().Raw()

		blobKey := blobUpload.Value("blob_key").String().Raw()
		Expect(blobKey).NotTo(BeEmpty())

		encoded := contentClient.GET("/api/v1/content/"+url.PathEscape(blobKey)).
			WithQuery("format", "base64").
			Expect().
			Status(200).
			JSON().Object()
		downloaded := encoded.Value("data").String().Raw()
		decoded, err := base64.StdEncoding.DecodeString(downloaded)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded).To(Equal(blobData))

		metadataClient.DELETE("/api/v1/files/" + fileID).Expect().Status(204)
		metadataClient.DELETE("/api/v1/files/" + blobFileID).Expect().Status(204)
		metadataClient.DELETE("/api/v1/directories/" + dirID).
			Expect().
			Status(204)

		metadataClient.GET("/api/v1/directories/" + dirID).Expect().Status(404)
	})
})
