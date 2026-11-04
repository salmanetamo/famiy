// Package relationship owns parent-child and partnership edges and the
// traversals over them. Parent->child is the atomic edge; siblings and
// genealogical distance are derived, never stored (ADR-001).
package relationship

import (
	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

var (
	ErrSelfParent      = apperr.Invalid("self_parent", "a person cannot be their own parent")
	ErrTooManyParents  = apperr.Conflict("too_many_parents", "a person can have at most two parents")
	ErrDuplicateParent = apperr.Conflict("duplicate_parent", "this parent link already exists")
	ErrCycle           = apperr.Conflict("would_create_cycle", "this parent link would make a person their own ancestor")
	ErrPersonMissing   = person.ErrNotFound // one value, so errors.Is works from any layer
)

// ParentLink is the atomic kinship edge.
type ParentLink struct {
	ParentID int64
	ChildID  int64
}

// Validate enforces the invariants that need no database. The database
// enforces them again (CHECK, PRIMARY KEY, triggers), and alone enforces the
// ones that need other rows: the two-parent limit and acyclicity (ErrCycle).
func (l ParentLink) Validate() error {
	if l.ParentID == l.ChildID {
		return ErrSelfParent
	}
	return nil
}

// Ancestor is a person reachable by walking parent links upward from a start
// person; Generations is the shortest path length (parent = 1).
type Ancestor struct {
	PersonID    int64
	Generations int
}
