package person_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

// The domain rejects these inputs first; this test bypasses it to prove the
// database enforces the same bounds on its own (migration 000002).
func TestDatabaseEnforcesPersonBounds(t *testing.T) {
	pool := testdb.New(t)
	repo := person.NewPgRepository(pool)
	owner := testdb.InsertUser(t, pool, "owner@example.com")
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
			if _, err := repo.Insert(context.Background(), owner, tc.p); err == nil {
				t.Fatal("database accepted a row the domain would reject")
			}
		})
	}

	t.Run("values at the limits are accepted", func(t *testing.T) {
		ok := strings.Repeat("é", person.MaxNameLen)
		_, err := repo.Insert(context.Background(), owner, person.Person{GivenName: ok, FamilyName: &ok, BirthDate: day(1000, 1, 1)})
		if err != nil {
			t.Fatalf("limit values rejected: %v", err)
		}
	})
}

func TestPersonsAreScopedToTheirOwner(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	svc := person.NewService(person.NewPgRepository(pool))
	alice := testdb.InsertUser(t, pool, "alice@example.com")
	bob := testdb.InsertUser(t, pool, "bob@example.com")

	p, err := svc.Create(ctx, alice, person.Input{GivenName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if p.OwnerID != alice {
		t.Errorf("OwnerID = %d, want %d", p.OwnerID, alice)
	}
	if _, err := svc.Get(ctx, alice, p.ID); err != nil {
		t.Fatalf("owner cannot read their own person: %v", err)
	}
	// Someone else's person looks exactly like a missing one.
	_, otherErr := svc.Get(ctx, bob, p.ID)
	_, missingErr := svc.Get(ctx, bob, p.ID+1000)
	if !errors.Is(otherErr, person.ErrNotFound) || !errors.Is(missingErr, person.ErrNotFound) {
		t.Fatalf("other owner: %v; missing: %v; both must be ErrNotFound", otherErr, missingErr)
	}
}

func TestOwnershipIsEnforcedByTheDatabase(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	repo := person.NewPgRepository(pool)
	alice := testdb.InsertUser(t, pool, "alice@example.com")
	bob := testdb.InsertUser(t, pool, "bob@example.com")
	p, err := repo.Insert(ctx, alice, person.Person{GivenName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a person needs an existing owner", func(t *testing.T) {
		if _, err := repo.Insert(ctx, 999999, person.Person{GivenName: "Orphan"}); err == nil {
			t.Fatal("inserted a person owned by a nonexistent account")
		}
	})
	t.Run("ownership cannot be reassigned", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE person SET owner_user_id = $1 WHERE id = $2`, bob, p.ID); err == nil {
			t.Fatal("owner_user_id was changed")
		}
	})
	t.Run("an account that owns people cannot be deleted", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `DELETE FROM app_user WHERE id = $1`, alice); err == nil {
			t.Fatal("deleted an account that still owns people")
		}
	})
}
