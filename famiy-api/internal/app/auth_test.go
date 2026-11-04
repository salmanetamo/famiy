package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/salmanetamo/famiy/famiy-api/internal/app"
	"github.com/salmanetamo/famiy/famiy-api/internal/identity"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

const pw = "correct horse battery staple"

type reqOpt func(*http.Request)

func withCookie(c *http.Cookie) reqOpt { return func(r *http.Request) { r.AddCookie(c) } }
func withHeader(k, v string) reqOpt    { return func(r *http.Request) { r.Header.Set(k, v) } }

func send(t *testing.T, h http.Handler, method, path, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "famiy_session" {
			return c
		}
	}
	t.Fatalf("no session cookie in response (status %d, body %s)", rec.Code, rec.Body)
	return nil
}

func creds(email, password string) string {
	return `{"email":"` + email + `","password":"` + password + `"}`
}

func TestAuthFlowOverHTTP(t *testing.T) {
	h, _ := newApp(t)

	rec := send(t, h, "POST", "/auth/register", creds("Ada@Example.com", pw))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "password") || strings.Contains(body, pw) {
		t.Fatalf("response leaks credentials: %s", body)
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Expires.IsZero() {
		t.Errorf("weak cookie attributes: %+v", c)
	}

	me := send(t, h, "GET", "/auth/me", "", withCookie(c))
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"ada@example.com"`) {
		t.Fatalf("me: %d %s", me.Code, me.Body)
	}
	if send(t, h, "GET", "/auth/me", "").Code != http.StatusUnauthorized {
		t.Error("/auth/me without a cookie must be 401")
	}
	bogus := &http.Cookie{Name: "famiy_session", Value: "not-a-real-token"}
	if send(t, h, "GET", "/auth/me", "", withCookie(bogus)).Code != http.StatusUnauthorized {
		t.Error("/auth/me with a bogus cookie must be 401")
	}

	login := send(t, h, "POST", "/auth/login", creds("ada@example.com", pw))
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	c2 := sessionCookie(t, login)

	out := send(t, h, "POST", "/auth/logout", "", withCookie(c2))
	if out.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", out.Code)
	}
	if cleared := sessionCookie(t, out); cleared.MaxAge >= 0 {
		t.Errorf("logout did not clear the cookie: %+v", cleared)
	}
	if send(t, h, "GET", "/auth/me", "", withCookie(c2)).Code != http.StatusUnauthorized {
		t.Error("session still valid after logout")
	}
}

func TestAuthErrorsOverHTTP(t *testing.T) {
	h, _ := newApp(t)
	send(t, h, "POST", "/auth/register", creds("ada@example.com", pw))

	tests := []struct {
		name, path, body string
		want             int
		code             string
	}{
		{"duplicate email", "/auth/register", creds("ada@example.com", pw), 409, "email_taken"},
		{"short password", "/auth/register", creds("bob@example.com", "short"), 400, "validation_failed"},
		{"bad email", "/auth/register", creds("nope", pw), 400, "validation_failed"},
		{"wrong password", "/auth/login", creds("ada@example.com", "not the password"), 401, "invalid_credentials"},
		{"unknown email", "/auth/login", creds("zed@example.com", pw), 401, "invalid_credentials"},
		{"unknown field", "/auth/login", `{"email":"a@b.co","password":"x","admin":true}`, 400, "invalid_json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := send(t, h, "POST", tc.path, tc.body)
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), `"`+tc.code+`"`) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, tc.want, tc.code)
			}
		})
	}
}

func TestLoginRateLimitOverHTTP(t *testing.T) {
	h, _ := newApp(t)
	send(t, h, "POST", "/auth/register", creds("ada@example.com", pw))
	for i := 0; i < 5; i++ {
		send(t, h, "POST", "/auth/login", creds("ada@example.com", "wrong password here"))
	}
	rec := send(t, h, "POST", "/auth/login", creds("ada@example.com", pw))
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "too_many_attempts") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestRegistrationControlsOverHTTP(t *testing.T) {
	build := func(t *testing.T, mutate func(*app.Config)) http.Handler {
		t.Helper()
		c := testConfig()
		mutate(&c)
		h, _, err := app.NewRouter(testdb.New(t), c)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	t.Run("kill switch", func(t *testing.T) {
		h := build(t, func(c *app.Config) { c.Identity.RegistrationEnabled = false })
		rec := send(t, h, "POST", "/auth/register", creds("ada@example.com", pw))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "registration_closed") {
			t.Fatalf("got %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("cap", func(t *testing.T) {
		h := build(t, func(c *app.Config) { c.Identity.MaxUsers = 1 })
		if rec := send(t, h, "POST", "/auth/register", creds("a@example.com", pw)); rec.Code != http.StatusCreated {
			t.Fatalf("first: %d", rec.Code)
		}
		rec := send(t, h, "POST", "/auth/register", creds("b@example.com", pw))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "registration_closed") {
			t.Fatalf("second: %d %s", rec.Code, rec.Body)
		}
	})
}

func TestCSRFGuards(t *testing.T) {
	h, _ := newApp(t)
	body := creds("ada@example.com", pw)

	t.Run("foreign origin is refused", func(t *testing.T) {
		rec := send(t, h, "POST", "/auth/register", body, withHeader("Origin", "https://evil.example"))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "origin_not_allowed") {
			t.Fatalf("got %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("same origin is accepted", func(t *testing.T) {
		// httptest requests carry Host "example.com".
		rec := send(t, h, "POST", "/auth/register", body, withHeader("Origin", "http://example.com"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("got %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("non-JSON body is refused", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=a&password=b"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("got %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("safe methods ignore origin", func(t *testing.T) {
		if rec := send(t, h, "GET", "/healthz", "", withHeader("Origin", "https://evil.example")); rec.Code != http.StatusOK {
			t.Fatalf("got %d", rec.Code)
		}
	})
	t.Run("configured extra origin is accepted", func(t *testing.T) {
		c := testConfig()
		c.AllowedOrigins = []string{"http://localhost:5173"}
		h2, _, err := app.NewRouter(testdb.New(t), c)
		if err != nil {
			t.Fatal(err)
		}
		rec := send(t, h2, "POST", "/auth/register", body, withHeader("Origin", "http://localhost:5173"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("got %d %s", rec.Code, rec.Body)
		}
	})
}

func TestInsecureCookieOnlyWhenConfigured(t *testing.T) {
	c := testConfig()
	c.Handler = identity.HandlerConfig{CookieSecure: false}
	h, _, err := app.NewRouter(testdb.New(t), c)
	if err != nil {
		t.Fatal(err)
	}
	rec := send(t, h, "POST", "/auth/register", creds("ada@example.com", pw))
	if sessionCookie(t, rec).Secure {
		t.Error("Secure attribute set although COOKIE_SECURE=false")
	}
}
