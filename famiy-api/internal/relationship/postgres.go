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
