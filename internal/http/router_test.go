package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	finnyHttp "github.com/MrJamesThe3rd/finny/internal/http"
	authHandler "github.com/MrJamesThe3rd/finny/internal/http/auth"
	documentHandler "github.com/MrJamesThe3rd/finny/internal/http/document"
	exportHandler "github.com/MrJamesThe3rd/finny/internal/http/export"
	importHandler "github.com/MrJamesThe3rd/finny/internal/http/importcsv"
	matchingHandler "github.com/MrJamesThe3rd/finny/internal/http/matching"
	orgHandler "github.com/MrJamesThe3rd/finny/internal/http/org"
	txHandler "github.com/MrJamesThe3rd/finny/internal/http/transaction"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

const testSecret = "router-test-secret-that-is-32bytes"

// unreachableMemberships fails the test if RequireOrg ever reaches it. Every
// case here is rejected before the lookup, so a call means the header check
// went missing.
type unreachableMemberships struct {
	t *testing.T
}

func (m unreachableMemberships) GetMembership(_ context.Context, _, _ uuid.UUID) (*org.Membership, error) {
	m.t.Error("membership lookup reached: the header check did not run")
	return nil, org.ErrNotFound
}

// newTestRouter builds the real route tree. Handlers are nil: every request in
// this file is rejected by middleware, so no handler body ever runs — which is
// precisely what is being asserted.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	return finnyHttp.New(
		(*txHandler.Handler)(nil),
		(*importHandler.Handler)(nil),
		(*matchingHandler.Handler)(nil),
		(*exportHandler.Handler)(nil),
		(*documentHandler.Handler)(nil),
		(*authHandler.Handler)(nil),
		(*orgHandler.Handler)(nil),
		unreachableMemberships{t: t},
		finnyHttp.Config{JWTSecret: testSecret, CORSAllowedOrigin: "http://localhost:5173"},
	)
}

func bearerToken(t *testing.T) string {
	t.Helper()

	token, err := auth.SignAccessToken(
		&auth.User{ID: uuid.New(), IsAdmin: true},
		testSecret,
		15*time.Minute,
	)
	require.NoError(t, err)

	return token
}

func errCode(t *testing.T, body []byte) string {
	t.Helper()

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}

	return resp.Error.Code
}

// Routing is default-allow, so this is a snapshot of which paths sit behind
// RequireOrg — not an invariant. The real backstop is that org.OrgID(ctx)
// errors, so an unscoped route dies at the store instead of leaking rows.
func TestRouter_ScopedRoutesRequireOrgHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		givePath   string
		giveMethod string
		wantScoped bool
	}{
		{name: "Transactions", giveMethod: http.MethodGet, givePath: "/api/v1/transactions", wantScoped: true},
		{name: "Matching", giveMethod: http.MethodGet, givePath: "/api/v1/matching/suggest?raw_description=x", wantScoped: true},
		{name: "Backends", giveMethod: http.MethodGet, givePath: "/api/v1/backends", wantScoped: true},
		{name: "OrgMembers", giveMethod: http.MethodDelete, givePath: "/api/v1/orgs/members/" + uuid.NewString(), wantScoped: true},

		// Outside RequireOrg by design: a user with no organization yet must
		// be able to reach these, and identity is not tenanted.
		{name: "OrgList", giveMethod: http.MethodGet, givePath: "/api/v1/orgs", wantScoped: false},
		{name: "AdminUsers", giveMethod: http.MethodGet, givePath: "/api/v1/admin/users", wantScoped: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.giveMethod, tt.givePath, nil)
			req.Header.Set("Authorization", "Bearer "+bearerToken(t))

			rec := httptest.NewRecorder()
			newTestRouter(t).ServeHTTP(rec, req)

			got := errCode(t, rec.Body.Bytes())

			if tt.wantScoped {
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Equal(t, "ORG_REQUIRED", got)

				return
			}

			assert.NotEqual(t, "ORG_REQUIRED", got, "route must not sit behind RequireOrg")
		})
	}
}

func TestRouter_UnauthenticatedIsRejectedBeforeOrg(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions", nil)

	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "UNAUTHORIZED", errCode(t, rec.Body.Bytes()))
}

// X-Org-ID must be allowed at preflight or every scoped browser request fails
// before it reaches Go. curl does not preflight, so only a test like this or a
// real browser catches a regression.
func TestRouter_CORSAllowsOrgHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/transactions", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "authorization,x-org-id")

	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	allowed := rec.Header().Get("Access-Control-Allow-Headers")
	assert.Contains(t, strings.ToLower(allowed), "x-org-id")
}
