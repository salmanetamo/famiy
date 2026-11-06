// Package graph is the read model for rendering a whole account's family graph:
// people, parent links and partnerships in one consistent snapshot. It is the
// query side of the system, so unlike the feature modules it reads the person,
// parent_child and partnership tables directly instead of calling into
// person and relationship (three separate calls could not share a snapshot).
// The domain here imports neither net/http nor pgx.
package graph

import (
	"context"
	"fmt"
	"time"
)

// Node is a person as the layout needs it. Profile text (bio, residence) is
// deliberately absent; clients fetch it per person.
type Node struct {
	ID          int64
	GivenName   string
	FamilyName  *string
	BirthDate   *time.Time
	BirthApprox bool
	DeathDate   *time.Time
}

// ParentLink is a parent -> child edge.
type ParentLink struct {
	ParentID int64
	ChildID  int64
}

// Partnership is an undirected edge between two people. The order of A and B is
// an internal detail of storage and carries no meaning.
type Partnership struct {
	A    int64
	B    int64
	Kind string
}

// Graph is everything one account owns. The slices are never nil.
type Graph struct {
	Persons      []Node
	ParentLinks  []ParentLink
	Partnerships []Partnership
}

// Repository is the persistence boundary. Graph returns only rows owned by
// ownerID, read from a single snapshot so a link never refers to a person
// that is missing from Persons.
type Repository interface {
	Graph(ctx context.Context, ownerID int64) (Graph, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Get returns the whole graph of the account ownerID.
func (s *Service) Get(ctx context.Context, ownerID int64) (Graph, error) {
	g, err := s.repo.Graph(ctx, ownerID)
	if err != nil {
		return Graph{}, fmt.Errorf("get graph: %w", err)
	}
	return g, nil
}
