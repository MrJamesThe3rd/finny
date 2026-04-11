package document

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/document"
	"github.com/MrJamesThe3rd/finny/internal/httputil"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type Handler struct {
	docSvc  *document.Service
	txSvc   *transaction.Service
	registry *document.Registry
}

func NewHandler(docSvc *document.Service, txSvc *transaction.Service, registry *document.Registry) *Handler {
	return &Handler{docSvc: docSvc, txSvc: txSvc, registry: registry}
}

func (h *Handler) TransactionDocumentRoutes(r chi.Router) {
	r.Post("/", h.uploadDocument)
	r.Get("/", h.downloadDocument)
	r.Delete("/", h.deleteDocument)
}

func (h *Handler) BackendRoutes(r chi.Router) {
	r.Get("/", h.listBackends)
	r.Post("/", h.createBackend)
	r.Patch("/{id}", h.updateBackend)
	r.Delete("/{id}", h.deleteBackend)
}

// ── Document upload ────────────────────────────────────────────────────────────

func (h *Handler) uploadDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid transaction ID.")
		return
	}

	if _, err := h.txSvc.Get(r.Context(), id); err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to get transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		httputil.BadRequest(w, "Failed to parse upload.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.BadRequest(w, "The file field is required.")
		return
	}
	defer file.Close()

	mimeType := detectMIMEType(file, header)

	doc, err := h.docSvc.Upload(r.Context(), header.Filename, mimeType, file)
	if err != nil {
		if errors.Is(err, document.ErrNoBackends) {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "NO_BACKEND",
				"No document backend is configured.")
			return
		}
		slog.Error("failed to upload document", "error", err)
		httputil.InternalError(w)
		return
	}

	if err := h.txSvc.AttachDocument(r.Context(), id, doc.ID); err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		if errors.Is(err, transaction.ErrDocumentAlreadyAttached) {
			// Concurrent upload won the race — clean up our upload.
			_ = h.docSvc.Delete(r.Context(), doc.ID)
			httputil.WriteError(w, http.StatusConflict, "DOCUMENT_EXISTS",
				"Transaction already has a document. Delete it first.")
			return
		}
		slog.Error("failed to attach document to transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toDocumentResponse(doc))
}

// ── Document download ──────────────────────────────────────────────────────────

func (h *Handler) downloadDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid transaction ID.")
		return
	}

	tx, err := h.txSvc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to get transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	if tx.DocumentID == nil {
		httputil.NotFound(w)
		return
	}

	rc, doc, err := h.docSvc.Download(r.Context(), *tx.DocumentID)
	if err != nil {
		slog.Error("failed to download document", "document_id", tx.DocumentID, "error", err)
		httputil.InternalError(w)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", doc.MIMEType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, doc.Filename))

	if _, err := io.Copy(w, rc); err != nil {
		slog.Error("failed to stream document", "error", err)
	}
}

// ── Document delete ────────────────────────────────────────────────────────────

func (h *Handler) deleteDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid transaction ID.")
		return
	}

	tx, err := h.txSvc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to get transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	if tx.DocumentID == nil {
		httputil.NotFound(w)
		return
	}

	if err := h.docSvc.Delete(r.Context(), *tx.DocumentID); err != nil {
		slog.Error("failed to delete document", "document_id", tx.DocumentID, "error", err)
		httputil.InternalError(w)
		return
	}

	if err := h.txSvc.DetachDocument(r.Context(), id); err != nil {
		slog.Error("failed to detach document from transaction", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Backend list ───────────────────────────────────────────────────────────────

func (h *Handler) listBackends(w http.ResponseWriter, r *http.Request) {
	backends, err := h.docSvc.ListBackends(r.Context())
	if err != nil {
		slog.Error("failed to list backends", "error", err)
		httputil.InternalError(w)
		return
	}

	resp := make([]backendResponse, 0, len(backends))
	for _, b := range backends {
		resp = append(resp, toBackendResponse(b))
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ── Backend create ─────────────────────────────────────────────────────────────

type createBackendRequest struct {
	Type   string          `json:"type"   validate:"required,oneof=paperless local"`
	Name   string          `json:"name"   validate:"required"`
	Config json.RawMessage `json:"config" validate:"required"`
}

func (h *Handler) createBackend(w http.ResponseWriter, r *http.Request) {
	var req createBackendRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
		return
	}

	// Validate config is structurally valid for this backend type.
	if _, err := h.registry.Create(req.Type, req.Config); err != nil {
		httputil.BadRequest(w, fmt.Sprintf("Invalid config for %s backend: %s", req.Type, err.Error()))
		return
	}

	cfg := &document.BackendConfig{
		UserID:  auth.UserID(r.Context()),
		Type:    req.Type,
		Name:    req.Name,
		Config:  req.Config,
		Enabled: true,
	}

	if err := h.docSvc.CreateBackend(r.Context(), cfg); err != nil {
		slog.Error("failed to create backend", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toBackendResponse(*cfg))
}

// ── Backend update ─────────────────────────────────────────────────────────────

type updateBackendRequest struct {
	Name    *string          `json:"name,omitempty"`
	Config  *json.RawMessage `json:"config,omitempty"`
	Enabled *bool            `json:"enabled,omitempty"`
}

func (h *Handler) updateBackend(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid backend ID.")
		return
	}

	var req updateBackendRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}

	var config json.RawMessage
	if req.Config != nil {
		config = *req.Config
	}

	if err := h.docSvc.UpdateBackend(r.Context(), id, req.Name, config, req.Enabled); err != nil {
		if errors.Is(err, document.ErrBackendNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to update backend", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Backend delete ─────────────────────────────────────────────────────────────

func (h *Handler) deleteBackend(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.BadRequest(w, "Invalid backend ID.")
		return
	}

	force := r.URL.Query().Get("force") == "true"

	if !force {
		hasDocuments, err := h.docSvc.BackendHasDocuments(r.Context(), id)
		if err != nil {
			slog.Error("failed to check backend documents", "id", id, "error", err)
			httputil.InternalError(w)
			return
		}

		if hasDocuments {
			httputil.WriteError(w, http.StatusConflict, "BACKEND_HAS_DOCUMENTS",
				"Backend still has documents. Use ?force=true to delete anyway.")
			return
		}
	}

	if err := h.docSvc.DeleteBackend(r.Context(), id); err != nil {
		if errors.Is(err, document.ErrBackendNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("failed to delete backend", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func detectMIMEType(file multipart.File, header *multipart.FileHeader) string {
	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		return ct
	}

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	file.Seek(0, io.SeekStart) //nolint:errcheck
	return http.DetectContentType(buf[:n])
}
