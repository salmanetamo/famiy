package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/salmanetamo/famiy/famiy-api/internal/app"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

func patchPerson(t *testing.T, h http.Handler, c *http.Cookie, id int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	return send(t, h, "PATCH", "/persons/"+itoa(id), body, withCookie(c))
}

func getPerson(t *testing.T, h http.Handler, c *http.Cookie, id int64) map[string]any {
	t.Helper()
	rec := send(t, h, "GET", "/persons/"+itoa(id), "", withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET person %d: %d %s", id, rec.Code, rec.Body)
	}
	return decode(t, rec)
}

func mkFull(t *testing.T, h http.Handler, c *http.Cookie) int64 {
	t.Helper()
	rec := send(t, h, "POST", "/persons",
		`{"given_name":"Ada","family_name":"Lovelace","birth_date":"1815-12-10","birth_approx":true,"death_date":"1852-11-27"}`, withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	return int64(decode(t, rec)["id"].(float64))
}

func TestPatchPersonMergeSemantics(t *testing.T) {
	// Each case starts from Ada Lovelace (born 1815-12-10 approx, died 1852-11-27).
	tests := []struct {
		name string
		body string
		want map[string]any // expected fields after the patch; others must be unchanged
	}{
		{"empty patch is a no-op", `{}`, nil},
		{"set given name", `{"given_name":"Augusta"}`, map[string]any{"given_name": "Augusta"}},
		{"names are trimmed", `{"given_name":"  Augusta ","family_name":" King "}`, map[string]any{"given_name": "Augusta", "family_name": "King"}},
		{"null clears family name", `{"family_name":null}`, map[string]any{"family_name": nil}},
		{"blank clears family name", `{"family_name":"  "}`, map[string]any{"family_name": nil}},
		{"null clears death date", `{"death_date":null}`, map[string]any{"death_date": nil}},
		{"set death date", `{"death_date":"1853-01-02"}`, map[string]any{"death_date": "1853-01-02"}},
		{"clear birth date and approx together", `{"birth_date":null,"birth_approx":false}`, map[string]any{"birth_date": nil, "birth_approx": false}},
		{"turn approx off", `{"birth_approx":false}`, map[string]any{"birth_approx": false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newApp(t)
			c, _ := signIn(t, h, "ada@example.com")
			id := mkFull(t, h, c)
			before := getPerson(t, h, c, id)

			rec := patchPerson(t, h, c, id, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("PATCH: %d %s", rec.Code, rec.Body)
			}
			got := decode(t, rec)
			if again := getPerson(t, h, c, id); !jsonEqual(got, again) {
				t.Errorf("PATCH response and a later GET differ:\n%v\n%v", got, again)
			}
			for k, v := range before {
				want := v
				if w, ok := tc.want[k]; ok {
					want = w
				}
				if got[k] != want {
					t.Errorf("%s = %v, want %v", k, got[k], want)
				}
			}
		})
	}
}

func jsonEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestPatchPersonRejectsBadInput(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	id := mkFull(t, h, c)
	before := getPerson(t, h, c, id)

	tests := []struct {
		name, body, code string
	}{
		{"given_name null", `{"given_name":null}`, "validation_failed"},
		{"birth_approx null", `{"birth_approx":null}`, "validation_failed"},
		{"blank given name", `{"given_name":"  "}`, "validation_failed"},
		{"NUL in given name", `{"given_name":"A\u0000"}`, "validation_failed"},
		{"NUL in family name", `{"family_name":"B\u0000"}`, "validation_failed"},
		{"too long", `{"family_name":"` + strings.Repeat("x", 201) + `"}`, "validation_failed"},
		{"birth date cleared but approx stays", `{"birth_date":null}`, "validation_failed"},
		{"death before the stored birth", `{"death_date":"1800-01-01"}`, "validation_failed"},
		{"future date", `{"death_date":"2999-01-01"}`, "validation_failed"},
		{"malformed date", `{"birth_date":"10/12/1815"}`, "validation_failed"},
		{"unknown field", `{"nickname":"A"}`, "invalid_json"},
		{"id is not patchable", `{"id":99}`, "invalid_json"},
		{"created_at is not patchable", `{"created_at":"2020-01-01T00:00:00Z"}`, "invalid_json"},
		{"wrong type", `{"given_name":5}`, "invalid_json"},
		{"not an object", `[]`, "invalid_json"},
		{"truncated", `{"given_name":`, "invalid_json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := patchPerson(t, h, c, id, tc.body)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"`+tc.code+`"`) {
				t.Fatalf("got %d %s, want 400 %s", rec.Code, rec.Body, tc.code)
			}
		})
	}
	if after := getPerson(t, h, c, id); !jsonEqual(before, after) {
		t.Errorf("a rejected patch changed the person:\n%v\n%v", before, after)
	}

	t.Run("bad id", func(t *testing.T) {
		rec := send(t, h, "PATCH", "/persons/abc", `{}`, withCookie(c))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_id") {
			t.Fatalf("got %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("wrong content type", func(t *testing.T) {
		rec := send(t, h, "PATCH", "/persons/"+itoa(id), `{}`, withCookie(c), withHeader("Content-Type", "application/merge-patch+json"))
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("got %d %s, want 415", rec.Code, rec.Body)
		}
	})
}

func TestPatchDuplicateKeysLastOneWins(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	id := mkFull(t, h, c)

	rec := patchPerson(t, h, c, id, `{"family_name":null,"family_name":"King"}`)
	if rec.Code != http.StatusOK || decode(t, rec)["family_name"] != "King" {
		t.Fatalf("null then value: %d %s, want family_name King", rec.Code, rec.Body)
	}
	rec = patchPerson(t, h, c, id, `{"family_name":"Byron","family_name":null}`)
	if rec.Code != http.StatusOK || decode(t, rec)["family_name"] != nil {
		t.Fatalf("value then null: %d %s, want family_name null", rec.Code, rec.Body)
	}
}

func TestPatchWithANullBodyIsANoOp(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	id := mkFull(t, h, c)
	before := getPerson(t, h, c, id)
	rec := patchPerson(t, h, c, id, `null`)
	if rec.Code != http.StatusOK || !jsonEqual(decode(t, rec), before) {
		t.Fatalf("got %d %s, want 200 and the unchanged person", rec.Code, rec.Body)
	}
}

func TestEditRoutesRequireASession(t *testing.T) {
	h, _ := newApp(t)
	for _, rt := range []struct{ method, body string }{{"PATCH", `{}`}, {"DELETE", ""}} {
		rec := send(t, h, rt.method, "/persons/1", rt.body)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
			t.Errorf("%s: got %d %s, want 401", rt.method, rec.Code, rec.Body)
		}
	}
}

func TestPatchAndDeleteMissingPerson(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"PATCH":  patchPerson(t, h, c, 4242, `{"given_name":"X"}`),
		"DELETE": send(t, h, "DELETE", "/persons/4242", "", withCookie(c)),
	} {
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"person_not_found"`) {
			t.Errorf("%s: got %d %s, want 404 person_not_found", name, rec.Code, rec.Body)
		}
	}
}

// Alice owns a small family; Bob owns people too (so the 404 is not an artefact
// of an empty account). Everything Bob tries against Alice's people must look
// exactly like a missing id, and must change nothing.
func TestPatchAndDeleteCannotTouchAnotherAccount(t *testing.T) {
	h, pool := newAppWithPool(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	mum, kid := mkFull(t, h, alice), mkPerson(t, h, alice, "kid")
	mkLink(t, h, alice, mum, kid)
	mkPartnership(t, pool, mum, kid, "marriage")
	mkPerson(t, h, bob, "bob-person")

	graphBefore := send(t, h, "GET", "/graph", "", withCookie(alice)).Body.String()
	personBefore := getPerson(t, h, alice, mum)

	for _, id := range []int64{mum, kid} {
		patched := patchPerson(t, h, bob, id, `{"given_name":"Hacked","family_name":null}`)
		missingPatch := patchPerson(t, h, bob, 999999, `{"given_name":"Hacked","family_name":null}`)
		if patched.Code != http.StatusNotFound || patched.Body.String() != missingPatch.Body.String() {
			t.Errorf("PATCH %d: got %d %s; want the same 404 as a missing id (%s)", id, patched.Code, patched.Body, missingPatch.Body)
		}
		deleted := send(t, h, "DELETE", "/persons/"+itoa(id), "", withCookie(bob))
		missingDelete := send(t, h, "DELETE", "/persons/999999", "", withCookie(bob))
		if deleted.Code != http.StatusNotFound || deleted.Body.String() != missingDelete.Body.String() {
			t.Errorf("DELETE %d: got %d %s; want the same 404 as a missing id (%s)", id, deleted.Code, deleted.Body, missingDelete.Body)
		}
	}

	if after := send(t, h, "GET", "/graph", "", withCookie(alice)).Body.String(); after != graphBefore {
		t.Errorf("Alice's graph changed:\n%s\n%s", graphBefore, after)
	}
	if after := getPerson(t, h, alice, mum); !jsonEqual(personBefore, after) {
		t.Errorf("Alice's person changed:\n%v\n%v", personBefore, after)
	}
}

func TestDeletePersonRemovesTheirLinksAndPartnershipsOnly(t *testing.T) {
	h, pool := newAppWithPool(t)
	c, _ := signIn(t, h, "ada@example.com")
	// grandma -> mum -> kid, dad -> kid, and mum is partnered with dad.
	grandma, mum, dad, kid := mkPerson(t, h, c, "grandma"), mkPerson(t, h, c, "mum"), mkPerson(t, h, c, "dad"), mkPerson(t, h, c, "kid")
	mkLink(t, h, c, grandma, mum)
	mkLink(t, h, c, mum, kid)
	mkLink(t, h, c, dad, kid)
	mkPartnership(t, pool, mum, dad, "marriage")

	rec := send(t, h, "DELETE", "/persons/"+itoa(mum), "", withCookie(c))
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("DELETE: %d %q, want 204 and no body", rec.Code, rec.Body)
	}

	if gone := send(t, h, "GET", "/persons/"+itoa(mum), "", withCookie(c)); gone.Code != http.StatusNotFound {
		t.Errorf("deleted person still readable: %d", gone.Code)
	}
	if again := send(t, h, "DELETE", "/persons/"+itoa(mum), "", withCookie(c)); again.Code != http.StatusNotFound {
		t.Errorf("second DELETE: %d, want 404", again.Code)
	}

	g, _ := getGraph(t, h, c)
	if got := g.ids(); len(got) != 3 || got[0] != grandma || got[1] != dad || got[2] != kid {
		t.Errorf("remaining people = %v, want grandma, dad and kid", got)
	}
	// Only dad -> kid is left: mum's link up (grandma -> mum) and down (mum -> kid) went with her.
	if len(g.ParentLinks) != 1 || g.ParentLinks[0].ParentID != dad || g.ParentLinks[0].ChildID != kid {
		t.Errorf("parent links = %+v, want only dad -> kid", g.ParentLinks)
	}
	if len(g.Partnerships) != 0 {
		t.Errorf("partnerships = %+v, want none", g.Partnerships)
	}
	// The neighbours themselves are untouched.
	for _, id := range []int64{grandma, dad, kid} {
		getPerson(t, h, c, id)
	}
}

func TestDeleteFreesCapacityUnderThePersonLimit(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPersonsPerAccount = 1
	h, _, err := app.NewRouter(testdb.New(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := signIn(t, h, "ada@example.com")
	first := mkPerson(t, h, c, "first")
	if rec := send(t, h, "POST", "/persons", `{"given_name":"second"}`, withCookie(c)); rec.Code != http.StatusConflict {
		t.Fatalf("over the cap: %d", rec.Code)
	}
	if rec := send(t, h, "DELETE", "/persons/"+itoa(first), "", withCookie(c)); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", rec.Code)
	}
	mkPerson(t, h, c, "second") // fits again
}
