package handler

import (
	"net/http"
	"strings"

	hzapp "github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

func getRequestID(c *hzapp.RequestContext) string {
	if v := strings.TrimSpace(string(c.GetHeader(requestIDHeader))); v != "" {
		return v
	}
	id := uuid.NewString()
	c.Response.Header.Set(requestIDHeader, id)
	return id
}

func respondError(c *hzapp.RequestContext, status int, code string, message string) {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	c.JSON(status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func respondJSON(c *hzapp.RequestContext, status int, payload any) {
	if status == 0 {
		status = http.StatusOK
	}
	c.JSON(status, payload)
}
