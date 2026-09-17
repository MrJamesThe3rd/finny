package paperless

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

// blockedPrefixes are ranges netip classifies as global unicast, so IsPrivate
// and IsLoopback both miss them. See knowledge-base.md §4.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT, and Tailscale's default range
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("::/96"),           // IPv4-compatible IPv6
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64
	netip.MustParsePrefix("2002::/16"),       // 6to4
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
}

// Transports are shared process-wide. A backend is constructed per operation
// (see document.Service), so building one per backend would leak a connection
// pool and its goroutines on every download.
var (
	guardedTransport = sync.OnceValue(func() http.RoundTripper {
		t := baseTransport()
		t.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: denyPrivateAddr}).DialContext
		// A proxy would make Control see only the proxy's address, with the real
		// destination travelling inside CONNECT — voiding the guard entirely.
		t.Proxy = nil

		return t
	})

	// directTransport keeps the environment proxy and no address guard, for
	// deployments that have opted into reaching their own network.
	directTransport = sync.OnceValue(func() http.RoundTripper { return baseTransport() })
)

// baseTransport clones the stdlib default so timeouts, HTTP/2 and idle-pool
// tuning are inherited rather than silently dropped.
func baseTransport() *http.Transport {
	return http.DefaultTransport.(*http.Transport).Clone() //nolint:errcheck,forcetypeassert
}

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

// newBackend is the internal constructor; tests pass short poll intervals and
// allowPrivate, since httptest servers listen on loopback.
func newBackend(cfg Config, allowPrivate bool, pollInterval, pollTimeout time.Duration) *Backend {
	transport := guardedTransport()
	if allowPrivate {
		transport = directTransport()
	}

	return &Backend{
		client:       &http.Client{Timeout: 30 * time.Second, Transport: transport},
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		token:        cfg.Token,
		pollInterval: pollInterval,
		pollTimeout:  pollTimeout,
	}
}

// Backends constructs Paperless backends. AllowPrivate lifts the address guard
// for deployments whose Paperless is on localhost or a LAN.
type Backends struct {
	AllowPrivate bool
}

func (b Backends) New(raw json.RawMessage) (document.Backend, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("paperless: invalid config: %w", err)
	}

	if err := validateBaseURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("paperless: %w", err)
	}

	return newBackend(cfg, b.AllowPrivate, defaultPollInterval, defaultPollTimeout), nil
}

// validateBaseURL is the cheap first gate; denyPrivateAddr is the one that holds.
func validateBaseURL(raw string) error {
	if raw == "" {
		return ErrBaseURLRequired
	}

	// url.Parse's error text quotes the URL, so it is reported as a class only.
	u, err := url.Parse(raw)
	if err != nil {
		return ErrBaseURLInvalid
	}

	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return ErrBaseURLInvalid
	}

	return nil
}

// denyPrivateAddr refuses connections to addresses that are not publicly
// routable. It runs per connection after DNS resolution, so a hostname that
// resolves inward, and any redirect, are both caught — which URL validation
// alone cannot do.
func denyPrivateAddr(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrAddressUnparseable
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return ErrAddressUnparseable
	}

	// Unmap first: ::ffff:127.0.0.1 must be judged as 127.0.0.1.
	addr = addr.Unmap()

	// IsGlobalUnicast already excludes loopback, link-local, multicast,
	// unspecified and broadcast, so those are not re-tested here.
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return ErrNonPublicAddress
	}

	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return ErrNonPublicAddress
		}
	}

	return nil
}

// requestError replaces a transport error with an opaque one: every layer of it
// carries the configured host, so unwrapping is not enough (see knowledge-base.md
// §4). The guard's own errors name no address and are kept as the diagnosis.
func requestError(op string, err error) error {
	if errors.Is(err, ErrNonPublicAddress) || errors.Is(err, ErrAddressUnparseable) {
		return fmt.Errorf("paperless: %s: %w", op, err)
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("paperless: %s: %w", op, err)
	}

	return fmt.Errorf("paperless: %s: %w", op, ErrRequestFailed)
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

	mw.Close() //nolint:errcheck

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.baseURL+"/api/documents/post_document/", &body)
	if err != nil {
		return "", fmt.Errorf("paperless: creating upload request: %w", err)
	}

	req.Header.Set("Content-Type", mw.FormDataContentType())
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", requestError("upload request", err)
	}

	defer resp.Body.Close() //nolint:errcheck

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
				return "", ErrNoDocumentID
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
//
// Every branch is validated: these values come from the remote server, and an
// unusable one becomes a key that uploads cleanly and never downloads.
func (b *Backend) resolveDocID(task *taskResponse) string {
	if task.ResultData != nil && task.ResultData.DuplicateOf != nil {
		return documentIDFromInt(*task.ResultData.DuplicateOf)
	}

	if task.ResultData != nil && task.ResultData.DocumentID != nil {
		return documentIDFromInt(*task.ResultData.DocumentID)
	}

	if len(task.RelatedDocumentIDs) > 0 {
		return documentIDFromInt(task.RelatedDocumentIDs[0])
	}

	if isDocumentID(task.RelatedDocument) {
		return task.RelatedDocument
	}

	return ""
}

func documentIDFromInt(id int) string {
	if id < 0 {
		return ""
	}

	return strconv.Itoa(id)
}

// isDocumentID reports whether key is a bare Paperless document ID. Leading
// zeros are rejected so one document cannot be keyed under several spellings.
func isDocumentID(key string) bool {
	if key == "" || (len(key) > 1 && key[0] == '0') {
		return false
	}

	_, err := strconv.ParseUint(key, 10, 64)

	return err == nil
}

func (b *Backend) fetchTask(ctx context.Context, taskID string) (*taskResponse, error) {
	endpoint := fmt.Sprintf("%s/api/tasks/?task_id=%s", b.baseURL, url.QueryEscape(taskID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("paperless: creating task request: %w", err)
	}

	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, requestError("task request", err)
	}

	defer resp.Body.Close() //nolint:errcheck

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
	// Keys predating this validation may be arbitrary strings, so check here as
	// well as on the way in.
	if !isDocumentID(key) {
		return nil, ErrInvalidDocumentKey
	}

	endpoint := fmt.Sprintf("%s/api/documents/%s/download/", b.baseURL, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("paperless: creating download request: %w", err)
	}

	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, requestError("download request", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close() //nolint:errcheck

		return nil, fmt.Errorf("paperless: download returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// Delete is not supported; document deletions in Paperless are managed
// through the Paperless UI, not the API integration.
func (b *Backend) Delete(_ context.Context, _ string) error {
	return ErrDeleteNotSupported
}

func (b *Backend) setAuth(req *http.Request) {
	if b.token != "" {
		req.Header.Set("Authorization", "Token "+b.token)
	}
}
