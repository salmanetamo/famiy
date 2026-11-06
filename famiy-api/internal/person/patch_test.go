package person

import (
	"reflect"
	"testing"
	"time"
)

func TestPatchApplyAndValidate(t *testing.T) {
	cur := Person{
		GivenName: "Ada", FamilyName: strp("Lovelace"),
		BirthDate: date(1815, 12, 10), BirthApprox: true, DeathDate: date(1852, 11, 27),
	}
	clear := func() Nullable[time.Time] { return Nullable[time.Time]{Set: true} }
	setDate := func(d *time.Time) Nullable[time.Time] { return Nullable[time.Time]{Set: true, Value: d} }
	f := false

	tests := []struct {
		name    string
		patch   Patch
		want    Input // expected result; ignored when wantErr is set
		wantErr string
	}{
		{name: "empty patch changes nothing", patch: Patch{},
			want: Input{GivenName: "Ada", FamilyName: strp("Lovelace"), BirthDate: date(1815, 12, 10), BirthApprox: true, DeathDate: date(1852, 11, 27)}},
		{name: "given name replaced", patch: Patch{GivenName: strp("Augusta")},
			want: Input{GivenName: "Augusta", FamilyName: strp("Lovelace"), BirthDate: date(1815, 12, 10), BirthApprox: true, DeathDate: date(1852, 11, 27)}},
		{name: "family name cleared", patch: Patch{FamilyName: Nullable[string]{Set: true}},
			want: Input{GivenName: "Ada", BirthDate: date(1815, 12, 10), BirthApprox: true, DeathDate: date(1852, 11, 27)}},
		{name: "blank family name clears too", patch: Patch{FamilyName: Nullable[string]{Set: true, Value: strp("  ")}},
			want: Input{GivenName: "Ada", BirthDate: date(1815, 12, 10), BirthApprox: true, DeathDate: date(1852, 11, 27)}},
		{name: "death date cleared", patch: Patch{DeathDate: clear()},
			want: Input{GivenName: "Ada", FamilyName: strp("Lovelace"), BirthDate: date(1815, 12, 10), BirthApprox: true}},
		{name: "birth date and approx cleared together", patch: Patch{BirthDate: clear(), BirthApprox: &f},
			want: Input{GivenName: "Ada", FamilyName: strp("Lovelace"), DeathDate: date(1852, 11, 27)}},
		{name: "clearing birth date alone leaves approx dangling",
			patch: Patch{BirthDate: clear()}, wantErr: "birth_approx requires birth_date"},
		{name: "death moved before the stored birth",
			patch: Patch{DeathDate: setDate(date(1800, 1, 1))}, wantErr: "death_date must not be before birth_date"},
		{name: "birth moved after the stored death",
			patch: Patch{BirthDate: setDate(date(1900, 1, 1))}, wantErr: "death_date must not be before birth_date"},
		{name: "birth in the future", patch: Patch{BirthDate: setDate(date(2030, 1, 1)), DeathDate: clear()}, wantErr: "birth_date must not be in the future"},
		{name: "blank given name", patch: Patch{GivenName: strp(" ")}, wantErr: "given_name is required"},
		{name: "NUL in given name", patch: Patch{GivenName: strp("A\x00")}, wantErr: "given_name must be valid text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := New(tc.patch.Apply(cur), testNow)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, err := New(tc.want, testNow)
			if err != nil {
				t.Fatalf("test expectation is itself invalid: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}
