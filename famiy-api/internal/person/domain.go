// Package person is the person module: domain model, service, repository
// interface, HTTP handlers and the pgx repository. The domain (this file and
// service.go) imports neither net/http nor pgx.
package person

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

// Limits. The database repeats them as CHECK constraints (migration 000002).
const (
	MaxNameLen = 200 // characters, per name field
)

// MinDate is the earliest date accepted for births and deaths. It keeps input
// inside what both time.Time and PostgreSQL DATE represent sensibly.
var MinDate = time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC)

// Search limits.
const (
	MaxQueryLen        = 100 // characters in the whole query
	MaxQueryTokens     = 5   // whitespace-separated words
	DefaultSearchLimit = 20
	MaxSearchLimit     = 50
)

// DefaultMaxPersons is the per-account cap when none is configured. It bounds
// the size of the GET /graph response (about 3 MB of JSON at the cap).
const DefaultMaxPersons = 10000

var ErrNotFound = apperr.NotFound("person_not_found", "person not found")

// ErrLimitReached is returned when an account already owns the maximum number of people.
var ErrLimitReached = apperr.Conflict("person_limit_reached", "this account has reached its limit on the number of people")

// Person is a node in the family graph. Dates are calendar dates (UTC midnight).
type Person struct {
	ID          int64
	OwnerID     int64 // the account that owns this person; set by the repository
	GivenName   string
	FamilyName  *string
	BirthDate   *time.Time
	BirthApprox bool
	DeathDate   *time.Time
	CreatedAt   time.Time
}

// Input is the data a caller supplies to create a person.
type Input struct {
	GivenName   string
	FamilyName  *string
	BirthDate   *time.Time
	BirthApprox bool
	DeathDate   *time.Time
}

// New validates in and returns a Person ready to persist (ID and CreatedAt are
// assigned by the repository, which also records the owner). now is the caller's clock, used to reject dates
// in the future; one day of slack covers clients ahead of UTC.
func New(in Input, now time.Time) (Person, error) {
	var problems []string

	given := strings.TrimSpace(in.GivenName)
	switch {
	case given == "":
		problems = append(problems, "given_name is required")
	case utf8.RuneCountInString(given) > MaxNameLen:
		problems = append(problems, fmt.Sprintf("given_name must be at most %d characters", MaxNameLen))
	}
	var family *string
	if in.FamilyName != nil {
		if f := strings.TrimSpace(*in.FamilyName); f != "" {
			if utf8.RuneCountInString(f) > MaxNameLen {
				problems = append(problems, fmt.Sprintf("family_name must be at most %d characters", MaxNameLen))
			}
			family = &f
		}
	}

	latest := now.UTC().AddDate(0, 0, 1)
	problems = append(problems, checkDate("birth_date", in.BirthDate, latest)...)
	problems = append(problems, checkDate("death_date", in.DeathDate, latest)...)
	if in.BirthDate != nil && in.DeathDate != nil && in.DeathDate.Before(*in.BirthDate) {
		problems = append(problems, "death_date must not be before birth_date")
	}
	if in.BirthApprox && in.BirthDate == nil {
		problems = append(problems, "birth_approx requires birth_date")
	}
	if len(problems) > 0 {
		return Person{}, apperr.Invalid("validation_failed", strings.Join(problems, "; "))
	}

	return Person{
		GivenName:   given,
		FamilyName:  family,
		BirthDate:   in.BirthDate,
		BirthApprox: in.BirthApprox,
		DeathDate:   in.DeathDate,
	}, nil
}

func checkDate(field string, d *time.Time, latest time.Time) []string {
	if d == nil {
		return nil
	}
	if d.Before(MinDate) {
		return []string{fmt.Sprintf("%s must be on or after %s", field, MinDate.Format("2006-01-02"))}
	}
	if d.After(latest) {
		return []string{field + " must not be in the future"}
	}
	return nil
}

// ParseQuery splits a search query into its words. Every word must appear in a
// person's name for them to match. The query is trimmed; it must not be empty.
func ParseQuery(q string) ([]string, error) {
	q = strings.TrimSpace(q)
	switch {
	// PostgreSQL text cannot hold NUL or invalid UTF-8; reject here rather than fail in the query.
	case !utf8.ValidString(q) || strings.ContainsRune(q, 0):
		return nil, apperr.Invalid("validation_failed", "q must be valid text")
	case q == "":
		return nil, apperr.Invalid("validation_failed", "q is required")
	case utf8.RuneCountInString(q) > MaxQueryLen:
		return nil, apperr.Invalid("validation_failed", fmt.Sprintf("q must be at most %d characters", MaxQueryLen))
	}
	tokens := strings.Fields(q)
	if len(tokens) > MaxQueryTokens {
		return nil, apperr.Invalid("validation_failed", fmt.Sprintf("q must have at most %d words", MaxQueryTokens))
	}
	return tokens, nil
}
