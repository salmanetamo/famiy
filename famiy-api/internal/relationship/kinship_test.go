package relationship

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name          string
		person, other int
		relation      string
		degree        int // 0 = nil
		removed       int // only checked for cousins
	}{
		{"same person", 0, 0, RelSelf, 0, 0},
		{"the other is a child", 0, 1, RelDescendant, 0, 0},
		{"the other is a grandchild", 0, 2, RelDescendant, 0, 0},
		{"the other is a great-great-grandchild", 0, 4, RelDescendant, 0, 0},
		{"the other is a parent", 1, 0, RelAncestor, 0, 0},
		{"the other is a grandparent", 2, 0, RelAncestor, 0, 0},
		{"the other is a great-great-grandparent", 4, 0, RelAncestor, 0, 0},
		{"siblings", 1, 1, RelSibling, 0, 0},
		{"the other is an aunt or uncle", 2, 1, RelAuntUncle, 0, 0},
		{"the other is a great-aunt or uncle", 3, 1, RelAuntUncle, 0, 0},
		{"the other is a niece or nephew", 1, 2, RelNieceNephew, 0, 0},
		{"the other is a great-niece or nephew", 1, 3, RelNieceNephew, 0, 0},
		{"first cousins", 2, 2, RelCousin, 1, 0},
		{"first cousins once removed (the other is younger)", 2, 3, RelCousin, 1, 1},
		{"first cousins once removed (the other is older)", 3, 2, RelCousin, 1, 1},
		{"second cousins", 3, 3, RelCousin, 2, 0},
		{"second cousins twice removed", 3, 5, RelCousin, 2, 2},
		{"tenth cousins", 11, 11, RelCousin, 10, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := Classify(tc.person, tc.other)
			if k.Relation != tc.relation || k.StepsPerson != tc.person || k.StepsOther != tc.other {
				t.Fatalf("Classify(%d, %d) = %+v, want relation %q and the same steps", tc.person, tc.other, k, tc.relation)
			}
			if tc.relation != RelCousin {
				if k.Degree != nil || k.Removed != nil {
					t.Fatalf("degree and removed are for cousins only, got %v and %v", k.Degree, k.Removed)
				}
				return
			}
			if k.Degree == nil || k.Removed == nil || *k.Degree != tc.degree || *k.Removed != tc.removed {
				t.Fatalf("degree/removed = %v/%v, want %d/%d", k.Degree, k.Removed, tc.degree, tc.removed)
			}
		})
	}
}

// Asking the question the other way round must turn each direction into its
// opposite and leave the symmetric relations alone.
func TestClassifyIsAntisymmetric(t *testing.T) {
	opposite := map[string]string{
		RelSelf: RelSelf, RelSibling: RelSibling, RelCousin: RelCousin,
		RelAncestor: RelDescendant, RelDescendant: RelAncestor,
		RelAuntUncle: RelNieceNephew, RelNieceNephew: RelAuntUncle,
	}
	for a := 0; a <= 6; a++ {
		for b := 0; b <= 6; b++ {
			forward, back := Classify(a, b), Classify(b, a)
			if back.Relation != opposite[forward.Relation] {
				t.Errorf("(%d,%d) is %s but (%d,%d) is %s", a, b, forward.Relation, b, a, back.Relation)
			}
		}
	}
}
