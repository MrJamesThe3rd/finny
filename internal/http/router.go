package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/MrJamesThe3rd/finny/internal/http/export"
	"github.com/MrJamesThe3rd/finny/internal/http/importcsv"
	"github.com/MrJamesThe3rd/finny/internal/http/matching"
	"github.com/MrJamesThe3rd/finny/internal/http/transaction"
)

func New(
	transactionsV1 *transaction.Handler,
	importV1 *importcsv.Handler,
	matchingV1 *matching.Handler,
	exportV1 *export.Handler,
) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Route("/api/v1", func(r chi.Router) {
		r.Route("/transactions", func(r chi.Router) {
			r.Use(middleware.AllowContentType("application/json"))
			transactionsV1.Routes(r)
		})

		r.Route("/import", importV1.Routes)

		r.Route("/matching", func(r chi.Router) {
			matchingV1.Routes(r)
		})

		r.Route("/export", func(r chi.Router) {
			r.Use(middleware.AllowContentType("application/json"))
			exportV1.Routes(r)
		})
	})

	return router
}
