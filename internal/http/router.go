package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	authHandler "github.com/MrJamesThe3rd/finny/internal/http/auth"
	documentHandler "github.com/MrJamesThe3rd/finny/internal/http/document"
	"github.com/MrJamesThe3rd/finny/internal/http/export"
	"github.com/MrJamesThe3rd/finny/internal/http/importcsv"
	"github.com/MrJamesThe3rd/finny/internal/http/matching"
	finnyMiddleware "github.com/MrJamesThe3rd/finny/internal/http/middleware"
	"github.com/MrJamesThe3rd/finny/internal/http/transaction"
)

// Config holds router-level options. Kept separate from the config package
// so the http package does not import config.
type Config struct {
	JWTSecret         string
	CORSAllowedOrigin string
}

func New(
	transactionsV1 *transaction.Handler,
	importV1 *importcsv.Handler,
	matchingV1 *matching.Handler,
	exportV1 *export.Handler,
	documentV1 *documentHandler.Handler,
	authV1 *authHandler.Handler,
	cfg Config,
) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.CORSAllowedOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// Public auth routes — no JWT required
	router.Route("/api/v1/auth", authV1.PublicRoutes)

	// All other routes require a valid JWT
	router.Group(func(r chi.Router) {
		r.Use(finnyMiddleware.RequireAuth(cfg.JWTSecret))

		r.Route("/api/v1", func(r chi.Router) {
			// Admin-only user management
			r.Route("/admin/users", func(r chi.Router) {
				r.Use(finnyMiddleware.RequireAdmin)
				authV1.AdminRoutes(r)
			})

			r.Route("/transactions", func(r chi.Router) {
				r.Use(middleware.AllowContentType("application/json"))
				transactionsV1.Routes(r)
				r.Route("/{id}/document", documentV1.TransactionDocumentRoutes)
			})

			r.Route("/import", importV1.Routes)

			r.Route("/matching", func(r chi.Router) {
				matchingV1.Routes(r)
			})

			r.Route("/export", exportV1.Routes)

			r.Route("/backends", func(r chi.Router) {
				r.Use(middleware.AllowContentType("application/json"))
				documentV1.BackendRoutes(r)
			})
		})
	})

	return router
}
