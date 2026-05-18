package validate

import (
	"errors"
	"fmt"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

var testFields = []FieldConfig{
	{Name: "state", Type: TypeText, AllowedOps: TextOps},
	{Name: "name", Type: TypeText, AllowedOps: TextOps},
	{Name: "description", Type: TypeText, AllowedOps: TextOps},
	{Name: "year", Type: TypeInteger, AllowedOps: NumericOps},
	{Name: "total", Type: TypeDecimal, AllowedOps: NumericOps},
	{Name: "active", Type: TypeBoolean, AllowedOps: BoolOps},
	{Name: "created_at", Type: TypeDate, AllowedOps: DateOps},
	{Name: "ttl", Type: TypeDuration, AllowedOps: DurationOps, Nested: true},
	{Name: "labels", Type: TypeText, AllowedOps: TextOps, Nested: true},
	{Name: "cluster", Type: TypeText, AllowedOps: TextOps},
	{Name: "customer_id", Type: TypeText, AllowedOps: TextOps},
	{Name: "tire_size", Type: TypeText, AllowedOps: append(TextOps, OpPresence)},
	{Name: "make", Type: TypeText, AllowedOps: TextOps},
	{Name: "offset", Type: TypeInteger, AllowedOps: NumericOps},
}

func TestValidate_ValidQueries(t *testing.T) {
	valid := []string{
		"state=draft",
		"state!=cancelled",
		"year>2020",
		"total>=50000",
		"total>=50000.50",
		"active=true",
		"created_at>=2026-01-01",
		"name=John*",
		"make=*yota",
		"description=*testing*",
		"state=draft AND customer_id=customer_john-doe",
		"(state=draft OR state=issued) AND total>50000",
		"NOT state=cancelled",
		"(labels.dev=jane OR labels.env=bar) AND NOT cluster=demo",
		"created_at:2026-01-01..2026-03-31",
		"ttl.duration>1d",
		"labels.dev=jane",
		"offset>=-10",
	}
	for _, q := range valid {
		t.Run(q, func(t *testing.T) {
			expr, err := parser.Parse(q, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			v := New(testFields)
			if err := v.Validate(expr); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidate_UnknownField(t *testing.T) {
	expr, _ := parser.Parse("nonexistent=value", 0)
	v := New(testFields)
	if err := v.Validate(expr); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_OperatorNotAllowed(t *testing.T) {
	bad := []string{
		"state>draft",
		"state>=draft",
		"year=202*",
		"active>true",
	}
	for _, q := range bad {
		t.Run(q, func(t *testing.T) {
			expr, _ := parser.Parse(q, 0)
			v := New(testFields)
			if err := v.Validate(expr); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidate_TypeMismatch(t *testing.T) {
	bad := []string{
		"year=notanumber",
		"active=maybe",
		"created_at>=notadate",
	}
	for _, q := range bad {
		t.Run(q, func(t *testing.T) {
			expr, _ := parser.Parse(q, 0)
			v := New(testFields)
			if err := v.Validate(expr); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidate_PresenceAllowed(t *testing.T) {
	expr, _ := parser.Parse("tire_size", 0)
	v := New(testFields)
	if err := v.Validate(expr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_PresenceNotAllowed(t *testing.T) {
	expr, _ := parser.Parse("active", 0)
	v := New(testFields)
	if err := v.Validate(expr); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	expr, _ := parser.Parse("unknown_field=x AND year=notanum", 0)
	v := New(testFields)
	err := v.Validate(expr)
	if err == nil {
		t.Fatal("expected errors")
	}
	var el ErrorList
	if !errors.As(err, &el) {
		t.Fatalf("expected ErrorList, got %T", err)
	}
	if len(el) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(el))
	}
}

func TestValidate_EmptyConfig(t *testing.T) {
	expr, _ := parser.Parse("state=draft", 0)
	v := New(nil)
	if err := v.Validate(expr); err == nil {
		t.Fatal("expected error")
	}
}

// fakeAstValidator is a configurable [AstValidator] used by the tests below.
type fakeAstValidator struct {
	lookup func(string) (FieldConfig, bool)
	rules  func(ast.Expression) error
}

func (f *fakeAstValidator) GetFieldConfig(name string) (FieldConfig, bool) {
	if f.lookup == nil {
		return FieldConfig{}, false
	}
	return f.lookup(name)
}

func (f *fakeAstValidator) ValidateCustomRules(node ast.Expression) error {
	if f.rules == nil {
		return nil
	}
	return f.rules(node)
}

// staticLookup returns a GetFieldConfig implementation backed by testFields.
func staticLookup() func(string) (FieldConfig, bool) {
	index := make(map[string]FieldConfig, len(testFields))
	for _, f := range testFields {
		index[f.Name] = f
	}
	return func(name string) (FieldConfig, bool) {
		cfg, ok := index[name]
		return cfg, ok
	}
}

func TestValidate_CustomValidator_TenantFieldAccess(t *testing.T) {
	// Scenario: tenant A can query "total", tenant B cannot. Rule is
	// enforced via GetFieldConfig override returning (_, false) for blocked
	// fields, even though they are declared in the static config.
	blocked := map[string]bool{"total": true, "year": true}
	tenant := &fakeAstValidator{
		lookup: func(name string) (FieldConfig, bool) {
			if blocked[name] {
				return FieldConfig{}, false
			}
			return staticLookup()(name)
		},
	}

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"allowed field permitted", "state=draft", false},
		{"blocked field rejected", "total>50000", true},
		{"blocked field in conjunction", "state=draft AND total>50000", true},
		{"allowed-only disjunction passes", "state=draft OR name=jane", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.query, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			v := New(testFields, WithCustomValidator(tenant))
			err = v.Validate(expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr {
				var el ErrorList
				if !errors.As(err, &el) {
					t.Fatalf("expected ErrorList, got %T", err)
				}
				if el[0].Kind != ErrFieldNotFound {
					t.Errorf("expected ErrFieldNotFound, got %v", el[0].Kind)
				}
			}
		})
	}
}

func TestValidate_CustomValidator_MutuallyExclusiveFields(t *testing.T) {
	// Rule: active=false AND deleted_at IS NULL (proxy here: active=false
	// cannot combine with state=draft). Demonstrates ValidateCustomRules
	// walking the AST with ast.Walk.
	rules := func(node ast.Expression) error {
		var hasInactive, hasDraft bool
		ast.Walk(node, func(e ast.Expression) bool {
			q, ok := e.(*ast.QualifierExpr)
			if !ok {
				return true
			}
			switch q.Field.String() {
			case "active":
				if q.Value.Type == ast.ValueBoolean && !q.Value.Bool {
					hasInactive = true
				}
			case "state":
				if q.Value.Type == ast.ValueString && q.Value.Str == "draft" {
					hasDraft = true
				}
			}
			return true
		})
		if hasInactive && hasDraft {
			return &Error{
				Message:  "active=false cannot combine with state=draft",
				Position: node.Pos(),
				Kind:     ErrCustomRule,
			}
		}
		return nil
	}

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"inactive alone", "active=false", false},
		{"draft alone", "state=draft", false},
		{"forbidden combination", "active=false AND state=draft", true},
		{"forbidden combination reversed", "state=draft AND active=false", true},
		{"inactive with other state", "active=false AND state=issued", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.query, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			v := New(testFields, WithCustomValidator(&fakeAstValidator{
				lookup: staticLookup(),
				rules:  rules,
			}))
			err = v.Validate(expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr {
				var el ErrorList
				if !errors.As(err, &el) {
					t.Fatalf("expected ErrorList, got %T", err)
				}
				if el[len(el)-1].Kind != ErrCustomRule {
					t.Errorf("expected last error to be ErrCustomRule, got %v", el[len(el)-1].Kind)
				}
			}
		})
	}
}

func TestValidate_CustomValidator_ValueRange(t *testing.T) {
	// Rule: "total" must be positive. Returns a plain error so we also
	// exercise the wrapping path in appendCustomErr.
	rules := func(node ast.Expression) error {
		var errs ErrorList
		ast.Walk(node, func(e ast.Expression) bool {
			q, ok := e.(*ast.QualifierExpr)
			if !ok || q.Field.String() != "total" {
				return true
			}
			var n float64
			switch q.Value.Type { //nolint:exhaustive // only numeric
			case ast.ValueInteger:
				n = float64(q.Value.Int)
			case ast.ValueFloat:
				n = q.Value.Float
			default:
				return true
			}
			if n < 0 {
				errs = append(errs, &Error{
					Message:  fmt.Sprintf("total must be positive, got %v", q.Value.Any()),
					Position: q.Position,
					Kind:     ErrCustomRule,
				})
			}
			return true
		})
		if len(errs) == 0 {
			return nil
		}
		return errs
	}

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"positive passes", "total>50000", false},
		{"zero is allowed", "total>=0", false},
		{"negative rejected", "total<-1", true},
		{"negative in conjunction", "state=draft AND total<-1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.query, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			v := New(testFields, WithCustomValidator(&fakeAstValidator{
				lookup: staticLookup(),
				rules:  rules,
			}))
			err = v.Validate(expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidate_CustomValidator_WrapsPlainError(t *testing.T) {
	// When ValidateCustomRules returns a non-structured error, the
	// validator wraps it as a single ErrCustomRule entry at the root.
	cv := &fakeAstValidator{
		lookup: staticLookup(),
		rules:  func(_ ast.Expression) error { return errors.New("boom") },
	}
	expr, _ := parser.Parse("state=draft", 0)
	v := New(testFields, WithCustomValidator(cv))
	err := v.Validate(expr)
	var el ErrorList
	if !errors.As(err, &el) {
		t.Fatalf("expected ErrorList, got %T", err)
	}
	if len(el) != 1 {
		t.Fatalf("expected 1 error, got %d", len(el))
	}
	if el[0].Kind != ErrCustomRule {
		t.Errorf("expected ErrCustomRule, got %v", el[0].Kind)
	}
	if el[0].Message != "boom" {
		t.Errorf("expected message %q, got %q", "boom", el[0].Message)
	}
}

func TestValidate_CustomValidator_CollectsAlongsideBuiltins(t *testing.T) {
	// Built-in errors (unknown field) and custom errors coexist in the
	// same ErrorList.
	cv := &fakeAstValidator{
		lookup: staticLookup(),
		rules:  func(_ ast.Expression) error { return errors.New("policy violation") },
	}
	expr, _ := parser.Parse("nonexistent=value", 0)
	v := New(testFields, WithCustomValidator(cv))
	err := v.Validate(expr)
	var el ErrorList
	if !errors.As(err, &el) {
		t.Fatalf("expected ErrorList, got %T", err)
	}
	if len(el) < 2 {
		t.Fatalf("expected at least 2 errors, got %d: %v", len(el), el)
	}
	var kinds []ErrorKind
	for _, e := range el {
		kinds = append(kinds, e.Kind)
	}
	var hasFieldNotFound, hasCustom bool
	for _, k := range kinds {
		if k == ErrFieldNotFound {
			hasFieldNotFound = true
		}
		if k == ErrCustomRule {
			hasCustom = true
		}
	}
	if !hasFieldNotFound || !hasCustom {
		t.Errorf("expected both ErrFieldNotFound and ErrCustomRule, got %v", kinds)
	}
}

func TestValidate_CustomValidator_OverridesStaticConfig(t *testing.T) {
	// If the static config declares a field but the custom validator's
	// GetFieldConfig returns (_, false), the field must be treated as
	// unknown. This is override semantics.
	cv := &fakeAstValidator{
		lookup: func(_ string) (FieldConfig, bool) { return FieldConfig{}, false },
	}
	expr, _ := parser.Parse("state=draft", 0)
	v := New(testFields, WithCustomValidator(cv))
	err := v.Validate(expr)
	if err == nil {
		t.Fatal("expected error — custom validator should override static config")
	}
}

func TestValidate_CustomValidator_NestedFieldOverride(t *testing.T) {
	// When the static config declares a nested field (labels.*), the
	// custom validator can allow its subpaths by returning the nested
	// config for the top-level segment.
	nestedLookup := func(name string) (FieldConfig, bool) {
		if name == "labels" {
			return FieldConfig{Name: "labels", Type: TypeText, AllowedOps: TextOps, Nested: true}, true
		}
		return FieldConfig{}, false
	}
	cv := &fakeAstValidator{lookup: nestedLookup}
	expr, _ := parser.Parse("labels.env=prod", 0)
	v := New(testFields, WithCustomValidator(cv))
	if err := v.Validate(expr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidate_DynamicValues_BypassTypeCheck — function and arithmetic values
// have opaque return types, so the validator must skip type-compatibility
// against the field config. Field references inside function args and
// arithmetic operands are still validated.
func TestValidate_DynamicValues_BypassTypeCheck(t *testing.T) {
	v := New(testFields)

	// total is TypeDecimal — a plain string value would fail typeCompatible.
	// Wrapping in ValueArith / ValueFunc must bypass that check.
	cases := []struct {
		name string
		q    string
	}{
		{"arith literal", "total>=50000*1.1"},
		{"arith paren precedence", "total>=(50000+1000)*1.1"},
		{"func value", "created_at>=now()"},
		{"func arith mix", "created_at>=now()-7d"},
		{"range with func endpoints", "created_at:daysAgo(30)..now()"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.q, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := v.Validate(expr); err != nil {
				t.Errorf("validate: unexpected error: %v", err)
			}
		})
	}
}

// TestValidate_DynamicValues_StillCatchesFieldErrors — even though the
// surrounding ValueArith / ValueFunc bypasses type checks, field references
// inside their subtrees must still be resolved.
func TestValidate_DynamicValues_StillCatchesFieldErrors(t *testing.T) {
	v := New(testFields)
	expr, err := parser.Parse("created_at>=daysAgo(unknown_field)", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := v.Validate(expr); err == nil {
		t.Fatal("expected validation error for unknown_field inside function arg")
	}
}

// TestValidate_Selector_BaseFieldRequired — every selector kind validates the
// base field reference; the inner expression is also validated recursively.
func TestValidate_Selector_BaseFieldRequired(t *testing.T) {
	fields := append([]FieldConfig{},
		FieldConfig{Name: "orders", Type: TypeText, AllowedOps: TextOps, Nested: true},
		FieldConfig{Name: "status", Type: TypeText, AllowedOps: TextOps},
	)
	v := New(fields)

	valid := []string{
		"orders@first",
		"orders@last",
		"orders@(status=shipped)",
		"orders@any(status=shipped)",
		"orders@all(status=shipped)",
		"orders@none(status=cancelled)",
	}
	for _, q := range valid {
		t.Run("ok/"+q, func(t *testing.T) {
			expr, err := parser.Parse(q, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := v.Validate(expr); err != nil {
				t.Errorf("validate: %v", err)
			}
		})
	}

	// Unknown base field is flagged.
	t.Run("unknown base", func(t *testing.T) {
		expr, _ := parser.Parse("missing@all(status=shipped)", 0)
		if err := v.Validate(expr); err == nil {
			t.Error("expected unknown-field error for missing base")
		}
	})
}
