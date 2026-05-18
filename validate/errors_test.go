package validate

import (
	"errors"
	"testing"

	"github.com/heyllave/query/token"
)

func TestError_Error(t *testing.T) {
	e := &Error{Message: "bad field", Position: token.Position{Offset: 3}}
	want := "position 3: bad field"
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
			list: ErrorList{{Message: "x", Position: token.Position{Offset: 0}}},
			want: "position 0: x",
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
	el := ErrorList{{Message: "a"}, {Message: "b"}}
	errs := el.Unwrap()
	if len(errs) != 2 {
		t.Errorf("got %d, want 2", len(errs))
	}
	var e *Error
	combined := error(el)
	if !errors.As(combined, &e) {
		t.Error("errors.As failed")
	}
}

func TestFieldValueType_String(t *testing.T) {
	tests := []struct {
		name string
		t    FieldValueType
		want string
	}{
		{"text", TypeText, "text"},
		{"integer", TypeInteger, "integer"},
		{"decimal", TypeDecimal, "decimal"},
		{"boolean", TypeBoolean, "boolean"},
		{"date", TypeDate, "date"},
		{"datetime", TypeDatetime, "datetime"},
		{"duration", TypeDuration, "duration"},
		{"unknown", FieldValueType(9999), "FieldValueType(9999)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
