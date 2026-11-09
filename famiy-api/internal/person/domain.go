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
	MaxNameLen      = 200  // characters, per name field
	MaxBioLen       = 2000 // characters; the bio may span several lines
	MaxResidenceLen = 200  // characters
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
	Bio         *string // short biography; nil when empty
	Residence   *string // current residence as free text; nil when empty
	CreatedAt   time.Time
}

// Input is the data a caller supplies to create a person.
type Input struct {
	GivenName   string
	FamilyName  *string
	BirthDate   *time.Time
	BirthApprox bool
	DeathDate   *time.Time
	Bio         *string
	Residence   *string
}

// New validates in and returns a Person ready to persist (ID and CreatedAt are
// assigned by the repository, which also records the owner). now is the caller's clock, used to reject dates
// in the future; one day of slack covers clients ahead of UTC.
func New(in Input, now time.Time) (Person, error) {
	var problems []string

	given := strings.TrimSpace(in.GivenName)
	switch {
	case !validText(given):
		problems = append(problems, "given_name must be valid text")
	case given == "":
		problems = append(problems, "given_name is required")
	case utf8.RuneCountInString(given) > MaxNameLen:
		problems = append(problems, fmt.Sprintf("given_name must be at most %d characters", MaxNameLen))
	}
	var family *string
	if in.FamilyName != nil {
		f := strings.TrimSpace(*in.FamilyName)
		switch {
		case !validText(f):
			problems = append(problems, "family_name must be valid text")
		case f != "":
			if utf8.RuneCountInString(f) > MaxNameLen {
				problems = append(problems, fmt.Sprintf("family_name must be at most %d characters", MaxNameLen))
			}
			family = &f
		}
	}

	bio, bad := cleanOptionalText("bio", in.Bio, MaxBioLen)
	problems = append(problems, bad...)
	residence, bad := cleanOptionalText("residence", in.Residence, MaxResidenceLen)
	problems = append(problems, bad...)

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
		Bio:         bio,
		Residence:   residence,
		BirthDate:   in.BirthDate,
		BirthApprox: in.BirthApprox,
		DeathDate:   in.DeathDate,
	}, nil
}

// cleanOptionalText trims an optional free-text field and turns a blank value
// into nil (one representation of "nothing"). It reports why a value cannot be
// stored: not valid text, or longer than maxLen characters. Line breaks are kept.
func cleanOptionalText(field string, in *string, maxLen int) (*string, []string) {
	if in == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*in)
	switch {
	case !validText(v):
		return nil, []string{field + " must be valid text"}
	case v == "":
		return nil, nil
	case utf8.RuneCountInString(v) > maxLen:
		return nil, []string{fmt.Sprintf("%s must be at most %d characters", field, maxLen)}
	}
	return &v, nil
}

// validText reports whether s can be stored in a PostgreSQL text column, which
// holds neither NUL nor invalid UTF-8. Without this check the insert fails with
// an internal error instead of a validation error.
func validText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }

// Nullable is one optional field of a Patch. Unset leaves the stored value
// alone; Set with a nil Value clears it; Set with a Value replaces it.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// Patch is a partial update with JSON Merge Patch semantics (RFC 7396). The
// required given name and the boolean cannot be cleared, so they are plain
// pointers (nil = unchanged).
type Patch struct {
	GivenName   *string
	FamilyName  Nullable[string]
	BirthDate   Nullable[time.Time]
	BirthApprox *bool
	DeathDate   Nullable[time.Time]
	Bio         Nullable[string]
	Residence   Nullable[string]
}

// Apply returns the Input that results from applying p to cur. It does not
// validate; pass the result to New.
func (p Patch) Apply(cur Person) Input {
	in := Input{
		GivenName:   cur.GivenName,
		FamilyName:  cur.FamilyName,
		BirthDate:   cur.BirthDate,
		BirthApprox: cur.BirthApprox,
		DeathDate:   cur.DeathDate,
		Bio:         cur.Bio,
		Residence:   cur.Residence,
	}
	if p.GivenName != nil {
		in.GivenName = *p.GivenName
	}
	if p.FamilyName.Set {
		in.FamilyName = p.FamilyName.Value
	}
	if p.BirthDate.Set {
		in.BirthDate = p.BirthDate.Value
	}
	if p.BirthApprox != nil {
		in.BirthApprox = *p.BirthApprox
	}
	if p.DeathDate.Set {
		in.DeathDate = p.DeathDate.Value
	}
	if p.Bio.Set {
		in.Bio = p.Bio.Value
	}
	if p.Residence.Set {
		in.Residence = p.Residence.Value
	}
	return in
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
