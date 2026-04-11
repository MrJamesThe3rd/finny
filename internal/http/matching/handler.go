package matching

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/MrJamesThe3rd/finny/internal/httputil"
	"github.com/MrJamesThe3rd/finny/internal/matching"
)

type Handler struct {
	svc *matching.Service
}

func NewHandler(svc *matching.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/suggest", h.suggest)
	r.Post("/", h.learn)
}

type suggestResponse struct {
	RawDescription       string `json:"raw_description"`
	PreferredDescription string `json:"preferred_description"`
}

func (h *Handler) suggest(w http.ResponseWriter, r *http.Request) {
	rawDesc := r.URL.Query().Get("raw_description")
	if rawDesc == "" {
		httputil.BadRequest(w, "The raw_description query parameter is required.")
		return
	}

	preferred, err := h.svc.Suggest(r.Context(), rawDesc)
	if err != nil {
		slog.Error("failed to suggest description", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, suggestResponse{
		RawDescription:       rawDesc,
		PreferredDescription: preferred,
	})
}

type learnRequest struct {
	RawPattern           string `json:"raw_pattern"            validate:"required"`
	PreferredDescription string `json:"preferred_description"  validate:"required"`
}

func (h *Handler) learn(w http.ResponseWriter, r *http.Request) {
	var req learnRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
		return
	}

	if err := h.svc.Learn(r.Context(), req.RawPattern, req.PreferredDescription); err != nil {
		slog.Error("failed to learn description mapping", "error", err)
		httputil.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
