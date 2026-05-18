package query_test

import (
	"strings"
	"testing"

	"github.com/heyllave/query"
	"github.com/heyllave/query/validate"
)

var testFields = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		opts    []query.Option
		wantErr bool
	}{
		{"simple parse", "state=draft", nil, false},
		{"max length exceeded", "state=draft", []query.Option{query.WithMaxLength(5)}, true},
		{"max length sufficient", "state=draft", []query.Option{query.WithMaxLength(100)}, false},
		{"max length disabled", strings.Repeat("a", 1000) + "=x", []query.Option{query.WithMaxLength(0)}, false},
		{"default exceeds 256", strings.Repeat("a", 300) + "=x", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := query.Parse(tt.input, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid query", "state=draft AND total>50000", false},
		{"unknown field", "nonexistent=value", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := query.Parse(tt.input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := query.Validate(expr, testFields); (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestParseAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "state=draft", false},
		{"parse error", "=invalid", true},
		{"validation error", "nonexistent=value", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := query.ParseAndValidate(tt.input, testFields)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestDefaultMaxLength(t *testing.T) {
	if query.DefaultMaxLength != 256 {
		t.Errorf("DefaultMaxLength = %d, want 256", query.DefaultMaxLength)
	}
}
