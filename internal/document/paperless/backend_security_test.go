package paperless

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		give    string
		wantErr error
	}{
		{name: "https", give: "https://paperless.example.com"},
		{name: "http with port", give: "http://paperless.example.com:8000"},
		{name: "empty", give: "", wantErr: ErrBaseURLRequired},
		{name: "file scheme", give: "file:///etc/passwd", wantErr: ErrBaseURLInvalid},
		{name: "gopher scheme", give: "gopher://example.com", wantErr: ErrBaseURLInvalid},
		{name: "no host", give: "http://", wantErr: ErrBaseURLInvalid},
		{name: "unparseable", give: "http://a\x7fb", wantErr: ErrBaseURLInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateBaseURL(tt.give)

			if tt.wantErr == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// The error must not quote the URL it rejected: it reaches logs and, via
// createBackend, an API response.
func TestValidateBaseURL_DoesNotEchoInput(t *testing.T) {
	t.Parallel()

	const secret = "http://internal-host.corp\x7f"

	err := validateBaseURL(secret)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "internal-host.corp")
}

func TestDenyPrivateAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		give    string
		wantErr error
	}{
		{name: "public v4", give: "8.8.8.8:443"},
		{name: "public v6", give: "[2606:4700:4700::1111]:443"},
		{name: "loopback", give: "127.0.0.1:8080", wantErr: ErrNonPublicAddress},
		{name: "private 10/8", give: "10.0.0.5:80", wantErr: ErrNonPublicAddress},
		{name: "private 192.168", give: "192.168.1.10:80", wantErr: ErrNonPublicAddress},
		{name: "private 172.16", give: "172.16.0.9:80", wantErr: ErrNonPublicAddress},
		{name: "cloud metadata", give: "169.254.169.254:80", wantErr: ErrNonPublicAddress},
		{name: "v6 loopback", give: "[::1]:80", wantErr: ErrNonPublicAddress},
		{name: "v4-mapped loopback", give: "[::ffff:127.0.0.1]:80", wantErr: ErrNonPublicAddress},
		{name: "v4-mapped private", give: "[::ffff:10.0.0.1]:80", wantErr: ErrNonPublicAddress},
		{name: "unspecified", give: "0.0.0.0:80", wantErr: ErrNonPublicAddress},
		{name: "multicast", give: "224.0.0.1:80", wantErr: ErrNonPublicAddress},
		{name: "broadcast", give: "255.255.255.255:80", wantErr: ErrNonPublicAddress},
		{name: "ipv6 ula", give: "[fc00::1]:80", wantErr: ErrNonPublicAddress},
		{name: "ipv6 link local", give: "[fe80::1]:80", wantErr: ErrNonPublicAddress},

		// Global unicast by netip's reckoning, so IsPrivate/IsLoopback miss them.
		{name: "cgnat tailscale", give: "100.64.0.1:80", wantErr: ErrNonPublicAddress},
		{name: "this network", give: "0.1.2.3:80", wantErr: ErrNonPublicAddress},
		{name: "ietf assignments", give: "192.0.0.1:80", wantErr: ErrNonPublicAddress},
		{name: "benchmarking", give: "198.18.0.1:80", wantErr: ErrNonPublicAddress},
		{name: "nat64 of loopback", give: "[64:ff9b::7f00:1]:80", wantErr: ErrNonPublicAddress},
		{name: "6to4 of loopback", give: "[2002:7f00:1::]:80", wantErr: ErrNonPublicAddress},
		{name: "ipv4-compatible v6", give: "[::127.0.0.1]:80", wantErr: ErrNonPublicAddress},

		{name: "not host:port", give: "not-an-address", wantErr: ErrAddressUnparseable},
		{name: "host is a name", give: "example.com:80", wantErr: ErrAddressUnparseable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := denyPrivateAddr("tcp", tt.give, nil)

			if tt.wantErr == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// httptest listens on loopback, so a guarded backend must refuse to reach it —
// and must not have issued the request at all.
func TestBackends_New_RefusesLoopbackWhenPrivateDisallowed(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		w.Write([]byte("secret")) //nolint:errcheck
	}))
	defer srv.Close()

	backend, err := Backends{AllowPrivate: false}.New(configFor(t, srv.URL))
	require.NoError(t, err, "config is structurally valid; the guard applies at dial time")

	_, err = backend.Download(context.Background(), "42")
	require.ErrorIs(t, err, ErrNonPublicAddress)
	assert.False(t, reached.Load(), "the request must never reach the server")
}

func TestBackends_New_AllowsLoopbackWhenPermitted(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pdf bytes")) //nolint:errcheck
	}))
	defer srv.Close()

	backend, err := Backends{AllowPrivate: true}.New(configFor(t, srv.URL))
	require.NoError(t, err)

	rc, err := backend.Download(context.Background(), "42")
	require.NoError(t, err)

	defer rc.Close() //nolint:errcheck

	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "pdf bytes", string(body))
}

// New must actually call validateBaseURL — tested directly elsewhere, so
// without this the call could be deleted and the first gate would vanish green.
func TestBackends_New_RejectsInvalidBaseURL(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(Config{BaseURL: "file:///etc/passwd"})
	require.NoError(t, err)

	_, err = Backends{}.New(raw)
	require.ErrorIs(t, err, ErrBaseURLInvalid)
}

func TestBackends_New_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := Backends{}.New(json.RawMessage(`{`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config")
}

// The transport error names the host in every layer — url.Error quotes the URL,
// net.OpError and net.DNSError name host:port. Asserting against srv.URL alone
// passes trivially, because "http://" never appears in a dial error.
func TestDownload_ErrorDoesNotLeakHost(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	hostPort := srv.Listener.Addr().String()

	srv.Close() // closed, so the request fails at dial

	backend, err := Backends{AllowPrivate: true}.New(configFor(t, srv.URL))
	require.NoError(t, err)

	_, err = backend.Download(context.Background(), "42")
	require.ErrorIs(t, err, ErrRequestFailed)
	assert.NotContains(t, err.Error(), hostPort, "host:port must not survive into the error")
	assert.NotContains(t, err.Error(), srv.URL)
}

func TestIsDocumentID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want bool
	}{
		{name: "digits", give: "42", want: true},
		{name: "zero", give: "0", want: true},
		{name: "empty", give: ""},
		{name: "traversal with query", give: "../../../v1/secrets?x="},
		{name: "traversal after id", give: "42/../../admin"},
		{name: "negative", give: "-1"},
		{name: "signed", give: "+1"},
		{name: "embedded space", give: "4 2"},
		{name: "letters", give: "abc"},
		{name: "leading zero", give: "042"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isDocumentID(tt.give))
		})
	}
}

func TestResolveDocID(t *testing.T) {
	t.Parallel()

	id := func(v int) *int { return &v }

	tests := []struct {
		name string
		give taskResponse
		want string
	}{
		{name: "duplicate_of wins", give: taskResponse{ResultData: &resultData{DuplicateOf: id(99), DocumentID: id(1)}}, want: "99"},
		{name: "document_id", give: taskResponse{ResultData: &resultData{DocumentID: id(7)}}, want: "7"},
		{name: "related_document_ids only", give: taskResponse{RelatedDocumentIDs: []int{5}}, want: "5"},
		{name: "legacy related_document only", give: taskResponse{RelatedDocument: "42"}, want: "42"},
		{name: "negative document_id rejected", give: taskResponse{ResultData: &resultData{DocumentID: id(-1)}}},
		{name: "negative duplicate_of rejected", give: taskResponse{ResultData: &resultData{DuplicateOf: id(-3)}}},
		{name: "negative related id rejected", give: taskResponse{RelatedDocumentIDs: []int{-2}}},
		{name: "non-numeric related_document rejected", give: taskResponse{RelatedDocument: "../../../api/secrets?x="}},
		{name: "nothing usable", give: taskResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := &Backend{}
			require.Equal(t, tt.want, b.resolveDocID(&tt.give))
		})
	}
}

// The legacy fallback is the only path that reads a remote string as a key.
func TestBackend_Upload_UsesLegacyRelatedDocument(t *testing.T) {
	t.Parallel()

	srv := newTaskServer(t, taskResponse{Status: taskSuccess, RelatedDocument: "42"})
	defer srv.Close()

	key, err := newTestBackend(srv).Upload(context.Background(), "invoice.pdf", strings.NewReader("pdf"))
	require.NoError(t, err)
	assert.Equal(t, "42", key)
}

func TestBackend_Upload_RejectsNonNumericRelatedDocument(t *testing.T) {
	t.Parallel()

	srv := newTaskServer(t, taskResponse{Status: taskSuccess, RelatedDocument: "../../../api/secrets?x="})
	defer srv.Close()

	_, err := newTestBackend(srv).Upload(context.Background(), "invoice.pdf", strings.NewReader("pdf"))
	require.ErrorIs(t, err, ErrNoDocumentID)
}

func TestBackend_Download_RejectsNonNumericKey(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))
	defer srv.Close()

	_, err := newTestBackend(srv).Download(context.Background(), "../../../etc/passwd")
	require.ErrorIs(t, err, ErrInvalidDocumentKey)
	assert.False(t, reached.Load(), "an invalid key must not produce a request")
}

// A key that passes isDocumentID must be safe to interpolate into a URL path.
func FuzzIsDocumentID(f *testing.F) {
	for _, seed := range []string{"42", "0", "042", "-1", "../x", "4 2", ""} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		if !isDocumentID(key) {
			return
		}

		assert.Equal(t, key, url.PathEscape(key), "accepted key must need no escaping")
		assert.NotContains(t, key, "/")
		assert.NotContains(t, key, "..")
	})
}

func FuzzValidateBaseURL(f *testing.F) {
	for _, seed := range []string{"https://a.example", "http://", "file:///x", "", "http://a\x7fb"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		if validateBaseURL(raw) != nil {
			return
		}

		u, err := url.Parse(raw)
		require.NoError(t, err, "accepted base_url must parse")
		assert.Contains(t, []string{"http", "https"}, u.Scheme)
		assert.NotEmpty(t, u.Hostname())
	})
}

func configFor(t *testing.T, baseURL string) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(Config{BaseURL: baseURL, Token: "test-token"})
	require.NoError(t, err)

	return raw
}

func newTaskServer(t *testing.T, task taskResponse) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/post_document/"):
			json.NewEncoder(w).Encode("task-uuid") //nolint:errcheck
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/":
			task.TaskID = "task-uuid"
			json.NewEncoder(w).Encode([]taskResponse{task}) //nolint:errcheck
		}
	}))
}
