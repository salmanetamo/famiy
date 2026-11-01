package person

import (
	"errors"
	"testing"
	"time"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

func date(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func strp(s string) *string { return &s }

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
		{name: "reports every problem", in: Input{GivenName: "", BirthApprox: true}, wantErr: "given_name is required; birth_approx requires birth_date"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.in)
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
	p, err := New(Input{GivenName: "  Ada ", FamilyName: strp("  ")})
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
