package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type searchBody struct {
	Persons []struct {
		ID         int64   `json:"id"`
		GivenName  string  `json:"given_name"`
		FamilyName *string `json:"family_name"`
	} `json:"persons"`
	HasMore bool `json:"has_more"`
}

func search(t *testing.T, h http.Handler, c *http.Cookie, query string) searchBody {
	t.Helper()
	rec := send(t, h, "GET", "/persons?"+query, "", withCookie(c))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /persons?%s: %d %s", query, rec.Code, rec.Body)
	}
	var s searchBody
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body)
	}
	return s
}

func (s searchBody) names() []string {
	out := []string{}
	for _, p := range s.Persons {
		n := p.GivenName
		if p.FamilyName != nil {
			n += " " + *p.FamilyName
		}
		out = append(out, n)
	}
	return out
}

func mkNamed(t *testing.T, h http.Handler, c *http.Cookie, given, family string) {
	t.Helper()
	in := map[string]string{"given_name": given}
	if family != "" {
		in["family_name"] = family
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	rec := send(t, h, "POST", "/persons", string(body), withCookie(c))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s %s: %d %s", given, family, rec.Code, rec.Body)
	}
}

func TestSearchMatchesByName(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	mkNamed(t, h, c, "Ada", "Lovelace")
	mkNamed(t, h, c, "Adam", "Smith")
	mkNamed(t, h, c, "Grace", "Hopper")
	mkNamed(t, h, c, "Cher", "") // no family name

	tests := []struct {
		name, q string
		want    []string
	}{
		{"given name, any case", "ADA", []string{"Ada Lovelace", "Adam Smith"}},
		{"family name substring", "hop", []string{"Grace Hopper"}},
		{"every word must match", "ada love", []string{"Ada Lovelace"}},
		{"words in any order", "lovelace ada", []string{"Ada Lovelace"}},
		{"no family name still found", "cher", []string{"Cher"}},
		{"no match", "zzz", []string{}},
		{"words must all match", "ada hopper", []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := search(t, h, c, "q="+url.QueryEscape(tc.q)).names()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("q=%q: got %v, want %v", tc.q, got, tc.want)
			}
		})
	}
}

func TestSearchOrdersByFamilyThenGivenName(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	mkNamed(t, h, c, "Zed", "Adams")
	mkNamed(t, h, c, "Amy", "Adams")
	mkNamed(t, h, c, "Bob", "Aaron")
	want := []string{"Bob Aaron", "Amy Adams", "Zed Adams"}
	if got := search(t, h, c, "q=a").names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// % and _ in the query are literal characters, not wildcards.
func TestSearchTreatsWildcardsAsLiterals(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	mkNamed(t, h, c, "Ada", "Lovelace")
	mkNamed(t, h, c, "Per%cent", "")
	mkNamed(t, h, c, "Under_score", "")
	mkNamed(t, h, c, `Back\slash`, "")

	for q, want := range map[string][]string{
		"%":        {"Per%cent"},
		"_":        {"Under_score"},
		"a%e":      {},
		"d_r":      {},
		`\`:        {`Back\slash`},
		"per%cent": {"Per%cent"},
	} {
		got := search(t, h, c, "q="+url.QueryEscape(q)).names()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("q=%q: got %v, want %v", q, got, want)
		}
	}
}

func TestSearchLimitAndHasMore(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	for _, n := range []string{"P1", "P2", "P3"} {
		mkNamed(t, h, c, n, "")
	}
	tests := []struct {
		query   string
		n       int
		hasMore bool
	}{
		{"q=p&limit=2", 2, true},
		{"q=p&limit=3", 3, false}, // exactly the limit: nothing more
		{"q=p&limit=50", 3, false},
		{"q=p", 3, false}, // default limit is 20
		{"q=p&limit=1", 1, true},
	}
	for _, tc := range tests {
		s := search(t, h, c, tc.query)
		if len(s.Persons) != tc.n || s.HasMore != tc.hasMore {
			t.Errorf("%s: got %d persons, has_more=%v; want %d, %v", tc.query, len(s.Persons), s.HasMore, tc.n, tc.hasMore)
		}
	}
}

func TestSearchRejectsBadParameters(t *testing.T) {
	h, _ := newApp(t)
	c, _ := signIn(t, h, "ada@example.com")
	for name, query := range map[string]string{
		"missing q":      "",
		"empty q":        "q=",
		"blank q":        "q=%20%20",
		"q too long":     "q=" + strings.Repeat("a", 101),
		"q has NUL":      "q=ad%00a",
		"q bad UTF-8":    "q=ad%ffa",
		"too many words": "q=a+b+c+d+e+f",
		"limit zero":     "q=a&limit=0",
		"limit negative": "q=a&limit=-1",
		"limit too big":  "q=a&limit=51",
		"limit not int":  "q=a&limit=ten",
	} {
		rec := send(t, h, "GET", "/persons?"+query, "", withCookie(c))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"validation_failed"`) {
			t.Errorf("%s: got %d %s, want 400 validation_failed", name, rec.Code, rec.Body)
		}
	}
}

func TestSearchRequiresASession(t *testing.T) {
	h, _ := newApp(t)
	rec := send(t, h, "GET", "/persons?q=a", "")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
		t.Fatalf("got %d %s, want 401 unauthenticated", rec.Code, rec.Body)
	}
}

// Both accounts have a match for the same query; an account whose people are
// all someone else's sees nothing.
func TestSearchIsScopedToTheCaller(t *testing.T) {
	h, _ := newApp(t)
	ada, _ := signIn(t, h, "ada@example.com")
	bob, _ := signIn(t, h, "bob@example.com")
	carol, _ := signIn(t, h, "carol@example.com") // owns nobody
	mkNamed(t, h, ada, "Sam", "Adaland")
	mkNamed(t, h, bob, "Sam", "Bobland")

	if got, want := search(t, h, ada, "q=sam").names(), []string{"Sam Adaland"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ada: got %v, want %v", got, want)
	}
	if got, want := search(t, h, bob, "q=sam").names(), []string{"Sam Bobland"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bob: got %v, want %v", got, want)
	}
	if s := search(t, h, carol, "q=sam"); len(s.Persons) != 0 || s.HasMore {
		t.Errorf("carol sees %+v", s)
	}
}
