// Package person is the person module: domain model, service, repository
// interface, HTTP handlers and the pgx repository. The domain (this file and
// service.go) imports neither net/http nor pgx.
package person

import (
	"strings"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

var ErrNotFound = apperr.NotFound("person_not_found", "person not found")

// Person is a node in the family graph. Dates are calendar dates (UTC midnight).
type Person struct {
	ID          int64
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
// assigned by the repository).
func New(in Input) (Person, error) {
	var problems []string

	given := strings.TrimSpace(in.GivenName)
	if given == "" {
		problems = append(problems, "given_name is required")
	}
	var family *string
	if in.FamilyName != nil {
		if f := strings.TrimSpace(*in.FamilyName); f != "" {
			family = &f
		}
	}
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
