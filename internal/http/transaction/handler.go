package transaction

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type Handler struct {
	svc *transaction.Service
}

func NewHandler(svc *transaction.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Delete("/{id}", h.delete)
	r.Patch("/{id}/invoice", h.updateInvoice)
	r.Patch("/{id}/status", h.updateStatus)
	r.Patch("/{id}", h.update)
}

type createTransactionRequest struct {
	Amount      int64            `json:"amount"`
	Type        transaction.Type `json:"type"`
	Description string           `json:"description"`
	Date        time.Time        `json:"date"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := h.svc.Create(r.Context(), transaction.CreateParams{
		Amount:      req.Amount,
		Type:        req.Type,
		Status:      transaction.StatusComplete,
		Description: req.Description,
		Date:        req.Date,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(toResponse(tx)); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter := transaction.ListFilter{}

	if s := r.URL.Query().Get("status"); s != "" {
		filter.Status = new(transaction.Status(s))
	}

	if s := r.URL.Query().Get("start_date"); s != "" {
		if t, err := time.Parse(time.DateOnly, s); err == nil {
			filter.StartDate = new(t)
		}
	}

	if s := r.URL.Query().Get("end_date"); s != "" {
		if t, err := time.Parse(time.DateOnly, s); err == nil {
			filter.EndDate = new(t)
		}
	}

	txs, err := h.svc.List(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(toResponseList(txs)); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	tx, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			http.Error(w, "transaction not found", http.StatusNotFound)
			return
		}

		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(toResponse(tx)); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateTransactionRequest struct {
	Description *string           `json:"description,omitempty"`
	Amount      *int64            `json:"amount,omitempty"`
	Type        *transaction.Type `json:"type,omitempty"`
	Date        *time.Time        `json:"date,omitempty"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req updateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			http.Error(w, "transaction not found", http.StatusNotFound)
			return
		}

		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	if req.Description != nil {
		tx.Description = *req.Description
	}

	if req.Amount != nil {
		tx.Amount = *req.Amount
	}

	if req.Type != nil {
		tx.Type = *req.Type
	}

	if req.Date != nil {
		tx.Date = *req.Date
	}

	if err := h.svc.Update(r.Context(), tx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(toResponse(tx)); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

type updateStatusRequest struct {
	Status transaction.Status `json:"status"`
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateStatus(r.Context(), id, req.Status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateInvoiceRequest struct {
	InvoiceURL string `json:"invoice_url"`
}

func (h *Handler) updateInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req updateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateInvoice(r.Context(), id, req.InvoiceURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
