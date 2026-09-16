package paperless

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBackend creates a Backend pointed at srv with a fast poll interval so
// tests complete in milliseconds instead of seconds.
func newTestBackend(srv *httptest.Server) *Backend {
	return newBackend(
		Config{BaseURL: srv.URL, Token: "test-token"},
		10*time.Millisecond,
		5*time.Second,
	)
}

func TestBackend_Upload_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/post_document/"):
			json.NewEncoder(w).Encode("task-uuid-1") //nolint:errcheck
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/":
			docID := 42
			json.NewEncoder(w).Encode([]taskResponse{{ //nolint:errcheck
				TaskID:          "task-uuid-1",
				Status:          taskSuccess,
				ResultData:      &resultData{DocumentID: &docID},
				RelatedDocument: "42",
			}})
		}
	}))
	defer srv.Close()

	key, err := newTestBackend(srv).Upload(context.Background(), "invoice.pdf", strings.NewReader("pdf"))
	require.NoError(t, err)
	assert.Equal(t, "42", key)
}

// TestBackend_Upload_Duplicate is the regression test for the reported bug:
// when Paperless detects a duplicate it returns status "success" with
// result_data.duplicate_of set to the existing document ID. Upload must return
// that ID rather than an error so the caller can still attach the document.
func TestBackend_Upload_Duplicate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/post_document/"):
			json.NewEncoder(w).Encode("task-uuid-2") //nolint:errcheck
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/":
			dup := 99
			json.NewEncoder(w).Encode([]taskResponse{{ //nolint:errcheck
				TaskID:             "task-uuid-2",
				Status:             taskSuccess,
				ResultData:         &resultData{DuplicateOf: &dup},
				RelatedDocumentIDs: []int{99},
			}})
		}
	}))
	defer srv.Close()

	key, err := newTestBackend(srv).Upload(context.Background(), "invoice.pdf", strings.NewReader("pdf"))
	require.NoError(t, err, "duplicate should be handled gracefully, not returned as an error")
	assert.Equal(t, "99", key)
}

func TestBackend_Upload_TaskFailure_NonDuplicate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/post_document/"):
			json.NewEncoder(w).Encode("task-uuid-3") //nolint:errcheck
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/":
			json.NewEncoder(w).Encode([]taskResponse{{ //nolint:errcheck
				TaskID: "task-uuid-3",
				Status: taskFailure,
				Result: "Failed to parse PDF: corrupt header",
			}})
		}
	}))
	defer srv.Close()

	_, err := newTestBackend(srv).Upload(context.Background(), "bad.pdf", strings.NewReader("not a pdf"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "corrupt header")
}

func TestBackend_Upload_PostDocumentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := newTestBackend(srv).Upload(context.Background(), "invoice.pdf", strings.NewReader("pdf"))
	require.Error(t, err)
}

func TestBackend_Upload_TaskTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/post_document/"):
			json.NewEncoder(w).Encode("task-uuid-4") //nolint:errcheck
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/":
			// Always return pending so the poll loop never exits normally.
			json.NewEncoder(w).Encode([]taskResponse{{ //nolint:errcheck
				TaskID: "task-uuid-4", Status: "pending",
			}})
		}
	}))
	defer srv.Close()

	b := newBackend(
		Config{BaseURL: srv.URL, Token: "test-token"},
		10*time.Millisecond,
		50*time.Millisecond, // very short timeout
	)
	_, err := b.Upload(context.Background(), "invoice.pdf", strings.NewReader("pdf"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestBackend_Delete_NotSupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	bk := newTestBackend(srv)
	err := bk.Delete(context.Background(), "42")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}
