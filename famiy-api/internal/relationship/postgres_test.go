package relationship_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
	"github.com/salmanetamo/famiy/famiy-api/internal/relationship"
)

type fixture struct {
	pool    *pgxpool.Pool
	persons *person.Service
	repo    *relationship.PgRepository
	svc     *relationship.Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	persons := person.NewService(person.NewPgRepository(pool))
	repo := relationship.NewPgRepository(pool)
	return fixture{pool, persons, repo, relationship.NewService(repo, persons)}
}

func (f fixture) person(t *testing.T, name string) int64 {
	t.Helper()
	p, err := f.persons.Create(context.Background(), person.Input{GivenName: name})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return p.ID
}

func (f fixture) link(t *testing.T, parent, child int64) {
	t.Helper()
	if err := f.svc.AddParentLink(context.Background(), relationship.ParentLink{ParentID: parent, ChildID: child}); err != nil {
		t.Fatalf("link %d->%d: %v", parent, child, err)
	}
}

func TestAncestorsChain(t *testing.T) {
	f := newFixture(t)
	grand, parent, child := f.person(t, "grand"), f.person(t, "parent"), f.person(t, "child")
	f.link(t, grand, parent)
	f.link(t, parent, child)

	got, err := f.svc.Ancestors(context.Background(), child)
	if err != nil {
		t.Fatal(err)
	}
	want := []relationship.Ancestor{{PersonID: parent, Generations: 1}, {PersonID: grand, Generations: 2}}
	assertAncestors(t, got, want)
}

// Cousin marriage: C's parents X and Y are cousins, so G1 is reachable by
// several paths. Y also has G1 as a direct parent, so the shortest path to G1
// is 2 even though longer paths (3) exist. Each ancestor must appear once, at
// its minimum depth.
func TestAncestorsDiamondCollapsesDuplicatePaths(t *testing.T) {
	f := newFixture(t)
	g1, g2 := f.person(t, "g1"), f.person(t, "g2")
	p1, p2 := f.person(t, "p1"), f.person(t, "p2")
	x, y, c := f.person(t, "x"), f.person(t, "y"), f.person(t, "c")

	f.link(t, g1, p1)
	f.link(t, g2, p1)
	f.link(t, g1, p2)
	f.link(t, p1, x)
	f.link(t, p2, y)
	f.link(t, g1, y) // shortcut: direct grandparent link
	f.link(t, x, c)
	f.link(t, y, c)

	got, err := f.svc.Ancestors(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	want := []relationship.Ancestor{
		{x, 1}, {y, 1},
		{g1, 2}, {p1, 2}, {p2, 2},
		{g2, 3},
	}
	assertAncestors(t, got, want)
}

func TestAncestorsNoParentsIsEmptyNotNil(t *testing.T) {
	f := newFixture(t)
	id := f.person(t, "orphan")
	got, err := f.svc.Ancestors(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", got)
	}
}

func TestAncestorsUnknownPerson(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Ancestors(context.Background(), 12345)
	if !errors.Is(err, person.ErrNotFound) {
		t.Fatalf("want person.ErrNotFound, got %v", err)
	}
}

func TestAddParentLinkConstraints(t *testing.T) {
	f := newFixture(t)
	a, b, c, child := f.person(t, "a"), f.person(t, "b"), f.person(t, "c"), f.person(t, "child")
	f.link(t, a, child)
	f.link(t, b, child)

	tests := []struct {
		name string
		link relationship.ParentLink
		want error
	}{
		{"third parent", relationship.ParentLink{ParentID: c, ChildID: child}, relationship.ErrTooManyParents},
		{"duplicate link", relationship.ParentLink{ParentID: a, ChildID: child}, relationship.ErrDuplicateParent},
		{"unknown parent", relationship.ParentLink{ParentID: 9999, ChildID: c}, relationship.ErrPersonMissing},
		{"unknown child", relationship.ParentLink{ParentID: a, ChildID: 9999}, relationship.ErrPersonMissing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := f.svc.AddParentLink(context.Background(), tc.link); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("self parent rejected by domain", func(t *testing.T) {
		err := f.svc.AddParentLink(context.Background(), relationship.ParentLink{ParentID: a, ChildID: a})
		if !errors.Is(err, relationship.ErrSelfParent) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("self parent rejected by database even if domain is bypassed", func(t *testing.T) {
		err := f.repo.AddParentLink(context.Background(), relationship.ParentLink{ParentID: a, ChildID: a})
		if !errors.Is(err, relationship.ErrSelfParent) {
			t.Fatalf("got %v", err)
		}
	})
}

// Proves the row lock in enforce_max_two_parents: without it, concurrent
// inserts each see "<2 parents" and a child can end up with more than two.
// Many children are raced at once because a single round only fails sometimes.
func TestTwoParentLimitHoldsUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	const contenders, children = 10, 15
	parents := make([]int64, contenders)
	for i := range parents {
		parents[i] = f.person(t, fmt.Sprintf("parent-%d", i))
	}
	kids := make([]int64, children)
	for i := range kids {
		kids[i] = f.person(t, fmt.Sprintf("child-%d", i))
	}

	var wg sync.WaitGroup
	errs := make([][]error, children)
	start := make(chan struct{})
	for ci, child := range kids {
		errs[ci] = make([]error, contenders)
		for pi, parent := range parents {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[ci][pi] = f.svc.AddParentLink(context.Background(), relationship.ParentLink{ParentID: parent, ChildID: child})
			}()
		}
	}
	close(start)
	wg.Wait()

	for ci, child := range kids {
		ok := 0
		for _, err := range errs[ci] {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, relationship.ErrTooManyParents):
			default:
				t.Errorf("child %d: unexpected error: %v", child, err)
			}
		}
		var n int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM parent_child WHERE child_id = $1`, child).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if ok != 2 || n != 2 {
			t.Errorf("child %d: %d inserts succeeded, %d rows stored; want 2 and 2", child, ok, n)
		}
	}
}

func assertAncestors(t *testing.T, got, want []relationship.Ancestor) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
