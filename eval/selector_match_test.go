package eval

import (
	"testing"

	"github.com/heyllave/query/validate"
)

// These tests cover the full selector matrix end-to-end through Compile +
// Match (map records) and CompileFor + MatchStruct (struct records). They
// complement selector_test.go by exercising every selector kind against
// every edge case (empty list, missing field, nil pointer element, mixed
// element shapes) and by hitting the struct-binding code path that the
// existing tests reach only via map[string]any.

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
