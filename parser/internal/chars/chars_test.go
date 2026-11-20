package chars

import "testing"

func TestClasses(t *testing.T) {
	for _, tc := range []struct {
		c                      byte
		digit, start, cont, op bool
	}{
		{'7', true, false, true, false},
		{'a', false, true, true, false},
		{'_', false, true, true, false},
		{'-', false, false, true, true},
		{'/', false, false, true, true},
		{'+', false, false, false, true},
		{'*', false, false, false, true},
		{'%', false, false, false, true},
		{'.', false, false, false, false},
		{0xC3, false, true, true, false}, // the first byte of é
	} {
		if IsDigit(tc.c) != tc.digit || IsIdentStart(tc.c) != tc.start || IsIdentChar(tc.c) != tc.cont || IsArithOp(tc.c) != tc.op {
			t.Errorf("%q: digit=%v start=%v cont=%v op=%v", tc.c, IsDigit(tc.c), IsIdentStart(tc.c), IsIdentChar(tc.c), IsArithOp(tc.c))
		}
	}
}

func TestSpaceWidth(t *testing.T) {
	for s, want := range map[string]int{" ": 1, "\t": 1, " ": 2, "　": 3, "a": 0, "é": 0} {
		if got := SpaceWidth(s, 0); got != want {
			t.Errorf("SpaceWidth(%q) = %d, want %d", s, got, want)
		}
	}
	if got := SkipSpace("   x", 0); got != 4 {
		t.Errorf("SkipSpace = %d, want 4", got)
	}
}

func TestIdentEnd(t *testing.T) {
	for s, want := range map[string]int{"abc=1": 3, "a-b/c.d": 5, "café x": 5, "日本": 6, "a b": 1, "=": 0} {
		if got := IdentEnd(s, 0); got != want {
			t.Errorf("IdentEnd(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestIsCallAt(t *testing.T) {
	for s, want := range map[string]bool{"now()": true, "now ()": false, "now": false, "(x)": false, "a-b(": true} {
		if got := IsCallAt(s, 0); got != want {
			t.Errorf("IsCallAt(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestSkipCallAndFieldRef(t *testing.T) {
	for _, tc := range []struct {
		s    string
		call int
	}{
		{"(a)", 3}, {"(a(b))x", 6}, {`(")")`, 5}, {`("\")")x`, 7}, {"(a", -1},
	} {
		if got := SkipCall(tc.s, 0); got != tc.call {
			t.Errorf("SkipCall(%q) = %d, want %d", tc.s, got, tc.call)
		}
	}
	for s, want := range map[string]int{"[a]": 2, `["]"]`: 4, "[a": -1} {
		if got := SkipFieldRef(s, 0); got != want {
			t.Errorf("SkipFieldRef(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestReadQuoted(t *testing.T) {
	for _, tc := range []struct {
		s, value string
		end      int
		closed   bool
	}{
		{`"a b"x`, "a b", 5, true},
		{`"a\"b"`, `a"b`, 6, true},
		{`"\n\t\r\\\q"`, "\n\t\r\\q", 12, true},
		{`"open`, "open", 5, false},
		{`"trailing\`, `trailing\`, 10, false},
	} {
		value, end, closed := ReadQuoted(tc.s, 0)
		if value != tc.value || end != tc.end || closed != tc.closed {
			t.Errorf("ReadQuoted(%q) = %q, %d, %v; want %q, %d, %v", tc.s, value, end, closed, tc.value, tc.end, tc.closed)
		}
	}
}

func TestAt(t *testing.T) {
	if At("ab", 1) != 'b' || At("ab", 2) != 0 || At("ab", -1) != 0 {
		t.Error("At does not bound its index")
	}
}
