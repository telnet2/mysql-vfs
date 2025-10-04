package handler

import (
	"context"
	"net/http"

	hzapp "github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"gorm.io/gorm"

	deps "github.com/telnet2/mysql-vfs/services/scheduler/internal/app"
	"github.com/telnet2/mysql-vfs/services/scheduler/internal/service"
)

type registerCronRequest struct {
	DirectoryID string  `json:"directory_id" binding:"required"`
	CronExpr    string  `json:"cron_expr" binding:"required"`
	Payload     string  `json:"payload"`
	Timezone    *string `json:"timezone"`
}

func RegisterCron(ctx context.Context, c *hzapp.RequestContext) {
	var req registerCronRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	requestID := uuid.NewString()
	svc := service.NewCronService(deps.Get().DB)
	job, err := svc.Register(ctx, service.RegisterInput{
		DirectoryID: req.DirectoryID,
		CronExpr:    req.CronExpr,
		Payload:     req.Payload,
		Timezone:    req.Timezone,
		RequestID:   requestID,
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, "register_failed", err.Error())
		return
	}
	respondJSON(c, http.StatusCreated, job)
}

func ListCrons(ctx context.Context, c *hzapp.RequestContext) {
	directoryID := string(c.QueryArgs().Peek("directory_id"))
	svc := service.NewCronService(deps.Get().DB)
	jobs, err := svc.List(ctx, directoryID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	respondJSON(c, http.StatusOK, map[string]any{"cron_jobs": jobs})
}

type triggerRequest struct {
	ExecutionKey string `json:"execution_key"`
}

func TriggerCron(ctx context.Context, c *hzapp.RequestContext) {
	cronID := c.Param("id")
	var req triggerRequest
	if err := c.BindAndValidate(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if req.ExecutionKey == "" {
		req.ExecutionKey = uuid.NewString()
	}
	svc := service.NewCronService(deps.Get().DB)
	if err := svc.Trigger(ctx, service.TriggerInput{
		CronJobID:    cronID,
		ExecutionKey: req.ExecutionKey,
		RequestID:    uuid.NewString(),
	}); err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "cron_not_found", err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, "trigger_failed", err.Error())
		return
	}
	c.Status(http.StatusAccepted)
}
