package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pgUniqueViolation = "23505"
	// Arbitrary app-wide advisory-lock key serialising account creation, so the
	// MAX_USERS check and the insert are atomic. (Distinct from the parent_link key.)
	userCreateLockKey = 727002
)

// PgRepository is the pgx implementation of Repository.
type PgRepository struct {
	pool *pgxpool.Pool
}

func NewPgRepository(pool *pgxpool.Pool) *PgRepository {
	return &PgRepository{pool: pool}
}

func (r *PgRepository) CreateUser(ctx context.Context, email, passwordHash string, maxUsers int) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful Commit

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, userCreateLockKey); err != nil {
		return User{}, fmt.Errorf("lock: %w", err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM app_user`).Scan(&n); err != nil {
		return User{}, fmt.Errorf("count users: %w", err)
	}
	if n >= maxUsers {
		return User{}, ErrRegistrationClosed
	}

	var u User
	err = tx.QueryRow(ctx,
		`INSERT INTO app_user (email, password_hash) VALUES ($1, $2) RETURNING id, email, created_at`,
		email, passwordHash).Scan(&u.ID, &u.Email, &u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == "app_user_email_key" {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit: %w", err)
	}
	return u, nil
}

func (r *PgRepository) UserByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, created_at, password_hash FROM app_user WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNoUser
	}
	if err != nil {
		return User{}, "", fmt.Errorf("select user: %w", err)
	}
	return u, hash, nil
}

func (r *PgRepository) CreateSession(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO session (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, userID, expiresAt); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func (r *PgRepository) UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.created_at
		FROM   session s JOIN app_user u ON u.id = s.user_id
		WHERE  s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now).
		Scan(&u.ID, &u.Email, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNoUser
	}
	if err != nil {
		return User{}, fmt.Errorf("select session: %w", err)
	}
	return u, nil
}

func (r *PgRepository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM session WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *PgRepository) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM session WHERE expires_at <= $1`, now); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
