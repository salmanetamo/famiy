package relationship

import (
	"errors"
	"testing"
)

func TestParentLinkValidate(t *testing.T) {
	tests := []struct {
		name string
		link ParentLink
		want error
	}{
		{"distinct people", ParentLink{ParentID: 1, ChildID: 2}, nil},
		{"self parent", ParentLink{ParentID: 3, ChildID: 3}, ErrSelfParent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.link.Validate(); !errors.Is(got, tc.want) {
				t.Fatalf("Validate() = %v, want %v", got, tc.want)
			}
		})
	}
}
