package paperless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MrJamesThe3rd/finny/internal/document"
)

// Config holds the Paperless-ngx connection details stored in document_backends.config.
type Config struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

// BuildConfig serialises a Paperless config into raw JSON for storage.
func BuildConfig(baseURL, token string) (json.RawMessage, error) {
	return json.Marshal(Config{BaseURL: baseURL, Token: token})
}

type Backend struct {
	client  *http.Client
	baseURL string
	token   string
}

// NewFromConfig creates a Paperless Backend from the JSONB config stored in the DB.
func NewFromConfig(raw json.RawMessage) (document.Backend, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("paperless: invalid config: %w", err)
	}

	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("paperless: base_url is required")
	}

	return &Backend{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.Token,
	}, nil
}

func (b *Backend) Type() string { return "paperless" }

// Download retrieves a document by its Paperless numeric ID.
func (b *Backend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/documents/%s/download/", b.baseURL, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if b.token != "" {
		req.Header.Set("Authorization", "Token "+b.token)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d for key %s", resp.StatusCode, key)
	}

	return resp.Body, nil
}

// Upload is not yet implemented; added in Phase 1 (document upload).
func (b *Backend) Upload(_ context.Context, _ string, _ io.Reader) (string, error) {
	return "", fmt.Errorf("paperless: upload not yet implemented")
}

// Delete is not yet implemented; added in Phase 1.
func (b *Backend) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("paperless: delete not yet implemented")
}
