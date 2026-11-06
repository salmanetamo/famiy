package person

import (
	"context"
	"fmt"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

// Repository is the persistence boundary for persons. Every operation is scoped
// to an owner (ADR-004). Get must return ErrNotFound both when no row matches
// and when the row belongs to a different owner, so existence does not leak.
type Repository interface {
	// Insert stores p unless the owner already has maxPersons people, in which
	// case it returns ErrLimitReached. The count and the insert are atomic.
	Insert(ctx context.Context, ownerID int64, p Person, maxPersons int) (Person, error)
	Get(ctx context.Context, ownerID, id int64) (Person, error)
	// Update reads the owner's person under a row lock, replaces it with the
	// result of merge (which may refuse with a domain error) and returns it.
	// It returns ErrNotFound for a missing person or one owned by someone else.
	Update(ctx context.Context, ownerID, id int64, merge func(current Person) (Person, error)) (Person, error)
	// Delete removes the owner's person; their parent links and partnerships go
	// with it (ON DELETE CASCADE). It returns ErrNotFound as Update does.
	Delete(ctx context.Context, ownerID, id int64) error
	// Search returns up to limit of the owner's people whose name contains every
	// token (case-insensitively), ordered by family name, given name, id.
	Search(ctx context.Context, ownerID int64, tokens []string, limit int) ([]Person, error)
}

type Service struct {
	repo       Repository
	now        func() time.Time
	maxPersons int
}

// Option customises a Service.
type Option func(*Service)

// WithMaxPersons sets how many people one account may own (default DefaultMaxPersons).
func WithMaxPersons(n int) Option { return func(s *Service) { s.maxPersons = n } }

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo, now: time.Now, maxPersons: DefaultMaxPersons}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) Create(ctx context.Context, ownerID int64, in Input) (Person, error) {
	p, err := New(in, s.now())
	if err != nil {
		return Person{}, err
	}
	created, err := s.repo.Insert(ctx, ownerID, p, s.maxPersons)
	if err != nil {
		return Person{}, fmt.Errorf("create person: %w", err)
	}
	return created, nil
}

// Get returns the person if it exists and belongs to ownerID.
func (s *Service) Get(ctx context.Context, ownerID, id int64) (Person, error) {
	p, err := s.repo.Get(ctx, ownerID, id)
	if err != nil {
		return Person{}, fmt.Errorf("get person %d: %w", id, err)
	}
	return p, nil
}

// SearchResult is one page of search matches. HasMore reports that further
// matches exist beyond the limit.
type SearchResult struct {
	Persons []Person
	HasMore bool
}

// Search finds the owner's people by name. limit must be between 1 and MaxSearchLimit.
func (s *Service) Search(ctx context.Context, ownerID int64, q string, limit int) (SearchResult, error) {
	tokens, err := ParseQuery(q)
	if err != nil {
		return SearchResult{}, err
	}
	if limit < 1 || limit > MaxSearchLimit {
		return SearchResult{}, apperr.Invalid("validation_failed", fmt.Sprintf("limit must be between 1 and %d", MaxSearchLimit))
	}
	// One extra row tells us whether there is more without a second query.
	found, err := s.repo.Search(ctx, ownerID, tokens, limit+1)
	if err != nil {
		return SearchResult{}, fmt.Errorf("search persons: %w", err)
	}
	res := SearchResult{Persons: found}
	if len(found) > limit {
		res.Persons, res.HasMore = found[:limit], true
	}
	return res, nil
}

// Update applies patch to one of ownerID's people. The merged result goes
// through New, so create and update share one set of rules, including the ones
// that span fields (birth_approx needs birth_date; death not before birth).
func (s *Service) Update(ctx context.Context, ownerID, id int64, patch Patch) (Person, error) {
	updated, err := s.repo.Update(ctx, ownerID, id, func(cur Person) (Person, error) {
		return New(patch.Apply(cur), s.now())
	})
	if err != nil {
		return Person{}, fmt.Errorf("update person %d: %w", id, err)
	}
	return updated, nil
}

// Delete permanently removes one of ownerID's people together with their
// parent links and partnerships. Their children and partners are kept.
//
// No advisory lock is taken: removing edges can never create a cycle, exceed
// two parents or join two owners, and a concurrent link insert that still sees
// the person either refuses conservatively or fails its foreign key
// (reported as person_not_found).
func (s *Service) Delete(ctx context.Context, ownerID, id int64) error {
	if err := s.repo.Delete(ctx, ownerID, id); err != nil {
		return fmt.Errorf("delete person %d: %w", id, err)
	}
	return nil
}
