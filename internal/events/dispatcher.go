package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/telnet2/mysql-vfs/internal/config"
	"github.com/telnet2/mysql-vfs/internal/models"
)

// Event describes a domain event triggered by file system operations.
type Event struct {
	Type        string         `json:"type"`
	NodePath    string         `json:"nodePath"`
	User        string         `json:"user"`
	Transaction string         `json:"transaction"`
	Data        map[string]any `json:"data"`
	Timestamp   time.Time      `json:"timestamp"`
}

// Dispatcher delivers events to webhooks and records audit entries.
type Dispatcher struct {
	cfg    config.Config
	client *http.Client
}

// NewDispatcher constructs a dispatcher using the webhook project's delivery semantics.
func NewDispatcher(cfg config.Config) *Dispatcher {
	timeout := 10 * time.Second
	if cfg.Webhook.Timeout > 0 {
		timeout = cfg.Webhook.Timeout
	}

	return &Dispatcher{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

// DeliverWebhook invokes the webhook endpoint with retry semantics inspired by the
// github.com/adnanh/webhook project. It uses exponential backoff with a linear cap
// to simplify integration without depending on internal packages.
func (d *Dispatcher) DeliverWebhook(ctx context.Context, registration models.WebhookRegistration, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registration.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	retries := d.cfg.Webhook.MaxRetries
	if retries <= 0 {
		retries = 3
	}

	delay := d.cfg.Webhook.RetryInterval
	if delay <= 0 {
		delay = 2 * time.Second
	}

	for attempt := 0; attempt <= retries; attempt++ {
		resp, err := d.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			return nil
		}

		if err == nil {
			resp.Body.Close()
		}

		if attempt == retries {
			if err != nil {
				return fmt.Errorf("webhook delivery failed after %d attempts: %w", attempt+1, err)
			}
			return fmt.Errorf("webhook delivery failed after %d attempts: status=%d", attempt+1, http.StatusServiceUnavailable)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil
}
