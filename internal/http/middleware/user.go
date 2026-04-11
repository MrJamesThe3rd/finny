package middleware

import (
	"net/http"

	"github.com/MrJamesThe3rd/finny/internal/auth"
)

// InjectDefaultUser is a placeholder middleware that injects the default user ID
// into every request context. Replaced by real JWT parsing in Phase 3.
func InjectDefaultUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithUserID(r.Context(), auth.DefaultUserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
