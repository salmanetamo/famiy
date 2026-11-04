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
	"unicode/utf8"
)

// Config is the service's behaviour, normally filled from the environment.
type Config struct {
	RegistrationEnabled bool          // kill switch; false closes registration
	MaxUsers            int           // registration closes at this many accounts; 0 closes it
	SessionTTL          time.Duration // absolute session lifetime
	Params              Params        // zero value means DefaultParams
	MaxConcurrentHashes int           // login-side concurrent hashes; zero means 2
}

const (
	maxLoginAttemptsPerKey = 5  // per (email, address) per loginWindow
	maxLoginAttemptsPerIP  = 30 // per address per loginWindow
	maxRegisterPerIP       = 10 // registration attempts per address per registerWindow
)

// ErrNoUser is returned by Repository.UserByEmail and UserBySession when no
// row matches. It stays inside this module: callers see ErrInvalidCredentials
// or ErrUnauthenticated.
var ErrNoUser = errors.New("no such user or session")

// Repository is the persistence boundary. CreateUser must atomically enforce
// maxUsers and return ErrRegistrationClosed / ErrEmailTaken.
type Repository interface {
	CreateUser(ctx context.Context, email, passwordHash string, maxUsers int) (User, error)
	CountUsers(ctx context.Context) (int, error)
	UserByEmail(ctx context.Context, email string) (u User, passwordHash string, err error)
	CreateSession(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error
	UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
}

type Service struct {
	repo   Repository
	hasher *Hasher // login: verification
	// registerHasher has its own single hash slot, so a flood of registrations
	// cannot occupy the slots that logins need.
	registerHasher *Hasher
	cfg            Config
	now            func() time.Time
	limiter        *attemptLimiter // login attempts, by (email, address) and by address
	regLimiter     *attemptLimiter // registration attempts, by address
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
	registerHasher, err := NewHasher(cfg.Params, 1)
	if err != nil {
		return nil, fmt.Errorf("create register hasher: %w", err)
	}
	now := time.Now
	return &Service{
		repo:           repo,
		hasher:         hasher,
		registerHasher: registerHasher,
		cfg:            cfg,
		now:            now,
		limiter:        newAttemptLimiter(now, loginWindow, maxLoginAttemptsPerKey, maxLoginAttemptsPerIP),
		regLimiter:     newAttemptLimiter(now, registerWindow, maxRegisterPerIP, maxRegisterPerIP),
	}, nil
}

// Register creates an account and signs it in. clientAddr throttles repeated
// attempts from one address.
func (s *Service) Register(ctx context.Context, rawEmail, password, clientAddr string) (User, Session, error) {
	if !s.cfg.RegistrationEnabled {
		return User{}, Session{}, ErrRegistrationClosed
	}
	if !s.regLimiter.reserve("", clientAddr) {
		return User{}, Session{}, ErrTooManyRegistrations
	}
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return User{}, Session{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return User{}, Session{}, err
	}
	// Cheap early refusal so a full instance does not spend an expensive hash on
	// every request. CreateUser re-checks atomically; this read is an
	// optimisation only.
	n, err := s.repo.CountUsers(ctx)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("count users: %w", err)
	}
	if n >= s.cfg.MaxUsers {
		return User{}, Session{}, ErrRegistrationClosed
	}
	hash, err := s.registerHasher.Hash(ctx, password)
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

// Login verifies credentials and starts a session. clientAddr keys the attempt
// limiter. Every credential failure looks the same to the caller.
func (s *Service) Login(ctx context.Context, rawEmail, password, clientAddr string) (User, Session, error) {
	email := limiterEmail(rawEmail)
	// Reserve the attempt up front: counting only after the slow verification
	// would let parallel requests all pass the check.
	if !s.limiter.reserve(email, clientAddr) {
		return User{}, Session{}, ErrTooManyAttempts
	}
	// Reject absurd passwords before spending a hash on them. No account can have
	// one, so this reveals nothing.
	if utf8.RuneCountInString(password) > MaxPasswordLen {
		return User{}, Session{}, ErrInvalidCredentials
	}

	normalized, err := NormalizeEmail(rawEmail)
	if err != nil {
		if derr := s.hasher.VerifyDummy(ctx, password); derr != nil {
			return User{}, Session{}, fmt.Errorf("verify: %w", derr)
		}
		return User{}, Session{}, ErrInvalidCredentials
	}
	u, hash, err := s.repo.UserByEmail(ctx, normalized)
	if errors.Is(err, ErrNoUser) {
		if derr := s.hasher.VerifyDummy(ctx, password); derr != nil {
			return User{}, Session{}, fmt.Errorf("verify: %w", derr)
		}
		return User{}, Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find user: %w", err)
	}
	ok, err := s.hasher.Verify(ctx, password, hash)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return User{}, Session{}, ErrInvalidCredentials
	}

	s.limiter.release(email, clientAddr)
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
