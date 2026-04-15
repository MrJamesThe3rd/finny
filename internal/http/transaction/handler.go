package transaction

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/httputil"
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
	r.Patch("/{id}/status", h.updateStatus)
	r.Patch("/{id}", h.update)
}

type createTransactionRequest struct {
	Amount      int64            `json:"amount"      validate:"required,ne=0"`
	Type        transaction.Type `json:"type"        validate:"required,oneof=income expense"`
	Description string           `json:"description" validate:"required"`
	Date        time.Time        `json:"date"        validate:"required"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createTransactionRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
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
		slog.Error("failed to create transaction", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toResponse(tx))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter := transaction.ListFilter{}

	if s := r.URL.Query().Get("status"); s != "" {
		v := transaction.Status(s)
		filter.Status = &v
	}

	if s := r.URL.Query().Get("start_date"); s != "" {
		if t, err := time.Parse(time.DateOnly, s); err == nil {
			filter.StartDate = &t
		}
	}

	if s := r.URL.Query().Get("end_date"); s != "" {
		if t, err := time.Parse(time.DateOnly, s); err == nil {
			filter.EndDate = &t
		}
	}

	txs, err := h.svc.List(r.Context(), filter)
	if err != nil {
		slog.Error("failed to list transactions", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, toResponseList(txs))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid transaction ID.")
		return
	}

	tx, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to get transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, toResponse(tx))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid transaction ID.")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to delete transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateTransactionRequest struct {
	Description *string           `json:"description,omitempty"`
	Amount      *int64            `json:"amount,omitempty"`
	Type        *transaction.Type `json:"type,omitempty"`
	Date        *time.Time        `json:"date,omitempty"`
	NoInvoice   *bool             `json:"no_invoice,omitempty"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid transaction ID.")
		return
	}

	var req updateTransactionRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}

	tx, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to get transaction", "id", id, "error", err)
		httputil.InternalError(w)
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

	// Auto-infer status from current state.
	noInvoice := req.NoInvoice != nil && *req.NoInvoice
	switch {
	case noInvoice:
		tx.Status = transaction.StatusNoInvoice
	case tx.Description != "" && tx.DocumentID != nil:
		tx.Status = transaction.StatusComplete
	case tx.Description != "":
		tx.Status = transaction.StatusPendingInvoice
	default:
		tx.Status = transaction.StatusDraft
	}

	if err := h.svc.Update(r.Context(), tx); err != nil {
		slog.Error("failed to update transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, toResponse(tx))
}

type updateStatusRequest struct {
	Status transaction.Status `json:"status" validate:"required,oneof=draft pending_invoice complete no_invoice"`
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid transaction ID.")
		return
	}

	var req updateStatusRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
		return
	}

	if err := h.svc.UpdateStatus(r.Context(), id, req.Status); err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to update transaction status", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

