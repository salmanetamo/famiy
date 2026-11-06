package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salmanetamo/famiy/famiy-api/internal/app"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

type graphBody struct {
	Persons []struct {
		ID          int64   `json:"id"`
		GivenName   string  `json:"given_name"`
		FamilyName  *string `json:"family_name"`
		BirthDate   *string `json:"birth_date"`
		BirthApprox bool    `json:"birth_approx"`
		DeathDate   *string `json:"death_date"`
	} `json:"persons"`
	ParentLinks []struct {
		ParentID int64 `json:"parent_id"`
		ChildID  int64 `json:"child_id"`
	} `json:"parent_links"`
	Partnerships []struct {
		Partners [2]int64 `json:"partners"`
		Kind     string   `json:"kind"`
	} `json:"partnerships"`
}

func (g graphBody) ids() []int64 {
	out := []int64{}
	for _, p := range g.Persons {
		out = append(out, p.ID)
	}
	return out
}

func getGraph(t *testing.T, h http.Handler, c *http.Cookie) (graphBody, *httptest.ResponseRecorder) {
	t.Helper()
	rec := send(t, h, "GET", "/graph", "", withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /graph: %d %s", rec.Code, rec.Body)
	}
	var g graphBody
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
		t.Fatalf("decode graph: %v (%s)", err, rec.Body)
	}
	return g, rec
}

func newAppWithPool(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	h, _, err := app.NewRouter(pool, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	return h, pool
}

// Partnerships have no API yet, so tests insert them directly (the database
// triggers still apply). a must be smaller than b: that is the stored order.
func mkPartnership(t *testing.T, pool *pgxpool.Pool, a, b int64, kind string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO partnership (partner_a, partner_b, kind) VALUES ($1, $2, $3)`, a, b, kind); err != nil {
		t.Fatalf("partnership %d-%d: %v", a, b, err)
	}
}

func TestGraphRequiresASession(t *testing.T) {
	h, _ := newApp(t)
	rec := send(t, h, "GET", "/graph", "")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
		t.Fatalf("got %d %s, want 401 unauthenticated", rec.Code, rec.Body)
	}
}

func TestGraphOfAnEmptyAccountHasEmptyListsNotNull(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	rec := send(t, h, "GET", "/graph", "", withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"persons", "parent_links", "partnerships"} {
		if string(raw[k]) != "[]" {
			t.Errorf("%s = %s, want []", k, raw[k])
		}
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestGraphShape(t *testing.T) {
	h, pool := newAppWithPool(t)
	c, _ := signIn(t, h, "ada@example.com")
	rec := send(t, h, "POST", "/persons",
		`{"given_name":"Ada","family_name":"Lovelace","birth_date":"1815-12-10","birth_approx":true,"death_date":"1852-11-27"}`, withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	child := mkPerson(t, h, c, "Byron")
	parent := int64(decode(t, rec)["id"].(float64))
	mkLink(t, h, c, parent, child)
	mkPartnership(t, pool, parent, child, "marriage") // not realistic kinship, only exercising the shape

	g, raw := getGraph(t, h, c)
	if strings.Contains(raw.Body.String(), "created_at") || strings.Contains(raw.Body.String(), "owner") {
		t.Errorf("response leaks fields that are not part of the contract: %s", raw.Body)
	}
	if len(g.Persons) != 2 || g.Persons[0].ID != parent || g.Persons[1].ID != child {
		t.Fatalf("persons = %+v, want ordered by id", g.Persons)
	}
	p := g.Persons[0]
	if p.GivenName != "Ada" || *p.FamilyName != "Lovelace" || *p.BirthDate != "1815-12-10" || !p.BirthApprox || *p.DeathDate != "1852-11-27" {
		t.Errorf("person = %+v", p)
	}
	if q := g.Persons[1]; q.FamilyName != nil || q.BirthDate != nil || q.DeathDate != nil {
		t.Errorf("unset optional fields should be null: %+v", q)
	}
	if len(g.ParentLinks) != 1 || g.ParentLinks[0].ParentID != parent || g.ParentLinks[0].ChildID != child {
		t.Errorf("parent_links = %+v", g.ParentLinks)
	}
	if len(g.Partnerships) != 1 || g.Partnerships[0].Kind != "marriage" ||
		!sameSet(g.Partnerships[0].Partners[:], []int64{parent, child}) {
		t.Errorf("partnerships = %+v", g.Partnerships)
	}
}

// Two accounts that both have people, links and partnerships, plus a third that
// has none. Each must see exactly its own data and nothing of the others'.
func TestGraphIsScopedToTheCaller(t *testing.T) {
	h, pool := newAppWithPool(t)
	ada, _ := signIn(t, h, "ada@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	carol, _ := signIn(t, h, "carol@example.com") // owns nothing

	a1, a2, a3 := mkPerson(t, h, ada, "A1"), mkPerson(t, h, ada, "A2"), mkPerson(t, h, ada, "A3")
	b1, b2, b3 := mkPerson(t, h, bob, "B1"), mkPerson(t, h, bob, "B2"), mkPerson(t, h, bob, "B3")
	mkLink(t, h, ada, a1, a2)
	mkLink(t, h, ada, a1, a3)
	mkLink(t, h, bob, b1, b2)
	mkPartnership(t, pool, a2, a3, "marriage")
	mkPartnership(t, pool, b2, b3, "partnership")

	ga, _ := getGraph(t, h, ada)
	if !reflect.DeepEqual(ga.ids(), []int64{a1, a2, a3}) {
		t.Errorf("ada's persons = %v, want %v", ga.ids(), []int64{a1, a2, a3})
	}
	if len(ga.ParentLinks) != 2 || len(ga.Partnerships) != 1 || ga.Partnerships[0].Kind != "marriage" {
		t.Errorf("ada's edges = %+v / %+v", ga.ParentLinks, ga.Partnerships)
	}
	gb, _ := getGraph(t, h, bob)
	if !reflect.DeepEqual(gb.ids(), []int64{b1, b2, b3}) {
		t.Errorf("bob's persons = %v, want %v", gb.ids(), []int64{b1, b2, b3})
	}
	if len(gb.ParentLinks) != 1 || gb.ParentLinks[0].ParentID != b1 || len(gb.Partnerships) != 1 || gb.Partnerships[0].Kind != "partnership" {
		t.Errorf("bob's edges = %+v / %+v", gb.ParentLinks, gb.Partnerships)
	}
	gc, _ := getGraph(t, h, carol)
	if len(gc.Persons) != 0 || len(gc.ParentLinks) != 0 || len(gc.Partnerships) != 0 {
		t.Errorf("an account with no people sees data: %+v", gc)
	}
}

func sameSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[int64]int{}
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
