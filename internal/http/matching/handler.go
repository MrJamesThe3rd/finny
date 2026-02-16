package matching

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

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
		http.Error(w, "raw_description query parameter is required", http.StatusBadRequest)
		return
	}

	preferred, err := h.svc.Suggest(r.Context(), rawDesc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(suggestResponse{
		RawDescription:       rawDesc,
		PreferredDescription: preferred,
	}); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

type learnRequest struct {
	RawPattern           string `json:"raw_pattern"`
	PreferredDescription string `json:"preferred_description"`
}

func (h *Handler) learn(w http.ResponseWriter, r *http.Request) {
	var req learnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.RawPattern == "" || req.PreferredDescription == "" {
		http.Error(w, "raw_pattern and preferred_description are required", http.StatusBadRequest)
		return
	}

	if err := h.svc.Learn(r.Context(), req.RawPattern, req.PreferredDescription); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
