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

// --- Selector matrix over map records --------------------------------------

func TestSelector_Matrix_MapElements(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "orders", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	}

	allShipped := map[string]any{"orders": []any{
		map[string]any{"status": "shipped", "price": 100.0},
		map[string]any{"status": "shipped", "price": 200.0},
	}}
	mixed := map[string]any{"orders": []any{
		map[string]any{"status": "shipped", "price": 100.0},
		map[string]any{"status": "pending", "price": 200.0},
	}}
	someCancelled := map[string]any{"orders": []any{
		map[string]any{"status": "shipped", "price": 100.0},
		map[string]any{"status": "cancelled", "price": 50.0},
	}}
	emptyList := map[string]any{"orders": []any{}}
	missingField := map[string]any{}
	nilField := map[string]any{"orders": nil}

	// Each row tests one (query, record) pairing. The matrix below covers all
	// six selector kinds × five shape categories where the result is
	// non-trivial.
	tests := []struct {
		name  string
		query string
		data  map[string]any
		want  bool
	}{
		// @first
		{"@first/all", "orders@first", allShipped, true},
		{"@first/mixed", "orders@first", mixed, true},
		{"@first/empty", "orders@first", emptyList, false},
		{"@first/missing", "orders@first", missingField, false},
		{"@first/nil", "orders@first", nilField, false},

		// @last (same semantics as @first, distinct AST node)
		{"@last/all", "orders@last", allShipped, true},
		{"@last/empty", "orders@last", emptyList, false},
		{"@last/missing", "orders@last", missingField, false},

		// @(inner) — EXISTS
		{"@anon/all-match", "orders@(status=shipped)", allShipped, true},
		{"@anon/some-match", "orders@(status=shipped)", mixed, true},
		{"@anon/none-match", "orders@(status=shipped)", someCancelled, true},
		{"@anon/empty", "orders@(status=shipped)", emptyList, false},
		{"@anon/missing", "orders@(status=shipped)", missingField, false},
		{"@anon/nil", "orders@(status=shipped)", nilField, false},

		// @any(inner) — alias of @(inner)
		{"@any/some-match", "orders@any(status=shipped)", mixed, true},
		{"@any/none-match", "orders@any(status=delivered)", mixed, false},
		{"@any/empty", "orders@any(status=shipped)", emptyList, false},

		// @all(inner) — universal
		{"@all/all-match", "orders@all(status=shipped)", allShipped, true},
		{"@all/some-match", "orders@all(status=shipped)", mixed, false},
		{"@all/none-match", "orders@all(status=shipped)", someCancelled, false},
		{"@all/empty/vacuous", "orders@all(status=shipped)", emptyList, true},
		{"@all/missing", "orders@all(status=shipped)", missingField, false},
		{"@all/numeric", "orders@all(price>0)", allShipped, true},
		{"@all/numeric-fails", "orders@all(price>=150)", allShipped, false},

		// @none(inner)
		{"@none/no-match", "orders@none(status=cancelled)", allShipped, true},
		{"@none/has-match", "orders@none(status=cancelled)", someCancelled, false},
		{"@none/empty", "orders@none(status=cancelled)", emptyList, true},
		// Missing field is treated as empty list, so @none is satisfied.
		{"@none/missing-≡-empty", "orders@none(status=cancelled)", missingField, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Match(tt.data); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Compound expressions involving selectors ------------------------------

func TestSelector_Composition(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "orders", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		{Name: "customer", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}

	rec := map[string]any{
		"customer": "acme",
		"total":    1500.0,
		"orders": []any{
			map[string]any{"status": "shipped", "price": 200.0},
			map[string]any{"status": "pending", "price": 50.0},
		},
	}

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		// AND with scalar
		{"AND/match", "orders@(status=shipped) AND total>1000", true},
		{"AND/scalar-fails", "orders@(status=shipped) AND total>5000", false},
		// OR of selectors
		{"OR/selectors", "orders@(status=shipped) OR orders@(status=delivered)", true},
		{"OR/all-false", "orders@(status=delivered) OR orders@(status=returned)", false},
		// NOT outside selector
		{"NOT/match", "NOT orders@(status=cancelled)", true},
		{"NOT/no-match", "NOT orders@(status=shipped)", false},
		// @all AND @none composition
		{"all-AND-none", "orders@all(price>0) AND orders@none(status=cancelled)", true},
		{"all-AND-none/all-fails", "orders@all(price>=100) AND orders@none(status=cancelled)", false},
		// Group around selectors
		{"group", "(orders@(status=shipped) OR orders@(status=delivered)) AND customer=acme", true},
		// Implicit AND
		{"implicit-AND", "orders@(status=shipped) customer=acme", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Match(rec); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Selectors against struct-bound records via MatchStruct ----------------

type lineItem struct {
	Status string  `query:"status"`
	Price  float64 `query:"price"`
	Qty    int64   `query:"qty"`
	SKU    string  `query:"sku"`
}

type cart struct {
	Customer string     `query:"customer"`
	Total    float64    `query:"total"`
	Orders   []lineItem `query:"orders"`
}

// TestSelector_MatchStruct_StructSlice_NoInner covers the subset of selector
// kinds that work with CompileFor[T] alone: @first / @last. Inner-predicate
// selectors need the inner element fields declared (see _Inner test below)
// because FieldsFromStruct does not recurse into slice element types.
func TestSelector_MatchStruct_StructSlice_NoInner(t *testing.T) {
	nonEmpty := cart{
		Customer: "acme",
		Orders: []lineItem{
			{Status: "shipped", Price: 200},
		},
	}
	empty := cart{Customer: "globex"}

	tests := []struct {
		name  string
		query string
		data  cart
		want  bool
	}{
		{"first/non-empty", "orders@first", nonEmpty, true},
		{"first/empty", "orders@first", empty, false},
		{"last/non-empty", "orders@last", nonEmpty, true},
		{"last/empty", "orders@last", empty, false},
		{"compound/first-AND-scalar", "orders@first AND customer=acme", nonEmpty, true},
		{"compound/first-AND-fails", "orders@first AND customer=globex", nonEmpty, false},
		{"NOT-first", "NOT orders@first", empty, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := CompileFor[cart](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(tt.data); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSelector_StructSlice_Inner exercises inner-predicate selectors against
// struct-bound records using the Compile + StructAccessor pattern, which is
// the path real consumers take when they want to filter on element-scoped
// fields. The element-scoped fields are declared explicitly in the field
// config because FieldsFromStruct only walks the container struct's tags.
func TestSelector_StructSlice_Inner(t *testing.T) {
	// Container fields, plus the element-scoped fields needed by the inner
	// predicate. The validator only checks they're declared; the eval engine
	// resolves them against each element via reflection.
	fields := append(
		FieldsFromStruct(cart{}),
		validate.FieldConfig{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
		validate.FieldConfig{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		validate.FieldConfig{Name: "qty", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
		validate.FieldConfig{Name: "sku", Type: validate.TypeText, AllowedOps: validate.TextOps},
	)

	allShipped := cart{
		Customer: "acme",
		Total:    1500.0,
		Orders: []lineItem{
			{Status: "shipped", Price: 200, Qty: 2, SKU: "abc"},
			{Status: "shipped", Price: 300, Qty: 1, SKU: "xyz"},
		},
	}
	mixed := cart{
		Customer: "acme",
		Total:    1500.0,
		Orders: []lineItem{
			{Status: "shipped", Price: 200},
			{Status: "pending", Price: 50},
			{Status: "cancelled", Price: 30},
		},
	}
	empty := cart{Customer: "globex", Total: 0}

	tests := []struct {
		name  string
		query string
		data  cart
		want  bool
	}{
		{"any/match", "orders@(status=shipped)", mixed, true},
		{"any/no-match", "orders@(status=delivered)", mixed, false},
		{"all/match", "orders@all(status=shipped)", allShipped, true},
		{"all/fails", "orders@all(status=shipped)", mixed, false},
		{"all/empty-vacuous", "orders@all(status=shipped)", empty, true},
		{"none/match", "orders@none(status=delivered)", allShipped, true},
		{"none/fails", "orders@none(status=cancelled)", mixed, false},
		{"compound/AND-scalar", "orders@(status=shipped) AND total>1000", allShipped, true},
		{"compound/sku-quoted", `orders@(sku="abc")`, allShipped, true},
		{"compound/numeric-inner", "orders@(price>=250)", allShipped, true},
		{"compound/numeric-inner-fails", "orders@(price>=500)", allShipped, false},
		{"compound/inner-AND", "orders@(price>=100 AND qty>=2)", allShipped, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchFunc(StructAccessor(tt.data)); got != tt.want {
				t.Errorf("MatchFunc(StructAccessor) = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Selectors against []*Struct (slice of pointers) ----------------------

type orderRef struct {
	Customer string      `query:"customer"`
	Orders   []*lineItem `query:"orders"`
}

// TestSelector_PointerSlice_Inner verifies that selector evaluation handles
// slices of pointer structs (including nil elements) using the manual
// Compile + StructAccessor path so we can validate inner field references.
func TestSelector_PointerSlice_Inner(t *testing.T) {
	fields := append(
		FieldsFromStruct(orderRef{}),
		validate.FieldConfig{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
		validate.FieldConfig{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	)

	rec := orderRef{
		Customer: "acme",
		Orders: []*lineItem{
			{Status: "shipped", Price: 100},
			{Status: "pending", Price: 50},
		},
	}
	withNil := orderRef{
		Customer: "globex",
		Orders: []*lineItem{
			nil, // nil element must not panic and must not match
			{Status: "shipped", Price: 50},
		},
	}

	tests := []struct {
		name  string
		query string
		data  orderRef
		want  bool
	}{
		{"any-shipped", "orders@(status=shipped)", rec, true},
		{"all-fails", "orders@all(status=shipped)", rec, false},
		{"nil-element-safe-any", "orders@(status=shipped)", withNil, true},
		{"nil-element-all-fails", "orders@all(status=shipped)", withNil, false},
		{"none-with-nil", "orders@none(status=cancelled)", withNil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchFunc(StructAccessor(tt.data)); got != tt.want {
				t.Errorf("MatchFunc(StructAccessor) = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Selectors against []any with mixed element shapes ---------------------

func TestSelector_MixedElementShapes(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "items", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}

	// Mixed: one map, one struct, one primitive. The primitive provides no
	// field accessor, so it never matches the inner predicate.
	rec := map[string]any{
		"items": []any{
			map[string]any{"status": "shipped"},
			lineItem{Status: "pending"},
			"primitive-string",
		},
	}

	prog, err := Compile("items@(status=shipped)", fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(rec) {
		t.Error("expected the map element to match")
	}

	prog2, err := Compile("items@all(status=shipped)", fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if prog2.Match(rec) {
		t.Error("@all should fail because the struct and primitive don't satisfy")
	}
}

// --- Selector validation paths --------------------------------------------

func TestSelector_Validation_AllKinds(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "orders", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}

	ok := []string{
		"orders@first",
		"orders@last",
		"orders@(status=shipped)",
		"orders@any(status=shipped)",
		"orders@all(status=shipped)",
		"orders@none(status=cancelled)",
	}
	for _, q := range ok {
		t.Run("ok/"+q, func(t *testing.T) {
			if _, err := Compile(q, fields); err != nil {
				t.Errorf("compile: %v", err)
			}
		})
	}

	fail := []string{
		"missing@first",                 // unknown base
		"missing@all(status=shipped)",   // unknown base, inner ok
		"orders@(bogus=x)",              // unknown inner field
		"orders@all(bogus=x)",           // unknown inner field
		"orders@none(status=x AND b=y)", // unknown nested inner
	}
	for _, q := range fail {
		t.Run("fail/"+q, func(t *testing.T) {
			if _, err := Compile(q, fields); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

// --- Round-trip for selectors through ast.String --------------------------

func TestSelector_RoundTrip_AllKinds(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "orders", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	}
	queries := []string{
		"orders@first",
		"orders@last",
		"orders@(status=shipped)",
		"orders@any(status=shipped)",
		"orders@all(status=shipped)",
		"orders@none(status=cancelled)",
		"orders@(price>=100 AND status=shipped)",
		"NOT orders@(status=cancelled)",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			prog, err := Compile(q, fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Stringify(); got != q {
				t.Errorf("round-trip:\n  got:  %q\n  want: %q", got, q)
			}
		})
	}
}
