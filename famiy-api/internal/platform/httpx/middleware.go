package httpx

import (
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
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

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// OriginCheck is a CSRF defence for cookie auth: a state-changing request that
// carries an Origin header must come from this server's own origin or one in
// allowed. Requests without an Origin (non-browser clients) pass; browsers
// always send Origin on cross-origin unsafe requests.
func OriginCheck(allowed ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			// Same-origin is judged by host only: behind TLS termination the scheme
			// the server sees is not the browser's. SameSite=Lax cookies cover the
			// rest. Explicitly allowed origins are compared in full.
			if u, err := url.Parse(origin); err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host) {
				next.ServeHTTP(w, r)
				return
			}
			for _, a := range allowed {
				if strings.EqualFold(origin, a) {
					next.ServeHTTP(w, r)
					return
				}
			}
			Error(w, r, apperr.Forbidden("origin_not_allowed", "request origin is not allowed"))
		})
	}
}

// SecurityHeaders marks every response as uncacheable (accounts, sessions and
// family data must never be stored by shared caches) and stops browsers from
// guessing content types.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// RequireJSON rejects unsafe requests that have a body but are not JSON. Together
// with SameSite cookies this closes simple cross-site form posts.
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSafeMethod(r.Method) && r.ContentLength != 0 {
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != "application/json" {
				Error(w, r, apperr.UnsupportedType("unsupported_media_type", "Content-Type must be application/json"))
				return
			}
		}
		next.ServeHTTP(w, r)
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
