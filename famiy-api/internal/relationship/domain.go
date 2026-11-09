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

// Descendant is a person reachable by walking parent links downward from a
// start person; Generations is the shortest path length (child = 1).
type Descendant struct {
	PersonID    int64
	Generations int
}

// Sibling is someone who shares at least one parent with a person. SharedParents
// is how many of the two possible parents they share (1 or 2). It is a count,
// not a full/half verdict: when only one parent is recorded, a sibling sharing
// it cannot be told apart from a true half-sibling.
type Sibling struct {
	PersonID      int64
	SharedParents int
}

// Relations describe what the OTHER person is to the person asked about.
const (
	RelSelf        = "self"
	RelAncestor    = "ancestor"     // the other is an ancestor (parent, grandparent, ...)
	RelDescendant  = "descendant"   // the other is a descendant (child, grandchild, ...)
	RelSibling     = "sibling"      // shares a parent (see Sibling.SharedParents)
	RelAuntUncle   = "aunt_uncle"   // the other is a sibling of one of the person's ancestors
	RelNieceNephew = "niece_nephew" // the other is a descendant of one of the person's siblings
	RelCousin      = "cousin"
)

// Kinship is the blood relationship derived from two distances to the closest
// common ancestor: StepsPerson generations up from the person, StepsOther up
// from the other. The steps are the source of truth; Relation is a category
// over them. Degree and Removed are set for cousins only (Degree 1 = first
// cousins; Removed is the difference in generations).
type Kinship struct {
	Relation    string
	StepsPerson int
	StepsOther  int
	Degree      *int
	Removed     *int
}

// Classify derives the kinship from the generational distances from each
// person up to their closest common ancestor (0 = the person is that ancestor).
// It is pure: the database only supplies the two distances, and both are >= 0.
func Classify(stepsPerson, stepsOther int) Kinship {
	k := Kinship{StepsPerson: stepsPerson, StepsOther: stepsOther}
	switch {
	case stepsPerson == 0 && stepsOther == 0:
		k.Relation = RelSelf
	case stepsPerson == 0:
		k.Relation = RelDescendant // the person is the common ancestor
	case stepsOther == 0:
		k.Relation = RelAncestor
	default:
		closest := min(stepsPerson, stepsOther)
		removed := stepsPerson - stepsOther
		if removed < 0 {
			removed = -removed
		}
		switch {
		case closest == 1 && removed == 0:
			k.Relation = RelSibling
		case closest == 1 && stepsPerson > stepsOther:
			k.Relation = RelAuntUncle // the other is one generation nearer the ancestor
		case closest == 1:
			k.Relation = RelNieceNephew
		default:
			degree := closest - 1
			k.Relation, k.Degree, k.Removed = RelCousin, &degree, &removed
		}
	}
	return k
}

// CommonAncestry is the closest common ancestry of two people: the best
// (StepsPerson, StepsOther) pair and every ancestor at exactly that pair.
type CommonAncestry struct {
	StepsPerson int
	StepsOther  int
	AncestorIDs []int64 // ascending
}

// Relationship answers "how is the other person related to this one?" by blood.
// Related is false when the two share no ancestor; partners and in-laws are not
// considered.
type Relationship struct {
	PersonID          int64
	OtherID           int64
	Related           bool
	Kinship           Kinship // meaningful only when Related
	CommonAncestorIDs []int64
}
