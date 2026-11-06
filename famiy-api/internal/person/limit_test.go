package person_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

func TestPersonCapIsPerAccount(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	svc := person.NewService(person.NewPgRepository(pool), person.WithMaxPersons(2))
	alice := testdb.InsertUser(t, pool, "alice@example.com")
	bob := testdb.InsertUser(t, pool, "bob@example.com")

	for i := 0; i < 2; i++ {
		if _, err := svc.Create(ctx, alice, person.Input{GivenName: fmt.Sprintf("A%d", i)}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := svc.Create(ctx, alice, person.Input{GivenName: "Over"}); !errors.Is(err, person.ErrLimitReached) {
		t.Fatalf("third create: got %v, want ErrLimitReached", err)
	}
	// Bob's allowance is his own: Alice being full does not affect him.
	if _, err := svc.Create(ctx, bob, person.Input{GivenName: "B"}); err != nil {
		t.Fatalf("another account was blocked by Alice's cap: %v", err)
	}
}

// The count and the insert must be atomic: without the advisory lock in
// Insert, concurrent creates each see "room left" and all succeed.
func TestPersonCapHoldsUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	const maxPersons, contenders = 3, 15
	pool := testdb.New(t)
	svc := person.NewService(person.NewPgRepository(pool), person.WithMaxPersons(maxPersons))
	owner := testdb.InsertUser(t, pool, "owner@example.com")

	var wg sync.WaitGroup
	errs := make([]error, contenders)
	start := make(chan struct{})
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = svc.Create(ctx, owner, person.Input{GivenName: fmt.Sprintf("P%d", i)})
		}()
	}
	close(start)
	wg.Wait()

	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, person.ErrLimitReached):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM person WHERE owner_user_id = $1`, owner).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if ok != maxPersons || n != maxPersons {
		t.Fatalf("%d creates succeeded and %d rows stored; want exactly %d", ok, n, maxPersons)
	}
}
