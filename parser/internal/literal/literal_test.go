package literal

import (
	"errors"
	"testing"
	"time"
)

func TestIsInteger(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"lone minus", "-", false},
		{"positive", "+42", true},
		{"has dot", "4.2", false},
		{"has letter", "4a", false},
		{"negative", "-10", true},
		{"plain", "42", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInteger(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFloat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"lone minus", "-", false},
		{"no dot", "42", false},
		{"two dots", "4.2.3", false},
		{"bad chars", "4a.5", false},
		{"plus float", "+4.2", true},
		{"negative float", "-4.2", true},
		{"plain float", "4.2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFloat(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid", "2026-01-01", true},
		{"empty", "", false},
		{"short", "2026-01-0", false},
		{"wrong separator", "2026/01/01", false},
		{"non-digit year", "abcd-01-01", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDate(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidWildcard(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"empty", "", true},
		{"plain", "plain", true},
		{"just star", "*", true},
		{"prefix", "a*", true},
		{"suffix", "*a", true},
		{"contains", "*a*", true},
		{"middle star", "a*b", false},
		{"multi stars", "a*b*c", false},
		{"four stars", "****", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidWildcard(tt.pattern); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDatetimeEnd(t *testing.T) {
	for s, want := range map[string]int{
		"2026-11-16T10:00:00Z-1h":      20,
		"2026-11-16T10:00:00+02:00-1h": 25,
		"2026-11-16T10:00:00-05:00":    25,
		"2026-11-16T10:00:00.5Z":       22,
		"2026-11-16T10:00:00-1h":       19,
		"2026-11-16T10:00":             -1,
		"2026-11-16 10:00:00":          -1,
		"2026-11-16":                   -1,
		"x":                            -1,
	} {
		if got := DatetimeEnd(s, 0); got != want {
			t.Errorf("DatetimeEnd(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestIsDatetime(t *testing.T) {
	for s, want := range map[string]bool{
		"2026-11-16T10:00:00Z": true, "2026-11-16 10:00:00": true, "2026-11-16t10:00:00z": true,
		"2026-11-16T10:00": false, "2026-11-16Tx0:00:00": false, "2026-11-16": false,
	} {
		if got := IsDatetime(s); got != want {
			t.Errorf("IsDatetime(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIsDatePrefix(t *testing.T) {
	for s, want := range map[string]bool{"2026": true, "2026-11": true, "2026-1": false, "202": false, "abcd": false} {
		if got := IsDatePrefix(s); got != want {
			t.Errorf("IsDatePrefix(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIsDuration(t *testing.T) {
	for s, want := range map[string]bool{"7d": true, "30m": true, "2w": true, "4h": true, "d": false, "7s": false, "1.5h": false, "1h30m": false} {
		if got := IsDuration(s); got != want {
			t.Errorf("IsDuration(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want time.Duration
		err  error
	}{
		{"30m", 30 * time.Minute, nil},
		{"2w", 14 * 24 * time.Hour, nil},
		{"d", 0, ErrDurationSyntax},
		{"7s", 0, ErrDurationUnit},
		{"1xh", 0, ErrDurationSyntax},
		{"99999999999999999999d", 0, ErrDurationRange},
	} {
		got, err := ParseDuration(tc.s)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v, %v", tc.s, got, err, tc.want, tc.err)
		}
	}
}

func TestIsFloat_EitherSideOfTheDotMayBeEmpty(t *testing.T) {
	for s, want := range map[string]bool{".5": true, "5.": true, ".": true, "-.": true, "": false, "-": false} {
		if got := IsFloat(s); got != want {
			t.Errorf("IsFloat(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestEncodePattern(t *testing.T) {
	for _, tc := range []struct {
		value string
		stars []int
		want  string
	}{
		{"a*", nil, "a*"},
		{"a*b", []int{1}, `a\*b`},
		{`a\b*`, nil, `a\\b*`},
	} {
		if got := EncodePattern(tc.value, tc.stars); got != tc.want {
			t.Errorf("EncodePattern(%q, %v) = %q, want %q", tc.value, tc.stars, got, tc.want)
		}
	}
}
