package shape

import "testing"

// TestIsArithmetic_TheGrammarPageSteps holds IsArithmetic to the Grammar
// page's table of how an unquoted value gets its type: only step 4 values
// are arithmetic, and every other step's examples are not.
func TestIsArithmetic_TheGrammarPageSteps(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		// Step 1: a complete date, alone or with a time. A plain date goes the
		// arithmetic path, where lexNumericLiteral takes all ten characters
		// as one date primary; a date with a time is read as one value.
		{"2026-11-16", true},
		{"2026-11-16T10:00:00Z", false},
		{"2026-11-16T10:00:00-05:00", false},
		// Step 2: a word makes the value text.
		{"10-x", false}, {"pr-10-3", false}, {"5+3x", false}, {"1e5", false},
		{"7days", false}, {"a/b", false}, {"550e8400-e29b-41d4", false}, {"check*", false},
		// Step 3: a number with more than one dot.
		{"1.2.3", false}, {"1.2.3-4", false}, {"10.0.0.1/24", false},
		// Step 4: an operator with an operand on both sides.
		{"10-3", true}, {"10/2", true}, {"1+1", true}, {"2*3", true}, {"10%3", true},
		{"7d-1h", true}, {"30m-1h", true}, {"10*-3", true}, {"2026-11-5", true},
		{"2026-11-16T10:00:00Z-1h", true}, {"2026-11-16T10:00:00-05:00+30m", true},
		{"2*[unit-price]", true}, {`1+"a"`, true},
		// A group, a field reference or a call at the start.
		{"(1+2)*3", true}, {"[base]*1.1", true}, {"now()-7d", true}, {"now()", true},
		// Steps 5 to 9: one primary, a pattern, or text.
		{"true", false}, {"7d", false}, {"42", false}, {"-5", false}, {"1.5", false},
		{"10*", false}, {"*0%", false}, {"5-", false}, {"10%", false}, {"1h30m", false},
		// A call left open is not arithmetic. A group at the start always is,
		// open or not; the parser reports the missing ')'.
		{"1+now(", false}, {"(1+2", true},
		// The run ends at a space, ')', ',' or '..'.
		{"1 +2", false}, {"1..2+3", false}, {"1,2+3", false},
	} {
		if got := IsArithmetic(tc.value, 0); got != tc.want {
			t.Errorf("IsArithmetic(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestIsArithmetic_StartsAtPos(t *testing.T) {
	if !IsArithmetic("f=10-3", 2) {
		t.Error("f=10-3 from 2 is not arithmetic")
	}
	if IsArithmetic("f=10-3", 6) {
		t.Error("a position past the end is arithmetic")
	}
}

func TestPrimaryEdges(t *testing.T) {
	for _, tc := range []struct {
		s     string
		i     int
		ends  bool
		begin bool
	}{
		{"7", 0, true, true},
		{"7d", 1, true, false},
		{"d", 0, false, false},
		{")", 0, true, false},
		{"]", 0, true, false},
		{"(", 0, false, true},
		{"[", 0, false, true},
		{`"`, 0, false, true},
		{"-3", 0, false, true},
		{"-x", 0, false, false},
		{"now()", 0, false, true},
		{"now", 0, false, false},
	} {
		if got := EndsPrimary(tc.s, tc.i); got != tc.ends {
			t.Errorf("EndsPrimary(%q, %d) = %v, want %v", tc.s, tc.i, got, tc.ends)
		}
		if got := BeginsPrimary(tc.s, tc.i); got != tc.begin {
			t.Errorf("BeginsPrimary(%q, %d) = %v, want %v", tc.s, tc.i, got, tc.begin)
		}
	}
}

func TestOperandsAround(t *testing.T) {
	for s, want := range map[string]bool{"1+2": true, "a*b": false, "+2": false, "1+": false, "1)*(2": true, "7d-1h": true} {
		i := 1
		if s == "+2" {
			i = 0
		}
		if s == "1)*(2" {
			i = 2
		}
		if s == "7d-1h" {
			i = 2
		}
		if got := OperandsAround(s, i); got != want {
			t.Errorf("OperandsAround(%q, %d) = %v, want %v", s, i, got, want)
		}
	}
}
