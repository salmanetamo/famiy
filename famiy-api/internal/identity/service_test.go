package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salmanetamo/famiy/famiy-api/internal/identity"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/testdb"
)

const goodPassword = "correct horse battery staple"

func cfg(mutate func(*identity.Config)) identity.Config {
	c := identity.Config{
		RegistrationEnabled: true,
		MaxUsers:            100,
		SessionTTL:          time.Hour,
		Params:              identity.Params{Memory: 8, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16},
	}
	if mutate != nil {
		mutate(&c)
	}
	return c
}

func newService(t *testing.T, mutate func(*identity.Config)) (*identity.Service, *pgxpool.Pool, *identity.PgRepository) {
	t.Helper()
	pool := testdb.New(t)
	repo := identity.NewPgRepository(pool)
	svc, err := identity.NewService(repo, cfg(mutate))
	if err != nil {
		t.Fatal(err)
	}
	return svc, pool, repo
}

func TestRegisterLoginAuthenticateLogout(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newService(t, nil)

	u, sess, err := svc.Register(ctx, "  Ada@Example.com ", goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "ada@example.com" {
		t.Errorf("email = %q, want normalised", u.Email)
	}
	if got, err := svc.Authenticate(ctx, sess.Token); err != nil || got.ID != u.ID {
		t.Fatalf("registration should sign in: %v %v", got, err)
	}

	_, loginSess, err := svc.Login(ctx, "ADA@example.com", goodPassword, "1.1.1.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if loginSess.Token == sess.Token {
		t.Error("login reused the registration token")
	}

	if err := svc.Logout(ctx, loginSess.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, loginSess.Token); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("after logout got %v, want ErrUnauthenticated", err)
	}
	if _, err := svc.Authenticate(ctx, sess.Token); err != nil {
		t.Errorf("logging out one session must not end another: %v", err)
	}
	if err := svc.Logout(ctx, "no-such-token"); err != nil {
		t.Errorf("logout of an unknown token should be a no-op, got %v", err)
	}
}

func TestSessionTokenIsStoredOnlyAsHash(t *testing.T) {
	ctx := context.Background()
	svc, pool, _ := newService(t, nil)
	_, sess, err := svc.Register(ctx, "ada@example.com", goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM session`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(sess.Token))
	if !bytes.Equal(stored, want[:]) {
		t.Fatal("stored value is not the SHA-256 of the token")
	}
	if bytes.Contains(stored, []byte(sess.Token)) {
		t.Fatal("raw token found in the database")
	}
	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM app_user`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, goodPassword) {
		t.Fatalf("password not stored as an argon2id hash: %q", hash)
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newService(t, nil)
	if _, _, err := svc.Register(ctx, "ada@example.com", goodPassword); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string][2]string{
		"wrong password": {"ada@example.com", "not the password at all"},
		"unknown email":  {"nobody@example.com", goodPassword},
		"invalid email":  {"not an email", goodPassword},
	} {
		_, _, err := svc.Login(ctx, c[0], c[1], "2.2.2.2")
		if !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Errorf("%s: got %v, want ErrInvalidCredentials", name, err)
		}
	}
}

func TestLoginIsRateLimitedAfterRepeatedFailures(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newService(t, nil)
	if _, _, err := svc.Register(ctx, "ada@example.com", goodPassword); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, _, err := svc.Login(ctx, "ada@example.com", "wrong password here", "3.3.3.3"); !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	// Blocked even with the right password, so guessing cannot continue.
	if _, _, err := svc.Login(ctx, "ada@example.com", goodPassword, "3.3.3.3"); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("got %v, want ErrTooManyAttempts", err)
	}
	// A different address is unaffected.
	if _, _, err := svc.Login(ctx, "ada@example.com", goodPassword, "4.4.4.4"); err != nil {
		t.Fatalf("other address blocked too: %v", err)
	}
}

func TestRegistrationRules(t *testing.T) {
	ctx := context.Background()

	t.Run("disabled", func(t *testing.T) {
		svc, _, _ := newService(t, func(c *identity.Config) { c.RegistrationEnabled = false })
		if _, _, err := svc.Register(ctx, "ada@example.com", goodPassword); !errors.Is(err, identity.ErrRegistrationClosed) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("cap of zero closes registration", func(t *testing.T) {
		svc, _, _ := newService(t, func(c *identity.Config) { c.MaxUsers = 0 })
		if _, _, err := svc.Register(ctx, "ada@example.com", goodPassword); !errors.Is(err, identity.ErrRegistrationClosed) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("cap reached", func(t *testing.T) {
		svc, _, _ := newService(t, func(c *identity.Config) { c.MaxUsers = 2 })
		for i := 0; i < 2; i++ {
			if _, _, err := svc.Register(ctx, fmt.Sprintf("u%d@example.com", i), goodPassword); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := svc.Register(ctx, "u3@example.com", goodPassword); !errors.Is(err, identity.ErrRegistrationClosed) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("duplicate email, case-insensitively", func(t *testing.T) {
		svc, _, _ := newService(t, nil)
		if _, _, err := svc.Register(ctx, "ada@example.com", goodPassword); err != nil {
			t.Fatal(err)
		}
		if _, _, err := svc.Register(ctx, "ADA@Example.com", goodPassword); !errors.Is(err, identity.ErrEmailTaken) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("validation", func(t *testing.T) {
		svc, _, _ := newService(t, nil)
		if _, _, err := svc.Register(ctx, "nope", goodPassword); err == nil {
			t.Error("accepted an invalid email")
		}
		if _, _, err := svc.Register(ctx, "ada@example.com", "short"); err == nil {
			t.Error("accepted a short password")
		}
	})
}

// The cap check and the insert must be atomic: without the advisory lock in
// CreateUser, concurrent signups each see "room left" and all succeed.
func TestRegistrationCapHoldsUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	const cap, contenders = 3, 15
	svc, pool, _ := newService(t, func(c *identity.Config) { c.MaxUsers = cap; c.MaxConcurrentHashes = 8 })

	var wg sync.WaitGroup
	errs := make([]error, contenders)
	start := make(chan struct{})
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, errs[i] = svc.Register(ctx, fmt.Sprintf("user%d@example.com", i), goodPassword)
		}()
	}
	close(start)
	wg.Wait()

	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, identity.ErrRegistrationClosed):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM app_user`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if ok != cap || n != cap {
		t.Fatalf("%d registrations succeeded and %d users stored; want exactly %d", ok, n, cap)
	}
}

func TestExpiredSessionsAreRejectedAndCleaned(t *testing.T) {
	ctx := context.Background()
	svc, pool, repo := newService(t, nil)
	u, _, err := svc.Register(ctx, "ada@example.com", goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("expired-token"))
	if err := repo.CreateSession(ctx, sum[:], u.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, "expired-token"); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("expired session accepted: %v", err)
	}
	if _, err := svc.Authenticate(ctx, ""); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("empty token: %v", err)
	}

	if _, _, err := svc.Login(ctx, "ada@example.com", goodPassword, "5.5.5.5"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session WHERE expires_at <= now()`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d expired sessions survived a login", n)
	}
}

func TestDeletingAUserEndsTheirSessions(t *testing.T) {
	ctx := context.Background()
	svc, pool, _ := newService(t, nil)
	_, sess, err := svc.Register(ctx, "ada@example.com", goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM app_user`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, sess.Token); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
}
