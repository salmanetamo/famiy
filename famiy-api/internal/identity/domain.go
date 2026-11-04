// Package identity owns accounts, passwords and server-side sessions (ADR-004).
// The domain files (this one, password.go, limiter.go, service.go) import
// neither net/http nor pgx.
package identity

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

const (
	MinPasswordLen = 12
	MaxPasswordLen = 128 // bounds hashing cost for hostile input
	maxEmailLen    = 254

	loginWindow    = 15 * time.Minute // failed-login counting window
	registerWindow = time.Hour        // registration attempt counting window
)

var (
	ErrInvalidCredentials   = apperr.Unauthenticated("invalid_credentials", "email or password is incorrect")
	ErrUnauthenticated      = apperr.Unauthenticated("unauthenticated", "authentication required")
	ErrRegistrationClosed   = apperr.Forbidden("registration_closed", "registration is closed")
	ErrEmailTaken           = apperr.Conflict("email_taken", "an account with this email already exists")
	ErrTooManyAttempts      = apperr.RateLimited("too_many_attempts", "too many failed attempts; try again later", loginWindow)
	ErrTooManyRegistrations = apperr.RateLimited("too_many_registrations", "too many registration attempts from this address; try again later", registerWindow)
)

// User is an account. The password hash never leaves the persistence layer
// except through Repository.UserByEmail.
type User struct {
	ID        int64
	Email     string
	CreatedAt time.Time
}

// Session is the one-time view of a freshly issued session. Token is the only
// copy of the secret; the database keeps just its SHA-256.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// NormalizeEmail trims and lower-cases an address and checks it is a bare
// addr-spec (no display name, no comments).
func NormalizeEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	bad := apperr.Invalid("validation_failed", "email must be a valid ASCII address of at most 254 characters")
	if len(e) < 3 || len(e) > maxEmailLen || !isASCII(e) {
		// ASCII only: Go and PostgreSQL can disagree on lower-casing non-ASCII
		// letters, and look-alike addresses would make confusable accounts.
		return "", bad
	}
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e {
		return "", bad
	}
	return e, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// ValidatePassword enforces length only: no composition rules.
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen || n > MaxPasswordLen {
		return apperr.Invalid("validation_failed",
			fmt.Sprintf("password must be between %d and %d characters", MinPasswordLen, MaxPasswordLen))
	}
	return nil
}
