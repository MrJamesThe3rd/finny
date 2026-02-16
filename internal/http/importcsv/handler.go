package importcsv

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/importer"
	"github.com/MrJamesThe3rd/finny/internal/matching"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type Handler struct {
	importSvc *importer.Service
	txSvc     *transaction.Service
	matchSvc  *matching.Service
}

func NewHandler(importSvc *importer.Service, txSvc *transaction.Service, matchSvc *matching.Service) *Handler {
	return &Handler{
		importSvc: importSvc,
		txSvc:     txSvc,
		matchSvc:  matchSvc,
	}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/", h.importCSV)
}

type transactionResponse struct {
	ID             uuid.UUID          `json:"id"`
	Amount         int64              `json:"amount"`
	Type           transaction.Type   `json:"type"`
	Status         transaction.Status `json:"status"`
	Description    string             `json:"description"`
	RawDescription string             `json:"raw_description,omitempty"`
	Date           time.Time          `json:"date"`
	CreatedAt      time.Time          `json:"created_at"`
}

type importResponse struct {
	Imported     int                   `json:"imported"`
	Transactions []transactionResponse `json:"transactions"`
}

func (h *Handler) importCSV(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	bank := importer.Bank(r.FormValue("bank"))
	if bank == "" {
		http.Error(w, "bank field is required", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	params, err := h.importSvc.Import(bank, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created := make([]transactionResponse, 0, len(params))

	for _, p := range params {
		if suggested, err := h.matchSvc.Suggest(r.Context(), p.RawDescription); err == nil && suggested != "" {
			p.Description = suggested
		}

		tx, err := h.txSvc.Create(r.Context(), p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		created = append(created, transactionResponse{
			ID:             tx.ID,
			Amount:         tx.Amount,
			Type:           tx.Type,
			Status:         tx.Status,
			Description:    tx.Description,
			RawDescription: tx.RawDescription,
			Date:           tx.Date,
			CreatedAt:      tx.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(importResponse{
		Imported:     len(created),
		Transactions: created,
	}); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}
