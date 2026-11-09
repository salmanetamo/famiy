package person

import (
	"strings"
	"testing"
)

func TestNewBioAndResidence(t *testing.T) {
	multiline := "Born in Fez.\nMoved to Rabat in 1970."
	tests := []struct {
		name     string
		bio, res *string
		wantBio  *string
		wantRes  *string
		wantErr  string
	}{
		{name: "both absent"},
		{name: "set", bio: strp("A teacher."), res: strp("Casablanca, Morocco"), wantBio: strp("A teacher."), wantRes: strp("Casablanca, Morocco")},
		{name: "trimmed", bio: strp("  A teacher.\n"), res: strp("  Fez "), wantBio: strp("A teacher."), wantRes: strp("Fez")},
		{name: "blank becomes nil", bio: strp(" \n\t "), res: strp(""), wantBio: nil, wantRes: nil},
		{name: "Unicode spaces count as blank", bio: strp("  　"), res: strp("\u0085  "), wantBio: nil, wantRes: nil},
		{name: "line breaks inside the bio are kept", bio: &multiline, wantBio: &multiline},
		{name: "bio at the limit", bio: strp(strings.Repeat("é", MaxBioLen)), wantBio: strp(strings.Repeat("é", MaxBioLen))},
		{name: "bio over the limit", bio: strp(strings.Repeat("é", MaxBioLen+1)), wantErr: "bio must be at most 2000 characters"},
		{name: "residence at the limit", res: strp(strings.Repeat("é", MaxResidenceLen)), wantRes: strp(strings.Repeat("é", MaxResidenceLen))},
		{name: "residence over the limit", res: strp(strings.Repeat("é", MaxResidenceLen+1)), wantErr: "residence must be at most 200 characters"},
		{name: "NUL in bio", bio: strp("a\x00b"), wantErr: "bio must be valid text"},
		{name: "invalid UTF-8 in residence", res: strp("a\xffb"), wantErr: "residence must be valid text"},
		{name: "reports both", bio: strp("a\x00"), res: strp(strings.Repeat("x", MaxResidenceLen+1)),
			wantErr: "bio must be valid text; residence must be at most 200 characters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(Input{GivenName: "Ada", Bio: tc.bio, Residence: tc.res}, testNow)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			eq := func(got, want *string) bool {
				if got == nil || want == nil {
					return got == nil && want == nil
				}
				return *got == *want
			}
			if !eq(p.Bio, tc.wantBio) || !eq(p.Residence, tc.wantRes) {
				t.Fatalf("bio = %v, residence = %v; want %v and %v", p.Bio, p.Residence, tc.wantBio, tc.wantRes)
			}
		})
	}
}

func TestPatchBioAndResidence(t *testing.T) {
	cur := Person{GivenName: "Ada", Bio: strp("Old bio"), Residence: strp("Old place")}
	clear := Nullable[string]{Set: true}
	set := func(s string) Nullable[string] { return Nullable[string]{Set: true, Value: &s} }

	tests := []struct {
		name    string
		patch   Patch
		wantBio string // "" means nil
		wantRes string
		wantErr string
	}{
		{name: "empty patch keeps both", patch: Patch{}, wantBio: "Old bio", wantRes: "Old place"},
		{name: "replace bio only", patch: Patch{Bio: set("New bio")}, wantBio: "New bio", wantRes: "Old place"},
		{name: "replace residence only", patch: Patch{Residence: set("New place")}, wantBio: "Old bio", wantRes: "New place"},
		{name: "clear bio", patch: Patch{Bio: clear}, wantBio: "", wantRes: "Old place"},
		{name: "blank clears residence", patch: Patch{Residence: set("   ")}, wantBio: "Old bio", wantRes: ""},
		{name: "too long", patch: Patch{Bio: set(strings.Repeat("x", MaxBioLen+1))}, wantErr: "bio must be at most 2000 characters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.patch.Apply(cur), testNow)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			str := func(s *string) string {
				if s == nil {
					return ""
				}
				return *s
			}
			if str(p.Bio) != tc.wantBio || str(p.Residence) != tc.wantRes {
				t.Fatalf("bio = %q, residence = %q; want %q and %q", str(p.Bio), str(p.Residence), tc.wantBio, tc.wantRes)
			}
		})
	}
}
