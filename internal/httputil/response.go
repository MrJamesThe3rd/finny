package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// DecodeJSON decodes the request body as JSON into v.
func DecodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ErrorBody is the inner object of every error response.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the envelope returned for all error responses.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// WriteError writes a structured JSON error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error: ErrorBody{Code: code, Message: message},
	})
}

// BadRequest writes a 400 JSON error.
func BadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, "BAD_REQUEST", message)
}

// NotFound writes a 404 JSON error.
func NotFound(w http.ResponseWriter) {
	WriteError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found.")
}

// InternalError writes a 500 JSON error. Always use this for unexpected errors —
// never send raw Go error strings to clients.
func InternalError(w http.ResponseWriter) {
	WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred.")
}

// Validate runs struct-tag validation on v. If validation fails it writes a
// BAD_REQUEST response and returns false; the caller must return immediately.
func Validate(w http.ResponseWriter, v any) bool {
	err := validate.Struct(v)
	if err == nil {
		return true
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) && len(ve) > 0 {
		fe := ve[0]
		BadRequest(w, fmt.Sprintf("Field '%s' failed validation: %s.", fe.Field(), fe.Tag()))
		return false
	}

	BadRequest(w, "Invalid request.")
	return false
}
