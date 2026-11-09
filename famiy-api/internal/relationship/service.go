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

	// AddPartnership stores the pair (in either order). It returns
	// ErrDuplicatePartnership if the pair already has one and ErrPersonMissing for
	// a missing or cross-owner person.
	AddPartnership(ctx context.Context, p Partnership) error
	// GetPartnership returns the pair's kind, or ErrPartnershipNotFound.
	GetPartnership(ctx context.Context, personID, partnerID int64) (Partnership, error)
	// UpdatePartnershipKind changes the pair's kind, or returns ErrPartnershipNotFound.
	UpdatePartnershipKind(ctx context.Context, personID, partnerID int64, kind string) (Partnership, error)
	// RemovePartnership deletes the pair, or returns ErrPartnershipNotFound.
	RemovePartnership(ctx context.Context, personID, partnerID int64) error
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
	return s.requireOwnedIDs(ctx, ownerID, l.ParentID, l.ChildID)
}

// requireOwnedIDs checks that every id is a person of ownerID. A person that is
// missing or owned by someone else is reported as person.ErrNotFound.
func (s *Service) requireOwnedIDs(ctx context.Context, ownerID int64, ids ...int64) error {
	for _, id := range ids {
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

// AddPartnership records that two of ownerID's people are partners. The order
// of the two ids does not matter. A person that is missing or belongs to
// someone else is reported as person.ErrNotFound; the database also refuses
// cross-owner pairs (same_owner) as a backstop.
//
// No advisory lock is taken: the only cross-row rules are the primary key and
// the same-owner trigger, and the database enforces both atomically.
func (s *Service) AddPartnership(ctx context.Context, ownerID int64, p Partnership) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := s.requireOwnedIDs(ctx, ownerID, p.PersonID, p.PartnerID); err != nil {
		return err
	}
	if err := s.repo.AddPartnership(ctx, p); err != nil {
		return fmt.Errorf("add partnership %d-%d: %w", p.PersonID, p.PartnerID, err)
	}
	return nil
}

// UpdatePartnership changes the kind of an existing partnership. A nil kind
// leaves it unchanged and returns the current partnership. Both people must be
// ownerID's; only then can ErrPartnershipNotFound be returned, so it leaks
// nothing across accounts.
func (s *Service) UpdatePartnership(ctx context.Context, ownerID, personID, partnerID int64, kind *string) (Partnership, error) {
	if personID == partnerID {
		return Partnership{}, ErrSelfPartner
	}
	if kind != nil && !ValidKind(*kind) {
		return Partnership{}, ErrInvalidKind
	}
	if err := s.requireOwnedIDs(ctx, ownerID, personID, partnerID); err != nil {
		return Partnership{}, err
	}
	var (
		p   Partnership
		err error
	)
	if kind == nil {
		p, err = s.repo.GetPartnership(ctx, personID, partnerID)
	} else {
		p, err = s.repo.UpdatePartnershipKind(ctx, personID, partnerID, *kind)
	}
	if err != nil {
		return Partnership{}, fmt.Errorf("update partnership %d-%d: %w", personID, partnerID, err)
	}
	return p, nil
}

// RemovePartnership deletes the partnership between two of ownerID's people.
// Like removing a parent link, it takes no lock: deleting an edge cannot break
// any rule.
func (s *Service) RemovePartnership(ctx context.Context, ownerID, personID, partnerID int64) error {
	if personID == partnerID {
		return ErrSelfPartner
	}
	if err := s.requireOwnedIDs(ctx, ownerID, personID, partnerID); err != nil {
		return err
	}
	if err := s.repo.RemovePartnership(ctx, personID, partnerID); err != nil {
		return fmt.Errorf("remove partnership %d-%d: %w", personID, partnerID, err)
	}
	return nil
}
