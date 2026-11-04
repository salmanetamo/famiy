package app_test

import (
	"context"
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
	"github.com/salmanetamo/famiy/famiy-api/internal/relationship"
)

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
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
	rec := do(t, h, http.MethodGet, "/persons/4242", "") // an error response
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("X-Request-Id header missing")
	}
}

func TestCreateAndGetPerson(t *testing.T) {
	h, _ := newApp(t)

	rec := do(t, h, http.MethodPost, "/persons",
		`{"given_name":"  Ada ","family_name":"Lovelace","birth_date":"1815-12-10","birth_approx":true}`)
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

	rec = do(t, h, http.MethodGet, "/persons/"+itoa(id), "")
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

func TestPersonErrors(t *testing.T) {
	h, _ := newApp(t)
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
			rec := do(t, h, tc.method, tc.path, tc.body)
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
	h, svcs := newApp(t)
	ids := map[string]int64{}
	for _, n := range []string{"grand", "parent", "child"} {
		rec := do(t, h, http.MethodPost, "/persons", `{"given_name":"`+n+`"}`)
		ids[n] = int64(decode(t, rec)["id"].(float64))
	}
	ctx := context.Background()
	for _, l := range []relationship.ParentLink{
		{ParentID: ids["grand"], ChildID: ids["parent"]},
		{ParentID: ids["parent"], ChildID: ids["child"]},
	} {
		if err := svcs.Relationships.AddParentLink(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	rec := do(t, h, http.MethodGet, "/persons/"+itoa(ids["child"])+"/ancestors", "")
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
