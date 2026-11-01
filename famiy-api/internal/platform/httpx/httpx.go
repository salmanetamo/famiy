// Package httpx holds JSON helpers and the single place where errors become
// HTTP responses.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON writes v as a JSON response.
func JSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.ErrorContext(r.Context(), "write response", "err", err)
	}
}

// Error maps err to a status code and writes a structured error body.
// Unrecognised errors are logged and reported as an opaque 500.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		slog.ErrorContext(r.Context(), "internal error", "err", err)
		JSON(w, r, http.StatusInternalServerError, errorBody{errorDetail{"internal", "internal server error"}})
		return
	}
	status := http.StatusInternalServerError
	switch ae.Kind {
	case apperr.KindInvalid:
		status = http.StatusBadRequest
	case apperr.KindNotFound:
		status = http.StatusNotFound
	case apperr.KindConflict:
		status = http.StatusConflict
	}
	JSON(w, r, status, errorBody{errorDetail{ae.Code, ae.Message}})
}

// Decode reads a JSON request body into dst, rejecting unknown fields and
// oversized bodies.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperr.Invalid("invalid_json", "request body is not valid JSON for this endpoint: "+err.Error())
	}
	return nil
}

// PathID parses the {id} URL parameter as a positive int64.
func PathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Invalid("invalid_id", "id must be a positive integer")
	}
	return id, nil
}
