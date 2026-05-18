package eval

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
	"github.com/heyllave/query/validate"
)

// TestSelector_DefensiveBranches exercises paths that the parser can't
// currently emit but that compileSelector / validateSelector must handle
// defensively (unusual bases, primitive element slices, nil inner).
func TestSelector_DefensiveBranches(t *testing.T) {
	t.Run("base is group — falls back to base matcher", func(t *testing.T) {
		// GroupExpr{QualifierExpr{name=draft}} as Base — no list semantics.
		// Fallback just evaluates the base (matches any record where name=draft).
		expr := &ast.SelectorExpr{
			Base: &ast.GroupExpr{Expr: &ast.QualifierExpr{
				Field:    ast.FieldPath{"name"},
				Operator: token.Eq,
				Value:    ast.Value{Type: ast.ValueString, Str: "draft"},
			}},
			Selector: "first",
		}
		m := compileMatcher(expr, BuiltinFunctions())
		got := m(func(f string) (any, bool) {
			if f == "name" {
				return "draft", true
			}
			return nil, false
		})
		if !got {
			t.Error("expected fallback to base matcher to succeed")
		}
	})

	t.Run("validate skips OpPresence check on list base", func(t *testing.T) {
		// 'items' has no OpPresence, but selector base doesn't require it.
		v := validate.New([]validate.FieldConfig{
			{Name: "items", Type: validate.TypeText, AllowedOps: validate.TextOps},
		})
		expr := &ast.SelectorExpr{
			Base:     &ast.PresenceExpr{Field: ast.FieldPath{"items"}},
			Selector: "first",
		}
		if err := v.Validate(expr); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("validate with non-presence base recurses", func(t *testing.T) {
		// Base is a GroupExpr wrapping a qualifier — falls through to v.validate.
		v := validate.New([]validate.FieldConfig{
			{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
		})
		expr := &ast.SelectorExpr{
			Base: &ast.GroupExpr{Expr: &ast.QualifierExpr{
				Field:    ast.FieldPath{"name"},
				Operator: token.Eq,
				Value:    ast.Value{Type: ast.ValueString, Str: "x"},
			}},
			Selector: "first",
		}
		if err := v.Validate(expr); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("validate with qualifier base calls validateQualifier", func(t *testing.T) {
		// Base is a QualifierExpr referencing an unknown field — should error.
		v := validate.New([]validate.FieldConfig{
			{Name: "known", Type: validate.TypeText, AllowedOps: validate.TextOps},
		})
		expr := &ast.SelectorExpr{
			Base: &ast.QualifierExpr{
				Field:    ast.FieldPath{"unknown"},
				Operator: token.Eq,
				Value:    ast.Value{Type: ast.ValueString, Str: "x"},
			},
			Selector: "first",
		}
		if err := v.Validate(expr); err == nil {
			t.Error("expected validation error on unknown field")
		}
	})

	t.Run("primitive slice yields no field access", func(t *testing.T) {
		// []string is a valid slice, but elements have no accessible fields.
		// Inner lookup must return (nil, false), so match is false.
		expr := &ast.SelectorExpr{
			Base: &ast.PresenceExpr{Field: ast.FieldPath{"tags"}},
			Inner: &ast.QualifierExpr{
				Field:    ast.FieldPath{"name"},
				Operator: token.Eq,
				Value:    ast.Value{Type: ast.ValueString, Str: "x"},
			},
		}
		m := compileMatcher(expr, BuiltinFunctions())
		got := m(func(f string) (any, bool) {
			if f == "tags" {
				return []string{"a", "b"}, true
			}
			return nil, false
		})
		if got {
			t.Error("primitive-element slice must not match field lookup")
		}
	})

	t.Run("nil pointer element is safe", func(t *testing.T) {
		var nilItem *orderItem
		expr := &ast.SelectorExpr{
			Base: &ast.PresenceExpr{Field: ast.FieldPath{"line_items"}},
			Inner: &ast.QualifierExpr{
				Field:    ast.FieldPath{"sku"},
				Operator: token.Eq,
				Value:    ast.Value{Type: ast.ValueString, Str: "x"},
			},
		}
		m := compileMatcher(expr, BuiltinFunctions())
		got := m(func(f string) (any, bool) {
			if f == "line_items" {
				return []*orderItem{nilItem}, true
			}
			return nil, false
		})
		if got {
			t.Error("nil element must not match")
		}
	})

	t.Run("nil inner returns false", func(t *testing.T) {
		// Selector with empty Selector and nil Inner — impossible from parser,
		// but the evaluator must short-circuit to false rather than panic.
		expr := &ast.SelectorExpr{
			Base: &ast.PresenceExpr{Field: ast.FieldPath{"items"}},
		}
		m := compileMatcher(expr, BuiltinFunctions())
		got := m(func(f string) (any, bool) {
			if f == "items" {
				return []any{"a"}, true
			}
			return nil, false
		})
		if got {
			t.Error("nil inner must not match")
		}
	})

	t.Run("nil slice value", func(t *testing.T) {
		expr := &ast.SelectorExpr{
			Base:     &ast.PresenceExpr{Field: ast.FieldPath{"items"}},
			Selector: "first",
		}
		m := compileMatcher(expr, BuiltinFunctions())
		got := m(func(f string) (any, bool) {
			if f == "items" {
				return nil, true
			}
			return nil, false
		})
		if got {
			t.Error("nil slice value must not match")
		}
	})
}

// selectorFields declares a small e-commerce-like schema used by selector tests.
// The "list" fields (orders, tags, line_items) carry TextOps purely so their
// presence in the config satisfies validation; actual per-element comparisons
// use the inner-scoped fields declared below.
var selectorFields = []validate.FieldConfig{
	// List fields — containers iterated by @(...), @first, @last.
	{Name: "orders", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "line_items", Type: validate.TypeText, AllowedOps: validate.TextOps},

	// Top-level scalars that co-compose with selectors.
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "customer", Type: validate.TypeText, AllowedOps: validate.TextOps},

	// Element-scoped fields resolved inside @(...).
	{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "qty", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "sku", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
}

// TestCompile_Selector_MapElements exercises the three selector forms against
// real-world shapes: slices of maps, empty slices, missing fields.
func TestCompile_Selector_MapElements(t *testing.T) {
	tests := []struct {
		name  string
		query string
		data  map[string]any
		want  bool
	}{
		{
			name:  "any shipped order",
			query: "orders@(status=shipped)",
			data: map[string]any{"orders": []any{
				map[string]any{"status": "pending"},
				map[string]any{"status": "shipped"},
			}},
			want: true,
		},
		{
			name:  "no matching order",
			query: "orders@(status=shipped)",
			data: map[string]any{"orders": []any{
				map[string]any{"status": "pending"},
				map[string]any{"status": "cancelled"},
			}},
			want: false,
		},
		{
			name:  "numeric inner predicate",
			query: "line_items@(price>100)",
			data: map[string]any{"line_items": []any{
				map[string]any{"price": 50.0},
				map[string]any{"price": 120.5},
			}},
			want: true,
		},
		{
			name:  "empty slice fails @first",
			query: "tags@first",
			data:  map[string]any{"tags": []any{}},
			want:  false,
		},
		{
			name:  "non-empty slice passes @first",
			query: "tags@first",
			data:  map[string]any{"tags": []any{"urgent"}},
			want:  true,
		},
		{
			name:  "non-empty slice passes @last",
			query: "tags@last",
			data:  map[string]any{"tags": []any{"urgent", "new"}},
			want:  true,
		},
		{
			name:  "missing field fails",
			query: "orders@(status=shipped)",
			data:  map[string]any{},
			want:  false,
		},
		{
			name:  "non-slice value fails",
			query: "orders@first",
			data:  map[string]any{"orders": "not a slice"},
			want:  false,
		},
		{
			name:  "composed with scalar",
			query: "orders@(status=shipped) AND total>500",
			data: map[string]any{
				"total": 750.0,
				"orders": []any{
					map[string]any{"status": "shipped"},
				},
			},
			want: true,
		},
		{
			name:  "composed with scalar — scalar fails",
			query: "orders@(status=shipped) AND total>500",
			data: map[string]any{
				"total": 100.0,
				"orders": []any{
					map[string]any{"status": "shipped"},
				},
			},
			want: false,
		},
		{
			name:  "negation",
			query: "NOT orders@(status=cancelled)",
			data: map[string]any{"orders": []any{
				map[string]any{"status": "shipped"},
			}},
			want: true,
		},
		{
			name:  "inner AND",
			query: "line_items@(price>100 AND qty>=2)",
			data: map[string]any{"line_items": []any{
				map[string]any{"price": 150.0, "qty": int64(1)}, // price ok, qty no
				map[string]any{"price": 50.0, "qty": int64(3)},  // qty ok, price no
				map[string]any{"price": 200.0, "qty": int64(2)}, // both
			}},
			want: true,
		},
		{
			name:  "inner AND — none matches both",
			query: "line_items@(price>100 AND qty>=2)",
			data: map[string]any{"line_items": []any{
				map[string]any{"price": 150.0, "qty": int64(1)},
				map[string]any{"price": 50.0, "qty": int64(3)},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, selectorFields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Match(tt.data); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
}

// orderItem demonstrates struct-tagged element shapes. The accessor used
// inside @(...) resolves fields by `query` tag, matching [StructAccessor].
type orderItem struct {
	Status string  `query:"status"`
	Qty    int64   `query:"qty"`
	Price  float64 `query:"price"`
	SKU    string  `query:"sku"`
}

// TestCompile_Selector_StructElements verifies @(...) iterates slices of
// tagged structs — the common case for struct-backed records.
func TestCompile_Selector_StructElements(t *testing.T) {
	tests := []struct {
		name  string
		query string
		data  map[string]any
		want  bool
	}{
		{
			name:  "slice of structs — match",
			query: "line_items@(sku=abc-123)",
			data: map[string]any{"line_items": []orderItem{
				{SKU: "xyz-000"},
				{SKU: "abc-123"},
			}},
			want: true,
		},
		{
			name:  "slice of struct pointers — match",
			query: "line_items@(price>50)",
			data: map[string]any{"line_items": []*orderItem{
				{Price: 10.0},
				{Price: 75.0},
			}},
			want: true,
		},
		{
			name:  "untagged fields not accessible",
			query: "line_items@(sku=abc-123)",
			data: map[string]any{"line_items": []struct {
				SKU string // no query tag
			}{
				{SKU: "abc-123"},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, selectorFields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Match(tt.data); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCompile_Selector_Validation ensures the validator accepts declared list
// fields without requiring OpPresence and rejects undeclared ones.
func TestCompile_Selector_Validation(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"declared list field", "orders@first", false},
		{"declared list with inner", "orders@(status=shipped)", false},
		{"undeclared list field", "unknown@first", true},
		{"undeclared inner field", "orders@(bogus=x)", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.query, selectorFields)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestCompile_Selector_RoundTrip verifies AST→string→AST for selector queries.
func TestCompile_Selector_RoundTrip(t *testing.T) {
	queries := []string{
		"orders@first",
		"orders@last",
		"orders@(status=shipped)",
		"line_items@(price>100)",
		"orders@(status=shipped) AND total>500",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			prog, err := Compile(q, selectorFields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Stringify(); got != q {
				t.Errorf("round-trip: got %q, want %q", got, q)
			}
		})
	}
}
