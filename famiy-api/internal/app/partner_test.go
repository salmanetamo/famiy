package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func partnerPath(person, partner int64) string {
	return "/persons/" + itoa(person) + "/partners/" + itoa(partner)
}

func addPartner(t *testing.T, h http.Handler, c *http.Cookie, person int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	return send(t, h, "POST", "/persons/"+itoa(person)+"/partners", body, withCookie(c))
}

func mkPartner(t *testing.T, h http.Handler, c *http.Cookie, person, partner int64, kind string) {
	t.Helper()
	rec := addPartner(t, h, c, person, `{"partner_id":`+itoa(partner)+`,"kind":"`+kind+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("partner %d-%d: %d %s", person, partner, rec.Code, rec.Body)
	}
}

func TestAddPartnerCreatesThePartnership(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	a, b, d := mkPerson(t, h, c, "a"), mkPerson(t, h, c, "b"), mkPerson(t, h, c, "d")

	// Default kind, and the pair named in descending order (b > a).
	rec := addPartner(t, h, c, b, `{"partner_id":`+itoa(a)+`}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	if m := decode(t, rec); int64(m["partner_id"].(float64)) != a || m["kind"] != "marriage" {
		t.Errorf("response = %v, want partner_id %d and kind marriage", m, a)
	}
	// A second partner for the same person (remarriage), with an explicit kind.
	mkPartner(t, h, c, a, d, "partnership")

	g, _ := getGraph(t, h, c)
	if len(g.Partnerships) != 2 {
		t.Fatalf("partnerships = %+v, want 2", g.Partnerships)
	}
	kinds := map[string]bool{}
	for _, p := range g.Partnerships {
		kinds[p.Kind] = true
		if !sameSet(p.Partners[:], []int64{a, b}) && !sameSet(p.Partners[:], []int64{a, d}) {
			t.Errorf("unexpected pair %v", p.Partners)
		}
	}
	if !kinds["marriage"] || !kinds["partnership"] {
		t.Errorf("kinds = %v", kinds)
	}
}

func TestAddPartnerErrors(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	a, b, third := mkPerson(t, h, c, "a"), mkPerson(t, h, c, "b"), mkPerson(t, h, c, "third")
	mkPartner(t, h, c, a, b, "marriage")

	tests := []struct {
		name         string
		person, body string
		status       int
		code         string
	}{
		{"duplicate", itoa(a), `{"partner_id":` + itoa(b) + `}`, 409, "duplicate_partnership"},
		{"duplicate in reverse", itoa(b), `{"partner_id":` + itoa(a) + `,"kind":"other"}`, 409, "duplicate_partnership"},
		{"self", itoa(a), `{"partner_id":` + itoa(a) + `}`, 400, "self_partner"},
		{"unknown kind", itoa(a), `{"partner_id":` + itoa(third) + `,"kind":"friends"}`, 400, "validation_failed"},
		{"empty kind", itoa(a), `{"partner_id":` + itoa(third) + `,"kind":""}`, 400, "validation_failed"},
		{"null kind is not the default", itoa(a), `{"partner_id":` + itoa(third) + `,"kind":null}`, 400, "validation_failed"},
		{"partner_id missing", itoa(a), `{}`, 400, "validation_failed"},
		{"partner_id zero", itoa(a), `{"partner_id":0}`, 400, "validation_failed"},
		{"unknown field", itoa(a), `{"partner_id":1,"since":"2001"}`, 400, "invalid_json"},
		{"kind wrong type", itoa(a), `{"partner_id":` + itoa(third) + `,"kind":5}`, 400, "invalid_json"},
		{"bad person id", "abc", `{"partner_id":1}`, 400, "invalid_id"},
		{"person missing", "999999", `{"partner_id":` + itoa(a) + `}`, 404, "person_not_found"},
		{"partner missing", itoa(a), `{"partner_id":999999}`, 404, "person_not_found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := send(t, h, "POST", "/persons/"+tc.person+"/partners", tc.body, withCookie(c))
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), `"`+tc.code+`"`) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
	if g, _ := getGraph(t, h, c); len(g.Partnerships) != 1 {
		t.Errorf("a rejected request changed the partnerships: %+v", g.Partnerships)
	}
}

func TestPartnerRoutesRequireASession(t *testing.T) {
	h, _ := newApp(t)
	for _, rt := range []struct{ method, path, body string }{
		{"POST", "/persons/1/partners", `{"partner_id":2}`},
		{"PATCH", "/persons/1/partners/2", `{"kind":"other"}`},
		{"DELETE", "/persons/1/partners/2", ""},
	} {
		rec := send(t, h, rt.method, rt.path, rt.body)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
			t.Errorf("%s %s: got %d %s, want 401", rt.method, rt.path, rec.Code, rec.Body)
		}
	}
}

func TestUpdatePartnerKind(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	a, b := mkPerson(t, h, c, "a"), mkPerson(t, h, c, "b")
	mkPartner(t, h, c, a, b, "partnership")

	kindOf := func() string {
		g, _ := getGraph(t, h, c)
		return g.Partnerships[0].Kind
	}
	// Change it, naming the pair in the opposite order to how it was created.
	rec := send(t, h, "PATCH", partnerPath(b, a), `{"kind":"marriage"}`, withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: %d %s", rec.Code, rec.Body)
	}
	if m := decode(t, rec); int64(m["partner_id"].(float64)) != a || m["kind"] != "marriage" {
		t.Errorf("response = %v, want partner_id %d (the person named last) and kind marriage", m, a)
	}
	if k := kindOf(); k != "marriage" {
		t.Errorf("stored kind = %q", k)
	}

	// {} changes nothing and returns the partnership.
	rec = send(t, h, "PATCH", partnerPath(a, b), `{}`, withCookie(c))
	if rec.Code != http.StatusOK || decode(t, rec)["kind"] != "marriage" {
		t.Errorf("{}: %d %s, want 200 and the unchanged kind", rec.Code, rec.Body)
	}

	for name, body := range map[string]string{
		"null kind":     `{"kind":null}`,
		"unknown kind":  `{"kind":"friends"}`,
		"wrong type":    `{"kind":7}`,
		"unknown field": `{"since":"2001"}`,
	} {
		rec := send(t, h, "PATCH", partnerPath(a, b), body, withCookie(c))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s, want 400", name, rec.Code, rec.Body)
		}
	}
	if k := kindOf(); k != "marriage" {
		t.Errorf("a rejected patch changed the kind to %q", k)
	}

	other := mkPerson(t, h, c, "other")
	rec = send(t, h, "PATCH", partnerPath(a, other), `{"kind":"other"}`, withCookie(c))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"partnership_not_found"`) {
		t.Errorf("unrelated pair: got %d %s, want 404 partnership_not_found", rec.Code, rec.Body)
	}
	if rec := send(t, h, "PATCH", partnerPath(a, a), `{"kind":"other"}`, withCookie(c)); rec.Code != http.StatusBadRequest {
		t.Errorf("self: got %d, want 400", rec.Code)
	}
}

func TestRemovePartner(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	a, b, d := mkPerson(t, h, c, "a"), mkPerson(t, h, c, "b"), mkPerson(t, h, c, "d")
	mkPartner(t, h, c, a, b, "marriage")
	mkPartner(t, h, c, a, d, "marriage")

	rec := send(t, h, "DELETE", partnerPath(b, a), "", withCookie(c)) // reverse order
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("DELETE: %d %q, want 204 and no body", rec.Code, rec.Body)
	}
	if g, _ := getGraph(t, h, c); len(g.Partnerships) != 1 || !sameSet(g.Partnerships[0].Partners[:], []int64{a, d}) {
		t.Errorf("partnerships = %+v, want only a-d", g.Partnerships)
	}
	for _, path := range []string{partnerPath(a, b), partnerPath(b, a)} {
		rec := send(t, h, "DELETE", path, "", withCookie(c))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"partnership_not_found"`) {
			t.Errorf("DELETE %s again: %d %s, want 404 partnership_not_found", path, rec.Code, rec.Body)
		}
	}
	if rec := send(t, h, "DELETE", partnerPath(a, a), "", withCookie(c)); rec.Code != http.StatusBadRequest {
		t.Errorf("self: got %d, want 400", rec.Code)
	}
	if rec := send(t, h, "DELETE", "/persons/x/partners/1", "", withCookie(c)); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: got %d, want 400", rec.Code)
	}
	if rec := send(t, h, "DELETE", partnerPath(a, 999999), "", withCookie(c)); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"person_not_found"`) {
		t.Errorf("missing person: got %d %s", rec.Code, rec.Body)
	}
}

func TestDeletingAPersonRemovesTheirPartnerships(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	a, b, d := mkPerson(t, h, c, "a"), mkPerson(t, h, c, "b"), mkPerson(t, h, c, "d")
	mkPartner(t, h, c, a, b, "marriage")
	mkPartner(t, h, c, b, d, "marriage")
	if rec := send(t, h, "DELETE", "/persons/"+itoa(b), "", withCookie(c)); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE person: %d", rec.Code)
	}
	if g, _ := getGraph(t, h, c); len(g.Partnerships) != 0 || len(g.Persons) != 2 {
		t.Errorf("graph = %+v, want a and d and no partnerships", g)
	}
}

// Alice owns a, b and c; Bob owns x. Everything Bob tries against Alice's
// people, and everything Alice tries against Bob's, must look like a missing
// person and change nothing. The case where BOTH people are Alice's is the one
// the database trigger cannot catch (it sees one owner); only the service can.
func TestPartnerRoutesCannotReachAnotherAccount(t *testing.T) {
	h, _ := newApp(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	a, b, c := mkPerson(t, h, alice, "a"), mkPerson(t, h, alice, "b"), mkPerson(t, h, alice, "c")
	x := mkPerson(t, h, bob, "x")
	mkPartner(t, h, alice, a, b, "marriage")

	aliceBefore := send(t, h, "GET", "/graph", "", withCookie(alice)).Body.String()
	bobBefore := send(t, h, "GET", "/graph", "", withCookie(bob)).Body.String()

	type try struct {
		name   string
		who    *http.Cookie
		method string
		path   string
		body   string
	}
	tries := []try{
		{"Bob adds Alice's pair", bob, "POST", "/persons/" + itoa(a) + "/partners", `{"partner_id":` + itoa(c) + `}`},
		{"Bob patches Alice's pair", bob, "PATCH", partnerPath(a, b), `{"kind":"other"}`},
		{"Bob deletes Alice's pair", bob, "DELETE", partnerPath(a, b), ""},
		{"Bob reads Alice's pair with {}", bob, "PATCH", partnerPath(a, b), `{}`},
		{"Bob adds his person to Alice's", bob, "POST", "/persons/" + itoa(x) + "/partners", `{"partner_id":` + itoa(a) + `}`},
		{"Alice adds her person to Bob's", alice, "POST", "/persons/" + itoa(a) + "/partners", `{"partner_id":` + itoa(x) + `}`},
		{"Alice adds Bob's person as the subject", alice, "POST", "/persons/" + itoa(x) + "/partners", `{"partner_id":` + itoa(a) + `}`},
		{"Alice patches a pair with Bob's person", alice, "PATCH", partnerPath(a, x), `{"kind":"other"}`},
		{"Alice deletes a pair with Bob's person", alice, "DELETE", partnerPath(a, x), ""},
	}
	for _, tc := range tries {
		t.Run(tc.name, func(t *testing.T) {
			got := send(t, h, tc.method, tc.path, tc.body, withCookie(tc.who))
			if got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), `"person_not_found"`) {
				t.Fatalf("got %d %s, want 404 person_not_found", got.Code, got.Body)
			}
			// Indistinguishable from a person that does not exist at all.
			var missingPath string
			switch tc.method {
			case "POST":
				missingPath = "/persons/999999/partners"
			default:
				missingPath = partnerPath(999999, 999998)
			}
			missing := send(t, h, tc.method, missingPath, tc.body, withCookie(tc.who))
			if missing.Code != got.Code || missing.Body.String() != got.Body.String() {
				t.Errorf("differs from a missing person:\n%d %s\n%d %s", got.Code, got.Body, missing.Code, missing.Body)
			}
		})
	}

	if after := send(t, h, "GET", "/graph", "", withCookie(alice)).Body.String(); after != aliceBefore {
		t.Errorf("Alice's graph changed:\n%s\n%s", aliceBefore, after)
	}
	if after := send(t, h, "GET", "/graph", "", withCookie(bob)).Body.String(); after != bobBefore {
		t.Errorf("Bob's graph changed:\n%s\n%s", bobBefore, after)
	}
}
