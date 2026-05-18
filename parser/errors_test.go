package parser

import (
	"errors"
	"testing"

	"github.com/heyllave/query/token"
)

func TestErrorKind_String(t *testing.T) {
	tests := []struct {
		name string
		kind ErrorKind
		want string
	}{
		{"syntax", ErrSyntax, "syntax error"},
		{"unexpected token", ErrUnexpectedToken, "unexpected token"},
		{"unexpected eof", ErrUnexpectedEOF, "unexpected end of input"},
		{"invalid value", ErrInvalidValue, "invalid value"},
		{"query too long", ErrQueryTooLong, "query too long"},
		{"invalid wildcard", ErrInvalidWildcard, "invalid wildcard"},
		{"invalid date", ErrInvalidDate, "invalid date"},
		{"invalid duration", ErrInvalidDuration, "invalid duration"},
		{"unknown", ErrorKind(9999), "ErrorKind(9999)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	e := &Error{Message: "something failed", Position: token.Position{Offset: 5}}
	want := "position 5: something failed"
	if got := e.Error(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestErrorList_Error(t *testing.T) {
	tests := []struct {
		name string
		list ErrorList
		want string
	}{
		{"empty", ErrorList{}, "no errors"},
		{
			name: "single",
			list: ErrorList{{Message: "bad", Position: token.Position{Offset: 0}}},
			want: "position 0: bad",
		},
		{
			name: "multiple",
			list: ErrorList{
				{Message: "first", Position: token.Position{Offset: 0}},
				{Message: "second", Position: token.Position{Offset: 5}},
			},
			want: "position 0: first; position 5: second",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.list.Error(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorList_Unwrap(t *testing.T) {
	el := ErrorList{{Message: "first"}, {Message: "second"}}
	errs := el.Unwrap()
	if len(errs) != 2 {
		t.Errorf("got %d, want 2", len(errs))
	}
}

func TestIsParseError(t *testing.T) {
	parseErr, _ := (func() (error, error) {
		_, err := Parse("=invalid", 0)
		return err, nil
	})()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"parse error", parseErr, true},
		{"nil", nil, false},
		{"plain error", errors.New("other"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsParseError(tt.err); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	listErr, _ := (func() (error, error) {
		_, err := Parse("=invalid", 0)
		return err, nil
	})()
	singleErr := &Error{Message: "x"}

	tests := []struct {
		name    string
		err     error
		wantLen int
	}{
		{"nil", nil, 0},
		{"plain error", errors.New("x"), 0},
		{"single *Error", singleErr, 1},
		{"ErrorList from parse", listErr, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Errors(tt.err)
			if len(got) != tt.wantLen {
				t.Errorf("got %d errors, want %d", len(got), tt.wantLen)
			}
		})
	}
}
