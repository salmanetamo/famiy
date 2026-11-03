package relationship

import (
	"context"
	"errors"
	"fmt"

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
			}
		}
	}
	return fmt.Errorf("insert parent_child: %w", err)
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
