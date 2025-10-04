package handler

import (
	"context"
	"net/http"

	hzapp "github.com/cloudwego/hertz/pkg/app"
	"gorm.io/gorm"

	deps "github.com/telnet2/mysql-vfs/services/webhook/internal/app"
	"github.com/telnet2/mysql-vfs/services/webhook/internal/service"
)

type createConfigRequest struct {
	ScopeType  string   `json:"scope_type" binding:"required"`
	ScopeID    *string  `json:"scope_id"`
	EventTypes []string `json:"event_types"`
	TargetURL  string   `json:"target_url" binding:"required"`
	Secret     string   `json:"secret"`
}

type ackRequest struct {
	DeliveryID   string `json:"delivery_id" binding:"required"`
	Success      bool   `json:"success"`
	ResponseBody string `json:"response_body"`
	ErrorMessage string `json:"error_message"`
}

func CreateConfig(ctx context.Context, c *hzapp.RequestContext) {
	var req createConfigRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	svc := service.NewConfigService(deps.Get().DB)
	cfg, err := svc.Create(ctx, service.CreateConfigInput{
		ScopeType:  req.ScopeType,
		ScopeID:    req.ScopeID,
		EventTypes: req.EventTypes,
		TargetURL:  req.TargetURL,
		Secret:     req.Secret,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}
	respondJSON(c, http.StatusCreated, cfg)
}

func ListConfigs(ctx context.Context, c *hzapp.RequestContext) {
	svc := service.NewConfigService(deps.Get().DB)
	configs, err := svc.List(ctx)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	respondJSON(c, http.StatusOK, map[string]any{"configs": configs})
}

func AckDelivery(ctx context.Context, c *hzapp.RequestContext) {
	var req ackRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	svc := service.NewJobService(deps.Get().DB)
	if err := svc.Ack(ctx, service.AckInput{
		DeliveryID:   req.DeliveryID,
		Success:      req.Success,
		Response:     req.ResponseBody,
		ErrorMessage: req.ErrorMessage,
	}); err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "delivery_not_found", err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, "ack_failed", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}
