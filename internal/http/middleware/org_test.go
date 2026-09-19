package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	finnyMiddleware "github.com/MrJamesThe3rd/finny/internal/http/middleware"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

// stubMemberships answers the one lookup RequireOrg makes.
type stubMemberships struct {
	membership *org.Membership
	err        error

	gotUserID uuid.UUID
	gotOrgID  uuid.UUID
}

func (s *stubMemberships) GetMembership(_ context.Context, userID, orgID uuid.UUID) (*org.Membership, error) {
	s.gotUserID = userID
	s.gotOrgID = orgID

	if s.err != nil {
		return nil, s.err
	}

	return s.membership, nil
}

// errorCode reads the code out of the standard error envelope.
func errorCode(t *testing.T, body []byte) string {
	t.Helper()

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}

	require.NoError(t, json.Unmarshal(body, &resp))

	return resp.Error.Code
}

func TestRequireOrg(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orgID := uuid.New()

	tests := []struct {
		name       string
		giveHeader string
		giveStub   *stubMemberships
		wantStatus int
		wantCode   string
		wantCalled bool
	}{
		{
			name:       "Member",
			giveHeader: orgID.String(),
			giveStub: &stubMemberships{membership: &org.Membership{
				UserID: userID,
				OrgID:  orgID,
				Role:   org.RoleOwner,
			}},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "MissingHeader",
			giveHeader: "",
			giveStub:   &stubMemberships{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "ORG_REQUIRED",
		},
		{
			name:       "MalformedHeader",
			giveHeader: "not-a-uuid",
			giveStub:   &stubMemberships{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "ORG_INVALID",
		},
		{
			name:       "NotAMember",
			giveHeader: orgID.String(),
			giveStub:   &stubMemberships{err: org.ErrNotFound},
			wantStatus: http.StatusForbidden,
			wantCode:   "ORG_FORBIDDEN",
		},
		{
			// The store filters revoked_at IS NULL, so a revoked membership
			// arrives as ErrNotFound — and must be answered identically to a
			// membership that never existed.
			name:       "RevokedMembership",
			giveHeader: orgID.String(),
			giveStub:   &stubMemberships{err: org.ErrNotFound},
			wantStatus: http.StatusForbidden,
			wantCode:   "ORG_FORBIDDEN",
		},
		{
			name:       "LookupFailure",
			giveHeader: orgID.String(),
			giveStub:   &stubMemberships{err: errors.New("db is down")},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				called        bool
				gotMembership *org.Membership
			)

			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				gotMembership, _ = org.CurrentMembership(r.Context())
			})

			req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
			req = req.WithContext(auth.WithUserID(req.Context(), userID))

			if tt.giveHeader != "" {
				req.Header.Set(finnyMiddleware.OrgHeader, tt.giveHeader)
			}

			rec := httptest.NewRecorder()
			finnyMiddleware.RequireOrg(tt.giveStub)(next).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCalled, called, "handler reached")

			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, errorCode(t, rec.Body.Bytes()))
			}

			if tt.wantCalled {
				require.NotNil(t, gotMembership, "membership must reach the handler")
				assert.Equal(t, orgID, gotMembership.OrgID)
				// The header selects among the caller's memberships; the
				// lookup is the authorization.
				assert.Equal(t, userID, tt.giveStub.gotUserID)
				assert.Equal(t, orgID, tt.giveStub.gotOrgID)
			}
		})
	}
}

// A body that depends on a request header must not be cached against the URL
// alone: a shared cache would otherwise serve org A's list to org B.
func TestRequireOrg_CacheHeaders(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	stub := &stubMemberships{membership: &org.Membership{OrgID: orgID, Role: org.RoleOwner}}

	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	req.Header.Set(finnyMiddleware.OrgHeader, orgID.String())

	rec := httptest.NewRecorder()
	// Pre-set by go-chi/cors in the real chain; RequireOrg must add, not clobber.
	rec.Header().Add("Vary", "Origin")

	finnyMiddleware.RequireOrg(stub)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, req)

	assert.Equal(t, []string{"Origin", finnyMiddleware.OrgHeader}, rec.Header().Values("Vary"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestRequireOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		giveRole   org.Role
		giveNoOrg  bool
		wantStatus int
		wantCode   string
		wantCalled bool
	}{
		{
			name:       "Owner",
			giveRole:   org.RoleOwner,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "Accountant",
			giveRole:   org.RoleAccountant,
			wantStatus: http.StatusForbidden,
			wantCode:   "ROLE_FORBIDDEN",
		},
		{
			// RequireOrg did not run: a routing bug, and it must not pass.
			name:       "WithoutRequireOrg",
			giveNoOrg:  true,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var called bool

			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })

			req := httptest.NewRequest(http.MethodDelete, "/backends/1", nil)
			if !tt.giveNoOrg {
				req = req.WithContext(org.WithMembership(req.Context(), &org.Membership{
					OrgID: uuid.New(),
					Role:  tt.giveRole,
				}))
			}

			rec := httptest.NewRecorder()
			finnyMiddleware.RequireOwner(next).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCalled, called, "handler reached")

			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, errorCode(t, rec.Body.Bytes()))
			}
		})
	}
}
