package person

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name    string
		q       string
		want    []string
		invalid bool
	}{
		{name: "one word", q: "ada", want: []string{"ada"}},
		{name: "trimmed and split on any whitespace", q: "  ada \t love\n", want: []string{"ada", "love"}},
		{name: "wildcards are just characters here", q: "100%_\\", want: []string{"100%_\\"}},
		{name: "unicode", q: "Zoë Müller", want: []string{"Zoë", "Müller"}},
		{name: "five words allowed", q: "a b c d e", want: []string{"a", "b", "c", "d", "e"}},
		{name: "six words rejected", q: "a b c d e f", invalid: true},
		{name: "NUL byte", q: "ad\x00a", invalid: true},
		{name: "invalid UTF-8", q: "ad\xffa", invalid: true},
		{name: "empty", q: "", invalid: true},
		{name: "blank", q: " \t ", invalid: true},
		{name: "100 characters allowed", q: strings.Repeat("é", MaxQueryLen), want: []string{strings.Repeat("é", MaxQueryLen)}},
		{name: "101 characters rejected", q: strings.Repeat("é", MaxQueryLen+1), invalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseQuery(tc.q)
			if tc.invalid {
				var ae *apperr.Error
				if !errors.As(err, &ae) || ae.Kind != apperr.KindInvalid || ae.Code != "validation_failed" {
					t.Fatalf("got %v, want a validation_failed error", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEscapeLike(t *testing.T) {
	tests := map[string]string{
		"plain":   "plain",
		"100%":    `100\%`,
		"a_b":     `a\_b`,
		`back\sl`: `back\\sl`,
		`%_\`:     `\%\_\\`,
	}
	for in, want := range tests {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
