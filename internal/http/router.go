package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/MrJamesThe3rd/finny/internal/http/export"
	"github.com/MrJamesThe3rd/finny/internal/http/importcsv"
	"github.com/MrJamesThe3rd/finny/internal/http/matching"
	"github.com/MrJamesThe3rd/finny/internal/http/transaction"
	documentHandler "github.com/MrJamesThe3rd/finny/internal/http/document"
	finnyMiddleware "github.com/MrJamesThe3rd/finny/internal/http/middleware"
)

func New(
	transactionsV1 *transaction.Handler,
	importV1 *importcsv.Handler,
	matchingV1 *matching.Handler,
	exportV1 *export.Handler,
	documentV1 *documentHandler.Handler,
) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(finnyMiddleware.InjectDefaultUser)

	router.Route("/api/v1", func(r chi.Router) {
		r.Route("/transactions", func(r chi.Router) {
			r.Use(middleware.AllowContentType("application/json"))
			transactionsV1.Routes(r)

			// Document sub-routes (no AllowContentType — upload is multipart)
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

	return router
}
