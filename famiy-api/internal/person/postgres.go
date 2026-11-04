package person

import (
	"context"
	"errors"
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

const personColumns = `id, owner_user_id, given_name, family_name, birth_date, birth_approx, death_date, created_at`

func (r *PgRepository) Insert(ctx context.Context, ownerID int64, p Person) (Person, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO person (owner_user_id, given_name, family_name, birth_date, birth_approx, death_date)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+personColumns,
		ownerID, p.GivenName, p.FamilyName, p.BirthDate, p.BirthApprox, p.DeathDate)
	out, err := scanPerson(row)
	if err != nil {
		return Person{}, fmt.Errorf("insert person: %w", err)
	}
	return out, nil
}

func (r *PgRepository) Get(ctx context.Context, ownerID, id int64) (Person, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+personColumns+` FROM person WHERE id = $1 AND owner_user_id = $2`, id, ownerID)
	p, err := scanPerson(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Person{}, ErrNotFound
	}
	if err != nil {
		return Person{}, fmt.Errorf("select person: %w", err)
	}
	return p, nil
}

func scanPerson(row pgx.Row) (Person, error) {
	var p Person
	err := row.Scan(&p.ID, &p.OwnerID, &p.GivenName, &p.FamilyName, &p.BirthDate, &p.BirthApprox, &p.DeathDate, &p.CreatedAt)
	return p, err
}
