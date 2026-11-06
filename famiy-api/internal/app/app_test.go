package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/app"
	"github.com/salmanetamo/famiy/famiy-api/internal/identity"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return send(t, h, method, path, body)
}

// testConfig opens registration and uses minimal argon2 cost so tests are fast.
func testConfig() app.Config {
	return app.Config{
		Identity: identity.Config{
			RegistrationEnabled: true,
			MaxUsers:            100,
			SessionTTL:          time.Hour,
			Params:              identity.Params{Memory: 8, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16},
		},
		Handler: identity.HandlerConfig{CookieSecure: true},
	}
}

func newApp(t *testing.T) (http.Handler, app.Services) {
	t.Helper()
	h, svcs, err := app.NewRouter(testdb.New(t), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	return h, svcs
}

// signIn registers an account over HTTP and returns its session cookie and id.
func signIn(t *testing.T, h http.Handler, email string) (*http.Cookie, int64) {
	t.Helper()
	rec := send(t, h, "POST", "/auth/register", creds(email, pw))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, rec.Code, rec.Body)
	}
	return sessionCookie(t, rec), int64(decode(t, rec)["id"].(float64))
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %q (%v)", rec.Body.String(), err)
	}
	return m
}

func TestHealthz(t *testing.T) {
	h, _ := newApp(t)
	rec := do(t, h, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if m := decode(t, rec); m["db"] != "up" {
		t.Fatalf("body = %v", m)
	}
}

func TestHealthzReportsDatabaseDown(t *testing.T) {
	pool := testdb.New(t)
	h, _, err := app.NewRouter(pool, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	pool.Close() // simulate the database going away
	rec := do(t, h, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestResponsesCarryRequestID(t *testing.T) {
	h, _ := newApp(t)
	rec := do(t, h, http.MethodGet, "/persons/4242", "") // an error response (401)
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("X-Request-Id header missing")
	}
}

func TestPersonRoutesRequireASession(t *testing.T) {
	h, _ := newApp(t)
	bogus := &http.Cookie{Name: identity.CookieName(true), Value: "not-a-real-token"}
	routes := []struct{ method, path, body string }{
		{"POST", "/persons", `{"given_name":"A"}`},
		{"GET", "/persons/1", ""},
		{"GET", "/persons/1/ancestors", ""},
	}
	for _, rt := range routes {
		for name, opts := range map[string][]reqOpt{"no cookie": nil, "bogus cookie": {withCookie(bogus)}} {
			rec := send(t, h, rt.method, rt.path, rt.body, opts...)
			if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
				t.Errorf("%s %s (%s): got %d %s, want 401 unauthenticated", rt.method, rt.path, name, rec.Code, rec.Body)
			}
		}
	}
}

func TestCreateAndGetPerson(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")

	rec := send(t, h, http.MethodPost, "/persons",
		`{"given_name":"  Ada ","family_name":"Lovelace","birth_date":"1815-12-10","birth_approx":true}`, withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", rec.Code, rec.Body)
	}
	created := decode(t, rec)
	id := int64(created["id"].(float64))
	if loc := rec.Header().Get("Location"); loc != "/persons/"+itoa(id) {
		t.Errorf("Location = %q", loc)
	}
	if created["given_name"] != "Ada" || created["birth_date"] != "1815-12-10" || created["birth_approx"] != true {
		t.Errorf("unexpected body: %v", created)
	}
	if _, leaked := created["owner_user_id"]; leaked {
		t.Error("response exposes owner_user_id")
	}

	rec = send(t, h, http.MethodGet, "/persons/"+itoa(id), "", withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	got := decode(t, rec)
	for _, k := range []string{"id", "given_name", "family_name", "birth_date", "birth_approx", "death_date"} {
		if got[k] != created[k] {
			t.Errorf("field %s: get=%v create=%v", k, got[k], created[k])
		}
	}
}

func TestAccountsCannotSeeEachOthersPeople(t *testing.T) {
	h, _ := newApp(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")

	rec := send(t, h, "POST", "/persons", `{"given_name":"Ada"}`, withCookie(alice))
	id := itoa(int64(decode(t, rec)["id"].(float64)))

	for _, path := range []string{"/persons/" + id, "/persons/" + id + "/ancestors"} {
		other := send(t, h, "GET", path, "", withCookie(bob))
		missing := send(t, h, "GET", strings.Replace(path, id, "424242", 1), "", withCookie(bob))
		if other.Code != http.StatusNotFound || !strings.Contains(other.Body.String(), "person_not_found") {
			t.Errorf("%s as another account: %d %s, want 404 person_not_found", path, other.Code, other.Body)
		}
		// Indistinguishable from a person that does not exist at all.
		if other.Body.String() != missing.Body.String() {
			t.Errorf("%s: other-owner and missing responses differ:\n%s\n%s", path, other.Body, missing.Body)
		}
		if mine := send(t, h, "GET", path, "", withCookie(alice)); mine.Code != http.StatusOK {
			t.Errorf("%s as the owner: %d", path, mine.Code)
		}
	}
}

func TestPersonErrors(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{"blank name", "POST", "/persons", `{"given_name":" "}`, 400, "validation_failed"},
		{"bad date", "POST", "/persons", `{"given_name":"A","birth_date":"10/12/1815"}`, 400, "validation_failed"},
		{"death before birth", "POST", "/persons", `{"given_name":"A","birth_date":"2000-01-02","death_date":"2000-01-01"}`, 400, "validation_failed"},
		{"unknown field", "POST", "/persons", `{"given_name":"A","nope":1}`, 400, "invalid_json"},
		{"malformed json", "POST", "/persons", `{`, 400, "invalid_json"},
		{"non-numeric id", "GET", "/persons/abc", "", 400, "invalid_id"},
		{"zero id", "GET", "/persons/0", "", 400, "invalid_id"},
		{"missing person", "GET", "/persons/4242", "", 404, "person_not_found"},
		{"ancestors of missing person", "GET", "/persons/4242/ancestors", "", 404, "person_not_found"},
		{"ancestors bad id", "GET", "/persons/x/ancestors", "", 400, "invalid_id"},
		{"trailing data", "POST", "/persons", `{"given_name":"A"} junk`, 400, "invalid_json"},
		{"wrong field type", "POST", "/persons", `{"given_name":5}`, 400, "invalid_json"},
		{"birth in the future", "POST", "/persons", `{"given_name":"A","birth_date":"2999-01-01"}`, 400, "validation_failed"},
		{"year zero", "POST", "/persons", `{"given_name":"A","birth_date":"0000-01-01"}`, 400, "validation_failed"},
		{"name too long", "POST", "/persons", `{"given_name":"` + strings.Repeat("x", 201) + `"}`, 400, "validation_failed"},
		{"oversized body", "POST", "/persons", `{"given_name":"` + strings.Repeat("x", 2<<20) + `"}`, 413, "request_too_large"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := send(t, h, tc.method, tc.path, tc.body, withCookie(c))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			errObj, _ := decode(t, rec)["error"].(map[string]any)
			if errObj["code"] != tc.wantCode {
				t.Fatalf("error code = %v, want %s (body %s)", errObj["code"], tc.wantCode, rec.Body)
			}
		})
	}
}

func TestAncestorsEndpoint(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	grand, parent, child := mkPerson(t, h, c, "grand"), mkPerson(t, h, c, "parent"), mkPerson(t, h, c, "child")
	mkLink(t, h, c, grand, parent)
	mkLink(t, h, c, parent, child)
	ids := map[string]int64{"grand": grand, "parent": parent, "child": child}

	rec := send(t, h, http.MethodGet, "/persons/"+itoa(ids["child"])+"/ancestors", "", withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	list, _ := body["ancestors"].([]any)
	if len(list) != 2 {
		t.Fatalf("ancestors = %v", body)
	}
	first := list[0].(map[string]any)
	second := list[1].(map[string]any)
	if int64(first["person_id"].(float64)) != ids["parent"] || first["generations"] != float64(1) ||
		int64(second["person_id"].(float64)) != ids["grand"] || second["generations"] != float64(2) {
		t.Fatalf("unexpected ancestors: %v", list)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestPersonLimitIsPerAccountOverHTTP(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPersonsPerAccount = 1
	h, _, err := app.NewRouter(testdb.New(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ada, _ := signIn(t, h, "ada@example.com")
	bob, _ := signIn(t, h, "bob@example.com")

	mkPerson(t, h, ada, "First")
	rec := send(t, h, "POST", "/persons", `{"given_name":"Second"}`, withCookie(ada))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"person_limit_reached"`) {
		t.Fatalf("over the cap: got %d %s, want 409 person_limit_reached", rec.Code, rec.Body)
	}
	mkPerson(t, h, bob, "Bob's first") // another account is unaffected
}
