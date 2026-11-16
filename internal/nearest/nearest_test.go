package nearest

import "testing"

func TestMatch_Suggestions(t *testing.T) {
	fields := []string{"name", "state", "total", "created_at", "replicas", "labels"}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"missing letter", "totl", "total"},
		{"swapped letters", "anme", "name"},
		{"wrong case", "Total", "total"},
		{"wrong case, long", "CREATED_AT", "created_at"},
		{"two edits on a long name", "replcas", "replicas"},
		{"extra letter", "statee", "state"},
		{"exact match is not a suggestion", "total", ""},
		{"too far", "price", ""},
		{"short names need an exact letter match", "ab", ""},
		{"empty", "", ""},
		{"unrelated word", "urgent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Match(tt.in, fields)

			// Assert
			if got != tt.want {
				t.Errorf("Match(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMatch_TiesDoNotDependOnOrder(t *testing.T) {
	// Arrange
	forward := []string{"cat", "bat"}
	backward := []string{"bat", "cat"}

	// Act
	a, b := Match("hat", forward), Match("hat", backward)

	// Assert
	if a != b || a != "bat" {
		t.Errorf("Match gave %q and %q, want %q for both", a, b, "bat")
	}
}

func TestMatch_NoCandidates(t *testing.T) {
	if got := Match("total", nil); got != "" {
		t.Errorf("Match with no candidates = %q, want empty", got)
	}
}
