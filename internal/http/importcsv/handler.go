package importcsv

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/httputil"
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
	Amount         int64            `json:"amount"  validate:"required,ne=0"`
	Type           transaction.Type `json:"type"    validate:"required,oneof=income expense"`
	Description    string           `json:"description"`
	RawDescription string           `json:"raw_description"`
	Date           time.Time        `json:"date"    validate:"required"`
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
		httputil.BadRequest(w, "Failed to parse multipart form.")
		return
	}

	bank := importer.Bank(r.FormValue("bank"))
	if bank == "" {
		httputil.BadRequest(w, "The bank field is required.")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		httputil.BadRequest(w, "The file field is required.")
		return
	}
	defer file.Close()

	params, err := h.importSvc.Import(bank, file)
	if err != nil {
		httputil.BadRequest(w, "Failed to parse CSV file.")
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
		slog.Error("failed to import transactions", "error", err)
		httputil.InternalError(w)
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
		httputil.WriteJSON(w, http.StatusConflict, resp)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toSuccessResponse(result.Imported))
}

func (h *Handler) confirmImport(w http.ResponseWriter, r *http.Request) {
	var req confirmRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}

	if len(req.Params) == 0 {
		httputil.BadRequest(w, "params must not be empty.")
		return
	}

	for _, p := range req.Params {
		if !httputil.Validate(w, p) {
			return
		}
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
		slog.Error("failed to confirm import", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toSuccessResponse(txs))
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
