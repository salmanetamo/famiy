package person_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

// The domain rejects these inputs first; this test bypasses it to prove the
// database enforces the same bounds on its own (migration 000002).
func TestDatabaseEnforcesPersonBounds(t *testing.T) {
	repo := person.NewPgRepository(testdb.New(t))
	day := func(y int, m time.Month, d int) *time.Time {
		v := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &v
	}
	long := strings.Repeat("x", person.MaxNameLen+1)

	tests := []struct {
		name string
		p    person.Person
	}{
		{"blank given name", person.Person{GivenName: "  "}},
		{"given name too long", person.Person{GivenName: long}},
		{"family name too long", person.Person{GivenName: "A", FamilyName: &long}},
		{"birth before 1000", person.Person{GivenName: "A", BirthDate: day(999, 12, 31)}},
		{"death before 1000", person.Person{GivenName: "A", DeathDate: day(500, 1, 1)}},
		{"death before birth", person.Person{GivenName: "A", BirthDate: day(2000, 1, 2), DeathDate: day(2000, 1, 1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := repo.Insert(context.Background(), tc.p); err == nil {
				t.Fatal("database accepted a row the domain would reject")
			}
		})
	}

	t.Run("values at the limits are accepted", func(t *testing.T) {
		ok := strings.Repeat("é", person.MaxNameLen)
		_, err := repo.Insert(context.Background(), person.Person{GivenName: ok, FamilyName: &ok, BirthDate: day(1000, 1, 1)})
		if err != nil {
			t.Fatalf("limit values rejected: %v", err)
		}
	})
}
