package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Config is the service's behaviour, normally filled from the environment.
type Config struct {
	RegistrationEnabled bool          // kill switch; false closes registration
	MaxUsers            int           // registration closes at this many accounts; 0 closes it
	SessionTTL          time.Duration // absolute session lifetime
	Params              Params        // zero value means DefaultParams
	MaxConcurrentHashes int           // zero means 2
}

// ErrNoUser is returned by Repository.UserByEmail and UserBySession when no
// row matches. It stays inside this module: callers see ErrInvalidCredentials
// or ErrUnauthenticated.
var ErrNoUser = errors.New("no such user or session")

// Repository is the persistence boundary. CreateUser must atomically enforce
// maxUsers and return ErrRegistrationClosed / ErrEmailTaken.
type Repository interface {
	CreateUser(ctx context.Context, email, passwordHash string, maxUsers int) (User, error)
	UserByEmail(ctx context.Context, email string) (u User, passwordHash string, err error)
	CreateSession(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error
	UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
}

type Service struct {
	repo    Repository
	hasher  *Hasher
	cfg     Config
	now     func() time.Time
	limiter *attemptLimiter
}

func NewService(repo Repository, cfg Config) (*Service, error) {
	if cfg.Params == (Params{}) {
		cfg.Params = DefaultParams
	}
	if cfg.MaxConcurrentHashes == 0 {
		cfg.MaxConcurrentHashes = 2
	}
	if cfg.SessionTTL <= 0 {
		return nil, errors.New("identity: SessionTTL must be positive")
	}
	hasher, err := NewHasher(cfg.Params, cfg.MaxConcurrentHashes)
	if err != nil {
		return nil, fmt.Errorf("create hasher: %w", err)
	}
	now := time.Now
	return &Service{repo: repo, hasher: hasher, cfg: cfg, now: now, limiter: newAttemptLimiter(now)}, nil
}

// Register creates an account and signs it in.
func (s *Service) Register(ctx context.Context, rawEmail, password string) (User, Session, error) {
	if !s.cfg.RegistrationEnabled {
		return User{}, Session{}, ErrRegistrationClosed
	}
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return User{}, Session{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return User{}, Session{}, err
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}
	u, err := s.repo.CreateUser(ctx, email, hash, s.cfg.MaxUsers)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("create user: %w", err)
	}
	sess, err := s.issueSession(ctx, u.ID)
	if err != nil {
		return User{}, Session{}, err
	}
	return u, sess, nil
}

// Login verifies credentials and starts a session. clientAddr keys the failed-
// attempt limiter. Every failure looks the same to the caller.
func (s *Service) Login(ctx context.Context, rawEmail, password, clientAddr string) (User, Session, error) {
	email := limiterEmail(rawEmail)
	if s.limiter.blocked(email, clientAddr) {
		return User{}, Session{}, ErrTooManyAttempts
	}
	fail := func() (User, Session, error) {
		s.limiter.fail(email, clientAddr)
		return User{}, Session{}, ErrInvalidCredentials
	}

	normalized, err := NormalizeEmail(rawEmail)
	if err != nil {
		if derr := s.hasher.VerifyDummy(ctx, password); derr != nil {
			return User{}, Session{}, fmt.Errorf("verify: %w", derr)
		}
		return fail()
	}
	u, hash, err := s.repo.UserByEmail(ctx, normalized)
	if errors.Is(err, ErrNoUser) {
		if derr := s.hasher.VerifyDummy(ctx, password); derr != nil {
			return User{}, Session{}, fmt.Errorf("verify: %w", derr)
		}
		return fail()
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find user: %w", err)
	}
	ok, err := s.hasher.Verify(ctx, password, hash)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return fail()
	}

	s.limiter.reset(email, clientAddr)
	if err := s.repo.DeleteExpiredSessions(ctx, s.now()); err != nil {
		// Housekeeping only; expired sessions are already ignored on lookup.
		slog.WarnContext(ctx, "delete expired sessions", "err", err)
	}
	sess, err := s.issueSession(ctx, u.ID)
	if err != nil {
		return User{}, Session{}, err
	}
	return u, sess, nil
}

// Logout ends the session identified by token. An unknown token is not an error.
func (s *Service) Logout(ctx context.Context, token string) error {
	if err := s.repo.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// Authenticate resolves a session token to its user, or ErrUnauthenticated.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrUnauthenticated
	}
	u, err := s.repo.UserBySession(ctx, hashToken(token), s.now())
	if errors.Is(err, ErrNoUser) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, fmt.Errorf("find session: %w", err)
	}
	return u, nil
}

func (s *Service) issueSession(ctx context.Context, userID int64) (Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := s.now().Add(s.cfg.SessionTTL)
	if err := s.repo.CreateSession(ctx, hashToken(token), userID, expires); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return Session{Token: token, ExpiresAt: expires}, nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// limiterEmail gives the limiter a stable, bounded key even for invalid input.
func limiterEmail(raw string) string {
	if len(raw) > maxEmailLen {
		raw = raw[:maxEmailLen]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}
