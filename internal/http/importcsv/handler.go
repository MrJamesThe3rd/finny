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
	r.Post("/confirm", h.confirmImport)
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

type importSuccessResponse struct {
	Imported     int                   `json:"imported"`
	Transactions []transactionResponse `json:"transactions"`
}

type createParamsDTO struct {
	Amount         int64            `json:"amount"`
	Type           transaction.Type `json:"type"`
	Description    string           `json:"description"`
	RawDescription string           `json:"raw_description"`
	Date           time.Time        `json:"date"`
}

type conflictDTO struct {
	Incoming createParamsDTO     `json:"incoming"`
	Existing transactionResponse `json:"existing"`
}

type importConflictResponse struct {
	New       []createParamsDTO `json:"new"`
	Conflicts []conflictDTO     `json:"conflicts"`
}

type confirmRequest struct {
	Params []createParamsDTO `json:"params"`
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

	for i, p := range params {
		suggested, err := h.matchSvc.Suggest(r.Context(), p.RawDescription)
		if err != nil {
			continue
		}

		if suggested == "" {
			continue
		}

		params[i].Description = suggested
	}

	result, err := h.txSvc.ImportBatch(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(result.Conflicts) > 0 {
		resp := importConflictResponse{
			New:       make([]createParamsDTO, 0, len(result.New)),
			Conflicts: make([]conflictDTO, 0, len(result.Conflicts)),
		}
		for _, p := range result.New {
			resp.New = append(resp.New, toParamsDTO(p))
		}

		for _, c := range result.Conflicts {
			resp.Conflicts = append(resp.Conflicts, conflictDTO{
				Incoming: toParamsDTO(c.Incoming),
				Existing: toTxResponse(c.Existing),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode response", "error", err)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(toSuccessResponse(result.Imported)); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (h *Handler) confirmImport(w http.ResponseWriter, r *http.Request) {
	var req confirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	params := make([]transaction.CreateParams, 0, len(req.Params))
	for _, p := range req.Params {
		params = append(params, transaction.CreateParams{
			Amount:         p.Amount,
			Type:           p.Type,
			Status:         transaction.StatusDraft,
			Description:    p.Description,
			RawDescription: p.RawDescription,
			Date:           p.Date,
		})
	}

	txs, err := h.txSvc.CreateBatch(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(toSuccessResponse(txs)); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func toSuccessResponse(txs []*transaction.Transaction) importSuccessResponse {
	responses := make([]transactionResponse, 0, len(txs))
	for _, tx := range txs {
		responses = append(responses, toTxResponse(tx))
	}

	return importSuccessResponse{
		Imported:     len(txs),
		Transactions: responses,
	}
}

func toTxResponse(tx *transaction.Transaction) transactionResponse {
	return transactionResponse{
		ID:             tx.ID,
		Amount:         tx.Amount,
		Type:           tx.Type,
		Status:         tx.Status,
		Description:    tx.Description,
		RawDescription: tx.RawDescription,
		Date:           tx.Date,
		CreatedAt:      tx.CreatedAt,
	}
}

func toParamsDTO(p transaction.CreateParams) createParamsDTO {
	return createParamsDTO{
		Amount:         p.Amount,
		Type:           p.Type,
		Description:    p.Description,
		RawDescription: p.RawDescription,
		Date:           p.Date,
	}
}
