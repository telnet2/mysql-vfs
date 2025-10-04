package vfsclient

import (
	"net/http"
	"time"
)

// Client is the main VFS client that provides access to both metadata and content services.
type Client struct {
	Metadata *MetadataClient
	Content  *ContentClient
}

// Config holds configuration for creating a VFS client.
type Config struct {
	MetadataURL string
	ContentURL  string
	Actor       string
	HTTPClient  *http.Client
	Timeout     time.Duration
}

// NewClient creates a new VFS client with the given configuration.
func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 15 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	return &Client{
		Metadata: NewMetadataClient(cfg.MetadataURL, cfg.Actor, httpClient),
		Content:  NewContentClient(cfg.ContentURL, httpClient),
	}
}
