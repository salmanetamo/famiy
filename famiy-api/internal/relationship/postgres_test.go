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
	owner   int64 // the account that owns every person the helpers create
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
	return fixture{testdb.InsertUser(t, pool, "owner@example.com"), pool, persons, repo, relationship.NewService(repo, persons)}
}

func (f fixture) person(t *testing.T, name string) int64 {
	t.Helper()
	p, err := f.persons.Create(context.Background(), f.owner, person.Input{GivenName: name})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return p.ID
}

func (f fixture) link(t *testing.T, parent, child int64) {
	t.Helper()
	if err := f.svc.AddParentLink(context.Background(), f.owner, relationship.ParentLink{ParentID: parent, ChildID: child}); err != nil {
		t.Fatalf("link %d->%d: %v", parent, child, err)
	}
}

func TestAncestorsChain(t *testing.T) {
	f := newFixture(t)
	grand, parent, child := f.person(t, "grand"), f.person(t, "parent"), f.person(t, "child")
	f.link(t, grand, parent)
	f.link(t, parent, child)

	got, err := f.svc.Ancestors(context.Background(), f.owner, child)
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

	got, err := f.svc.Ancestors(context.Background(), f.owner, c)
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
	got, err := f.svc.Ancestors(context.Background(), f.owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", got)
	}
}

func TestAncestorsUnknownPerson(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Ancestors(context.Background(), f.owner, 12345)
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
			if err := f.svc.AddParentLink(context.Background(), f.owner, tc.link); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("self parent rejected by domain", func(t *testing.T) {
		err := f.svc.AddParentLink(context.Background(), f.owner, relationship.ParentLink{ParentID: a, ChildID: a})
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
				errs[ci][pi] = f.svc.AddParentLink(context.Background(), f.owner, relationship.ParentLink{ParentID: parent, ChildID: child})
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

func TestAddParentLinkRejectsCycles(t *testing.T) {
	f := newFixture(t)
	a, b, c, d := f.person(t, "a"), f.person(t, "b"), f.person(t, "c"), f.person(t, "d")
	f.link(t, a, b) // a is b's parent
	f.link(t, b, c) // b is c's parent
	f.link(t, c, d) // c is d's parent

	tests := []struct {
		name string
		link relationship.ParentLink
		want error
	}{
		{"direct reversal", relationship.ParentLink{ParentID: b, ChildID: a}, relationship.ErrCycle},
		{"two-step loop", relationship.ParentLink{ParentID: c, ChildID: a}, relationship.ErrCycle},
		{"three-step loop", relationship.ParentLink{ParentID: d, ChildID: a}, relationship.ErrCycle},
		{"valid shortcut across generations is not a cycle", relationship.ParentLink{ParentID: a, ChildID: d}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := f.svc.AddParentLink(context.Background(), f.owner, tc.link)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// A->B and B->A inserted simultaneously lock different child rows, so only the
// advisory lock in enforce_acyclic stops both from committing. Many pairs are
// raced at once because a single pair only fails sometimes without the lock.
func TestOpposingLinksCannotBothCommit(t *testing.T) {
	f := newFixture(t)
	const pairs = 25
	type pair struct{ x, y int64 }
	ps := make([]pair, pairs)
	for i := range ps {
		ps[i] = pair{f.person(t, fmt.Sprintf("x%d", i)), f.person(t, fmt.Sprintf("y%d", i))}
	}

	var wg sync.WaitGroup
	forward := make([]error, pairs)
	backward := make([]error, pairs)
	start := make(chan struct{})
	for i, p := range ps {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			forward[i] = f.svc.AddParentLink(context.Background(), f.owner, relationship.ParentLink{ParentID: p.x, ChildID: p.y})
		}()
		go func() {
			defer wg.Done()
			<-start
			backward[i] = f.svc.AddParentLink(context.Background(), f.owner, relationship.ParentLink{ParentID: p.y, ChildID: p.x})
		}()
	}
	close(start)
	wg.Wait()

	for i, p := range ps {
		okF, okB := forward[i] == nil, backward[i] == nil
		if okF == okB {
			t.Errorf("pair %d: forward ok=%v (%v), backward ok=%v (%v); exactly one must succeed", i, okF, forward[i], okB, backward[i])
			continue
		}
		loser := forward[i]
		if okF {
			loser = backward[i]
		}
		if !errors.Is(loser, relationship.ErrCycle) {
			t.Errorf("pair %d: losing insert returned %v, want ErrCycle", i, loser)
		}
		var n int
		if err := f.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM parent_child WHERE (parent_id=$1 AND child_id=$2) OR (parent_id=$2 AND child_id=$1)`,
			p.x, p.y).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("pair %d: %d edges stored, want exactly 1", i, n)
		}
	}
}

func TestLinksCannotCrossOwners(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	bob := testdb.InsertUser(t, f.pool, "bob@example.com")
	mine := f.person(t, "mine")
	other, err := f.persons.Create(ctx, bob, person.Input{GivenName: "theirs"})
	if err != nil {
		t.Fatal(err)
	}
	theirs := other.ID

	t.Run("the service treats another account's person as not found, in both directions", func(t *testing.T) {
		for _, l := range []relationship.ParentLink{{ParentID: mine, ChildID: theirs}, {ParentID: theirs, ChildID: mine}} {
			if err := f.svc.AddParentLink(ctx, f.owner, l); !errors.Is(err, person.ErrNotFound) {
				t.Errorf("%+v: got %v, want person.ErrNotFound", l, err)
			}
		}
	})
	t.Run("the database refuses it even if the service is bypassed", func(t *testing.T) {
		for _, l := range []relationship.ParentLink{{ParentID: mine, ChildID: theirs}, {ParentID: theirs, ChildID: mine}} {
			if err := f.repo.AddParentLink(ctx, l); !errors.Is(err, relationship.ErrPersonMissing) {
				t.Errorf("%+v: got %v, want ErrPersonMissing", l, err)
			}
		}
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM parent_child`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%d edges stored (err %v), want 0", n, err)
		}
	})
	t.Run("ancestors of another account's person is not found", func(t *testing.T) {
		if _, err := f.svc.Ancestors(ctx, f.owner, theirs); !errors.Is(err, person.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("partnerships cannot cross owners either", func(t *testing.T) {
		a, b := min(mine, theirs), max(mine, theirs)
		if _, err := f.pool.Exec(ctx, `INSERT INTO partnership (partner_a, partner_b) VALUES ($1, $2)`, a, b); err == nil {
			t.Fatal("cross-owner partnership was accepted")
		}
	})
	t.Run("a partnership cannot be re-pointed at another account's person by UPDATE", func(t *testing.T) {
		x, y := f.person(t, "px"), f.person(t, "py")
		x, y = min(x, y), max(x, y)
		if _, err := f.pool.Exec(ctx, `INSERT INTO partnership (partner_a, partner_b) VALUES ($1, $2)`, x, y); err != nil {
			t.Fatal(err)
		}
		// Choose the new partner so the canonical-order CHECK (a < b) holds and
		// only the owner rule can reject the change: a foreign person with a
		// larger id than x, created now.
		foreign, err := f.persons.Create(ctx, bob, person.Input{GivenName: "foreign"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `UPDATE partnership SET partner_b = $1 WHERE partner_a = $2`, foreign.ID, x); err == nil {
			t.Fatal("UPDATE moved a partnership onto another account's person")
		}
	})
	t.Run("same-owner partnerships still work", func(t *testing.T) {
		second := f.person(t, "second")
		a, b := min(mine, second), max(mine, second)
		if _, err := f.pool.Exec(ctx, `INSERT INTO partnership (partner_a, partner_b) VALUES ($1, $2)`, a, b); err != nil {
			t.Fatalf("same-owner partnership rejected: %v", err)
		}
	})
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

func TestRemoveParentLink(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	mum, dad, kid := f.person(t, "mum"), f.person(t, "dad"), f.person(t, "kid")
	f.link(t, mum, kid)
	f.link(t, dad, kid)

	remove := func(parent, child int64) error {
		return f.svc.RemoveParentLink(ctx, f.owner, relationship.ParentLink{ParentID: parent, ChildID: child})
	}

	t.Run("removes the link and only that link", func(t *testing.T) {
		if err := remove(mum, kid); err != nil {
			t.Fatal(err)
		}
		got, err := f.svc.Ancestors(ctx, f.owner, kid)
		if err != nil {
			t.Fatal(err)
		}
		assertAncestors(t, got, []relationship.Ancestor{{PersonID: dad, Generations: 1}})
	})
	t.Run("removing it again reports that there is no such link", func(t *testing.T) {
		if err := remove(mum, kid); !errors.Is(err, relationship.ErrLinkNotFound) {
			t.Fatalf("got %v, want ErrLinkNotFound", err)
		}
	})
	t.Run("two people who were never linked", func(t *testing.T) {
		stranger := f.person(t, "stranger")
		if err := remove(stranger, kid); !errors.Is(err, relationship.ErrLinkNotFound) {
			t.Fatalf("got %v, want ErrLinkNotFound", err)
		}
	})
	t.Run("a link cannot be removed in the wrong direction", func(t *testing.T) {
		if err := remove(kid, dad); !errors.Is(err, relationship.ErrLinkNotFound) {
			t.Fatalf("got %v, want ErrLinkNotFound", err)
		}
	})
	t.Run("an unknown person is reported as not found", func(t *testing.T) {
		if err := remove(99999, kid); !errors.Is(err, person.ErrNotFound) {
			t.Fatalf("got %v, want person.ErrNotFound", err)
		}
	})
	t.Run("removal frees a parent slot", func(t *testing.T) {
		third := f.person(t, "third")
		f.link(t, third, kid) // kid had {dad} left, so there is room for a second parent
		other := f.person(t, "other")
		if err := f.svc.AddParentLink(ctx, f.owner, relationship.ParentLink{ParentID: other, ChildID: kid}); !errors.Is(err, relationship.ErrTooManyParents) {
			t.Fatalf("kid should be full, got %v", err)
		}
		if err := remove(third, kid); err != nil {
			t.Fatal(err)
		}
		f.link(t, other, kid)
	})
}

func TestRemovingALinkCanUnblockAnotherThatWouldHaveBeenACycle(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a, b, c := f.person(t, "a"), f.person(t, "b"), f.person(t, "c")
	f.link(t, a, b)
	f.link(t, b, c)

	loop := relationship.ParentLink{ParentID: c, ChildID: a} // c as a's parent closes a -> b -> c -> a
	if err := f.svc.AddParentLink(ctx, f.owner, loop); !errors.Is(err, relationship.ErrCycle) {
		t.Fatalf("got %v, want ErrCycle", err)
	}
	if err := f.svc.RemoveParentLink(ctx, f.owner, relationship.ParentLink{ParentID: a, ChildID: b}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AddParentLink(ctx, f.owner, loop); err != nil {
		t.Fatalf("link should be allowed once the chain is broken: %v", err)
	}
}

// Like linking, removal inside another account's tree is stopped only by the
// service's ownership check: the database has no objection to a DELETE.
func TestRemoveParentLinkCannotTouchAnotherAccountsTree(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	bob := testdb.InsertUser(t, f.pool, "bob@example.com")
	p, err := f.persons.Create(ctx, bob, person.Input{GivenName: "bob-parent"})
	if err != nil {
		t.Fatal(err)
	}
	k, err := f.persons.Create(ctx, bob, person.Input{GivenName: "bob-kid"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AddParentLink(ctx, bob, relationship.ParentLink{ParentID: p.ID, ChildID: k.ID}); err != nil {
		t.Fatal(err)
	}

	err = f.svc.RemoveParentLink(ctx, f.owner, relationship.ParentLink{ParentID: p.ID, ChildID: k.ID})
	if !errors.Is(err, person.ErrNotFound) {
		t.Fatalf("got %v, want person.ErrNotFound", err)
	}
	got, err := f.svc.Ancestors(ctx, bob, k.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertAncestors(t, got, []relationship.Ancestor{{PersonID: p.ID, Generations: 1}})
}
