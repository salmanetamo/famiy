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
	Ancestors(ctx context.Context, personID int64) ([]Ancestor, error)
}

// PersonLookup is the part of person.Service this module needs.
type PersonLookup interface {
	Get(ctx context.Context, id int64) (person.Person, error)
}

type Service struct {
	repo    Repository
	persons PersonLookup
}

func NewService(repo Repository, persons PersonLookup) *Service {
	return &Service{repo: repo, persons: persons}
}

func (s *Service) AddParentLink(ctx context.Context, l ParentLink) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if err := s.repo.AddParentLink(ctx, l); err != nil {
		return fmt.Errorf("add parent link %d->%d: %w", l.ParentID, l.ChildID, err)
	}
	return nil
}

// Ancestors returns all ancestors of personID, nearest generation first.
func (s *Service) Ancestors(ctx context.Context, personID int64) ([]Ancestor, error) {
	if _, err := s.persons.Get(ctx, personID); err != nil {
		return nil, err
	}
	as, err := s.repo.Ancestors(ctx, personID)
	if err != nil {
		return nil, fmt.Errorf("ancestors of %d: %w", personID, err)
	}
	return as, nil
}
