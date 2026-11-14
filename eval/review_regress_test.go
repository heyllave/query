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
	// Ordering through a function over a text field stays allowed when the
	// field allows wildcards: that is the only way to order on a derived
	// number.
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

// The result must not depend on how the walk is done: skipping whole weeks
// has to land on the same day as stepping one day at a time, from every
// weekday, in both directions, with and without holidays in the way.
func TestRegress_BusinessDaysMatchDayByDayWalk(t *testing.T) {
	walk := func(start time.Time, n int, holidays map[string]bool) time.Time {
		step := 1
		if n < 0 {
			step, n = -1, -n
		}
		for n > 0 {
			start = start.AddDate(0, 0, step)
			if isBusinessDay(start, holidays) {
				n--
			}
		}
		return start
	}
	base := time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC) // a Monday
	holidayList := []any{
		time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC),
	}
	holidays := holidaySet(holidayList)
	for offset := 0; offset < 7; offset++ {
		start := base.AddDate(0, 0, offset)
		for _, n := range []int{0, 1, 4, 5, 6, 9, 10, 11, 23, 64, 250, 251, 1000, -1, -5, -6, -10, -11, -64, -250, -1000} {
			got, err := fnAddBusinessDays(start, n)
			if want := walk(start, n, nil); err != nil || !want.Equal(got.(time.Time)) {
				t.Errorf("addBusinessDays(%s, %d) = %v, %v; the walk gives %s", start.Format("Mon 2006-01-02"), n, got, err, want.Format("Mon 2006-01-02"))
			}
			got, err = fnAddBusinessDays(start, n, holidayList)
			if want := walk(start, n, holidays); err != nil || !want.Equal(got.(time.Time)) {
				t.Errorf("addBusinessDays(%s, %d, holidays) = %v, %v; the walk gives %s", start.Format("Mon 2006-01-02"), n, got, err, want.Format("Mon 2006-01-02"))
			}
		}
	}
}

// The largest accepted count must cost no more than a small one: at the cap
// the old day-by-day walk took a third of a second per record.
func TestRegress_BusinessDaysCostIndependentOfCount(t *testing.T) {
	prog, err := Compile("d<addBusinessDays(d, 3999999) AND d>addBusinessDays(d, -3999999)", regressFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rec := map[string]any{"d": time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)}
	start := time.Now()
	for i := 0; i < 200; i++ {
		if !prog.Match(rec) {
			t.Fatal("Match = false, want true")
		}
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("200 matches took %v; the walk is linear in the day count again", took)
	}
}

func TestRegress_Gaps(t *testing.T) {
	type yamlMap = map[any]any
	type labelSet map[string]string
	tests := []struct {
		name  string
		query string
		rec   map[string]any
		want  bool
	}{
		// A label key with dots inside a nested map.
		{"dotted key in nested map", "labels.example.com/team=x", map[string]any{"labels": map[string]any{"example.com/team": "x"}}, true},
		{"dotted key in string map", "labels.app.kubernetes.io/name=x", map[string]any{"labels": map[string]string{"app.kubernetes.io/name": "x"}}, true},
		{"dotted key beside a shorter one", "labels.a.b=x", map[string]any{"labels": map[string]any{"a": "scalar", "a.b": "x"}}, true},
		{"flat prefix then nested", "labels.a.b=x", map[string]any{"labels.a": map[string]any{"b": "x"}}, true},
		{"fully nested still wins", "labels.a.b=x", map[string]any{"labels": map[string]any{"a": map[string]any{"b": "x"}}}, true},
		{"yaml map", "labels.dev=x", map[string]any{"labels": yamlMap{"dev": "x"}}, true},
		{"named map type", "labels.dev=x", map[string]any{"labels": labelSet{"dev": "x"}}, true},
		{"nil map", "labels.dev=x", map[string]any{"labels": map[string]any(nil)}, false},
		{"scalar where a map is expected", "labels.dev=x", map[string]any{"labels": "dev"}, false},
		{"deep path over a shallow record", "labels.a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p=x", map[string]any{"labels": map[string]any{"a": map[string]any{"b": map[string]any{}}}}, false},
		// NOT IN: false for a listed value and, like SQL, for a missing field.
		{"not in miss", "name NOT IN (a, b)", map[string]any{"name": "c"}, true},
		{"not in hit", "name NOT IN (a, b)", map[string]any{"name": "a"}, false},
		{"not in missing field", "name NOT IN (a, b)", map[string]any{}, false},
		{"not in wildcard", "name NOT IN (jo*, x)", map[string]any{"name": "john"}, false},
		// IN on a function.
		{"function in", "lower(name) IN (ab, cd)", map[string]any{"name": "AB"}, true},
		{"function not in", "lower(name) NOT IN (ab, cd)", map[string]any{"name": "AB"}, false},
		{"function in numbers", "len(name) IN (1, 2)", map[string]any{"name": "AB"}, true},
		// A term next to a group is an implicit AND, not a call.
		{"term then group", "name (n=1 OR n=2)", map[string]any{"name": "x", "n": 2}, true},
		// Arithmetic in arguments and signed operands.
		{"arithmetic argument", "n=abs(1-8)", map[string]any{"n": 7}, true},
		{"signed operand", "n=10*-3", map[string]any{"n": -30}, true},
		// An address is one value.
		{"cidr", "name=10.0.0.1/24", map[string]any{"name": "10.0.0.1/24"}, true},
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

func TestRegress_RestrictedFieldThroughCompile(t *testing.T) {
	for _, q := range []string{
		`startsWith(secret, "a")`, `contains(secret, "b")`, `lower(secret)>="a"`,
		"len(secret)>2", "n>len(secret)", "name!=[secret]", "secret NOT IN (a)",
	} {
		if _, err := Compile(q, regressFields); err == nil {
			t.Errorf("Compile(%q) error = nil; secret allows only =", q)
		}
	}
	if _, err := Compile(`startsWith(name, "a")`, regressFields, WithAllowedOps(validate.OpEq)); err == nil {
		t.Error("a predicate compiled under WithAllowedOps(OpEq)")
	}
}

func TestRegress_AbsOfMinInt(t *testing.T) {
	prog, err := CompileValue("abs(-9223372036854775808)", nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := prog.Eval(nil)
	if f, ok := got.(float64); err != nil || !ok || f != 9223372036854775808 {
		t.Errorf("abs(MinInt64) = %v (%T), %v; want 9.223372036854775808e18", got, got, err)
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
