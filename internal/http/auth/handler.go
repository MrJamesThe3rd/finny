package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/httputil"
)

// Handler serves the /auth/* and /admin/users/* routes.
type Handler struct {
	svc *auth.Service
}

func NewHandler(svc *auth.Service) *Handler {
	return &Handler{svc: svc}
}

// PublicRoutes registers the unauthenticated auth endpoints.
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)
}

// AdminRoutes registers the admin-only user management endpoints.
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Post("/", h.createUser)
	r.Get("/", h.listUsers)
	r.Delete("/{userID}", h.deleteUser)
}

// --- Request / Response types ---

type loginRequest struct {
	Login    string `json:"login"    validate:"required"`
	Password string `json:"password" validate:"required"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type createUserRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Username string `json:"username" validate:"required"`
	Name     string `json:"name"`
	Password string `json:"password" validate:"required"`
	IsAdmin  bool   `json:"is_admin"`
}

type userResponse struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Username  string  `json:"username"`
	Name      string  `json:"name"`
	IsAdmin   bool    `json:"is_admin"`
	CreatedAt string  `json:"created_at"`
	LastLogin *string `json:"last_login_at"`
}

func toUserResponse(u *auth.User) userResponse {
	r := userResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Username:  u.Username,
		Name:      u.Name,
		IsAdmin:   u.IsAdmin,
		CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
	}
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.UTC().Format(time.RFC3339)
		r.LastLogin = &s
	}
	return r
}

// --- Handlers ---

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
		return
	}

	result, err := h.svc.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			httputil.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials.")
			return
		}
		slog.Error("login failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
		return
	}

	result, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrTokenInvalid) {
			httputil.WriteError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid or expired refresh token.")
			return
		}
		slog.Error("token refresh failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
		return
	}

	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		slog.Error("logout failed", "error", err)
		// Always return 204 — don't leak whether the token existed
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "Invalid request body.")
		return
	}
	if !httputil.Validate(w, req) {
		return
	}

	user, err := h.svc.CreateUser(r.Context(), auth.CreateUserParams{
		Email:    req.Email,
		Username: req.Username,
		Name:     req.Name,
		Password: req.Password,
		IsAdmin:  req.IsAdmin,
	})
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) {
			httputil.BadRequest(w, "Password must be at least 8 characters.")
			return
		}
		slog.Error("create user failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toUserResponse(user))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		slog.Error("list users failed", "error", err)
		httputil.InternalError(w)
		return
	}

	resp := make([]userResponse, len(users))
	for i, u := range users {
		resp[i] = toUserResponse(u)
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		httputil.BadRequest(w, "Invalid user ID.")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims != nil && claims.UserID == id {
		httputil.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Cannot delete your own account.")
		return
	}

	if err := h.svc.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			httputil.NotFound(w)
			return
		}
		slog.Error("delete user failed", "id", id, "error", err)
		httputil.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
