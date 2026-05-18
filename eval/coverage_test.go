package eval

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
	"github.com/heyllave/query/validate"
)

// TestQualifiers_Extract ensures ast.Qualifiers is exercised.
func TestQualifiers_Extract(t *testing.T) {
	prog, err := Compile("state=draft AND year>2020", testFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	quals := ast.Qualifiers(prog.AST())
	if len(quals) != 2 {
		t.Errorf("got %d, want 2", len(quals))
	}
}

// TestValidatePresence_Errors exercises validate presence error paths.
func TestValidatePresence_Errors(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		fields []validate.FieldConfig
	}{
		{
			name:   "unknown field",
			query:  "unknown_field",
			fields: []validate.FieldConfig{},
		},
		{
			name:  "presence not allowed",
			query: "year",
			fields: []validate.FieldConfig{
				{Name: "year", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.query, tt.fields)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

// TestValidateFuncCallFields_Errors exercises function call field validation.
func TestValidateFuncCallFields_Errors(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		fields []validate.FieldConfig
	}{
		{
			name:  "unknown field in func arg",
			query: "lower(unknown)=x",
			fields: []validate.FieldConfig{
				{Name: "known", Type: validate.TypeText, AllowedOps: validate.TextOps},
			},
		},
		{
			name:  "unknown field in standalone call",
			query: "contains(unknown, other)",
			fields: []validate.FieldConfig{
				{Name: "other", Type: validate.TypeText, AllowedOps: validate.TextOps},
			},
		},
		{
			name:  "unknown field in nested call",
			query: "int(year(unknown))=2026",
			fields: []validate.FieldConfig{
				{Name: "known", Type: validate.TypeText, AllowedOps: validate.TextOps},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.query, tt.fields)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

// TestSelectorPath covers the Selector AST branch in validate and matcher.
func TestSelectorPath(t *testing.T) {
	// @first on a known list field.
	first := &ast.SelectorExpr{
		Base:     &ast.PresenceExpr{Field: ast.FieldPath{"items"}},
		Selector: "first",
	}
	m := compileMatcher(first, BuiltinFunctions())
	got := m(func(f string) (any, bool) {
		if f == "items" {
			return []any{"a"}, true
		}
		return nil, false
	})
	if !got {
		t.Error("expected @first match on non-empty slice")
	}

	// @(inner) against a slice of maps.
	inner := &ast.SelectorExpr{
		Base: &ast.PresenceExpr{Field: ast.FieldPath{"items"}},
		Inner: &ast.QualifierExpr{
			Field:    ast.FieldPath{"name"},
			Operator: token.Eq,
			Value:    ast.Value{Type: ast.ValueString, Str: "draft"},
		},
	}
	m = compileMatcher(inner, BuiltinFunctions())
	got = m(func(f string) (any, bool) {
		if f == "items" {
			return []any{map[string]any{"name": "draft"}}, true
		}
		return nil, false
	})
	if !got {
		t.Error("expected @(inner) match")
	}

	// Validator accepts selectors whose base field exists without OpPresence.
	v := validate.New([]validate.FieldConfig{
		{Name: "items", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	})
	if err := v.Validate(inner); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
