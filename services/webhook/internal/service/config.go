package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type ConfigService struct {
	DB *gorm.DB
}

type CreateConfigInput struct {
	ScopeType  string
	ScopeID    *string
	EventTypes []string
	TargetURL  string
	Secret     string
}

type ConfigDTO struct {
	ID         string   `json:"id"`
	ScopeType  string   `json:"scope_type"`
	ScopeID    *string  `json:"scope_id"`
	EventTypes []string `json:"event_types"`
	TargetURL  string   `json:"target_url"`
	Secret     string   `json:"secret"`
}

func NewConfigService(db *gorm.DB) *ConfigService {
	return &ConfigService{DB: db}
}

func (s *ConfigService) Create(ctx context.Context, in CreateConfigInput) (ConfigDTO, error) {
	eventTypes := strings.Join(in.EventTypes, ",")
	cfg := db.WebhookConfig{
		ID:         uuid.NewString(),
		ScopeType:  in.ScopeType,
		ScopeID:    in.ScopeID,
		EventTypes: eventTypes,
		TargetURL:  in.TargetURL,
		Secret:     in.Secret,
	}
	if err := s.DB.WithContext(ctx).Create(&cfg).Error; err != nil {
		return ConfigDTO{}, err
	}
	return mapConfig(cfg), nil
}

func (s *ConfigService) List(ctx context.Context) ([]ConfigDTO, error) {
	var configs []db.WebhookConfig
	if err := s.DB.WithContext(ctx).Order("created_at ASC").Find(&configs).Error; err != nil {
		return nil, err
	}
	result := make([]ConfigDTO, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, mapConfig(cfg))
	}
	return result, nil
}

func mapConfig(cfg db.WebhookConfig) ConfigDTO {
	types := []string{}
	if strings.TrimSpace(cfg.EventTypes) != "" {
		for _, evt := range strings.Split(cfg.EventTypes, ",") {
			evt = strings.TrimSpace(evt)
			if evt != "" {
				types = append(types, evt)
			}
		}
	}
	return ConfigDTO{
		ID:         cfg.ID,
		ScopeType:  cfg.ScopeType,
		ScopeID:    cfg.ScopeID,
		EventTypes: types,
		TargetURL:  cfg.TargetURL,
		Secret:     cfg.Secret,
	}
}
