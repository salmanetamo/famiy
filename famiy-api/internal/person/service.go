package person

import (
	"context"
	"fmt"
	"time"
)

// Repository is the persistence boundary for persons. Every operation is scoped
// to an owner (ADR-004). Get must return ErrNotFound both when no row matches
// and when the row belongs to a different owner, so existence does not leak.
type Repository interface {
	Insert(ctx context.Context, ownerID int64, p Person) (Person, error)
	Get(ctx context.Context, ownerID, id int64) (Person, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) Create(ctx context.Context, ownerID int64, in Input) (Person, error) {
	p, err := New(in, s.now())
	if err != nil {
		return Person{}, err
	}
	created, err := s.repo.Insert(ctx, ownerID, p)
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
