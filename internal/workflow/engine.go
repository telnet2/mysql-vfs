package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/models"
)

// TransitionRule defines workflow transition metadata.
type TransitionRule struct {
	From          string         `json:"from"`
	To            []string       `json:"to"`
	Timeout       time.Duration  `json:"timeout"`
	TimeoutAction map[string]any `json:"timeoutAction"`
}

// Engine handles workflow decisions based on template configuration.
type Engine struct {
	db *gorm.DB
}

// NewEngine constructs a workflow engine.
func NewEngine(db *gorm.DB) *Engine {
	return &Engine{db: db}
}

// AllowedTransitions returns the list of paths to which a node can be moved.
func (e *Engine) AllowedTransitions(ctx context.Context, templatePath, fromPath string) ([]string, error) {
	var state models.WorkflowState
	if err := e.db.WithContext(ctx).Where("template_path = ? AND state_path = ?", templatePath, fromPath).Take(&state).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	var transitions []string
	if err := json.Unmarshal(state.AllowedMoves, &transitions); err != nil {
		return nil, err
	}

	return transitions, nil
}

// ValidateTransition checks whether a workflow move is allowed.
func (e *Engine) ValidateTransition(ctx context.Context, templatePath, fromPath, toPath string) error {
	allowed, err := e.AllowedTransitions(ctx, templatePath, fromPath)
	if err != nil {
		return err
	}

	for _, candidate := range allowed {
		if candidate == toPath {
			return nil
		}
	}

	return fmt.Errorf("transition from %s to %s is not allowed", fromPath, toPath)
}

// TimeoutAction resolves the timeout action for the given state.
func (e *Engine) TimeoutAction(ctx context.Context, templatePath, statePath string) (time.Duration, map[string]any, error) {
	var state models.WorkflowState
	if err := e.db.WithContext(ctx).Where("template_path = ? AND state_path = ?", templatePath, statePath).Take(&state).Error; err != nil {
		return 0, nil, err
	}

	var action map[string]any
	if len(state.TimeoutAction) > 0 {
		if err := json.Unmarshal(state.TimeoutAction, &action); err != nil {
			return 0, nil, err
		}
	}

	return time.Duration(state.Timeout) * time.Second, action, nil
}
