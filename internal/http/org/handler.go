package org

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/httputil"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

// Handler serves /orgs. The list and create routes sit outside RequireOrg —
// a user with no organization yet must be able to reach them — while the
// member routes are header-addressed and sit inside RequireOrg + RequireOwner.
type Handler struct {
	svc *org.Service
}

func NewHandler(svc *org.Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the routes reachable with only a JWT.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
}

// MemberRoutes registers the owner-only member management routes.
func (h *Handler) MemberRoutes(r chi.Router) {
	r.Post("/", h.addMember)
	r.Delete("/{userID}", h.revokeMember)
}

type orgResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	NIF       string    `json:"nif,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toResponse(o *org.Organization) orgResponse {
	return orgResponse{
		ID:        o.ID,
		Name:      o.Name,
		NIF:       o.NIF,
		CreatedAt: o.CreatedAt,
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.svc.ListForUser(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		slog.Error("failed to list organizations", "error", err)
		httputil.InternalError(w)

		return
	}

	resp := make([]orgResponse, 0, len(orgs))
	for _, o := range orgs {
		resp = append(resp, toResponse(o))
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

type createOrgRequest struct {
	Name string `json:"name" validate:"required"`
	NIF  string `json:"nif"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createOrgRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}

	if !httputil.Validate(w, req) {
		return
	}

	o, err := h.svc.Create(r.Context(), auth.UserID(r.Context()), org.CreateOrgParams{
		Name: req.Name,
		NIF:  req.NIF,
	})
	if err != nil {
		if errors.Is(err, org.ErrInvalidNIF) {
			httputil.BadRequest(w, "Invalid NIF.")
			return
		}

		slog.Error("failed to create organization", "error", err)
		httputil.InternalError(w)

		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toResponse(o))
}

type addMemberRequest struct {
	UserID uuid.UUID `json:"user_id" validate:"required"`
	Role   org.Role  `json:"role"    validate:"required,oneof=owner accountant"`
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	var req addMemberRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}

	if !httputil.Validate(w, req) {
		return
	}

	orgID, err := org.OrgID(r.Context())
	if err != nil {
		slog.Error("no organization in context", "path", r.URL.Path)
		httputil.InternalError(w)

		return
	}

	if err := h.svc.AddMember(r.Context(), orgID, req.UserID, req.Role); err != nil {
		if errors.Is(err, org.ErrInvalidRole) {
			httputil.BadRequest(w, "Invalid role.")
			return
		}

		slog.Error("failed to add member", "org_id", orgID, "error", err)
		httputil.InternalError(w)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) revokeMember(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		httputil.BadRequest(w, "Invalid user ID.")
		return
	}

	orgID, err := org.OrgID(r.Context())
	if err != nil {
		slog.Error("no organization in context", "path", r.URL.Path)
		httputil.InternalError(w)

		return
	}

	if err := h.svc.RevokeMember(r.Context(), orgID, userID); err != nil {
		if errors.Is(err, org.ErrNotFound) {
			httputil.NotFound(w)
			return
		}

		if errors.Is(err, org.ErrLastOwner) {
			httputil.WriteError(w, http.StatusConflict, "LAST_OWNER",
				"An organization must keep at least one owner.")

			return
		}

		slog.Error("failed to revoke member", "org_id", orgID, "error", err)
		httputil.InternalError(w)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
