package relationship_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
	"github.com/salmanetamo/famiy/famiy-api/internal/relationship"
)

// The domain rejects an unknown kind first; this bypasses it to prove the
// database enforces the same list on its own (migration 000006).
func TestDatabaseEnforcesPartnershipKind(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a, b := f.person(t, "a"), f.person(t, "b")

	_, err := f.pool.Exec(ctx, `INSERT INTO partnership (partner_a, partner_b, kind) VALUES ($1, $2, 'friends')`, a, b)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "partnership_kind_allowed" {
		t.Fatalf("insert with an unknown kind: got %v, want a partnership_kind_allowed violation", err)
	}
	// And the repository maps that constraint name back to a domain error.
	if err := f.repo.AddPartnership(ctx, relationship.Partnership{PersonID: a, PartnerID: b, Kind: "friends"}); !errors.Is(err, relationship.ErrInvalidKind) {
		t.Fatalf("repo: got %v, want ErrInvalidKind", err)
	}
	for _, k := range []string{relationship.KindMarriage, relationship.KindPartnership, relationship.KindOther} {
		if _, err := f.pool.Exec(ctx, `UPDATE partnership SET kind = $1`, k); err != nil {
			t.Errorf("kind %q refused: %v", k, err)
		}
	}
}

// The domain stops a self-partnership first; bypassing it proves the database
// refuses it (partner_order) and the repository maps that back to the domain error.
func TestDatabaseRefusesSelfPartnership(t *testing.T) {
	f := newFixture(t)
	a := f.person(t, "a")
	if err := f.repo.AddPartnership(context.Background(), relationship.Partnership{PersonID: a, PartnerID: a, Kind: relationship.KindMarriage}); !errors.Is(err, relationship.ErrSelfPartner) {
		t.Fatalf("got %v, want ErrSelfPartner", err)
	}
}

func TestAddPartnershipAcceptsEitherOrderAndRefusesTheSecond(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a, b := f.person(t, "a"), f.person(t, "b")
	lo, hi := a, b // creation order makes a < b

	if err := f.svc.AddPartnership(ctx, f.owner, relationship.Partnership{PersonID: hi, PartnerID: lo, Kind: relationship.KindMarriage}); err != nil {
		t.Fatalf("add in descending order: %v", err)
	}
	var ra, rb int64
	if err := f.pool.QueryRow(ctx, `SELECT partner_a, partner_b FROM partnership`).Scan(&ra, &rb); err != nil || ra != lo || rb != hi {
		t.Fatalf("stored as (%d, %d) %v; want (%d, %d)", ra, rb, err, lo, hi)
	}
	for _, p := range []relationship.Partnership{
		{PersonID: hi, PartnerID: lo, Kind: relationship.KindMarriage},
		{PersonID: lo, PartnerID: hi, Kind: relationship.KindOther},
	} {
		if err := f.svc.AddPartnership(ctx, f.owner, p); !errors.Is(err, relationship.ErrDuplicatePartnership) {
			t.Errorf("second add %+v: got %v, want ErrDuplicatePartnership", p, err)
		}
	}
}

func TestPartnershipLifecycleInEitherOrder(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a, b, c := f.person(t, "a"), f.person(t, "b"), f.person(t, "c")
	add := func(x, y int64) {
		if err := f.svc.AddPartnership(ctx, f.owner, relationship.Partnership{PersonID: x, PartnerID: y, Kind: relationship.KindMarriage}); err != nil {
			t.Fatal(err)
		}
	}
	add(a, b)
	add(c, a) // a has two partners (remarriage)

	p, err := f.svc.UpdatePartnership(ctx, f.owner, b, a, ptr(relationship.KindPartnership)) // named in reverse
	if err != nil || p.Kind != relationship.KindPartnership {
		t.Fatalf("update: %+v %v", p, err)
	}
	if got, err := f.svc.UpdatePartnership(ctx, f.owner, a, b, nil); err != nil || got.Kind != relationship.KindPartnership {
		t.Fatalf("read back with a nil kind: %+v %v", got, err)
	}
	if err := f.svc.RemovePartnership(ctx, f.owner, b, a); err != nil {
		t.Fatalf("remove in reverse order: %v", err)
	}
	if err := f.svc.RemovePartnership(ctx, f.owner, a, b); !errors.Is(err, relationship.ErrPartnershipNotFound) {
		t.Fatalf("second remove: got %v, want ErrPartnershipNotFound", err)
	}
	if _, err := f.svc.UpdatePartnership(ctx, f.owner, a, b, ptr(relationship.KindOther)); !errors.Is(err, relationship.ErrPartnershipNotFound) {
		t.Fatalf("update after remove: got %v, want ErrPartnershipNotFound", err)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM partnership`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("%d partnerships left (%v), want only c-a", n, err)
	}
}

func ptr(s string) *string { return &s }

// Both people belong to Bob, so the database trigger sees nothing wrong; only
// the service's ownership check stops Alice from editing Bob's partnerships.
func TestPartnershipServiceChecksOwnership(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	bob := testdb.InsertUser(t, f.pool, "bob@example.com")
	mk := func(owner int64, name string) int64 {
		p, err := f.persons.Create(ctx, owner, person.Input{GivenName: name})
		if err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	b1, b2, a1 := mk(bob, "b1"), mk(bob, "b2"), f.person(t, "a1")
	if err := f.svc.AddPartnership(ctx, bob, relationship.Partnership{PersonID: b1, PartnerID: b2, Kind: relationship.KindMarriage}); err != nil {
		t.Fatal(err)
	}

	for name, fn := range map[string]func() error{
		"add (both Bob's)": func() error {
			return f.svc.AddPartnership(ctx, f.owner, relationship.Partnership{PersonID: b1, PartnerID: b2, Kind: relationship.KindOther})
		},
		"add (one each)": func() error {
			return f.svc.AddPartnership(ctx, f.owner, relationship.Partnership{PersonID: a1, PartnerID: b1, Kind: relationship.KindMarriage})
		},
		"update (both Bob's)": func() error {
			_, err := f.svc.UpdatePartnership(ctx, f.owner, b1, b2, ptr(relationship.KindOther))
			return err
		},
		"read (both Bob's)":   func() error { _, err := f.svc.UpdatePartnership(ctx, f.owner, b1, b2, nil); return err },
		"remove (both Bob's)": func() error { return f.svc.RemovePartnership(ctx, f.owner, b1, b2) },
	} {
		if err := fn(); !errors.Is(err, relationship.ErrPersonMissing) {
			t.Errorf("%s: got %v, want person_not_found", name, err)
		}
	}
	var kind string
	if err := f.pool.QueryRow(ctx, `SELECT kind FROM partnership`).Scan(&kind); err != nil || kind != relationship.KindMarriage {
		t.Fatalf("Bob's partnership changed: %q %v", kind, err)
	}
}

// The database refuses a pair that spans two accounts even if the service is
// bypassed (same_owner), and reports it as a missing person.
func TestDatabaseRefusesCrossOwnerPartnership(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	bob := testdb.InsertUser(t, f.pool, "bob@example.com")
	other, err := f.persons.Create(ctx, bob, person.Input{GivenName: "other"})
	if err != nil {
		t.Fatal(err)
	}
	mine := f.person(t, "mine")
	if err := f.repo.AddPartnership(ctx, relationship.Partnership{PersonID: mine, PartnerID: other.ID, Kind: relationship.KindMarriage}); !errors.Is(err, relationship.ErrPersonMissing) {
		t.Fatalf("got %v, want ErrPersonMissing", err)
	}
	if err := f.repo.AddPartnership(ctx, relationship.Partnership{PersonID: mine, PartnerID: 999999, Kind: relationship.KindMarriage}); !errors.Is(err, relationship.ErrPersonMissing) {
		t.Fatalf("missing person: got %v, want ErrPersonMissing", err)
	}
}

// The same pair, named in both orders at once, must yield exactly one
// partnership. Without canonical ordering the descending inserts would be
// refused by the partner_order CHECK instead of being treated as the same pair.
func TestConcurrentAddsOfTheSamePairCreateOne(t *testing.T) {
	ctx := context.Background()
	const contenders = 20
	f := newFixture(t)
	a, b := f.person(t, "a"), f.person(t, "b")

	var wg sync.WaitGroup
	errs := make([]error, contenders)
	start := make(chan struct{})
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			x, y := a, b
			if i%2 == 1 {
				x, y = b, a
			}
			errs[i] = f.svc.AddPartnership(ctx, f.owner, relationship.Partnership{PersonID: x, PartnerID: y, Kind: relationship.KindMarriage})
		}()
	}
	close(start)
	wg.Wait()

	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, relationship.ErrDuplicatePartnership):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM partnership`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if ok != 1 || n != 1 {
		t.Fatalf("%d adds succeeded and %d rows stored; want exactly 1 (%s)", ok, n, fmt.Sprint(errs))
	}
}
