package person

import (
	"context"
	"fmt"
	"time"
)

// Repository is the persistence boundary for persons. Implementations must
// return ErrNotFound from Get when no row matches.
type Repository interface {
	Insert(ctx context.Context, p Person) (Person, error)
	Get(ctx context.Context, id int64) (Person, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) Create(ctx context.Context, in Input) (Person, error) {
	p, err := New(in, s.now())
	if err != nil {
		return Person{}, err
	}
	created, err := s.repo.Insert(ctx, p)
	if err != nil {
		return Person{}, fmt.Errorf("create person: %w", err)
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Person, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return Person{}, fmt.Errorf("get person %d: %w", id, err)
	}
	return p, nil
}
