package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Recoverer is a last-resort safety net: handlers must not panic, but if one
// does, the client gets a structured 500 and the stack is logged. It only
// writes a response if the handler had not started one, and it re-panics
// http.ErrAbortHandler, which net/http uses deliberately to abort a response.
//
// Install it inside RequestLogger so recovered panics are logged as 500s.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			slog.ErrorContext(r.Context(), "panic in handler", "panic", rec, "stack", string(debug.Stack()))
			if ww.Status() == 0 {
				JSON(ww, r, http.StatusInternalServerError, errorBody{errorDetail{"internal", "internal server error"}})
			}
		}()
		next.ServeHTTP(ww, r)
	})
}

// RequestLogger logs one structured line per request and echoes the request id
// in the X-Request-Id response header so clients can quote it. It must run
// after chi's middleware.RequestID.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := middleware.GetReqID(r.Context())
		w.Header().Set("X-Request-Id", id)
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", id,
		)
	})
}
