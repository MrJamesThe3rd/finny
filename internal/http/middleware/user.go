package middleware

import (
	"net/http"
	"strings"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/httputil"
)

// RequireAuth returns a middleware that validates the JWT in the Authorization header.
// On success, it stores the user ID and full claims in the request context.
// On failure, it writes a 401 and halts the chain.
func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				httputil.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header.")
				return
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := auth.VerifyToken(tokenStr, secret)
			if err != nil {
				httputil.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token.")
				return
			}

			ctx := auth.WithUserID(r.Context(), claims.UserID)
			ctx = auth.WithClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
