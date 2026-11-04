package relationship

import (
	"context"
	"fmt"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
)

// Repository is the persistence boundary for edges and traversals.
// AddParentLink must map database failures to the Err* values in domain.go.
type Repository interface {
	AddParentLink(ctx context.Context, l ParentLink) error
	// RemoveParentLink deletes the edge, or returns ErrLinkNotFound if there is none.
	RemoveParentLink(ctx context.Context, l ParentLink) error
	Ancestors(ctx context.Context, personID int64) ([]Ancestor, error)
}

// PersonLookup is the part of person.Service this module needs. Get must report
// person.ErrNotFound for a person that is missing OR owned by someone else.
type PersonLookup interface {
	Get(ctx context.Context, ownerID, id int64) (person.Person, error)
}

type Service struct {
	repo    Repository
	persons PersonLookup
}

func NewService(repo Repository, persons PersonLookup) *Service {
	return &Service{repo: repo, persons: persons}
}

// AddParentLink links two of ownerID's persons. A person that is missing or
// belongs to someone else is reported as person.ErrNotFound; the database also
// refuses cross-owner links (same_owner) as a backstop.
func (s *Service) AddParentLink(ctx context.Context, ownerID int64, l ParentLink) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if err := s.requireOwned(ctx, ownerID, l); err != nil {
		return err
	}
	if err := s.repo.AddParentLink(ctx, l); err != nil {
		return fmt.Errorf("add parent link %d->%d: %w", l.ParentID, l.ChildID, err)
	}
	return nil
}

// RemoveParentLink deletes a link between two of ownerID's persons. A person
// that is missing or belongs to someone else is reported as person.ErrNotFound;
// only when both are the caller's can ErrLinkNotFound be returned, so it leaks
// nothing across accounts.
//
// No advisory lock is taken: removing an edge can never create a cycle, exceed
// two parents or join two owners, and a concurrent insert that still sees the
// edge merely refuses conservatively.
func (s *Service) RemoveParentLink(ctx context.Context, ownerID int64, l ParentLink) error {
	if err := s.requireOwned(ctx, ownerID, l); err != nil {
		return err
	}
	if err := s.repo.RemoveParentLink(ctx, l); err != nil {
		return fmt.Errorf("remove parent link %d->%d: %w", l.ParentID, l.ChildID, err)
	}
	return nil
}

// requireOwned checks that both ends of l are persons of ownerID.
func (s *Service) requireOwned(ctx context.Context, ownerID int64, l ParentLink) error {
	for _, id := range [...]int64{l.ParentID, l.ChildID} {
		if _, err := s.persons.Get(ctx, ownerID, id); err != nil {
			return err
		}
	}
	return nil
}

// Ancestors returns all ancestors of one of ownerID's persons, nearest
// generation first. Links never cross owners, so every ancestor is theirs too.
func (s *Service) Ancestors(ctx context.Context, ownerID, personID int64) ([]Ancestor, error) {
	if _, err := s.persons.Get(ctx, ownerID, personID); err != nil {
		return nil, err
	}
	as, err := s.repo.Ancestors(ctx, personID)
	if err != nil {
		return nil, fmt.Errorf("ancestors of %d: %w", personID, err)
	}
	return as, nil
}
