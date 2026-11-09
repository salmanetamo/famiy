package app_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreatePersonWithBioAndResidence(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")

	rec := send(t, h, "POST", "/persons",
		`{"given_name":"Ada","bio":"  Mathematician.\nWrote the first program.  ","residence":" London, UK "}`, withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	created := decode(t, rec)
	if created["bio"] != "Mathematician.\nWrote the first program." || created["residence"] != "London, UK" {
		t.Errorf("created = %v, want trimmed text with the line break kept", created)
	}
	id := int64(created["id"].(float64))
	if got := getPerson(t, h, c, id); !jsonEqual(got, created) {
		t.Errorf("GET differs from the create response:\n%v\n%v", got, created)
	}

	// Left out, or blank: both read back as null.
	bare := decode(t, send(t, h, "POST", "/persons", `{"given_name":"Bare"}`, withCookie(c)))
	blank := decode(t, send(t, h, "POST", "/persons", `{"given_name":"Blank","bio":"  ","residence":""}`, withCookie(c)))
	for name, m := range map[string]map[string]any{"absent": bare, "blank": blank} {
		if v, ok := m["bio"]; !ok || v != nil {
			t.Errorf("%s: bio = %v (present=%v), want null", name, v, ok)
		}
		if v, ok := m["residence"]; !ok || v != nil {
			t.Errorf("%s: residence = %v (present=%v), want null", name, v, ok)
		}
	}
}

func TestCreatePersonRejectsBadBioAndResidence(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	for name, body := range map[string]string{
		"bio too long":       `{"given_name":"A","bio":"` + strings.Repeat("x", 2001) + `"}`,
		"residence too long": `{"given_name":"A","residence":"` + strings.Repeat("x", 201) + `"}`,
		"NUL in bio":         `{"given_name":"A","bio":"a\u0000b"}`,
		"NUL in residence":   `{"given_name":"A","residence":"a\u0000b"}`,
	} {
		rec := send(t, h, "POST", "/persons", body, withCookie(c))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"validation_failed"`) {
			t.Errorf("%s: got %d %s, want 400 validation_failed", name, rec.Code, rec.Body)
		}
	}
	if rec := send(t, h, "POST", "/persons", `{"given_name":"A","bio":5}`, withCookie(c)); rec.Code != http.StatusBadRequest {
		t.Errorf("bio of the wrong type: got %d, want 400", rec.Code)
	}
	// At the limits is fine.
	ok := `{"given_name":"A","bio":"` + strings.Repeat("é", 2000) + `","residence":"` + strings.Repeat("é", 200) + `"}`
	if rec := send(t, h, "POST", "/persons", ok, withCookie(c)); rec.Code != http.StatusCreated {
		t.Errorf("at the limits: got %d %s", rec.Code, rec.Body)
	}
}

func TestPatchBioAndResidence(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	id := mkFull(t, h, c)
	if rec := patchPerson(t, h, c, id, `{"bio":"First","residence":"Here"}`); rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body)
	}

	steps := []struct {
		name, body string
		bio, res   any // expected afterwards (nil = null)
	}{
		{"an unrelated patch leaves both alone", `{"given_name":"Augusta"}`, "First", "Here"},
		{"replace bio only", `{"bio":"Second"}`, "Second", "Here"},
		{"null clears residence", `{"residence":null}`, "Second", nil},
		{"blank clears bio", `{"bio":"   "}`, nil, nil},
		{"set both again", `{"bio":"Third\nline two","residence":"There"}`, "Third\nline two", "There"},
	}
	for _, s := range steps {
		rec := patchPerson(t, h, c, id, s.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", s.name, rec.Code, rec.Body)
		}
		got := decode(t, rec)
		if got["bio"] != s.bio || got["residence"] != s.res {
			t.Errorf("%s: bio = %v, residence = %v; want %v and %v", s.name, got["bio"], got["residence"], s.bio, s.res)
		}
		if again := getPerson(t, h, c, id); !jsonEqual(again, got) {
			t.Errorf("%s: GET differs from the PATCH response", s.name)
		}
	}

	before := getPerson(t, h, c, id)
	for name, body := range map[string]string{
		"too long":      `{"bio":"` + strings.Repeat("x", 2001) + `"}`,
		"NUL":           `{"residence":"a\u0000"}`,
		"wrong type":    `{"bio":["x"]}`,
		"residence 201": `{"residence":"` + strings.Repeat("x", 201) + `"}`,
	} {
		if rec := patchPerson(t, h, c, id, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s, want 400", name, rec.Code, rec.Body)
		}
	}
	if after := getPerson(t, h, c, id); !jsonEqual(before, after) {
		t.Errorf("a rejected patch changed the person:\n%v\n%v", before, after)
	}
}

// Bio and residence are free text about possibly living people. They are
// returned only by the per-person endpoints: not in the graph (layout only) and
// not in search results.
func TestBioAndResidenceStayOutOfTheGraphAndSearch(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	send(t, h, "POST", "/persons", `{"given_name":"Ada","bio":"SECRET-BIO","residence":"SECRET-PLACE"}`, withCookie(c))

	for _, path := range []string{"/graph", "/persons?q=ada"} {
		body := send(t, h, "GET", path, "", withCookie(c)).Body.String()
		if !strings.Contains(body, "Ada") {
			t.Fatalf("%s: the person is missing: %s", path, body)
		}
		if strings.Contains(body, "SECRET") || strings.Contains(body, `"bio"`) || strings.Contains(body, `"residence"`) {
			t.Errorf("%s leaks profile text: %s", path, body)
		}
	}
	// Search is by name only: a word that appears only in the bio finds nobody.
	if s := search(t, h, c, "q=SECRET-BIO"); len(s.Persons) != 0 {
		t.Errorf("search matched on the bio: %+v", s.Persons)
	}
}

func TestAnotherAccountCannotReadOrChangeBioAndResidence(t *testing.T) {
	h, _ := newApp(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	rec := send(t, h, "POST", "/persons", `{"given_name":"Ada","bio":"ALICE-ONLY","residence":"ALICE-PLACE"}`, withCookie(alice))
	id := int64(decode(t, rec)["id"].(float64))
	mkPerson(t, h, bob, "bob-person")

	get := send(t, h, "GET", "/persons/"+itoa(id), "", withCookie(bob))
	patch := patchPerson(t, h, bob, id, `{"bio":"HACKED","residence":null}`)
	for name, r := range map[string]string{"GET": get.Body.String(), "PATCH": patch.Body.String()} {
		if strings.Contains(r, "ALICE") || !strings.Contains(r, `"person_not_found"`) {
			t.Errorf("%s by Bob: %s, want a plain person_not_found", name, r)
		}
	}
	if get.Code != http.StatusNotFound || patch.Code != http.StatusNotFound {
		t.Errorf("status GET %d, PATCH %d; want 404 both", get.Code, patch.Code)
	}
	if got := getPerson(t, h, alice, id); got["bio"] != "ALICE-ONLY" || got["residence"] != "ALICE-PLACE" {
		t.Errorf("Alice's profile changed: %v", got)
	}
}
