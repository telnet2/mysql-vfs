package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	hzapp "github.com/cloudwego/hertz/pkg/app"
	deps "github.com/telnet2/mysql-vfs/services/metadata/internal/app"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/service"
)

const (
	storageModeInlineJSON = "inline_json"
	storageModeBlob       = "blob"
	storageModeS3Blob     = "s3_blob"
)

type createDirectoryRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
}

type listDirectoryQuery struct {
	ParentID  string `query:"parent_id"`
	Recursive bool   `query:"recursive"`
}

type updateDirectoryRequest struct {
	Name     *string `json:"name"`
	ParentID *string `json:"parent_id"`
	Version  *int64  `json:"version"`
}

type createFileRequest struct {
	DirectoryID  string           `json:"directory_id"`
	Name         string           `json:"name"`
	OriginFileID *string          `json:"origin_file_id"`
	StorageMode  string           `json:"storage_mode"`
	BlobKey      *string          `json:"blob_key"`
	JSONPayload  *json.RawMessage `json:"json_payload"`
	Metadata     map[string]any   `json:"metadata"`
	Checksum     *string          `json:"checksum"`
	Size         *int64           `json:"size"`
	MimeType     *string          `json:"mime_type"`
	Actor        string           `json:"actor"`
}

type updateFileRequest struct {
	Name        *string          `json:"name"`
	DirectoryID *string          `json:"directory_id"`
	StorageMode *string          `json:"storage_mode"`
	BlobKey     *string          `json:"blob_key"`
	JSONPayload *json.RawMessage `json:"json_payload"`
	Metadata    map[string]any   `json:"metadata"`
	Checksum    *string          `json:"checksum"`
	Size        *int64           `json:"size"`
	MimeType    *string          `json:"mime_type"`
	Version     *int64           `json:"version"`
	Actor       string           `json:"actor"`
}

type resolvePolicyQuery struct {
	DirectoryID string `query:"directory_id"`
	Path        string `query:"path"`
	Type        string `query:"type"`
}

func CreateDirectory(ctx context.Context, c *hzapp.RequestContext) {
	var req createDirectoryRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	svc := service.NewDirectoryService(deps.Get().DB)
	dto, err := svc.Create(ctx, service.CreateDirectoryInput{
		Name:      req.Name,
		ParentID:  req.ParentID,
		RequestID: getRequestID(c),
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	respondJSON(c, http.StatusCreated, dto)
}

func ListDirectory(ctx context.Context, c *hzapp.RequestContext) {
	var query listDirectoryQuery
	if err := c.BindAndValidate(&query); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}

	svc := service.NewDirectoryService(deps.Get().DB)
	var parentID *string
	if query.ParentID != "" {
		parentID = &query.ParentID
	}
	output, err := svc.List(ctx, service.ListDirectoryInput{ParentID: parentID, Recursive: query.Recursive})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	respondJSON(c, http.StatusOK, output)
}

func GetDirectory(ctx context.Context, c *hzapp.RequestContext) {
	id := c.Param("id")
	svc := service.NewDirectoryService(deps.Get().DB)
	dto, err := svc.GetByID(ctx, id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, dto)
}

func ResolveDirectory(ctx context.Context, c *hzapp.RequestContext) {
	path := string(c.QueryArgs().Peek("path"))
	if path == "" {
		respondError(c, http.StatusBadRequest, "missing_path", "path query parameter is required")
		return
	}
	svc := service.NewDirectoryService(deps.Get().DB)
	dto, err := svc.ResolvePath(ctx, path)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, dto)
}

func UpdateDirectory(ctx context.Context, c *hzapp.RequestContext) {
	id := c.Param("id")
	var req updateDirectoryRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	svc := service.NewDirectoryService(deps.Get().DB)
	dto, err := svc.Update(ctx, service.UpdateDirectoryInput{
		DirectoryID:     id,
		NewName:         req.Name,
		NewParentID:     req.ParentID,
		ExpectedVersion: req.Version,
		RequestID:       getRequestID(c),
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, dto)
}

func DeleteDirectory(ctx context.Context, c *hzapp.RequestContext) {
	id := c.Param("id")
	force := string(c.QueryArgs().Peek("force")) == "true"
	versionParam := string(c.QueryArgs().Peek("version"))
	var version *int64
	if versionParam != "" {
		if v, err := strconv.ParseInt(versionParam, 10, 64); err == nil {
			version = &v
		} else {
			respondError(c, http.StatusBadRequest, "invalid_version", "version must be an integer")
			return
		}
	}
	svc := service.NewDirectoryService(deps.Get().DB)
	if err := svc.Delete(ctx, service.DeleteDirectoryInput{
		DirectoryID:     id,
		ExpectedVersion: version,
		RequestID:       getRequestID(c),
		Force:           force,
	}); err != nil {
		if errors.Is(err, service.ErrDirectoryNotFound) {
			respondError(c, http.StatusNotFound, "directory_not_found", err.Error())
			return
		}
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func CreateFile(ctx context.Context, c *hzapp.RequestContext) {
	var req createFileRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	actor := resolveActor(c, req.Actor)
	versionData, err := buildVersionData(req.StorageMode, req.BlobKey, req.JSONPayload, req.Metadata, req.Checksum, req.Size, req.MimeType, actor)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_storage", err.Error())
		return
	}

	dependencies := deps.Get()
	svc := service.NewFileService(dependencies.DB, dependencies.PolicyRegistry, dependencies.PolicyEvaluator, dependencies.PolicyValidator, dependencies.PolicyTriggerEngine)
	dto, err := svc.Create(ctx, service.CreateFileInput{
		DirectoryID:  req.DirectoryID,
		Name:         req.Name,
		OriginFileID: req.OriginFileID,
		VersionData:  versionData,
		RequestID:    getRequestID(c),
		Actor:        actor,
	})
	if err != nil {
		if errors.Is(err, service.ErrDirectoryNotFound) {
			respondError(c, http.StatusBadRequest, "directory_not_found", err.Error())
			return
		}
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusCreated, dto)
}

func GetFile(ctx context.Context, c *hzapp.RequestContext) {
	id := c.Param("id")
	dep := deps.Get()
	svc := service.NewFileService(dep.DB, dep.PolicyRegistry, dep.PolicyEvaluator, dep.PolicyValidator, dep.PolicyTriggerEngine)
	dto, err := svc.GetByID(ctx, id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, dto)
}

func ResolveFile(ctx context.Context, c *hzapp.RequestContext) {
	path := string(c.QueryArgs().Peek("path"))
	if path == "" {
		respondError(c, http.StatusBadRequest, "missing_path", "path query parameter is required")
		return
	}
	dep := deps.Get()
	svc := service.NewFileService(dep.DB, dep.PolicyRegistry, dep.PolicyEvaluator, dep.PolicyValidator, dep.PolicyTriggerEngine)
	dto, err := svc.ResolvePath(ctx, path)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, dto)
}

func ListFileVersions(ctx context.Context, c *hzapp.RequestContext) {
	id := c.Param("id")
	dep := deps.Get()
	svc := service.NewFileService(dep.DB, dep.PolicyRegistry, dep.PolicyEvaluator, dep.PolicyValidator, dep.PolicyTriggerEngine)
	versions, err := svc.ListVersions(ctx, id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, versions)
}

func UpdateFile(ctx context.Context, c *hzapp.RequestContext) {
	id := c.Param("id")
	var req updateFileRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	actor := resolveActor(c, req.Actor)
	var versionData *service.FileVersionData
	if req.StorageMode != nil || req.BlobKey != nil || req.JSONPayload != nil || req.Metadata != nil || req.Checksum != nil || req.Size != nil || req.MimeType != nil {
		if req.StorageMode == nil {
			respondError(c, http.StatusBadRequest, "invalid_storage", "storage_mode is required when updating content")
			return
		}
		vd, err := buildVersionData(*req.StorageMode, req.BlobKey, req.JSONPayload, req.Metadata, req.Checksum, req.Size, req.MimeType, actor)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid_storage", err.Error())
			return
		}
		versionData = &vd
	}

	dep := deps.Get()
	svc := service.NewFileService(dep.DB, dep.PolicyRegistry, dep.PolicyEvaluator, dep.PolicyValidator, dep.PolicyTriggerEngine)
	dto, err := svc.Update(ctx, service.UpdateFileInput{
		FileID:          id,
		NewName:         req.Name,
		NewDirectoryID:  req.DirectoryID,
		VersionData:     versionData,
		ExpectedVersion: req.Version,
		RequestID:       getRequestID(c),
		Actor:           actor,
	})
	if err != nil {
		if errors.Is(err, service.ErrDirectoryNotFound) {
			respondError(c, http.StatusBadRequest, "directory_not_found", err.Error())
			return
		}
		handleServiceError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, dto)
}

func DeleteFile(ctx context.Context, c *hzapp.RequestContext) {
	id := c.Param("id")
	versionParam := string(c.QueryArgs().Peek("version"))
	var version *int64
	if versionParam != "" {
		if v, err := strconv.ParseInt(versionParam, 10, 64); err == nil {
			version = &v
		} else {
			respondError(c, http.StatusBadRequest, "invalid_version", "version must be an integer")
			return
		}
	}
	dep := deps.Get()
	svc := service.NewFileService(dep.DB, dep.PolicyRegistry, dep.PolicyEvaluator, dep.PolicyValidator, dep.PolicyTriggerEngine)
	err := svc.Delete(ctx, service.DeleteFileInput{
		FileID:          id,
		ExpectedVersion: version,
		RequestID:       getRequestID(c),
		Actor:           resolveActor(c, ""),
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func ResolvePolicy(ctx context.Context, c *hzapp.RequestContext) {
	var query resolvePolicyQuery
	if err := c.BindAndValidate(&query); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}

	var typeFilter *policy.Type
	if strings.TrimSpace(query.Type) != "" {
		typ, ok := policy.ParseType(query.Type)
		if !ok {
			respondError(c, http.StatusBadRequest, "invalid_type", "unsupported policy type")
			return
		}
		typeFilter = &typ
	}

	dep := deps.Get()
	dirSvc := service.NewDirectoryService(dep.DB)
	fileSvc := service.NewFileService(dep.DB, dep.PolicyRegistry, dep.PolicyEvaluator, dep.PolicyValidator, dep.PolicyTriggerEngine)
	resolver := service.NewPolicyResolver(dep.PolicyRegistry, dirSvc, fileSvc)
	result, err := resolver.Resolve(ctx, query.DirectoryID, query.Path, typeFilter)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	respondJSON(c, http.StatusOK, map[string]any{
		"directory_id": result.DirectoryID,
		"manifests":    result.Manifests,
		"principals":   result.Principals,
	})
}

func handleServiceError(c *hzapp.RequestContext, err error) {
	var schemaErr *policy.SchemaValidationError
	if errors.As(err, &schemaErr) {
		details := make([]map[string]any, 0, len(schemaErr.Failures))
		for _, failure := range schemaErr.Failures {
			details = append(details, map[string]any{
				"manifest": failure.Manifest.Name,
				"errors":   failure.Errors,
			})
		}
		respondJSON(c, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"code":    "schema_validation_failed",
				"message": schemaErr.Error(),
				"details": details,
			},
		})
		return
	}

	switch {
	case errors.Is(err, service.ErrDirectoryNotFound):
		respondError(c, http.StatusNotFound, "directory_not_found", err.Error())
	case errors.Is(err, service.ErrFileNotFound):
		respondError(c, http.StatusNotFound, "file_not_found", err.Error())
	case errors.Is(err, service.ErrNameConflict):
		respondError(c, http.StatusConflict, "name_conflict", err.Error())
	case errors.Is(err, service.ErrInvalidRequest):
		respondError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, service.ErrVersionConflict):
		respondError(c, http.StatusConflict, "version_conflict", err.Error())
	case errors.Is(err, service.ErrPolicyForbidden):
		respondError(c, http.StatusForbidden, "policy_forbidden", "operation forbidden by policy")
	case errors.Is(err, service.ErrSchemaValidation):
		respondError(c, http.StatusBadRequest, "schema_validation_failed", err.Error())
	default:
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
}

func buildVersionData(mode string, blobKey *string, jsonPayload *json.RawMessage, metadata map[string]any, checksum *string, size *int64, mimeType *string, actor string) (service.FileVersionData, error) {
	mode = strings.TrimSpace(mode)
	vd := service.FileVersionData{
		StorageMode: mode,
		Metadata:    metadata,
		Checksum:    checksum,
		Size:        size,
		MimeType:    mimeType,
		Actor:       actor,
	}
	switch mode {
	case storageModeInlineJSON:
		if jsonPayload == nil {
			return vd, errors.New("json_payload required for inline_json storage")
		}
		buf := &bytes.Buffer{}
		if err := json.Compact(buf, *jsonPayload); err != nil {
			return vd, errors.New("json_payload must be valid JSON")
		}
		vd.JSONPayload = buf.Bytes()
		if vd.MimeType == nil || strings.TrimSpace(*vd.MimeType) == "" {
			mt := "application/json"
			vd.MimeType = &mt
		}
	case storageModeBlob, storageModeS3Blob:
		if blobKey != nil {
			trimmed := strings.TrimSpace(*blobKey)
			if trimmed != "" {
				key := trimmed
				vd.BlobKey = &key
			}
		}
	default:
		return vd, errors.New("unknown storage_mode")
	}
	return vd, nil
}
