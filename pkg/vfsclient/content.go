package vfsclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ContentClient handles content service operations.
type ContentClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewContentClient creates a new content service client.
func NewContentClient(baseURL string, httpClient *http.Client) *ContentClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &ContentClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// Upload uploads content and returns storage information.
func (c *ContentClient) Upload(ctx context.Context, name, mimeType string, data []byte) (UploadResponse, error) {
	payload := map[string]any{
		"name":      name,
		"mime_type": mimeType,
		"data":      base64.StdEncoding.EncodeToString(data),
	}
	endpoint := fmt.Sprintf("%s/api/v1/content", c.baseURL)
	var out UploadResponse
	if err := c.do(ctx, http.MethodPost, endpoint, payload, &out); err != nil {
		return UploadResponse{}, err
	}
	if out.MimeType == "" {
		out.MimeType = mimeType
	}
	return out, nil
}

// Download downloads content by blob key.
func (c *ContentClient) Download(ctx context.Context, key string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/api/v1/content/%s", c.baseURL, url.PathEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, parseAPIError(resp)
	}
	return io.ReadAll(resp.Body)
}

func (c *ContentClient) do(ctx context.Context, method, endpoint string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
		reader = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
