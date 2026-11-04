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

var ErrNotFound = apperr.NotFound("person_not_found", "person not found")

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
