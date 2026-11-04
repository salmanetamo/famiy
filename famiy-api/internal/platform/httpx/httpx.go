// Package httpx holds JSON helpers and the single place where errors become
// HTTP responses.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
	case apperr.KindTooLarge:
		status = http.StatusRequestEntityTooLarge
	case apperr.KindUnauthenticated:
		status = http.StatusUnauthorized
	case apperr.KindForbidden:
		status = http.StatusForbidden
	case apperr.KindRateLimited:
		status = http.StatusTooManyRequests
	case apperr.KindUnsupportedType:
		status = http.StatusUnsupportedMediaType
	}
	JSON(w, r, status, errorBody{errorDetail{ae.Code, ae.Message}})
}

// Decode reads exactly one JSON value from the request body into dst,
// rejecting unknown fields, trailing data and oversized bodies.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	// Anything after the first value (other than whitespace) is an error.
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return decodeError(err)
		}
		return apperr.Invalid("invalid_json", "request body must contain a single JSON value")
	}
	return nil
}

// decodeError turns a decoder failure into a client-safe apperr. Raw decoder
// text is not echoed, because it can expose Go type and field names.
func decodeError(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return apperr.TooLarge("request_too_large", fmt.Sprintf("request body exceeds %d bytes", tooBig.Limit))
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return apperr.Invalid("invalid_json", fmt.Sprintf("field %q has the wrong type", typeErr.Field))
	}
	// Unknown-field errors are useful to the client and contain only their own input.
	if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return apperr.Invalid("invalid_json", "unknown field "+field)
	}
	return apperr.Invalid("invalid_json", "request body is not valid JSON")
}

// PathID parses the {id} URL parameter as a positive int64.
func PathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Invalid("invalid_id", "id must be a positive integer")
	}
	return id, nil
}
