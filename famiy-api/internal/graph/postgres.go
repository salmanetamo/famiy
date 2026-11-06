package graph

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgRepository is the pgx implementation of Repository.
type PgRepository struct {
	pool *pgxpool.Pool
}

func NewPgRepository(pool *pgxpool.Pool) *PgRepository {
	return &PgRepository{pool: pool}
}

// Graph reads the three tables inside one REPEATABLE READ, READ-ONLY transaction
// so all of them come from the same snapshot. Edges carry no owner column; their
// owner is the owner of the person at the parent / partner_a end (the same_owner
// triggers guarantee the other end matches).
func (r *PgRepository) Graph(ctx context.Context, ownerID int64) (Graph, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Graph{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // read-only: nothing to commit

	g := Graph{Persons: []Node{}, ParentLinks: []ParentLink{}, Partnerships: []Partnership{}}

	rows, err := tx.Query(ctx, `
		SELECT id, given_name, family_name, birth_date, birth_approx, death_date
		FROM person WHERE owner_user_id = $1 ORDER BY id`, ownerID)
	if err != nil {
		return Graph{}, fmt.Errorf("select persons: %w", err)
	}
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.GivenName, &n.FamilyName, &n.BirthDate, &n.BirthApprox, &n.DeathDate); err != nil {
			rows.Close()
			return Graph{}, fmt.Errorf("scan person: %w", err)
		}
		g.Persons = append(g.Persons, n)
	}
	if err := rows.Err(); err != nil {
		return Graph{}, fmt.Errorf("read persons: %w", err)
	}

	rows, err = tx.Query(ctx, `
		SELECT pc.parent_id, pc.child_id
		FROM parent_child pc JOIN person p ON p.id = pc.parent_id
		WHERE p.owner_user_id = $1
		ORDER BY pc.parent_id, pc.child_id`, ownerID)
	if err != nil {
		return Graph{}, fmt.Errorf("select parent links: %w", err)
	}
	for rows.Next() {
		var l ParentLink
		if err := rows.Scan(&l.ParentID, &l.ChildID); err != nil {
			rows.Close()
			return Graph{}, fmt.Errorf("scan parent link: %w", err)
		}
		g.ParentLinks = append(g.ParentLinks, l)
	}
	if err := rows.Err(); err != nil {
		return Graph{}, fmt.Errorf("read parent links: %w", err)
	}

	rows, err = tx.Query(ctx, `
		SELECT pa.partner_a, pa.partner_b, pa.kind
		FROM partnership pa JOIN person p ON p.id = pa.partner_a
		WHERE p.owner_user_id = $1
		ORDER BY pa.partner_a, pa.partner_b`, ownerID)
	if err != nil {
		return Graph{}, fmt.Errorf("select partnerships: %w", err)
	}
	for rows.Next() {
		var p Partnership
		if err := rows.Scan(&p.A, &p.B, &p.Kind); err != nil {
			rows.Close()
			return Graph{}, fmt.Errorf("scan partnership: %w", err)
		}
		g.Partnerships = append(g.Partnerships, p)
	}
	if err := rows.Err(); err != nil {
		return Graph{}, fmt.Errorf("read partnerships: %w", err)
	}
	return g, nil
}
