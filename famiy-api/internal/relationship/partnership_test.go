package relationship

import (
	"errors"
	"testing"
)

func TestPartnershipValidate(t *testing.T) {
	tests := []struct {
		name string
		p    Partnership
		want error
	}{
		{"marriage", Partnership{PersonID: 1, PartnerID: 2, Kind: KindMarriage}, nil},
		{"partnership", Partnership{PersonID: 2, PartnerID: 1, Kind: KindPartnership}, nil},
		{"other", Partnership{PersonID: 1, PartnerID: 2, Kind: KindOther}, nil},
		{"self", Partnership{PersonID: 3, PartnerID: 3, Kind: KindMarriage}, ErrSelfPartner},
		{"self wins over a bad kind", Partnership{PersonID: 3, PartnerID: 3, Kind: "x"}, ErrSelfPartner},
		{"unknown kind", Partnership{PersonID: 1, PartnerID: 2, Kind: "friends"}, ErrInvalidKind},
		{"empty kind", Partnership{PersonID: 1, PartnerID: 2}, ErrInvalidKind},
		{"kind is case sensitive", Partnership{PersonID: 1, PartnerID: 2, Kind: "Marriage"}, ErrInvalidKind},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.p.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPartnershipOrderedIgnoresTheOrderGiven(t *testing.T) {
	for _, tc := range [][2]int64{{1, 2}, {2, 1}, {10, 3}, {3, 10}} {
		a, b := Partnership{PersonID: tc[0], PartnerID: tc[1]}.Ordered()
		if a >= b {
			t.Errorf("Ordered(%d, %d) = (%d, %d), want ascending", tc[0], tc[1], a, b)
		}
		if (a != tc[0] || b != tc[1]) && (a != tc[1] || b != tc[0]) {
			t.Errorf("Ordered(%d, %d) = (%d, %d) lost an id", tc[0], tc[1], a, b)
		}
	}
}
