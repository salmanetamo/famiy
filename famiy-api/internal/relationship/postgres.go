package relationship

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQL SQLSTATE codes we translate into domain errors.
const (
	pgForeignKeyViolation = "23503"
	pgUniqueViolation     = "23505"
	pgCheckViolation      = "23514" // CHECK constraints and the max-two-parents trigger
)

// PgRepository is the pgx implementation of Repository.
type PgRepository struct {
	pool *pgxpool.Pool
}

func NewPgRepository(pool *pgxpool.Pool) *PgRepository {
	return &PgRepository{pool: pool}
}

func (r *PgRepository) AddParentLink(ctx context.Context, l ParentLink) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO parent_child (parent_id, child_id) VALUES ($1, $2)`,
		l.ParentID, l.ChildID)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgForeignKeyViolation:
			return ErrPersonMissing
		case pgUniqueViolation:
			return ErrDuplicateParent
		case pgCheckViolation:
			// Match on the constraint name the migrations set, never on message text.
			switch pgErr.ConstraintName {
			case "no_self_parent":
				return ErrSelfParent
			case "max_two_parents":
				return ErrTooManyParents
			case "no_cycle":
				return ErrCycle
			case "same_owner":
				// Reported as "not found" so a caller cannot learn that the
				// other person exists under another account.
				return ErrPersonMissing
			}
		}
	}
	return fmt.Errorf("insert parent_child: %w", err)
}

func (r *PgRepository) RemoveParentLink(ctx context.Context, l ParentLink) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM parent_child WHERE parent_id = $1 AND child_id = $2`, l.ParentID, l.ChildID)
	if err != nil {
		return fmt.Errorf("delete parent_child: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLinkNotFound
	}
	return nil
}

// ancestorsSQL is PoC-1's Q1. UNION ALL (not UNION) because rows carry a depth
// column; CYCLE guards against accidental loops; min(depth)+GROUP BY collapses
// duplicate paths created by diamonds (e.g. cousin marriages).
const ancestorsSQL = `
WITH RECURSIVE anc(id, depth) AS (
        SELECT parent_id, 1
        FROM   parent_child
        WHERE  child_id = $1
    UNION ALL
        SELECT pc.parent_id, a.depth + 1
        FROM   parent_child pc
        JOIN   anc a ON pc.child_id = a.id
) CYCLE id SET is_cycle USING path
SELECT id, min(depth)::int AS generations
FROM   anc
GROUP  BY id
ORDER  BY generations, id`

// Ancestors returns every ancestor of personID. The result is not paginated and
// the recursion has no depth cap: at this app's scale (a person has at most a
// few hundred ancestors) that is deliberate, and the CYCLE clause plus the
// acyclicity trigger keep the recursion finite.
func (r *PgRepository) Ancestors(ctx context.Context, personID int64) ([]Ancestor, error) {
	rows, err := r.pool.Query(ctx, ancestorsSQL, personID)
	if err != nil {
		return nil, fmt.Errorf("query ancestors: %w", err)
	}
	defer rows.Close()

	out := []Ancestor{}
	for rows.Next() {
		var a Ancestor
		if err := rows.Scan(&a.PersonID, &a.Generations); err != nil {
			return nil, fmt.Errorf("scan ancestor: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ancestors: %w", err)
	}
	return out, nil
}

func (r *PgRepository) AddPartnership(ctx context.Context, p Partnership) error {
	a, b := p.Ordered()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO partnership (partner_a, partner_b, kind) VALUES ($1, $2, $3)`, a, b, p.Kind)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgForeignKeyViolation:
			return ErrPersonMissing
		case pgUniqueViolation:
			return ErrDuplicatePartnership
		case pgCheckViolation:
			// Match on the constraint name the migrations set, never on message text.
			switch pgErr.ConstraintName {
			case "partner_order":
				return ErrSelfPartner // a < b is strict, so only a == b can fail it here
			case "partnership_kind_allowed":
				return ErrInvalidKind
			case "same_owner":
				// Reported as "not found" so a caller cannot learn that the
				// other person exists under another account.
				return ErrPersonMissing
			}
		}
	}
	return fmt.Errorf("insert partnership: %w", err)
}

func (r *PgRepository) GetPartnership(ctx context.Context, personID, partnerID int64) (Partnership, error) {
	p := Partnership{PersonID: personID, PartnerID: partnerID}
	a, b := p.Ordered()
	err := r.pool.QueryRow(ctx,
		`SELECT kind FROM partnership WHERE partner_a = $1 AND partner_b = $2`, a, b).Scan(&p.Kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return Partnership{}, ErrPartnershipNotFound
	}
	if err != nil {
		return Partnership{}, fmt.Errorf("select partnership: %w", err)
	}
	return p, nil
}

func (r *PgRepository) UpdatePartnershipKind(ctx context.Context, personID, partnerID int64, kind string) (Partnership, error) {
	p := Partnership{PersonID: personID, PartnerID: partnerID}
	a, b := p.Ordered()
	err := r.pool.QueryRow(ctx,
		`UPDATE partnership SET kind = $3 WHERE partner_a = $1 AND partner_b = $2 RETURNING kind`, a, b, kind).Scan(&p.Kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return Partnership{}, ErrPartnershipNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgCheckViolation && pgErr.ConstraintName == "partnership_kind_allowed" {
		return Partnership{}, ErrInvalidKind
	}
	if err != nil {
		return Partnership{}, fmt.Errorf("update partnership: %w", err)
	}
	return p, nil
}

func (r *PgRepository) RemovePartnership(ctx context.Context, personID, partnerID int64) error {
	a, b := Partnership{PersonID: personID, PartnerID: partnerID}.Ordered()
	tag, err := r.pool.Exec(ctx, `DELETE FROM partnership WHERE partner_a = $1 AND partner_b = $2`, a, b)
	if err != nil {
		return fmt.Errorf("delete partnership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPartnershipNotFound
	}
	return nil
}

// descendantsSQL is PoC-1's Q2: the mirror of ancestorsSQL.
const descendantsSQL = `
WITH RECURSIVE descendant(id, depth) AS (
        SELECT child_id, 1
        FROM   parent_child
        WHERE  parent_id = $1
    UNION ALL
        SELECT pc.child_id, d.depth + 1
        FROM   parent_child pc
        JOIN   descendant d ON pc.parent_id = d.id
) CYCLE id SET is_cycle USING path
SELECT id, min(depth)::int AS generations
FROM   descendant
GROUP  BY id
ORDER  BY generations, id`

// Descendants returns every descendant of personID, unpaginated and without a
// depth cap for the same reasons as Ancestors.
func (r *PgRepository) Descendants(ctx context.Context, personID int64) ([]Descendant, error) {
	rows, err := r.pool.Query(ctx, descendantsSQL, personID)
	if err != nil {
		return nil, fmt.Errorf("query descendants: %w", err)
	}
	defer rows.Close()

	out := []Descendant{}
	for rows.Next() {
		var d Descendant
		if err := rows.Scan(&d.PersonID, &d.Generations); err != nil {
			return nil, fmt.Errorf("scan descendant: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate descendants: %w", err)
	}
	return out, nil
}

// Siblings derives siblings from shared parents; nothing is stored. Each shared
// parent contributes one row, so count(*) is the number of parents in common.
func (r *PgRepository) Siblings(ctx context.Context, personID int64) ([]Sibling, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sib.child_id, count(*)::int
		FROM   parent_child me
		JOIN   parent_child sib ON sib.parent_id = me.parent_id AND sib.child_id <> me.child_id
		WHERE  me.child_id = $1
		GROUP  BY sib.child_id
		ORDER  BY sib.child_id`, personID)
	if err != nil {
		return nil, fmt.Errorf("query siblings: %w", err)
	}
	defer rows.Close()

	out := []Sibling{}
	for rows.Next() {
		var s Sibling
		if err := rows.Scan(&s.PersonID, &s.SharedParents); err != nil {
			return nil, fmt.Errorf("scan sibling: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate siblings: %w", err)
	}
	return out, nil
}

// commonAncestrySQL is PoC-1's Q3, changed in two ways. It keeps the best
// (da, db) pair instead of one arbitrary row, preferring the smallest total
// distance, then the most balanced pair, then the smaller ANCESTOR id. That last
// key must not depend on which person is asked about: breaking the tie by da
// would make (a, b) and (b, a) pick different ancestors under pedigree collapse
// and call each other niece and nephew. And it returns every common ancestor at
// exactly that pair, so full siblings list both parents and half-siblings one.
//
// The recursion enumerates paths (UNION ALL), not distinct nodes; min(depth)
// and GROUP BY collapse them afterwards. Repeated cousin marriages multiply the
// paths, which is fine at this app's scale and generation depth but would not be
// for a deeply inbred pedigree.
const commonAncestrySQL = `
WITH RECURSIVE anc(root, id, depth) AS (
        SELECT v.id, v.id, 0
        FROM  (VALUES ($1::bigint), ($2::bigint)) AS v(id)
    UNION ALL
        SELECT a.root, pc.parent_id, a.depth + 1
        FROM   anc a
        JOIN   parent_child pc ON pc.child_id = a.id
) CYCLE id SET is_cycle USING path,
a_anc AS (SELECT id, min(depth) AS da FROM anc WHERE root = $1 GROUP BY id),
b_anc AS (SELECT id, min(depth) AS db FROM anc WHERE root = $2 GROUP BY id),
common AS (SELECT id, da, db FROM a_anc JOIN b_anc USING (id)),
best AS (SELECT da, db FROM common ORDER BY da + db, abs(da - db), id LIMIT 1)
SELECT c.id, c.da::int, c.db::int
FROM   common c JOIN best b ON b.da = c.da AND b.db = c.db
ORDER  BY c.id`

func (r *PgRepository) CommonAncestry(ctx context.Context, personID, otherID int64) (CommonAncestry, bool, error) {
	rows, err := r.pool.Query(ctx, commonAncestrySQL, personID, otherID)
	if err != nil {
		return CommonAncestry{}, false, fmt.Errorf("query common ancestry: %w", err)
	}
	defer rows.Close()

	ca := CommonAncestry{AncestorIDs: []int64{}}
	found := false
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id, &ca.StepsPerson, &ca.StepsOther); err != nil {
			return CommonAncestry{}, false, fmt.Errorf("scan common ancestor: %w", err)
		}
		ca.AncestorIDs = append(ca.AncestorIDs, id)
		found = true
	}
	if err := rows.Err(); err != nil {
		return CommonAncestry{}, false, fmt.Errorf("iterate common ancestors: %w", err)
	}
	return ca, found, nil
}
