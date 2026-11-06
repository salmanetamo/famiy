package person

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// personCreateLockClass namespaces the two-key advisory lock that serialises
// person creation per owner, so the cap check and the insert are atomic.
// (The two-argument lock form is a separate keyspace from the one-argument keys
// used elsewhere.)
const personCreateLockClass = 727003

const personColumns = `id, owner_user_id, given_name, family_name, birth_date, birth_approx, death_date, created_at`

func (r *PgRepository) Insert(ctx context.Context, ownerID int64, p Person, maxPersons int) (Person, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Person{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful Commit

	// Without this lock concurrent creates each see "room left" and all succeed.
	// int32(ownerID) may collide for huge ids; a collision only adds waiting.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, $2)`, int32(personCreateLockClass), int32(ownerID)); err != nil {
		return Person{}, fmt.Errorf("lock: %w", err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM person WHERE owner_user_id = $1`, ownerID).Scan(&n); err != nil {
		return Person{}, fmt.Errorf("count persons: %w", err)
	}
	if n >= maxPersons {
		return Person{}, ErrLimitReached
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO person (owner_user_id, given_name, family_name, birth_date, birth_approx, death_date)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+personColumns,
		ownerID, p.GivenName, p.FamilyName, p.BirthDate, p.BirthApprox, p.DeathDate)
	out, err := scanPerson(row)
	if err != nil {
		return Person{}, fmt.Errorf("insert person: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Person{}, fmt.Errorf("commit: %w", err)
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

// Search matches every token as a case-insensitive substring of the full name.
// The tokens are escaped so %, _ and \ in user input are literal characters.
// At the per-account cap this is a scan of at most a few thousand rows found
// through idx_person_owner; no trigram index is needed at this scale.
func (r *PgRepository) Search(ctx context.Context, ownerID int64, tokens []string, limit int) ([]Person, error) {
	patterns := make([]string, len(tokens))
	for i, t := range tokens {
		patterns[i] = "%" + escapeLike(t) + "%"
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+personColumns+` FROM person
		WHERE owner_user_id = $1 AND concat_ws(' ', given_name, family_name) ILIKE ALL ($2::text[])
		ORDER BY family_name, given_name, id
		LIMIT $3`, ownerID, patterns, limit)
	if err != nil {
		return nil, fmt.Errorf("search persons: %w", err)
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read persons: %w", err)
	}
	return out, nil
}

// escapeLike makes s safe to embed in a LIKE/ILIKE pattern (backslash is the default escape).
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
