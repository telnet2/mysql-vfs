package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	hzapp "github.com/cloudwego/hertz/pkg/app"

	deps "github.com/telnet2/mysql-vfs/services/content/internal/app"
	"github.com/telnet2/mysql-vfs/services/content/internal/service"
)

type uploadRequest struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data" binding:"required"`
}

func UploadContent(ctx context.Context, c *hzapp.RequestContext) {
	var req uploadRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	payload, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_base64", err.Error())
		return
	}
	d := deps.Get()
	storage := d.Storage
	if storage == nil {
		storage = service.NewStorageService(d.Bucket, d.Config.InlineJSONMaxBytes(), d.Config.InlineJSONMediaTypes())
	}
	result, err := storage.Store(ctx, service.StoreRequest{
		Data:     payload,
		FileName: req.Name,
		MimeType: req.MimeType,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	resp := map[string]any{
		"storage_mode": result.StorageMode,
		"checksum":     result.Checksum,
		"size":         result.Size,
		"mime_type":    result.MimeType,
	}
	if result.BlobKey != nil {
		resp["blob_key"] = *result.BlobKey
	}
	if len(result.JSONPayload) > 0 {
		resp["json_payload"] = json.RawMessage(result.JSONPayload)
	}
	c.JSON(http.StatusCreated, resp)
}

func DownloadContent(ctx context.Context, c *hzapp.RequestContext) {
	blobKey := strings.TrimSpace(c.Param("pointer"))
	if blobKey != "" && blobKey[0] == '/' {
		blobKey = blobKey[1:]
	}
	if decoded, err := url.PathUnescape(blobKey); err == nil {
		blobKey = decoded
	}
	if blobKey == "" {
		respondError(c, http.StatusBadRequest, "missing_pointer", "blob pointer is required")
		return
	}
	d := deps.Get()
	storage := d.Storage
	if storage == nil {
		storage = service.NewStorageService(d.Bucket, d.Config.InlineJSONMaxBytes(), d.Config.InlineJSONMediaTypes())
	}
	result, err := storage.Load(ctx, blobKey)
	if err != nil {
		respondError(c, http.StatusNotFound, "content_not_found", err.Error())
		return
	}
	if string(c.QueryArgs().Peek("format")) == "base64" {
		c.JSON(http.StatusOK, map[string]any{
			"data":     base64.StdEncoding.EncodeToString(result.Data),
			"checksum": result.Checksum,
			"size":     result.Size,
		})
		return
	}
	c.SetContentType("application/octet-stream")
	c.Response.Header.Set("X-Checksum", result.Checksum)
	c.Status(http.StatusOK)
	c.Write(result.Data)
}

func respondError(c *hzapp.RequestContext, status int, code, message string) {
	c.JSON(status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
