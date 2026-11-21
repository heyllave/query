package parser

import (
	"errors"
	"strings"
	"testing"
)

// TestParse_GluedValue pins the error for a value glued to the next term:
// where it points, what it names, and which hint it carries. Each of these
// read as two conditions under the default grammar before.
func TestParse_GluedValue(t *testing.T) {
	const duration = "a duration is a number and one unit"
	const general = "put an operator between two values"
	tests := []struct {
		name, input, names, hint string
		at                       int
	}{
		{"word after a call", "a=now()x", "x", general, 7},
		{"keyword is named whole", "a=now()NOT x=1", "NOT", general, 7},
		{"word after a field reference", "a=[b]c", "c", general, 5},
		{"field reference after a date", "a=2026-11-16[b]", "[", general, 12},
		{"count after a duration", "at>now()-1h30m", "30m", duration, 11},
		{"word after a number", "a=now()-7x", "x", duration, 9},
		{"field reference after a number", "a=10[b]", "[", general, 4},
		{"field reference after text", "a=abc[b]", "[", general, 5},
		{"field reference after a pattern", "a=ab*[b]", "[", general, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseWith(tt.input, Config{})

			var list ErrorList
			if !errors.As(err, &list) || len(list) == 0 {
				t.Fatalf("%q parsed; want an error at %d", tt.input, tt.at)
			}
			e := list[0]
			if e.Code != CodeUnexpected || e.Position.Offset != tt.at {
				t.Fatalf("got %s at %d (%s), want %s at %d", e.Code, e.Position.Offset, e.Message, CodeUnexpected, tt.at)
			}
			if got := tt.input[e.Position.Offset : e.Position.Offset+e.Position.Length]; got != tt.names {
				t.Errorf("error names %q, want %q", got, tt.names)
			}
			if !strings.HasPrefix(e.Hint, tt.hint) {
				t.Errorf("hint %q, want one starting %q", e.Hint, tt.hint)
			}
		})
	}
}

// TestParse_GluedValueStillJoins pins what a glued term does not refuse.
func TestParse_GluedValueStillJoins(t *testing.T) {
	for _, q := range []string{"total>[base]AND x=1", "a=now() x", "a=10 [b]=1", "a=x AND [b]=1", "labels.team=x"} {
		t.Run(q, func(t *testing.T) {
			if _, err := ParseWith(q, Config{}); err != nil {
				t.Fatalf("%q: %v", q, err)
			}
		})
	}
}
