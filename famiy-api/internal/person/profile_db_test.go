package person_test

import (
	"context"
	"strings"
	"testing"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

// The domain rejects these first; this bypasses it to prove the database
// enforces the same bounds on its own (migration 000007).
func TestDatabaseEnforcesBioAndResidenceBounds(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	repo := person.NewPgRepository(pool)
	owner := testdb.InsertUser(t, pool, "owner@example.com")
	sp := func(s string) *string { return &s }

	bad := []struct {
		name string
		p    person.Person
	}{
		{"blank bio", person.Person{GivenName: "A", Bio: sp("  \n ")}},
		{"empty bio", person.Person{GivenName: "A", Bio: sp("")}},
		{"bio too long", person.Person{GivenName: "A", Bio: sp(strings.Repeat("x", person.MaxBioLen+1))}},
		{"bio too long in 2-byte characters", person.Person{GivenName: "A", Bio: sp(strings.Repeat("é", person.MaxBioLen+1))}},
		{"bio too long in 4-byte characters", person.Person{GivenName: "A", Bio: sp(strings.Repeat("😀", person.MaxBioLen+1))}},
		{"blank residence", person.Person{GivenName: "A", Residence: sp(" ")}},
		{"residence too long", person.Person{GivenName: "A", Residence: sp(strings.Repeat("x", person.MaxResidenceLen+1))}},
		{"residence too long in 4-byte characters", person.Person{GivenName: "A", Residence: sp(strings.Repeat("😀", person.MaxResidenceLen+1))}},
		// The database's idea of "blank" must agree with the domain's for these:
		// line breaks, tabs and Unicode spaces (probed on PostgreSQL 16, en_US.utf8).
		{"bio of line breaks and tabs", person.Person{GivenName: "A", Bio: sp("\n\t\n")}},
		{"bio of an em space", person.Person{GivenName: "A", Bio: sp(" ")}},
		{"bio of an ideographic space", person.Person{GivenName: "A", Bio: sp("　")}},
		{"residence of a tab", person.Person{GivenName: "A", Residence: sp("\t")}},
		{"residence of an em space", person.Person{GivenName: "A", Residence: sp(" ")}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := repo.Insert(ctx, owner, tc.p, person.DefaultMaxPersons); err == nil {
				t.Fatal("database accepted a row the domain would reject")
			}
		})
	}

	t.Run("values at the limits are accepted and read back", func(t *testing.T) {
		bio, res := strings.Repeat("😀", person.MaxBioLen), strings.Repeat("😀", person.MaxResidenceLen) // 4 bytes each: length is in characters
		p, err := repo.Insert(ctx, owner, person.Person{GivenName: "A", Bio: &bio, Residence: &res}, person.DefaultMaxPersons)
		if err != nil {
			t.Fatalf("limit values rejected: %v", err)
		}
		got, err := repo.Get(ctx, owner, p.ID)
		if err != nil || got.Bio == nil || *got.Bio != bio || got.Residence == nil || *got.Residence != res {
			t.Fatalf("read back %+v (%v)", got, err)
		}
	})
	t.Run("an update is held to the same bounds", func(t *testing.T) {
		p, err := repo.Insert(ctx, owner, person.Person{GivenName: "B"}, person.DefaultMaxPersons)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ column, value string }{
			{"bio", strings.Repeat("x", person.MaxBioLen+1)},
			{"bio", "\n"},
			{"residence", strings.Repeat("x", person.MaxResidenceLen+1)},
			{"residence", " "},
		} {
			if _, err := pool.Exec(ctx, `UPDATE person SET `+tc.column+` = $1 WHERE id = $2`, tc.value, p.ID); err == nil {
				t.Errorf("update of %s to %q... was accepted", tc.column, tc.value[:min(len(tc.value), 8)])
			}
		}
	})
}
