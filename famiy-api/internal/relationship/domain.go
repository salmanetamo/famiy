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
	ErrLinkNotFound    = apperr.NotFound("parent_link_not_found", "that person is not a parent of this person")
	ErrCycle           = apperr.Conflict("would_create_cycle", "this parent link would make a person their own ancestor")
	ErrSelfPartner     = apperr.Invalid("self_partner", "a person cannot be their own partner")
	ErrInvalidKind     = apperr.Invalid("validation_failed", "kind must be one of: marriage, partnership, other")

	ErrDuplicatePartnership = apperr.Conflict("duplicate_partnership", "these two people already have a partnership")
	ErrPartnershipNotFound  = apperr.NotFound("partnership_not_found", "these two people are not partners")

	ErrPersonMissing = person.ErrNotFound // one value, so errors.Is works from any layer
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

// Partnership kinds. The database repeats this list as a CHECK constraint
// (migration 000006); extend both together.
const (
	KindMarriage    = "marriage"
	KindPartnership = "partnership" // an unmarried couple
	KindOther       = "other"
)

// DefaultKind is used when a client does not say.
const DefaultKind = KindMarriage

// ValidKind reports whether k is an allowed partnership kind.
func ValidKind(k string) bool {
	switch k {
	case KindMarriage, KindPartnership, KindOther:
		return true
	}
	return false
}

// Partnership is an undirected edge between two people. PersonID and PartnerID
// are in whatever order the caller named them: storage keeps each pair once in
// ascending order, and that stays an internal detail (see Ordered).
type Partnership struct {
	PersonID  int64
	PartnerID int64
	Kind      string
}

// Validate enforces the invariants that need no database. The database enforces
// them again (CHECK partner_order, the kind constraint, PRIMARY KEY).
func (p Partnership) Validate() error {
	if p.PersonID == p.PartnerID {
		return ErrSelfPartner
	}
	if !ValidKind(p.Kind) {
		return ErrInvalidKind
	}
	return nil
}

// Ordered returns the pair in storage order (smaller id first), so (a, b) and
// (b, a) are the same partnership.
func (p Partnership) Ordered() (a, b int64) {
	if p.PersonID < p.PartnerID {
		return p.PersonID, p.PartnerID
	}
	return p.PartnerID, p.PersonID
}
