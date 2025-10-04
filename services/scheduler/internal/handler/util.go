package handler

import hzapp "github.com/cloudwego/hertz/pkg/app"

func respondJSON(c *hzapp.RequestContext, status int, payload any) {
	c.JSON(status, payload)
}

func respondError(c *hzapp.RequestContext, status int, code, message string) {
	c.JSON(status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
