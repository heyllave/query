package parser

import "testing"

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
