package parser

import "testing"

func TestIsIntegerLiteral(t *testing.T) {
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
			if got := isIntegerLiteral(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFloatLiteral(t *testing.T) {
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
			if got := isFloatLiteral(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDateLiteral(t *testing.T) {
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
			if got := isDateLiteral(tt.input); got != tt.want {
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
			if got := isValidWildcard(tt.pattern); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParse_ErrorPaths(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"EOF after operator", "state="},
		{"unclosed paren", "(state=draft"},
		{"missing right operand", "state=draft AND"},
		{"missing range start", "x:"},
		{"missing range separator", "x:1"},
		{"missing range end", "x:1.."},
		{"dot without field", "labels.=jane"},
		{"extra token", "state=draft )"},
		{"or missing right", "a=1 OR"},
		{"not missing expr", "NOT ("},
		{"func leading comma", "f(,a)"},
		{"func eof mid-args", "f(a,"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input, 0); err == nil {
				t.Errorf("expected error for %q", tt.input)
			}
		})
	}
}

func TestParse_TooLong(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := Parse(string(long), 256); err == nil {
		t.Error("expected length error")
	}
}

func TestParse_FuncCallEmpty(t *testing.T) {
	if _, err := Parse("now()", 0); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
