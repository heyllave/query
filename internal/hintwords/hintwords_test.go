package hintwords

import "testing"

func TestSQL_Words(t *testing.T) {
	tests := []struct {
		name string
		word string
		want bool
	}{
		{"upper case", "LIKE", true},
		{"lower case", "between", true},
		{"mixed case", "Null", true},
		{"second word of order by", "by", true},
		{"negation", "NOT", true},
		{"ordinary field", "total", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			hint, ok := SQL(tt.word)

			// Assert
			if ok != tt.want || (ok && hint == "") {
				t.Errorf("SQL(%q) = %q, %v; want found=%v with a hint", tt.word, hint, ok, tt.want)
			}
		})
	}
}

func TestIsMissingTest_Words(t *testing.T) {
	for word, want := range map[string]bool{"IS": true, "null": true, "NOT": false, "name": false} {
		if got := IsMissingTest(word); got != want {
			t.Errorf("IsMissingTest(%q) = %v, want %v", word, got, want)
		}
	}
}

func TestDurationUnit_Words(t *testing.T) {
	tests := []struct {
		word string
		unit string
		ok   bool
	}{
		{"days", "d", true},
		{"Hour", "h", true},
		{"mins", "m", true},
		{"weeks", "w", true},
		{"months", "", false},
		{"d", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			// Act
			unit, ok := DurationUnit(tt.word)

			// Assert
			if unit != tt.unit || ok != tt.ok {
				t.Errorf("DurationUnit(%q) = %q, %v; want %q, %v", tt.word, unit, ok, tt.unit, tt.ok)
			}
		})
	}
}

func TestIsNumber_Shapes(t *testing.T) {
	for s, want := range map[string]bool{"42": true, "1.5": true, "": false, "-3": false, "1e3": false, "a1": false} {
		if got := IsNumber(s); got != want {
			t.Errorf("IsNumber(%q) = %v, want %v", s, got, want)
		}
	}
}
