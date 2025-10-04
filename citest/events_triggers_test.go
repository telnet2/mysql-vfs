package citest

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	triggerManifestFileName = ".events"
	triggerFileName         = "trigger.json"
	nonMatchingFileName     = "ignored.txt"
	webhookTargetURL        = "https://hooks.example.local/test"
	workflowTriggerName     = "cleanup-workflow"
	auditEventType          = "ext.audit.triggered"
)

var _ = Describe("Events triggers integration", Ordered, func() {
	var (
		metadataClient *httpexpect.Expect
		contentClient  *httpexpect.Expect
		directoryID    string
		manifestFileID string
	)

	BeforeAll(func() {
		Expect(mysqlDSN).NotTo(BeEmpty())

		metadataClient = httpexpect.New(GinkgoT(), metadataURL)
		contentClient = httpexpect.New(GinkgoT(), contentURL)

		dir := metadataClient.POST("/api/v1/directories").
			WithJSON(map[string]any{"name": "events-trigger-tests"}).
			Expect().
			Status(201).
			JSON().Object()
		directoryID = dir.Value("id").String().Raw()

		manifest := map[string]any{
			"scope": "file",
			"triggers": []any{
				map[string]any{
					"name":  "on-create",
					"on":    "file.created",
					"scope": "file",
					"match": map[string]any{
						"file_name":    []string{triggerFileName},
						"storage_mode": []string{"inline_json"},
					},
					"actions": []any{
						map[string]any{
							"type":    "call_webhook",
							"webhook": webhookTargetURL,
						},
						map[string]any{
							"type":       "emit_event",
							"event_type": auditEventType,
						},
					},
				},
				map[string]any{
					"name":  "on-delete",
					"on":    "file.deleted",
					"scope": "file",
					"match": map[string]any{
						"file_name": []string{triggerFileName},
					},
					"actions": []any{
						map[string]any{
							"type":     "invoke_workflow",
							"workflow": workflowTriggerName,
						},
					},
				},
			},
		}
		manifestBytes, err := json.Marshal(manifest)
		Expect(err).NotTo(HaveOccurred())

		upload := uploadContent(contentClient, triggerManifestFileName, "application/json", manifestBytes)
		created := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         triggerManifestFileName,
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    upload.Value("mime_type").String().Raw(),
				"actor":        "admin",
			}).
			Expect().
			Status(201).
			JSON().Object()
		manifestFileID = created.Value("id").String().Raw()
	})

	AfterAll(func() {
		if manifestFileID != "" {
			metadataClient.DELETE("/api/v1/files/"+manifestFileID).
				WithHeader("X-VFS-Actor", "admin").
				Expect().
				Status(204)
		}
		metadataClient.DELETE("/api/v1/directories/" + directoryID).
			Expect().
			Status(204)
	})

	It("enqueues actions defined in .events manifests for matching lifecycle events", func() {
		payload := map[string]any{"message": "trigger"}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		upload := uploadContent(contentClient, triggerFileName, "application/json", payloadBytes)

		created := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         triggerFileName,
				"storage_mode": upload.Value("storage_mode").String().Raw(),
				"json_payload": upload.Value("json_payload").Raw(),
				"checksum":     upload.Value("checksum").String().Raw(),
				"size":         upload.Value("size").Number().Raw(),
				"mime_type":    upload.Value("mime_type").String().Raw(),
				"actor":        "trigger-actor",
			}).
			Expect().
			Status(201).
			JSON().Object()
		fileID := created.Value("id").String().Raw()

		Eventually(func() int {
			count, err := eventCountForSubject("ext.webhook.triggered", fileID)
			Expect(err).NotTo(HaveOccurred())
			return count
		}).Should(Equal(1))

		_, webhookEnvelope, err := fetchEventEnvelope("ext.webhook.triggered", fileID)
		Expect(err).NotTo(HaveOccurred())
		Expect(webhookEnvelope["event_type"]).To(Equal("ext.webhook.triggered"))
		Expect(webhookEnvelope["subject_id"]).To(Equal(fileID))
		Expect(webhookEnvelope["source"]).To(Equal("extension"))

		data, ok := toMap(webhookEnvelope["data"])
		Expect(ok).To(BeTrue())
		action, ok := toMap(data["action"])
		Expect(ok).To(BeTrue())
		Expect(action["type"]).To(Equal("call_webhook"))
		Expect(action["webhook"]).To(Equal(webhookTargetURL))
		context, ok := toMap(data["context"])
		Expect(ok).To(BeTrue())
		Expect(context["event_type"]).To(Equal("file.created"))
		Expect(context["file_id"]).To(Equal(fileID))
		Expect(context["file_name"]).To(Equal(triggerFileName))
		Expect(context["directory_id"]).To(Equal(directoryID))

		scopes, ok := toMap(webhookEnvelope["scopes"])
		Expect(ok).To(BeTrue())
		dirs, ok := toStringSlice(scopes["directories"])
		Expect(ok).To(BeTrue())
		Expect(dirs).To(ContainElement(directoryID))
		files, ok := toStringSlice(scopes["files"])
		Expect(ok).To(BeTrue())
		Expect(files).To(ContainElement(fileID))

		Eventually(func() int {
			count, err := eventCountForSubject(auditEventType, fileID)
			Expect(err).NotTo(HaveOccurred())
			return count
		}).Should(Equal(1))
		_, auditEnvelope, err := fetchEventEnvelope(auditEventType, fileID)
		Expect(err).NotTo(HaveOccurred())
		data, ok = toMap(auditEnvelope["data"])
		Expect(ok).To(BeTrue())
		action, ok = toMap(data["action"])
		Expect(ok).To(BeTrue())
		Expect(action["type"]).To(Equal("emit_event"))
		Expect(action["event_type"]).To(Equal(auditEventType))
		context, ok = toMap(data["context"])
		Expect(ok).To(BeTrue())
		Expect(context["event_type"]).To(Equal("file.created"))

		metadataClient.DELETE("/api/v1/files/"+fileID).
			WithHeader("X-VFS-Actor", "trigger-actor").
			Expect().
			Status(204)

		Eventually(func() int {
			count, err := eventCountForSubject("ext.workflow.triggered", fileID)
			Expect(err).NotTo(HaveOccurred())
			return count
		}).Should(Equal(1))
		_, workflowEnvelope, err := fetchEventEnvelope("ext.workflow.triggered", fileID)
		Expect(err).NotTo(HaveOccurred())
		data, ok = toMap(workflowEnvelope["data"])
		Expect(ok).To(BeTrue())
		action, ok = toMap(data["action"])
		Expect(ok).To(BeTrue())
		Expect(action["type"]).To(Equal("invoke_workflow"))
		Expect(action["workflow"]).To(Equal(workflowTriggerName))
		context, ok = toMap(data["context"])
		Expect(ok).To(BeTrue())
		Expect(context["event_type"]).To(Equal("file.deleted"))

		otherPayload := map[string]any{"message": "ignored"}
		otherBytes, err := json.Marshal(otherPayload)
		Expect(err).NotTo(HaveOccurred())
		otherUpload := uploadContent(contentClient, nonMatchingFileName, "application/json", otherBytes)
		other := metadataClient.POST("/api/v1/files").
			WithJSON(map[string]any{
				"directory_id": directoryID,
				"name":         nonMatchingFileName,
				"storage_mode": otherUpload.Value("storage_mode").String().Raw(),
				"json_payload": otherUpload.Value("json_payload").Raw(),
				"checksum":     otherUpload.Value("checksum").String().Raw(),
				"size":         otherUpload.Value("size").Number().Raw(),
				"mime_type":    otherUpload.Value("mime_type").String().Raw(),
				"actor":        "trigger-actor",
			}).
			Expect().
			Status(201).
			JSON().Object()
		otherFileID := other.Value("id").String().Raw()

		Consistently(func() bool {
			_, _, err := fetchEventEnvelope("ext.webhook.triggered", otherFileID)
			if errors.Is(err, sql.ErrNoRows) {
				return true
			}
			Expect(err).NotTo(HaveOccurred())
			return false
		}, "1s", "200ms").Should(BeTrue())

		metadataClient.DELETE("/api/v1/files/"+otherFileID).
			WithHeader("X-VFS-Actor", "trigger-actor").
			Expect().
			Status(204)
	})
})

func eventCountForSubject(eventType, subjectID string) (int, error) {
	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM events WHERE type = ? AND subject_id = ?", eventType, subjectID).Scan(&count)
	return count, err
}

func fetchEventEnvelope(eventType, subjectID string) (string, map[string]any, error) {
	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return "", nil, err
	}
	defer db.Close()

	var (
		id      string
		payload []byte
	)
	row := db.QueryRow("SELECT id, payload FROM events WHERE type = ? AND subject_id = ? ORDER BY created_at DESC LIMIT 1", eventType, subjectID)
	if err := row.Scan(&id, &payload); err != nil {
		return "", nil, err
	}
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", nil, err
	}
	return id, envelope, nil
}

func toMap(value any) (map[string]any, bool) {
	m, ok := value.(map[string]any)
	return m, ok
}

func toStringSlice(value any) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		dup := make([]string, len(v))
		copy(dup, v)
		return dup, true
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result, true
	default:
		return nil, false
	}
}
