package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/httputil"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

// OrgHeader names the organization a request addresses. It selects among the
// caller's memberships; the membership lookup is the authorization.
const OrgHeader = "X-Org-ID"

// MembershipGetter is the one thing RequireOrg needs. Consumer-owned, so this
// package depends on a method rather than on *org.Service.
type MembershipGetter interface {
	GetMembership(ctx context.Context, userID, orgID uuid.UUID) (*org.Membership, error)
}

// RequireOrg resolves X-Org-ID to the caller's membership and puts it in the
// request context. It is the single authorization gate for scoped routes.
// Must be used after RequireAuth.
func RequireOrg(memberships MembershipGetter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A body that depends on a request header must not be cached
			// against the URL alone: Authorization is treated as no-store by
			// most shared caches, a custom header is not. Added, not Set —
			// go-chi/cors has already written Vary: Origin.
			w.Header().Add("Vary", OrgHeader)
			w.Header().Set("Cache-Control", "no-store")

			raw := r.Header.Get(OrgHeader)
			if raw == "" {
				httputil.WriteError(w, http.StatusBadRequest, "ORG_REQUIRED",
					"The X-Org-ID header is required.")
				return
			}

			orgID, err := uuid.Parse(raw)
			if err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "ORG_INVALID",
					"The X-Org-ID header is not a valid organization ID.")
				return
			}

			membership, err := memberships.GetMembership(r.Context(), auth.UserID(r.Context()), orgID)
			if err != nil {
				if !errors.Is(err, org.ErrNotFound) {
					slog.Error("membership lookup failed", "org_id", orgID, "error", err)
					httputil.InternalError(w)

					return
				}

				// The org is invisible in the URL and the access log, so log
				// the parsed id here. "Not yours" and "does not exist" get the
				// same answer, deliberately.
				slog.Info("org access denied", "org_id", orgID)
				httputil.WriteError(w, http.StatusForbidden, "ORG_FORBIDDEN",
					"You do not have access to this organization.")

				return
			}

			next.ServeHTTP(w, r.WithContext(org.WithMembership(r.Context(), membership)))
		})
	}
}

// RequireOwner rejects a caller who is not an owner of the active
// organization. Must be used after RequireOrg.
func RequireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		membership, err := org.CurrentMembership(r.Context())
		if err != nil {
			// RequireOrg did not run: a routing bug, not a client error.
			slog.Error("RequireOwner without RequireOrg", "path", r.URL.Path)
			httputil.InternalError(w)

			return
		}

		if membership.Role != org.RoleOwner {
			httputil.WriteError(w, http.StatusForbidden, "ROLE_FORBIDDEN",
				"This action is restricted to the organization's owner.")

			return
		}

		next.ServeHTTP(w, r)
	})
}
