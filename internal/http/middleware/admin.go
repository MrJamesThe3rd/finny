package middleware

import (
	"net/http"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/httputil"
)

// RequireAdmin is a middleware that rejects requests from non-admin users.
// Must be used after RequireAuth (depends on claims being in context).
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil || !claims.IsAdmin {
			httputil.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Admin access required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
