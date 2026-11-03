package person

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

func date(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func strp(s string) *string { return &s }

var testNow = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		in      Input
		wantErr string // substring of the message; "" means success
	}{
		{name: "minimal", in: Input{GivenName: "Ada"}},
		{name: "full", in: Input{GivenName: "Ada", FamilyName: strp("Lovelace"),
			BirthDate: date(1815, 12, 10), BirthApprox: true, DeathDate: date(1852, 11, 27)}},
		{name: "blank given name", in: Input{GivenName: "   "}, wantErr: "given_name is required"},
		{name: "death before birth", in: Input{GivenName: "X", BirthDate: date(2000, 1, 2), DeathDate: date(2000, 1, 1)},
			wantErr: "death_date must not be before birth_date"},
		{name: "death same day as birth is ok", in: Input{GivenName: "X", BirthDate: date(2000, 1, 1), DeathDate: date(2000, 1, 1)}},
		{name: "approx without birth date", in: Input{GivenName: "X", BirthApprox: true}, wantErr: "birth_approx requires birth_date"},
		{name: "name at the limit", in: Input{GivenName: strings.Repeat("é", MaxNameLen)}},
		{name: "given name too long", in: Input{GivenName: strings.Repeat("é", MaxNameLen+1)}, wantErr: "given_name must be at most 200 characters"},
		{name: "family name too long", in: Input{GivenName: "A", FamilyName: strp(strings.Repeat("x", MaxNameLen+1))}, wantErr: "family_name must be at most 200 characters"},
		{name: "birth before minimum", in: Input{GivenName: "A", BirthDate: date(999, 12, 31)}, wantErr: "birth_date must be on or after 1000-01-01"},
		{name: "year zero", in: Input{GivenName: "A", BirthDate: date(0, 1, 1)}, wantErr: "birth_date must be on or after 1000-01-01"},
		{name: "minimum date ok", in: Input{GivenName: "A", BirthDate: date(1000, 1, 1)}},
		{name: "birth in the future", in: Input{GivenName: "A", BirthDate: date(2026, 11, 5)}, wantErr: "birth_date must not be in the future"},
		{name: "birth tomorrow tolerated (client ahead of UTC)", in: Input{GivenName: "A", BirthDate: date(2026, 11, 3)}},
		{name: "death in the future", in: Input{GivenName: "A", DeathDate: date(2027, 1, 1)}, wantErr: "death_date must not be in the future"},
		{name: "reports every problem", in: Input{GivenName: "", BirthApprox: true}, wantErr: "given_name is required; birth_approx requires birth_date"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.in, testNow)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Kind != apperr.KindInvalid {
				t.Fatalf("want KindInvalid apperr, got %v (person %+v)", err, p)
			}
			if ae.Message != tc.wantErr {
				t.Fatalf("message = %q, want %q", ae.Message, tc.wantErr)
			}
		})
	}
}

func TestNewTrimsNames(t *testing.T) {
	p, err := New(Input{GivenName: "  Ada ", FamilyName: strp("  ")}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if p.GivenName != "Ada" {
		t.Errorf("GivenName = %q, want trimmed", p.GivenName)
	}
	if p.FamilyName != nil {
		t.Errorf("blank family name should become nil, got %q", *p.FamilyName)
	}
}
