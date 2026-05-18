package eval

import (
	"errors"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/validate"
)

// tenantValidator is a test AstValidator that blocks a denylist of fields
// and rejects any query mentioning the literal string "secret".
type tenantValidator struct {
	blocked map[string]bool
	static  map[string]validate.FieldConfig
}

func (t *tenantValidator) GetFieldConfig(name string) (validate.FieldConfig, bool) {
	if t.blocked[name] {
		return validate.FieldConfig{}, false
	}
	cfg, ok := t.static[name]
	return cfg, ok
}

func (t *tenantValidator) ValidateCustomRules(node ast.Expression) error {
	var rejected bool
	ast.Walk(node, func(e ast.Expression) bool {
		q, ok := e.(*ast.QualifierExpr)
		if !ok {
			return true
		}
		if q.Value.Type == ast.ValueString && q.Value.Str == "secret" {
			rejected = true
		}
		return true
	})
	if rejected {
		return errors.New("queries referencing 'secret' are not allowed")
	}
	return nil
}

func TestWithCustomValidator(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		{Name: "notes", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	cv := &tenantValidator{
		blocked: map[string]bool{"total": true},
		static: map[string]validate.FieldConfig{
			"state": fields[0],
			"notes": fields[2],
		},
	}

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"allowed field + clean value", "state=draft", false},
		{"blocked field rejected", "total>100", true},
		{"custom rule rejects secret", "notes=secret", true},
		{"unrelated value passes", "notes=hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.query, fields, WithCustomValidator(cv))
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestWithCustomValidator_MatchStillWorks(t *testing.T) {
	// A compiled program with a custom validator is still matchable.
	fields := []validate.FieldConfig{
		{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	cv := &tenantValidator{
		static: map[string]validate.FieldConfig{"state": fields[0]},
	}
	prog, err := Compile("state=draft", fields, WithCustomValidator(cv))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"state": "draft"}) {
		t.Error("expected match")
	}
	if prog.Match(map[string]any{"state": "issued"}) {
		t.Error("expected no match")
	}
}

func TestWithMaxLength(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}

	tests := []struct {
		name    string
		maxLen  int
		wantErr bool
	}{
		{"exceeds limit", 5, true},
		{"within limit", 100, false},
		{"disabled", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile("state=draft", fields, WithMaxLength(tt.maxLen))
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestWithMaxDepth(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "a", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
		{Name: "b", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
		{Name: "c", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	}
	tests := []struct {
		name    string
		query   string
		max     int
		wantErr bool
	}{
		{"depth 1 allowed", "a=1", 1, false},
		{"depth exceeds limit", "(a=1 OR b=2) AND c=3", 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.query, fields, WithMaxDepth(tt.max))
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestCompileArgResolvers_NestedCall(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	prog, err := Compile("lower(upper(name))=john", fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tests := []struct {
		name  string
		input string
	}{
		{"uppercase input", "JOHN"},
		{"mixed case", "John"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !prog.Match(map[string]any{"name": tt.input}) {
				t.Error("expected match")
			}
		})
	}
}

func TestCompileArgResolvers_MissingFunc(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	_, err := Compile("lower(nonexistent(name))=john", fields,
		WithNoBuiltins(),
		WithFunctions(Func{
			Name: "lower",
			Call: func(args ...any) (any, error) {
				if args[0] == nil {
					return "", nil
				}
				return args[0], nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
}
