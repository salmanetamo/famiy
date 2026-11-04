package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mkPerson(t *testing.T, h http.Handler, c *http.Cookie, name string) int64 {
	t.Helper()
	rec := send(t, h, "POST", "/persons", `{"given_name":"`+name+`"}`, withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: %d %s", name, rec.Code, rec.Body)
	}
	return int64(decode(t, rec)["id"].(float64))
}

func linkBody(parent int64) string { return `{"parent_id":` + itoa(parent) + `}` }

func mkLink(t *testing.T, h http.Handler, c *http.Cookie, parent, child int64) {
	t.Helper()
	rec := send(t, h, "POST", "/persons/"+itoa(child)+"/parents", linkBody(parent), withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("link %d -> %d: %d %s", parent, child, rec.Code, rec.Body)
	}
}

func TestAddParentCreatesTheLink(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	mum, dad, kid := mkPerson(t, h, c, "mum"), mkPerson(t, h, c, "dad"), mkPerson(t, h, c, "kid")

	rec := send(t, h, "POST", "/persons/"+itoa(kid)+"/parents", linkBody(mum), withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	if int64(body["child_id"].(float64)) != kid || int64(body["parent_id"].(float64)) != mum {
		t.Errorf("unexpected body: %v", body)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("a link has no URL of its own, got Location %q", loc)
	}

	mkLink(t, h, c, dad, kid)

	anc := decode(t, send(t, h, "GET", "/persons/"+itoa(kid)+"/ancestors", "", withCookie(c)))
	list, _ := anc["ancestors"].([]any)
	if len(list) != 2 {
		t.Fatalf("ancestors = %v, want both parents", anc)
	}
	for _, a := range list {
		if a.(map[string]any)["generations"] != float64(1) {
			t.Errorf("parent at wrong distance: %v", a)
		}
	}
}

func TestAddParentErrors(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	a, b, cc, d := mkPerson(t, h, c, "a"), mkPerson(t, h, c, "b"), mkPerson(t, h, c, "c"), mkPerson(t, h, c, "d")
	mkLink(t, h, c, a, b) // a is b's parent
	mkLink(t, h, c, b, cc)
	mkLink(t, h, c, a, d)
	mkLink(t, h, c, cc, d) // d now has two parents: a and cc

	tests := []struct {
		name, path, body string
		wantStatus       int
		wantCode         string
	}{
		{"own parent", "/persons/" + itoa(a) + "/parents", linkBody(a), 400, "self_parent"},
		{"duplicate link", "/persons/" + itoa(b) + "/parents", linkBody(a), 409, "duplicate_parent"},
		{"third parent", "/persons/" + itoa(d) + "/parents", linkBody(b), 409, "too_many_parents"},
		{"would close a loop", "/persons/" + itoa(a) + "/parents", linkBody(cc), 409, "would_create_cycle"},
		{"parent does not exist", "/persons/" + itoa(a) + "/parents", linkBody(99999), 404, "person_not_found"},
		{"child does not exist", "/persons/99999/parents", linkBody(a), 404, "person_not_found"},
		{"bad child id", "/persons/abc/parents", linkBody(a), 400, "invalid_id"},
		{"zero parent id", "/persons/" + itoa(a) + "/parents", `{"parent_id":0}`, 400, "validation_failed"},
		{"negative parent id", "/persons/" + itoa(a) + "/parents", `{"parent_id":-3}`, 400, "validation_failed"},
		{"missing parent id", "/persons/" + itoa(a) + "/parents", `{}`, 400, "validation_failed"},
		{"parent id as a string", "/persons/" + itoa(a) + "/parents", `{"parent_id":"2"}`, 400, "invalid_json"},
		{"unknown field", "/persons/" + itoa(a) + "/parents", `{"parent_id":2,"child_id":3}`, 400, "invalid_json"},
		{"empty body", "/persons/" + itoa(a) + "/parents", ``, 400, "invalid_json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := send(t, h, "POST", tc.path, tc.body, withCookie(c))
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), `"`+tc.wantCode+`"`) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestAddParentRequiresASession(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	p, k := mkPerson(t, h, c, "p"), mkPerson(t, h, c, "k")

	rec := send(t, h, "POST", "/persons/"+itoa(k)+"/parents", linkBody(p)) // no cookie
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d %s, want 401", rec.Code, rec.Body)
	}
	// And nothing was created.
	anc := decode(t, send(t, h, "GET", "/persons/"+itoa(k)+"/ancestors", "", withCookie(c)))
	if list, _ := anc["ancestors"].([]any); len(list) != 0 {
		t.Fatalf("an unauthenticated request created a link: %v", anc)
	}
}

// One account must not be able to attach its people to another account's, nor
// learn that those people exist.
func TestAddParentCannotReachAnotherAccount(t *testing.T) {
	h, _ := newApp(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	aliceKid, alicePerson := mkPerson(t, h, alice, "alice-kid"), mkPerson(t, h, alice, "alice-parent")
	bobPerson := mkPerson(t, h, bob, "bob-person")

	cases := []struct {
		name         string
		who          *http.Cookie
		child, other int64
	}{
		{"Alice names Bob's person as the parent", alice, aliceKid, bobPerson},
		{"Alice names Bob's person as the child", alice, bobPerson, alicePerson},
		{"Bob names Alice's person as the parent", bob, bobPerson, alicePerson},
		{"Bob names Alice's person as the child", bob, aliceKid, bobPerson},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			crossed := send(t, h, "POST", "/persons/"+itoa(tc.child)+"/parents", linkBody(tc.other), withCookie(tc.who))
			missing := send(t, h, "POST", "/persons/"+itoa(tc.child)+"/parents", linkBody(99999), withCookie(tc.who))
			if crossed.Code != http.StatusNotFound || !strings.Contains(crossed.Body.String(), "person_not_found") {
				t.Fatalf("got %d %s, want 404 person_not_found", crossed.Code, crossed.Body)
			}
			// When the child is the caller's own, "other account's person" must look
			// exactly like "no such person".
			if tc.child == aliceKid && tc.who == alice && crossed.Body.String() != missing.Body.String() {
				t.Errorf("responses differ:\n%s\n%s", crossed.Body, missing.Body)
			}
		})
	}

	// Neither account gained any ancestors.
	for _, id := range []int64{aliceKid, bobPerson} {
		cookie := alice
		if id == bobPerson {
			cookie = bob
		}
		anc := decode(t, send(t, h, "GET", "/persons/"+itoa(id)+"/ancestors", "", withCookie(cookie)))
		if list, _ := anc["ancestors"].([]any); len(list) != 0 {
			t.Errorf("person %d gained ancestors: %v", id, anc)
		}
	}
}

// The database refuses links that mix two accounts' people, but it cannot refuse
// this: both people belong to Bob, so only the service's ownership check stops
// Alice from rewiring Bob's family tree.
func TestAddParentCannotRewireAnotherAccountsTree(t *testing.T) {
	h, _ := newApp(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	bobParent, bobKid := mkPerson(t, h, bob, "bob-parent"), mkPerson(t, h, bob, "bob-kid")

	rec := send(t, h, "POST", "/persons/"+itoa(bobKid)+"/parents", linkBody(bobParent), withCookie(alice))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "person_not_found") {
		t.Fatalf("got %d %s, want 404 person_not_found", rec.Code, rec.Body)
	}
	anc := decode(t, send(t, h, "GET", "/persons/"+itoa(bobKid)+"/ancestors", "", withCookie(bob)))
	if list, _ := anc["ancestors"].([]any); len(list) != 0 {
		t.Fatalf("Alice created a link inside Bob's tree: %v", anc)
	}
}

func unlink(t *testing.T, h http.Handler, c *http.Cookie, child, parent int64, opts ...reqOpt) *httptest.ResponseRecorder {
	t.Helper()
	return send(t, h, "DELETE", "/persons/"+itoa(child)+"/parents/"+itoa(parent), "", append(opts, withCookie(c))...)
}

func ancestorCount(t *testing.T, h http.Handler, c *http.Cookie, id int64) int {
	t.Helper()
	anc := decode(t, send(t, h, "GET", "/persons/"+itoa(id)+"/ancestors", "", withCookie(c)))
	list, _ := anc["ancestors"].([]any)
	return len(list)
}

func TestRemoveParentDeletesTheLink(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	mum, dad, kid := mkPerson(t, h, c, "mum"), mkPerson(t, h, c, "dad"), mkPerson(t, h, c, "kid")
	mkLink(t, h, c, mum, kid)
	mkLink(t, h, c, dad, kid)

	rec := unlink(t, h, c, kid, mum)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("got %d with body %q, want 204 and no body", rec.Code, rec.Body)
	}
	if n := ancestorCount(t, h, c, kid); n != 1 {
		t.Fatalf("kid has %d ancestors after removing one parent, want 1", n)
	}

	again := unlink(t, h, c, kid, mum)
	if again.Code != http.StatusNotFound || !strings.Contains(again.Body.String(), "parent_link_not_found") {
		t.Fatalf("second removal: %d %s, want 404 parent_link_not_found", again.Code, again.Body)
	}
	// The freed slot can be reused.
	mkLink(t, h, c, mum, kid)
	if n := ancestorCount(t, h, c, kid); n != 2 {
		t.Fatalf("kid has %d ancestors after re-adding, want 2", n)
	}
}

func TestRemoveParentErrors(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	a, b := mkPerson(t, h, c, "a"), mkPerson(t, h, c, "b")
	mkLink(t, h, c, a, b)

	tests := []struct {
		name, path string
		wantStatus int
		wantCode   string
	}{
		{"people exist but are not linked", "/persons/" + itoa(a) + "/parents/" + itoa(b), 404, "parent_link_not_found"},
		{"parent does not exist", "/persons/" + itoa(b) + "/parents/99999", 404, "person_not_found"},
		{"child does not exist", "/persons/99999/parents/" + itoa(a), 404, "person_not_found"},
		{"bad child id", "/persons/abc/parents/" + itoa(a), 400, "invalid_id"},
		{"bad parent id", "/persons/" + itoa(b) + "/parents/xyz", 400, "invalid_id"},
		{"zero parent id", "/persons/" + itoa(b) + "/parents/0", 400, "invalid_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := send(t, h, "DELETE", tc.path, "", withCookie(c))
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), `"`+tc.wantCode+`"`) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, tc.wantStatus, tc.wantCode)
			}
		})
	}
	if n := ancestorCount(t, h, c, b); n != 1 {
		t.Fatalf("the real link was damaged by the failed requests: %d ancestors", n)
	}
}

func TestRemoveParentGuards(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	p, k := mkPerson(t, h, c, "p"), mkPerson(t, h, c, "k")
	mkLink(t, h, c, p, k)

	t.Run("requires a session", func(t *testing.T) {
		rec := send(t, h, "DELETE", "/persons/"+itoa(k)+"/parents/"+itoa(p), "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d %s, want 401", rec.Code, rec.Body)
		}
	})
	t.Run("a foreign origin cannot delete with the victim's cookie", func(t *testing.T) {
		rec := unlink(t, h, c, k, p, withHeader("Origin", "https://evil.example"))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "origin_not_allowed") {
			t.Fatalf("got %d %s, want 403 origin_not_allowed", rec.Code, rec.Body)
		}
	})
	if n := ancestorCount(t, h, c, k); n != 1 {
		t.Fatalf("a rejected request still removed the link: %d ancestors", n)
	}
}

func TestRemoveParentCannotTouchAnotherAccountsTree(t *testing.T) {
	h, _ := newApp(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	bobParent, bobKid := mkPerson(t, h, bob, "bob-parent"), mkPerson(t, h, bob, "bob-kid")
	mkLink(t, h, bob, bobParent, bobKid)
	aliceKid := mkPerson(t, h, alice, "alice-kid")

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"both people are Bob's":             unlink(t, h, alice, bobKid, bobParent),
		"child is Alice's, parent is Bob's": unlink(t, h, alice, aliceKid, bobParent),
	} {
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "person_not_found") {
			t.Errorf("%s: got %d %s, want 404 person_not_found", name, rec.Code, rec.Body)
		}
	}
	if n := ancestorCount(t, h, bob, bobKid); n != 1 {
		t.Fatalf("Alice removed a link inside Bob's tree: Bob's kid has %d ancestors", n)
	}
}
