package httpx_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
)

type body struct {
	Name string `json:"name"`
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantKind apperr.Kind // 0 = success
		wantCode string
		wantMsg  string // substring
	}{
		{name: "valid", body: `{"name":"a"}`},
		{name: "trailing whitespace ok", body: "{\"name\":\"a\"}\n  "},
		{name: "malformed", body: `{`, wantKind: apperr.KindInvalid, wantCode: "invalid_json", wantMsg: "not valid JSON"},
		{name: "unknown field", body: `{"nope":1}`, wantKind: apperr.KindInvalid, wantCode: "invalid_json", wantMsg: `unknown field "nope"`},
		{name: "wrong type hides Go internals", body: `{"name":5}`, wantKind: apperr.KindInvalid, wantCode: "invalid_json", wantMsg: `field "name" has the wrong type`},
		{name: "trailing value", body: `{"name":"a"} {"name":"b"}`, wantKind: apperr.KindInvalid, wantCode: "invalid_json", wantMsg: "single JSON value"},
		{name: "trailing garbage", body: `{"name":"a"} junk`, wantKind: apperr.KindInvalid, wantCode: "invalid_json"},
		{name: "oversized", body: `{"name":"` + strings.Repeat("x", 2<<20) + `"}`, wantKind: apperr.KindTooLarge, wantCode: "request_too_large"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var dst body
			err := httpx.Decode(rec, req, &dst)
			if tc.wantKind == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ae *apperr.Error
			if !errors.As(err, &ae) {
				t.Fatalf("want apperr, got %v", err)
			}
			if ae.Kind != tc.wantKind || ae.Code != tc.wantCode {
				t.Fatalf("got kind=%d code=%s (%q), want kind=%d code=%s", ae.Kind, ae.Code, ae.Message, tc.wantKind, tc.wantCode)
			}
			if !strings.Contains(ae.Message, tc.wantMsg) {
				t.Fatalf("message %q does not contain %q", ae.Message, tc.wantMsg)
			}
		})
	}
}

func TestErrorStatusMapping(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{apperr.Invalid("c", "m"), http.StatusBadRequest},
		{apperr.NotFound("c", "m"), http.StatusNotFound},
		{apperr.Conflict("c", "m"), http.StatusConflict},
		{apperr.TooLarge("c", "m"), http.StatusRequestEntityTooLarge},
		{apperr.Unauthenticated("c", "m"), http.StatusUnauthorized},
		{apperr.Forbidden("c", "m"), http.StatusForbidden},
		{apperr.RateLimited("c", "m", 0), http.StatusTooManyRequests},
		{apperr.UnsupportedType("c", "m"), http.StatusUnsupportedMediaType},
		{errors.New("db exploded: password=hunter2"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		httpx.Error(rec, httptest.NewRequest(http.MethodGet, "/", nil), tc.err)
		if rec.Code != tc.want {
			t.Errorf("%v: status = %d, want %d", tc.err, rec.Code, tc.want)
		}
		if tc.want == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "hunter2") {
			t.Errorf("internal error text leaked to client: %s", rec.Body)
		}
	}
}

func TestRecoverer(t *testing.T) {
	t.Run("panic before any output gives a JSON 500", func(t *testing.T) {
		h := httpx.Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"internal"`) {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
		}
	})
	t.Run("panic after output started does not append a second response", func(t *testing.T) {
		h := httpx.Recoverer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("partial"))
			panic("boom")
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusAccepted || rec.Body.String() != "partial" {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body)
		}
	})
	t.Run("ErrAbortHandler is re-panicked", func(t *testing.T) {
		h := httpx.Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
		defer func() {
			if rec := recover(); !errors.Is(asError(rec), http.ErrAbortHandler) {
				t.Fatalf("recovered %v, want http.ErrAbortHandler", rec)
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

func asError(v any) error {
	err, _ := v.(error)
	return err
}

func TestRetryAfterHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.Error(rec, httptest.NewRequest(http.MethodGet, "/", nil), apperr.RateLimited("c", "m", 15*time.Minute))
	if got := rec.Header().Get("Retry-After"); got != "900" {
		t.Errorf("Retry-After = %q, want 900", got)
	}
	rec = httptest.NewRecorder()
	httpx.Error(rec, httptest.NewRequest(http.MethodGet, "/", nil), apperr.RateLimited("c", "m", 0))
	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Errorf("Retry-After = %q, want none when no delay is known", got)
	}
}

func TestOriginCheck(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := httpx.OriginCheck("http://localhost:5173")(ok)
	tests := []struct {
		name, method, origin string
		want                 int
	}{
		{"no origin", "POST", "", 204},
		{"same host", "POST", "https://example.com", 204},
		{"same host, different case", "POST", "https://EXAMPLE.com", 204},
		{"allow-listed, different case", "POST", "HTTP://LOCALHOST:5173", 204},
		{"foreign", "POST", "https://evil.example", 403},
		{"opaque null origin", "POST", "null", 403},
		{"look-alike suffix", "POST", "https://example.com.evil.example", 403},
		{"safe method ignores origin", "GET", "https://evil.example", 204},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, "/", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}
