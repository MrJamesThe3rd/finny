package paperless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
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

const (
	defaultPollInterval = 2 * time.Second
	defaultPollTimeout  = 2 * time.Minute

	taskSuccess = "success"
	taskFailure = "failure"
)

type resultData struct {
	DocumentID  *int `json:"document_id"`
	DuplicateOf *int `json:"duplicate_of"`
}

type taskResponse struct {
	TaskID             string      `json:"task_id"`
	Status             string      `json:"status"`
	Result             string      `json:"result"`
	ResultData         *resultData `json:"result_data"`
	RelatedDocument    string      `json:"related_document"`
	RelatedDocumentIDs []int       `json:"related_document_ids"`
}

type Backend struct {
	client       *http.Client
	baseURL      string
	token        string
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// newBackend is the internal constructor; tests pass short poll intervals.
func newBackend(cfg Config, pollInterval, pollTimeout time.Duration) *Backend {
	return &Backend{
		client:       &http.Client{Timeout: 30 * time.Second},
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		token:        cfg.Token,
		pollInterval: pollInterval,
		pollTimeout:  pollTimeout,
	}
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

	return newBackend(cfg, defaultPollInterval, defaultPollTimeout), nil
}

func (b *Backend) Type() string { return "paperless" }

// Upload sends the file to Paperless and polls until consumption completes.
// If Paperless reports a duplicate, the existing document's ID is returned
// transparently — the caller can attach it without knowing it already existed.
func (b *Backend) Upload(ctx context.Context, filename string, content io.Reader) (string, error) {
	taskID, err := b.postDocument(ctx, filename, content)
	if err != nil {
		return "", err
	}

	return b.pollTask(ctx, taskID)
}

func (b *Backend) postDocument(ctx context.Context, filename string, content io.Reader) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	fw, err := mw.CreateFormFile("document", filename)
	if err != nil {
		return "", fmt.Errorf("paperless: building upload form: %w", err)
	}

	if _, err := io.Copy(fw, content); err != nil {
		return "", fmt.Errorf("paperless: writing file content: %w", err)
	}

	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.baseURL+"/api/documents/post_document/", &body)
	if err != nil {
		return "", fmt.Errorf("paperless: creating upload request: %w", err)
	}

	req.Header.Set("Content-Type", mw.FormDataContentType())
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("paperless: upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("paperless: upload returned status %d", resp.StatusCode)
	}

	// Paperless returns the task UUID as a bare JSON string.
	var taskID string
	if err := json.NewDecoder(resp.Body).Decode(&taskID); err != nil {
		return "", fmt.Errorf("paperless: decoding task ID: %w", err)
	}

	return taskID, nil
}

func (b *Backend) pollTask(ctx context.Context, taskID string) (string, error) {
	deadline := time.Now().Add(b.pollTimeout)

	for {
		task, err := b.fetchTask(ctx, taskID)
		if err != nil {
			return "", err
		}

		switch task.Status {
		case taskSuccess:
			docID := b.resolveDocID(task)
			if docID == "" {
				return "", fmt.Errorf("paperless: task succeeded but returned no document ID")
			}
			return docID, nil

		case taskFailure:
			return "", fmt.Errorf("paperless: document consumption failed: %s", task.Result)
		}

		// PENDING / STARTED / RETRY — keep polling.
		if time.Now().After(deadline) {
			return "", fmt.Errorf("paperless: timed out waiting for document %s", taskID)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(b.pollInterval):
		}
	}
}

// resolveDocID extracts the document ID from a completed task.
// In v10, duplicates are reported as success with result_data.duplicate_of
// instead of a failure. We prefer that, then fall back to result_data.document_id,
// related_document_ids, and finally related_document (legacy).
func (b *Backend) resolveDocID(task *taskResponse) string {
	if task.ResultData != nil && task.ResultData.DuplicateOf != nil {
		return strconv.Itoa(*task.ResultData.DuplicateOf)
	}
	if task.ResultData != nil && task.ResultData.DocumentID != nil {
		return strconv.Itoa(*task.ResultData.DocumentID)
	}
	if len(task.RelatedDocumentIDs) > 0 {
		return strconv.Itoa(task.RelatedDocumentIDs[0])
	}
	return task.RelatedDocument
}

func (b *Backend) fetchTask(ctx context.Context, taskID string) (*taskResponse, error) {
	url := fmt.Sprintf("%s/api/tasks/?task_id=%s", b.baseURL, taskID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("paperless: creating task request: %w", err)
	}

	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paperless: task request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("paperless: task API returned status %d", resp.StatusCode)
	}

	var tasks []taskResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("paperless: decoding task response: %w", err)
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("paperless: task %s not found", taskID)
	}

	return &tasks[0], nil
}

// Download retrieves a document by its Paperless numeric ID.
func (b *Backend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/documents/%s/download/", b.baseURL, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	b.setAuth(req)

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

// Delete is not supported; document deletions in Paperless are managed
// through the Paperless UI, not the API integration.
func (b *Backend) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("paperless: delete not supported")
}

func (b *Backend) setAuth(req *http.Request) {
	if b.token != "" {
		req.Header.Set("Authorization", "Token "+b.token)
	}
}
