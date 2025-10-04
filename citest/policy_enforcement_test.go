package citest

import (
	"encoding/json"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Policy enforcement end-to-end", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
		directoryID    string
		policyFileID   string
		schemaFileID   string
		profileFileID  string
	)

	BeforeAll(func() {
		metadataClient = httpexpect.New(GinkgoT(), metadataURL)
		contentClient = httpexpect.New(GinkgoT(), contentURL)

		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "policy-e2e"}).
			Expect().
			Status(201).
			JSON().Object()
		directoryID = dir.Value("id").String().Raw()
	})

	AfterAll(func() {
		if profileFileID != "" {
			metadataClient.DELETE("/api/v1/files/"+profileFileID).
				WithHeader("X-VFS-Actor", "admin").
				Expect().
				Status(204)
		}
		if schemaFileID != "" {
			metadataClient.DELETE("/api/v1/files/"+schemaFileID).
				WithHeader("X-VFS-Actor", "admin").
				Expect().
				Status(204)
		}
		if policyFileID != "" {
			metadataClient.DELETE("/api/v1/files/"+policyFileID).
				WithHeader("X-VFS-Actor", "admin").
				Expect().
				Status(204)
		}
		metadataClient.DELETE("/api/v1/directories/" + directoryID).
			Expect().
			Status(204)
	})

	It("enforces admin access for policy files", func() {
		payload := map[string]any{"module": "package policy\nallow = true"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, ".rego", "application/json", payloadBytes)

		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         ".rego",
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    upload.Value("mime_type").String().Raw(),
				"actor":        "citest",
			}).
			Expect().
			Status(403).
			JSON().Object().
			Value("error").Object().
			Value("code").String().Equal("policy_forbidden")

		adminPayload := map[string]any{"module": "package policy\nallow { true }"}
		adminPayloadBytes, err := json.Marshal(adminPayload)
		Expect(err).NotTo(HaveOccurred())
		adminUpload := uploadContent(contentClient, ".rego", "application/json", adminPayloadBytes)

		created := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         ".rego",
				"storage_mode": adminUpload.Value("storage_mode").String().Raw(),
				"json_payload": adminUpload.Value("json_payload").Raw(),
				"checksum":     adminUpload.Value("checksum").String().Raw(),
				"size":         adminUpload.Value("size").Number().Raw(),
				"mime_type":    adminUpload.Value("mime_type").String().Raw(),
				"actor":        "admin",
			}).
			Expect().
			Status(201).
			JSON().Object()
		policyFileID = created.Value("id").String().Raw()
		created.Value("version").Number().Equal(1)

		metadataClient.PATCH("/api/v1/files/" + policyFileID).
			WithJSON(map[string]any{
				"storage_mode": adminUpload.Value("storage_mode").String().Raw(),
				"json_payload": adminUpload.Value("json_payload").Raw(),
				"checksum":     adminUpload.Value("checksum").String().Raw(),
				"size":         adminUpload.Value("size").Number().Raw(),
				"mime_type":    adminUpload.Value("mime_type").String().Raw(),
				"actor":        "citest",
			}).
			Expect().
			Status(403).
			JSON().Object().
			Value("error").Object().
			Value("code").String().Equal("policy_forbidden")

		updatePayload := map[string]any{
			"module": `package policy

default allow = false

allow {
    input.actor == "admin"
}`,
		}
		updatePayloadBytes, err := json.Marshal(updatePayload)
		Expect(err).NotTo(HaveOccurred())
		updateUpload := uploadContent(contentClient, ".rego", "application/json", updatePayloadBytes)

		updated := metadataClient.PATCH("/api/v1/files/" + policyFileID).
			WithJSON(map[string]any{
				"storage_mode": updateUpload.Value("storage_mode").String().Raw(),
				"json_payload": updateUpload.Value("json_payload").Raw(),
				"checksum":     updateUpload.Value("checksum").String().Raw(),
				"size":         updateUpload.Value("size").Number().Raw(),
				"mime_type":    updateUpload.Value("mime_type").String().Raw(),
				"actor":        "admin",
			}).
			Expect().
			Status(200).
			JSON().Object()
		updated.Value("version").Number().Equal(2)

		nonAdminPayload := map[string]any{"body": "user data"}
		nonAdminPayloadBytes, err := json.Marshal(nonAdminPayload)
		Expect(err).NotTo(HaveOccurred())
		nonAdminUpload := uploadContent(contentClient, "records.json", "application/json", nonAdminPayloadBytes)

		metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "records.json",
				"storage_mode": nonAdminUpload.Value("storage_mode").String().Raw(),
				"json_payload": nonAdminUpload.Value("json_payload").Raw(),
				"checksum":     nonAdminUpload.Value("checksum").String().Raw(),
				"size":         nonAdminUpload.Value("size").Number().Raw(),
				"mime_type":    nonAdminUpload.Value("mime_type").String().Raw(),
				"actor":        "citest",
			}).
			Expect().
			Status(403).
			JSON().Object().
			Value("error").Object().
			Value("code").String().Equal("policy_forbidden")

		adminUploadPayload := map[string]any{"body": "admin"}
		adminUploadBytes, err := json.Marshal(adminUploadPayload)
		Expect(err).NotTo(HaveOccurred())
		adminUploadContent := uploadContent(contentClient, "records.json", "application/json", adminUploadBytes)

		regularFile := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "records.json",
				"storage_mode": adminUploadContent.Value("storage_mode").String().Raw(),
				"json_payload": adminUploadContent.Value("json_payload").Raw(),
				"checksum":     adminUploadContent.Value("checksum").String().Raw(),
				"size":         adminUploadContent.Value("size").Number().Raw(),
				"mime_type":    adminUploadContent.Value("mime_type").String().Raw(),
				"actor":        "admin",
			}).
			Expect().
			Status(201).
			JSON().Object()
		regularFileID := regularFile.Value("id").String().Raw()

		metadataClient.DELETE("/api/v1/files/"+regularFileID).
			WithHeader("X-VFS-Actor", "citest").
			Expect().
			Status(403).
			JSON().Object().
			Value("error").Object().
			Value("code").String().Equal("policy_forbidden")

		metadataClient.DELETE("/api/v1/files/"+regularFileID).
			WithHeader("X-VFS-Actor", "admin").
			Expect().
			Status(204)

		metadataClient.DELETE("/api/v1/files/"+policyFileID).
			WithHeader("X-VFS-Actor", "citest").
			Expect().
			Status(403).
			JSON().Object().
			Value("error").Object().
			Value("code").String().Equal("policy_forbidden")

		metadataClient.DELETE("/api/v1/files/"+policyFileID).
			WithHeader("X-VFS-Actor", "admin").
			Expect().
			Status(204)

		metadataClient.GET("/api/v1/files/" + policyFileID).
			Expect().
			Status(404)
		policyFileID = ""
	})

	It("validates json schema manifests", func() {
		schemaPayload := map[string]any{
			"schema": map[string]any{
				"type":     "object",
				"required": []string{"payload"},
				"properties": map[string]any{
					"payload": map[string]any{
						"type":                 "object",
						"required":             []string{"name", "age"},
						"additionalProperties": false,
						"properties": map[string]any{
							"name": map[string]any{"type": "string"},
							"age":  map[string]any{"type": "integer", "minimum": 0},
						},
					},
				},
			},
			"applies_to": []string{"*.profile.json"},
		}
		schema := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         ".jsonschema",
				"storage_mode": "inline_json",
				"json_payload": schemaPayload,
				"actor":        "admin",
			}).
			Expect().
			Status(201).
			JSON().Object()
		schema.Value("version").Number().Equal(1)
		schemaFileID = schema.Value("id").String().Raw()

		validProfile := map[string]any{"name": "alice", "age": 30}
		profile := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "alice.profile.json",
				"storage_mode": "inline_json",
				"json_payload": validProfile,
				"actor":        "citest",
			}).
			Expect().
			Status(201).
			JSON().Object()
		profile.Value("version").Number().Equal(1)
		profileFileID = profile.Value("id").String().Raw()

		invalidCreate := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         "bob.profile.json",
				"storage_mode": "inline_json",
				"json_payload": map[string]any{"name": "bob"},
				"actor":        "citest",
			}).
			Expect().
			Status(400).
			JSON().Object().
			Value("error").Object()
		invalidCreate.Value("code").String().Equal("schema_validation_failed")
		invalidCreate.Value("details").Array().NotEmpty()

		invalidUpdate := metadataClient.PATCH("/api/v1/files/" + profileFileID).
			WithJSON(map[string]any{
				"storage_mode": "inline_json",
				"json_payload": map[string]any{"name": "alice", "age": -1},
				"actor":        "citest",
			}).
			Expect().
			Status(400).
			JSON().Object().
			Value("error").Object()
		invalidUpdate.Value("code").String().Equal("schema_validation_failed")
		invalidUpdate.Value("details").Array().NotEmpty()

		updated := metadataClient.PATCH("/api/v1/files/" + profileFileID).
			WithJSON(map[string]any{
				"storage_mode": "inline_json",
				"json_payload": map[string]any{"name": "alice", "age": 31},
				"actor":        "citest",
			}).
			Expect().
			Status(200).
			JSON().Object()
		updated.Value("version").Number().Equal(2)

		metadataClient.DELETE("/api/v1/files/"+profileFileID).
			WithHeader("X-VFS-Actor", "admin").
			Expect().
			Status(204)
		profileFileID = ""

		metadataClient.DELETE("/api/v1/files/"+schemaFileID).
			WithHeader("X-VFS-Actor", "admin").
			Expect().
			Status(204)
		schemaFileID = ""
	})
})
