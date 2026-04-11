package export

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/export"
	"github.com/MrJamesThe3rd/finny/internal/httputil"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type Handler struct {
	svc *export.Service
}

func NewHandler(svc *export.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/", h.metadata)
	r.Post("/download", h.download)
}

type exportRequest struct {
	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`
}

type transactionResponse struct {
	ID             uuid.UUID          `json:"id"`
	Amount         int64              `json:"amount"`
	Type           transaction.Type   `json:"type"`
	Status         transaction.Status `json:"status"`
	Description    string             `json:"description"`
	RawDescription string             `json:"raw_description,omitempty"`
	Date           time.Time          `json:"date"`
	DocumentID     *uuid.UUID         `json:"document_id,omitempty"`
}

type exportMetadataResponse struct {
	Transactions []transactionResponse `json:"transactions"`
	EmailBody    string                `json:"email_body"`
}

func toTransactionResponse(tx *transaction.Transaction) transactionResponse {
	return transactionResponse{
		ID:             tx.ID,
		Amount:         tx.Amount,
		Type:           tx.Type,
		Status:         tx.Status,
		Description:    tx.Description,
		RawDescription: tx.RawDescription,
		Date:           tx.Date,
		DocumentID:     tx.DocumentID,
	}
}

func (h *Handler) metadata(w http.ResponseWriter, r *http.Request) {
	var req exportRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}

	filter := transaction.ListFilter{
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}

	tmpDir, err := os.MkdirTemp("", "finny-export-*")
	if err != nil {
		slog.Error("failed to create temp dir for export", "error", err)
		httputil.InternalError(w)
		return
	}
	defer os.RemoveAll(tmpDir)

	items, err := h.svc.Export(r.Context(), filter, tmpDir)
	if err != nil {
		slog.Error("failed to export transactions", "error", err)
		httputil.InternalError(w)
		return
	}

	txResponses := make([]transactionResponse, 0, len(items))
	for _, item := range items {
		txResponses = append(txResponses, toTransactionResponse(item.Transaction))
	}

	httputil.WriteJSON(w, http.StatusOK, exportMetadataResponse{
		Transactions: txResponses,
		EmailBody:    h.svc.GenerateSummary(items),
	})
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	var req exportRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}

	filter := transaction.ListFilter{
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}

	tmpDir, err := os.MkdirTemp("", "finny-export-*")
	if err != nil {
		slog.Error("failed to create temp dir for download", "error", err)
		httputil.InternalError(w)
		return
	}
	defer os.RemoveAll(tmpDir)

	items, err := h.svc.Export(r.Context(), filter, tmpDir)
	if err != nil {
		slog.Error("failed to export transactions for download", "error", err)
		httputil.InternalError(w)
		return
	}

	emailBody := h.svc.GenerateSummary(items)
	if err := os.WriteFile(filepath.Join(tmpDir, "email_body.txt"), []byte(emailBody), 0o644); err != nil {
		slog.Error("failed to write email body", "error", err)
		httputil.InternalError(w)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"export_%s.zip\"", time.Now().Format("20060102")))

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(tmpDir, path)

		zf, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(zf, f)
		return err
	})
	if err != nil {
		slog.Error("failed to create zip", "error", err)
	}
}
