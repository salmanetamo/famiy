package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// family is a small tree that exercises every relation:
//
//	ggp
//	 |
//	gp1 + gp2            (a couple)
//	 |-------+
//	 p       u            (siblings; p's partner is s, u's partner is t)
//	 |       |
//	c1 c2 h1 cz          (c1, c2: children of p and s; h1: child of p and s2; cz: child of u and t)
//	          |
//	         kid         (child of cz and an unrecorded parent)
//
// x is unrelated to everyone.
type family struct{ ggp, gp1, gp2, p, u, s, s2, t, c1, c2, h1, cz, kid, x int64 }

func buildFamily(t *testing.T, h http.Handler, c *http.Cookie) family {
	t.Helper()
	f := family{}
	for _, e := range []struct {
		id   *int64
		name string
	}{
		{&f.ggp, "ggp"}, {&f.gp1, "gp1"}, {&f.gp2, "gp2"}, {&f.p, "p"}, {&f.u, "u"}, {&f.s, "s"}, {&f.s2, "s2"},
		{&f.t, "t"}, {&f.c1, "c1"}, {&f.c2, "c2"}, {&f.h1, "h1"}, {&f.cz, "cz"}, {&f.kid, "kid"}, {&f.x, "x"},
	} {
		*e.id = mkPerson(t, h, c, e.name)
	}
	for _, l := range [][2]int64{
		{f.ggp, f.gp1},
		{f.gp1, f.p}, {f.gp2, f.p}, {f.gp1, f.u}, {f.gp2, f.u},
		{f.p, f.c1}, {f.s, f.c1}, {f.p, f.c2}, {f.s, f.c2},
		{f.p, f.h1}, {f.s2, f.h1},
		{f.u, f.cz}, {f.t, f.cz},
		{f.cz, f.kid},
	} {
		mkLink(t, h, c, l[0], l[1])
	}
	// Partners are not blood relatives of each other.
	mkPartner(t, h, c, f.p, f.s, "marriage")
	mkPartner(t, h, c, f.u, f.t, "marriage")
	return f
}

type relBody struct {
	PersonID          int64   `json:"person_id"`
	OtherID           int64   `json:"other_id"`
	Related           bool    `json:"related"`
	Relation          *string `json:"relation"`
	StepsPerson       *int    `json:"steps_person"`
	StepsOther        *int    `json:"steps_other"`
	Degree            *int    `json:"degree"`
	Removed           *int    `json:"removed"`
	CommonAncestorIDs []int64 `json:"common_ancestor_ids"`
}

func getRel(t *testing.T, h http.Handler, c *http.Cookie, a, b int64) (relBody, *httptest.ResponseRecorder) {
	t.Helper()
	rec := send(t, h, "GET", "/persons/"+itoa(a)+"/relationship/"+itoa(b), "", withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("relationship %d-%d: %d %s", a, b, rec.Code, rec.Body)
	}
	var r relBody
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body)
	}
	return r, rec
}

func ip(n int) *int { return &n }

func TestRelationshipDerivesKinship(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	f := buildFamily(t, h, c)

	tests := []struct {
		name                    string
		person, other           int64
		relation                string
		stepsPerson, stepsOther int
		degree, removed         *int
		commonAncestors         []int64
	}{
		{"siblings with both parents recorded", f.c1, f.c2, "sibling", 1, 1, nil, nil, []int64{f.p, f.s}},
		{"half-siblings share one parent", f.c1, f.h1, "sibling", 1, 1, nil, nil, []int64{f.p}},
		{"the other is a parent", f.c1, f.p, "ancestor", 1, 0, nil, nil, []int64{f.p}},
		{"the other is a grandparent", f.c1, f.gp1, "ancestor", 2, 0, nil, nil, []int64{f.gp1}},
		{"the other is a great-grandparent", f.c1, f.ggp, "ancestor", 3, 0, nil, nil, []int64{f.ggp}},
		{"the other is a child", f.p, f.c1, "descendant", 0, 1, nil, nil, []int64{f.p}},
		{"the other is a great-grandchild", f.ggp, f.c1, "descendant", 0, 3, nil, nil, []int64{f.ggp}},
		{"the other is an aunt or uncle", f.c1, f.u, "aunt_uncle", 2, 1, nil, nil, []int64{f.gp1, f.gp2}},
		{"the other is a niece or nephew", f.u, f.c1, "niece_nephew", 1, 2, nil, nil, []int64{f.gp1, f.gp2}},
		{"first cousins", f.c1, f.cz, "cousin", 2, 2, ip(1), ip(0), []int64{f.gp1, f.gp2}},
		{"first cousins once removed, the other is younger", f.c1, f.kid, "cousin", 2, 3, ip(1), ip(1), []int64{f.gp1, f.gp2}},
		{"first cousins once removed, the other is older", f.kid, f.c1, "cousin", 3, 2, ip(1), ip(1), []int64{f.gp1, f.gp2}},
		{"a great-aunt", f.kid, f.p, "aunt_uncle", 3, 1, nil, nil, []int64{f.gp1, f.gp2}},
		{"the same person", f.c1, f.c1, "self", 0, 0, nil, nil, []int64{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, rec := getRel(t, h, c, tc.person, tc.other)
			if !got.Related || got.Relation == nil || *got.Relation != tc.relation {
				t.Fatalf("got %s, want relation %s", rec.Body, tc.relation)
			}
			if got.PersonID != tc.person || got.OtherID != tc.other {
				t.Errorf("ids = %d, %d", got.PersonID, got.OtherID)
			}
			if *got.StepsPerson != tc.stepsPerson || *got.StepsOther != tc.stepsOther {
				t.Errorf("steps = %d, %d; want %d, %d", *got.StepsPerson, *got.StepsOther, tc.stepsPerson, tc.stepsOther)
			}
			if !reflect.DeepEqual(got.Degree, tc.degree) || !reflect.DeepEqual(got.Removed, tc.removed) {
				t.Errorf("degree/removed = %v/%v, want %v/%v", got.Degree, got.Removed, tc.degree, tc.removed)
			}
			if !reflect.DeepEqual(got.CommonAncestorIDs, tc.commonAncestors) {
				t.Errorf("common ancestors = %v, want %v", got.CommonAncestorIDs, tc.commonAncestors)
			}
		})
	}
}

func TestRelationshipOfPeopleWithoutABloodTie(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	f := buildFamily(t, h, c)

	for name, pair := range map[string][2]int64{
		"no connection at all":       {f.c1, f.x},
		"partners (not by blood)":    {f.p, f.s},
		"a child's other parent":     {f.s2, f.c1},
		"in-law of a blood relative": {f.t, f.c1},
	} {
		got, rec := getRel(t, h, c, pair[0], pair[1])
		if got.Related || got.Relation != nil || got.StepsPerson != nil || got.StepsOther != nil || got.Degree != nil || got.Removed != nil {
			t.Errorf("%s: %s, want related:false with null fields", name, rec.Body)
		}
		if got.CommonAncestorIDs == nil || len(got.CommonAncestorIDs) != 0 {
			t.Errorf("%s: common_ancestor_ids = %v, want []", name, got.CommonAncestorIDs)
		}
		if !strings.Contains(rec.Body.String(), `"relation":null`) {
			t.Errorf("%s: the keys should be present as null: %s", name, rec.Body)
		}
	}
}

// A cousin marriage: d's parents c1 and cz are first cousins, so every grandparent
// is reachable by two routes of the same length. The closest relationship wins
// and nobody is listed twice.
func TestRelationshipAcrossACousinMarriage(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	f := buildFamily(t, h, c)
	d := mkPerson(t, h, c, "d")
	mkLink(t, h, c, f.c1, d)
	mkLink(t, h, c, f.cz, d)

	got, rec := getRel(t, h, c, d, f.gp1)
	if *got.Relation != "ancestor" || *got.StepsPerson != 3 || !reflect.DeepEqual(got.CommonAncestorIDs, []int64{f.gp1}) {
		t.Errorf("d to gp1: %s", rec.Body)
	}
	// c1 is d's parent, so d is c1's descendant, one step down.
	got, rec = getRel(t, h, c, f.c1, d)
	if *got.Relation != "descendant" || *got.StepsOther != 1 {
		t.Errorf("c1 to d: %s", rec.Body)
	}
	// c2 is d's parent's sibling: an aunt or uncle at steps (2, 1). The longer route through
	// cz's side of the family (steps 3, 2) is not reported.
	got, rec = getRel(t, h, c, d, f.c2)
	if *got.Relation != "aunt_uncle" || *got.StepsPerson != 2 || *got.StepsOther != 1 {
		t.Errorf("d to c2: %s", rec.Body)
	}
}

func TestDescendants(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	f := buildFamily(t, h, c)

	type d struct{ id, gen int64 }
	read := func(id int64) []d {
		rec := send(t, h, "GET", "/persons/"+itoa(id)+"/descendants", "", withCookie(c))
		if rec.Code != http.StatusOK {
			t.Fatalf("descendants of %d: %d %s", id, rec.Code, rec.Body)
		}
		var body struct {
			PersonID    int64 `json:"person_id"`
			Descendants []struct {
				PersonID    int64 `json:"person_id"`
				Generations int64 `json:"generations"`
			} `json:"descendants"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.PersonID != id || body.Descendants == nil {
			t.Fatalf("body = %s (%v)", rec.Body, err)
		}
		out := []d{}
		for _, x := range body.Descendants {
			out = append(out, d{x.PersonID, x.Generations})
		}
		return out
	}

	want := []d{{f.gp1, 1}, {f.p, 2}, {f.u, 2}, {f.c1, 3}, {f.c2, 3}, {f.h1, 3}, {f.cz, 3}, {f.kid, 4}}
	// Ordered by generations, then id (the helper creates ids in ascending order).
	if got := read(f.ggp); !reflect.DeepEqual(got, want) {
		t.Errorf("descendants of ggp = %v, want %v", got, want)
	}
	if got := read(f.p); !reflect.DeepEqual(got, []d{{f.c1, 1}, {f.c2, 1}, {f.h1, 1}}) {
		t.Errorf("descendants of p = %v", got)
	}
	if got := read(f.kid); len(got) != 0 {
		t.Errorf("a leaf has descendants: %v", got)
	}
	// Partners are not descendants.
	if got := read(f.s); !reflect.DeepEqual(got, []d{{f.c1, 1}, {f.c2, 1}}) {
		t.Errorf("descendants of s = %v", got)
	}
}

// e is a child of p AND of p's own child c1, so e is reachable from p by two
// routes (1 step and 2 steps). It must appear once, at the shorter distance.
func TestDescendantsCollapseDuplicatePaths(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	f := buildFamily(t, h, c)
	e := mkPerson(t, h, c, "e")
	mkLink(t, h, c, f.p, e)
	mkLink(t, h, c, f.c1, e)

	rec := send(t, h, "GET", "/persons/"+itoa(f.p)+"/descendants", "", withCookie(c))
	var body struct {
		Descendants []struct {
			PersonID    int64 `json:"person_id"`
			Generations int   `json:"generations"`
		} `json:"descendants"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, x := range body.Descendants {
		if x.PersonID == e {
			seen++
			if x.Generations != 1 {
				t.Errorf("e at %d generations, want the shortest (1)", x.Generations)
			}
		}
	}
	if seen != 1 {
		t.Errorf("e appears %d times in %s, want once", seen, rec.Body)
	}
}

func TestSiblings(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	f := buildFamily(t, h, c)

	type sib struct{ id, shared int64 }
	read := func(id int64) []sib {
		rec := send(t, h, "GET", "/persons/"+itoa(id)+"/siblings", "", withCookie(c))
		if rec.Code != http.StatusOK {
			t.Fatalf("siblings of %d: %d %s", id, rec.Code, rec.Body)
		}
		var body struct {
			PersonID int64 `json:"person_id"`
			Siblings []struct {
				PersonID      int64 `json:"person_id"`
				SharedParents int64 `json:"shared_parents"`
			} `json:"siblings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.PersonID != id || body.Siblings == nil {
			t.Fatalf("body = %s (%v)", rec.Body, err)
		}
		out := []sib{}
		for _, x := range body.Siblings {
			out = append(out, sib{x.PersonID, x.SharedParents})
		}
		return out
	}

	// c1 shares both parents with c2 and only p with h1; cousins and itself are not siblings.
	if got, want := read(f.c1), []sib{{f.c2, 2}, {f.h1, 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("siblings of c1 = %v, want %v", got, want)
	}
	if got, want := read(f.h1), []sib{{f.c1, 1}, {f.c2, 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("siblings of h1 = %v, want %v", got, want)
	}
	if got, want := read(f.p), []sib{{f.u, 2}}; !reflect.DeepEqual(got, want) {
		t.Errorf("siblings of p = %v, want %v", got, want)
	}
	for _, id := range []int64{f.ggp, f.kid, f.x} { // no parents, or an only child
		if got := read(id); len(got) != 0 {
			t.Errorf("siblings of %d = %v, want none", id, got)
		}
	}
}

func TestKinshipRoutesRequireASession(t *testing.T) {
	h, _ := newApp(t)
	for _, path := range []string{"/persons/1/descendants", "/persons/1/siblings", "/persons/1/relationship/2"} {
		rec := send(t, h, "GET", path, "")
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
			t.Errorf("%s: got %d %s, want 401", path, rec.Code, rec.Body)
		}
	}
}

func TestKinshipRoutesRejectBadIds(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	for _, path := range []string{
		"/persons/abc/descendants", "/persons/0/siblings", "/persons/abc/relationship/1",
		"/persons/1/relationship/abc", "/persons/1/relationship/0", "/persons/1/relationship/-3",
	} {
		rec := send(t, h, "GET", path, "", withCookie(c))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_id"`) {
			t.Errorf("%s: got %d %s, want 400 invalid_id", path, rec.Code, rec.Body)
		}
	}
}

// Bob must learn nothing about Alice's family from any of the three routes, and
// each answer must look exactly like asking about people that do not exist. The
// pair where BOTH people are Alice's is the case no database trigger can stop.
func TestKinshipRoutesCannotReachAnotherAccount(t *testing.T) {
	h, _ := newApp(t)
	alice, _ := signIn(t, h, "alice@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	f := buildFamily(t, h, alice)
	bx := mkPerson(t, h, bob, "bob-person")

	type try struct {
		name string
		who  *http.Cookie
		path string
		miss string // the same request about people that do not exist
	}
	tries := []try{
		{"Bob: descendants of Alice's person", bob, "/persons/" + itoa(f.ggp) + "/descendants", "/persons/999999/descendants"},
		{"Bob: siblings of Alice's person", bob, "/persons/" + itoa(f.c1) + "/siblings", "/persons/999999/siblings"},
		{"Bob: relationship of two of Alice's people", bob, "/persons/" + itoa(f.c1) + "/relationship/" + itoa(f.cz), "/persons/999999/relationship/999998"},
		{"Bob: his person and Alice's", bob, "/persons/" + itoa(bx) + "/relationship/" + itoa(f.c1), "/persons/" + itoa(bx) + "/relationship/999998"},
		{"Bob: Alice's person and his", bob, "/persons/" + itoa(f.c1) + "/relationship/" + itoa(bx), "/persons/999999/relationship/" + itoa(bx)},
		{"Alice: her person and Bob's", alice, "/persons/" + itoa(f.c1) + "/relationship/" + itoa(bx), "/persons/" + itoa(f.c1) + "/relationship/999998"},
		{"Alice: Bob's person and hers", alice, "/persons/" + itoa(bx) + "/relationship/" + itoa(f.c1), "/persons/999999/relationship/" + itoa(f.c1)},
		{"Alice: descendants of Bob's person", alice, "/persons/" + itoa(bx) + "/descendants", "/persons/999999/descendants"},
	}
	for _, tc := range tries {
		t.Run(tc.name, func(t *testing.T) {
			got := send(t, h, "GET", tc.path, "", withCookie(tc.who))
			if got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), `"person_not_found"`) {
				t.Fatalf("got %d %s, want 404 person_not_found", got.Code, got.Body)
			}
			missing := send(t, h, "GET", tc.miss, "", withCookie(tc.who))
			if missing.Code != got.Code || missing.Body.String() != got.Body.String() {
				t.Errorf("differs from asking about nobody:\n%d %s\n%d %s", got.Code, got.Body, missing.Code, missing.Body)
			}
		})
	}
}

// Pedigree collapse: x is a's parent and b's great-grandparent (steps 1, 3), while
// y is b's parent and a's great-grandparent (steps 3, 1). The two routes tie on
// total distance and on balance, so the answer must not depend on which person
// is asked about first, or both directions would call the other person a niece.
func TestRelationshipIsTheMirrorImageWhenTwoRoutesTie(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	x, y := mkPerson(t, h, c, "x"), mkPerson(t, h, c, "y") // x has the smaller id
	a2, a1, a := mkPerson(t, h, c, "a2"), mkPerson(t, h, c, "a1"), mkPerson(t, h, c, "a")
	b2, b1, b := mkPerson(t, h, c, "b2"), mkPerson(t, h, c, "b1"), mkPerson(t, h, c, "b")
	for _, l := range [][2]int64{{y, a2}, {a2, a1}, {a1, a}, {x, a}, {x, b2}, {b2, b1}, {b1, b}, {y, b}} {
		mkLink(t, h, c, l[0], l[1])
	}

	ab, rec := getRel(t, h, c, a, b)
	if *ab.Relation != "niece_nephew" || *ab.StepsPerson != 1 || *ab.StepsOther != 3 || !reflect.DeepEqual(ab.CommonAncestorIDs, []int64{x}) {
		t.Errorf("a to b: %s, want niece_nephew (1, 3) through x", rec.Body)
	}
	ba, rec := getRel(t, h, c, b, a)
	if *ba.Relation != "aunt_uncle" || *ba.StepsPerson != 3 || *ba.StepsOther != 1 || !reflect.DeepEqual(ba.CommonAncestorIDs, []int64{x}) {
		t.Errorf("b to a: %s, want aunt_uncle (3, 1) through the same ancestor x", rec.Body)
	}
}
