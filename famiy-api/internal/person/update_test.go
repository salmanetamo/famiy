package person_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

// Two updates to different fields must both survive. Without the row lock in
// Update, both read the same old row and the later write puts the old value of
// the other field back (a lost update).
func TestConcurrentUpdatesToDifferentFieldsAreBothKept(t *testing.T) {
	ctx := context.Background()
	const rounds = 25
	pool := testdb.New(t)
	svc := person.NewService(person.NewPgRepository(pool))
	owner := testdb.InsertUser(t, pool, "owner@example.com")

	for i := 0; i < rounds; i++ {
		p, err := svc.Create(ctx, owner, person.Input{GivenName: "Start"})
		if err != nil {
			t.Fatal(err)
		}
		given, family := fmt.Sprintf("Given%d", i), fmt.Sprintf("Family%d", i)

		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, errs[0] = svc.Update(ctx, owner, p.ID, person.Patch{GivenName: &given})
		}()
		go func() {
			defer wg.Done()
			<-start
			_, errs[1] = svc.Update(ctx, owner, p.ID, person.Patch{FamilyName: person.Nullable[string]{Set: true, Value: &family}})
		}()
		close(start)
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}

		got, err := svc.Get(ctx, owner, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.GivenName != given || got.FamilyName == nil || *got.FamilyName != family {
			t.Fatalf("round %d: lost update, got given=%q family=%v; want %q and %q", i, got.GivenName, got.FamilyName, given, family)
		}
	}
}
