package eval

import (
	"errors"
	"testing"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/validate"
)

// Regression tests for the November 2026 review findings. Each case names
// the behaviour that was wrong and pins the corrected one.

var regressFields = []validate.FieldConfig{
	{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "n", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "secret", Type: validate.TypeText, AllowedOps: []validate.Op{validate.OpEq}},
	{Name: "d", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	{Name: "created_at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
	{Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
	{Name: "zip", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "active", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
	{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
}

func TestRegress_Matches(t *testing.T) {
	tests := []struct {
		name  string
		query string
		rec   map[string]any
		want  bool
	}{
		// Decimal comparisons no longer truncate the record value.
		{"decimal gt int literal", "price>3", map[string]any{"price": 3.5}, true},
		{"decimal eq int literal", "price=3", map[string]any{"price": 3.9}, false},
		{"decimal neq int literal", "price!=3", map[string]any{"price": 3.9}, true},
		// Every Go numeric kind is read, and a non-number is never zero.
		{"uint", "n=5", map[string]any{"n": uint(5)}, true},
		{"uint not zero", "n=0", map[string]any{"n": uint(5)}, false},
		{"int16", "n=5", map[string]any{"n": int16(5)}, true},
		{"int8", "n=5", map[string]any{"n": int8(5)}, true},
		{"uint64", "n=5", map[string]any{"n": uint64(5)}, true},
		{"string is not zero", "n=0", map[string]any{"n": "abc"}, false},
		{"string neq zero", "n!=0", map[string]any{"n": "abc"}, false},
		// Negated operators on function-wrapped qualifiers desugar to NOT.
		{"func negated op short", "len(name)!>5", map[string]any{"name": "ab"}, true},
		{"func negated op long", "len(name)!>5", map[string]any{"name": "abcdefgh"}, false},
		// A missing field inside a function is no value, not "<nil>".
		{"len missing", "len(name)=5", map[string]any{}, false},
		{"len missing gt", "len(name)>3", map[string]any{}, false},
		{"lower missing", `lower(name)="<nil>"`, map[string]any{}, false},
		{"contains missing", `contains(name, "nil")`, map[string]any{}, false},
		{"len nil", "len(name)>3", map[string]any{"name": nil}, false},
		// Text fields accept literals the lexer typed otherwise.
		{"text integer literal", "zip=90210", map[string]any{"zip": "90210"}, true},
		{"text leading zeros", "zip=00123", map[string]any{"zip": "00123"}, true},
		{"text float literal", "name=1.5", map[string]any{"name": "1.5"}, true},
		{"text bool literal", "name=true", map[string]any{"name": "true"}, true},
		{"text date literal", "name=2026-01-01", map[string]any{"name": "2026-01-01"}, true},
		{"text duration literal", "name=1d", map[string]any{"name": "1d"}, true},
		// Datetime literals and unparseable dates.
		{"quoted rfc3339", `created_at>="2026-01-01T10:00:00Z"`, map[string]any{"created_at": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}, true},
		{"bare rfc3339", "created_at>=2026-01-01T10:00:00Z", map[string]any{"created_at": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}, true},
		{"space separated timestamp", "created_at>2026-01-01", map[string]any{"created_at": "2026-05-05 10:00:00"}, true},
		{"unparseable date lt", "created_at<2026-01-01", map[string]any{"created_at": "yesterday"}, false},
		{"unparseable date gt", "created_at>2026-01-01", map[string]any{"created_at": "yesterday"}, false},
		// A day literal is the whole UTC day (issue #24); an instant literal is exact.
		{"day eq midnight", "created_at=2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)}, true},
		{"day eq afternoon", "created_at=2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 45, 12, 0, time.UTC)}, true},
		{"day eq next midnight", "created_at=2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}, false},
		{"day neq afternoon", "created_at!=2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC)}, false},
		{"day neq next day", "created_at!=2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC)}, true},
		{"day lte afternoon", "created_at<=2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 28, 23, 59, 0, 0, time.UTC)}, true},
		{"day lt afternoon", "created_at<2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC)}, false},
		{"day gt afternoon", "created_at>2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC)}, false},
		{"day gt next day", "created_at>2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}, true},
		{"day gte afternoon", "created_at>=2026-09-28", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC)}, true},
		{"day range covers last day", "created_at:2026-09-01..2026-09-30", map[string]any{"created_at": time.Date(2026, 9, 30, 18, 0, 0, 0, time.UTC)}, true},
		{"day range excludes next day", "created_at:2026-09-01..2026-09-30", map[string]any{"created_at": time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}, false},
		{"instant eq exact", "created_at=2026-09-28T13:45:12Z", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 45, 12, 0, time.UTC)}, true},
		{"instant eq other", "created_at=2026-09-28T13:45:12Z", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 45, 13, 0, time.UTC)}, false},
		{"instant lte exact", "created_at<=2026-09-28T13:45:12Z", map[string]any{"created_at": time.Date(2026, 9, 28, 13, 45, 12, 0, time.UTC)}, true},
		{"quoted day on datetime", `created_at="2026-09-28"`, map[string]any{"created_at": time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC)}, true},
		{"date field midnight", "d=2026-09-28", map[string]any{"d": time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)}, true},
		// Non-ASCII bare words (issue #23).
		{"non-ascii value", "name=café", map[string]any{"name": "Café"}, true},
		{"non-ascii wildcard", "name=caf*", map[string]any{"name": "café"}, true},
		{"non-ascii function arg", "lower(name)=日本", map[string]any{"name": "日本"}, true},
		// Nested maps and flat keys.
		{"nested map", "labels.dev=jane", map[string]any{"labels": map[string]any{"dev": "jane"}}, true},
		{"flat key", "labels.dev=jane", map[string]any{"labels.dev": "jane"}, true},
		{"nested string map", "labels.dev=jane", map[string]any{"labels": map[string]string{"dev": "jane"}}, true},
		// Presence means non-empty.
		{"presence empty string", "name", map[string]any{"name": ""}, false},
		{"presence nil", "name", map[string]any{"name": nil}, false},
		{"presence value", "name", map[string]any{"name": "x"}, true},
		{"presence zero-length list", "tags", map[string]any{"tags": []string{}}, false},
		// != on a wildcard negates the pattern.
		{"wildcard neq hit", "name!=jo*", map[string]any{"name": "john"}, false},
		{"wildcard neq miss", "name!=jo*", map[string]any{"name": "mary"}, true},
		{"wildcard neq missing", "name!=jo*", map[string]any{}, false},
		// Escaped stars are literal.
		{"escaped star literal", `name=\*`, map[string]any{"name": "*"}, true},
		{"escaped star no match", `name=\*`, map[string]any{"name": "x"}, false},
		{"escaped star in pattern", `name=a\*b*`, map[string]any{"name": "a*bc"}, true},
		{"escaped star in pattern no match", `name=a\*b*`, map[string]any{"name": "axbc"}, false},
		{"lone star", "name=*", map[string]any{"name": "anything"}, true},
		{"backslash then wildcard", `name=a\\*`, map[string]any{"name": `a\b`}, true},
		{"backslash then wildcard no match", `name=a\\*`, map[string]any{"name": "ab"}, false},
		{"backslash then literal star", `name=a\\\*`, map[string]any{"name": `a\*`}, true},
		// Arithmetic forms.
		{"subtraction", "n>10-3", map[string]any{"n": 8}, true},
		{"spaced subtraction", "n>10 - 3", map[string]any{"n": 6}, false},
		{"negative arg", "n>abs(-5)", map[string]any{"n": 6}, true},
		{"negative in list", "n IN (-1, 4)", map[string]any{"n": -1}, true},
		{"wildcard in list", "name IN (jo*, x)", map[string]any{"name": "john"}, true},
		// A duration suffix followed by an operator is still a duration.
		{"duration chain", "d>addDays(d, 0)-30m-1h", map[string]any{"d": time.Now()}, true},
		{"duration in group", "n>=10-(2-3%2)", map[string]any{"n": 9}, true},
		// A function call's arguments do not make the value a bareword string.
		{"call args in arithmetic", "n>=2*abs(price)", map[string]any{"n": 10, "price": -3.0}, true},
		{"call args in arithmetic below", "n>=2*abs(price)", map[string]any{"n": 5, "price": -3.0}, false},
		{"quoted arg in arithmetic", `n>=1+len("abc")`, map[string]any{"n": 4}, true},
		// Unquoted values with letters are strings, never arithmetic.
		{"uuid", "name=550e8400-e29b-41d4-a716-446655440000", map[string]any{"name": "550e8400-e29b-41d4-a716-446655440000"}, true},
		{"version", "name=version-1.2.3-4", map[string]any{"name": "version-1.2.3-4"}, true},
		// Boolean against a non-boolean record value is false either way.
		{"bool vs string yes", "active=false", map[string]any{"active": "yes"}, false},
		{"bool vs string true", "active=true", map[string]any{"active": "TRUE"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, regressFields)
			if err != nil {
				t.Fatalf("compile %q: %v", tt.query, err)
			}
			if got := prog.Match(tt.rec); got != tt.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tt.query, tt.rec, got, tt.want)
			}
		})
	}
}

func TestRegress_CompileErrors(t *testing.T) {
	tests := []struct {
		name  string
		query string
		opts  []Option
		want  error // sentinel to errors.Is against, or nil for any error
	}{
		{"unknown function", "lenn(name)>3", nil, ErrUnknownFunction},
		{"unknown bool function", "bogus(name)", nil, ErrUnknownFunction},
		{"arity too many", "lower(name, name)=abc", nil, ErrArity},
		{"arity nested", "len(lower())>3", nil, ErrArity},
		{"wildcard through function on eq-only field", "trim(secret)=a*", nil, nil},
		{"global op restriction through function", "trim(name)>a", []Option{WithAllowedOps(validate.OpEq)}, nil},
		{"arith on text field", "name=10-3", nil, nil},
		{"allowed fields with custom validator", "secret=x", []Option{WithAllowedFields("name"), WithCustomValidator(regressCV{})}, nil},
		{"allowed ops with custom validator", "price>1", []Option{WithAllowedOps(validate.OpEq), WithCustomValidator(regressCV{})}, nil},
		{"allowed fields inside function", "trim(secret)=a", []Option{WithAllowedFields("name")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.query, regressFields, tt.opts...)
			if err == nil {
				t.Fatalf("Compile(%q) error = nil, want error", tt.query)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("Compile(%q) error = %v, want %v", tt.query, err, tt.want)
			}
		})
	}
}

// regressCV resolves every regressFields entry, as a tenant hook would.
type regressCV struct{}

func (regressCV) GetFieldConfig(n string) (validate.FieldConfig, bool) {
	for _, f := range regressFields {
		if f.Name == n {
			return f, true
		}
	}
	return validate.FieldConfig{}, false
}
func (regressCV) ValidateCustomRules(ast.Expression) error { return nil }

func TestRegress_FunctionsAllowedThroughOps(t *testing.T) {
	// Ordering through a function over a text field stays allowed: that is
	// the only way to order on a derived number.
	for _, q := range []string{"len(name)>5", "lower(name)=john*", "len(name):1..5"} {
		if _, err := Compile(q, regressFields); err != nil {
			t.Errorf("Compile(%q) error: %v", q, err)
		}
	}
}

func TestRegress_BusinessDaysBounded(t *testing.T) {
	prog, err := Compile("addBusinessDays(d, 9000000000000000000)>d", regressFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	done := make(chan bool, 1)
	go func() { done <- prog.Match(map[string]any{"d": time.Now()}) }()
	select {
	case got := <-done:
		if got {
			t.Error("an out-of-range day count should resolve to no value")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("addBusinessDays with a huge count did not return")
	}
}

type regressRec struct {
	U    uint     `query:"u"`
	I16  int16    `query:"i16"`
	PS   *string  `query:"ps"`
	PF   *float64 `query:"pf"`
	priv string   `query:"priv"` //nolint:unused // the tag must be ignored, not read
	Name string   `query:"name"`
}

func TestRegress_StructFields(t *testing.T) {
	s, f := "john", 9.5
	rec := regressRec{U: 5, I16: 7, PS: &s, PF: &f, Name: "john"}
	tests := map[string]bool{
		"u=5": true, "u=0": false, "i16=7": true,
		"ps=john": true, "pf>5": true, "pf=0": false, "name=john": true,
	}
	for q, want := range tests {
		tp, err := CompileFor[regressRec](q)
		if err != nil {
			t.Fatalf("CompileFor(%q): %v", q, err)
		}
		if got := tp.MatchStruct(rec); got != want {
			t.Errorf("MatchStruct(%q) = %v, want %v", q, got, want)
		}
	}
	// A nil pointer field is missing, not a zero.
	tp, _ := CompileFor[regressRec]("pf=0")
	if tp.MatchStruct(regressRec{}) {
		t.Error("nil *float64 matched pf=0")
	}
	// A tag on an unexported field is ignored rather than panicking.
	if _, err := CompileFor[regressRec]("priv=p"); err == nil {
		t.Error("expected unknown field for an unexported tagged field")
	}
	for _, fc := range FieldsFromStruct(regressRec{}) {
		if fc.Name == "priv" {
			t.Error("FieldsFromStruct declared an unexported field")
		}
	}
}

func TestRegress_ArithmeticOverflowPromotes(t *testing.T) {
	v, err := CompileValue("9223372036854775807+1", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Eval(nil)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := got.(float64); !ok || f < 9.2e18 {
		t.Errorf("overflowing add = %v (%T), want a positive float64", got, got)
	}
	r, _ := CompileValue("round(1.5, 400)", nil)
	if _, err := r.Eval(nil); !errors.Is(err, ErrNoValue) {
		t.Errorf("round with an absurd scale = %v, want ErrNoValue", err)
	}
}

func TestRegress_DepthCountsNesting(t *testing.T) {
	prog, err := Compile("name=a name=b name=c name=d name=e name=f name=g name=h name=i", regressFields, WithMaxDepth(3))
	if err != nil {
		t.Fatalf("nine terms exceeded a depth of 3: %v", err)
	}
	if d := ast.Depth(prog.AST()); d != 2 {
		t.Errorf("depth of a flat AND chain = %d, want 2", d)
	}
	if _, err := Compile("NOT (name=a OR (name=b AND NOT name=c))", regressFields, WithMaxDepth(3)); err == nil {
		t.Error("a genuinely nested query should exceed a depth of 3")
	}
}

func TestRegress_FieldsAndWalkSeeFunctions(t *testing.T) {
	prog, err := Compile(`lower(name)=john AND created_at>=now() AND contains(tags, "x")`, regressFields)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(ast.Calls(prog.AST())); got != 3 {
		t.Errorf("Calls = %d, want 3", got)
	}
	for _, f := range prog.Fields() {
		if f.String() == "lower" {
			t.Error("Fields reported the function name as a field")
		}
	}
	if got, want := prog.Stringify(), `lower(name)=john AND created_at>=now() AND contains(tags, "x")`; got != want {
		t.Errorf("Stringify = %q, want %q", got, want)
	}
}
