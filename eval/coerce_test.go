package eval

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

type cents int64
type ratio float32
type flag uint8

func TestNumberOf_Kinds(t *testing.T) {
	tests := []struct {
		in      any
		wantI   int64
		wantF   float64
		wantInt bool
		wantOK  bool
	}{
		{int(5), 5, 5, true, true},
		{int8(-5), -5, -5, true, true},
		{int16(5), 5, 5, true, true},
		{int32(5), 5, 5, true, true},
		{int64(5), 5, 5, true, true},
		{uint(5), 5, 5, true, true},
		{uint8(5), 5, 5, true, true},
		{uint16(5), 5, 5, true, true},
		{uint32(5), 5, 5, true, true},
		{uint64(5), 5, 5, true, true},
		{uint64(math.MaxUint64), 0, float64(math.MaxUint64), false, true},
		{float32(2.5), 2, 2.5, false, true},
		{float64(2.5), 2, 2.5, false, true},
		{cents(7), 7, 7, true, true},
		{ratio(0.5), 0, 0.5, false, true},
		{flag(3), 3, 3, true, true},
		{uintptr(9), 9, 9, true, true},
		{"5", 0, 0, false, false},
		{true, 0, 0, false, false},
		{nil, 0, 0, false, false},
		{time.Hour, 0, 0, false, false},
		{[]int{1}, 0, 0, false, false},
		{struct{}{}, 0, 0, false, false},
	}
	for _, tt := range tests {
		i, f, isInt, ok := numberOf(tt.in)
		if ok != tt.wantOK || isInt != tt.wantInt || (ok && (i != tt.wantI || f != tt.wantF)) {
			t.Errorf("numberOf(%T %v) = (%d, %v, %v, %v), want (%d, %v, %v, %v)",
				tt.in, tt.in, i, f, isInt, ok, tt.wantI, tt.wantF, tt.wantInt, tt.wantOK)
		}
	}
	if toInt64("x") != 0 || toFloat64("x") != 0 {
		t.Error("toInt64/toFloat64 of a non-number should be 0")
	}
	if !isIntKind(cents(1)) || isIntKind(2.0) || !isNumericKind(2.0) || isNumericKind("2") {
		t.Error("isIntKind/isNumericKind disagree with numberOf")
	}
}

func TestToTime_Layouts(t *testing.T) {
	now := time.Now()
	ok := []any{now, &now, "2026-01-01", "2026-01-01T10:00:00Z", "2026-01-01T10:00:00.5+02:00", "2026-01-01T10:00:00", "2026-01-01 10:00:00", "2026-01-01 10:00:00Z"}
	for _, v := range ok {
		if _, got := toTime(v); !got {
			t.Errorf("toTime(%v) not ok", v)
		}
	}
	var nilTime *time.Time
	bad := []any{nil, nilTime, "yesterday", "2026-13-01", "2026-01-01T25:00:00Z", 1700000000, 3.5, true}
	for _, v := range bad {
		if _, got := toTime(v); got {
			t.Errorf("toTime(%v) ok, want not ok", v)
		}
	}
}

func TestToDuration_Forms(t *testing.T) {
	tests := map[any]time.Duration{time.Hour: time.Hour, "1h30m": 90 * time.Minute, "2d": 48 * time.Hour, "1w": 7 * 24 * time.Hour, "30m": 30 * time.Minute}
	for in, want := range tests {
		got, ok := toDuration(in)
		if !ok || got != want {
			t.Errorf("toDuration(%v) = (%v, %v), want %v", in, got, ok, want)
		}
	}
	for _, in := range []any{nil, "soon", 5, 2.5} {
		if _, ok := toDuration(in); ok {
			t.Errorf("toDuration(%v) ok, want not ok", in)
		}
	}
}

func TestIsEmpty(t *testing.T) {
	var nilMap map[string]any
	var nilPtr *int
	var nilIface any
	empty := []any{nil, "", []any{}, []string{}, map[string]any{}, nilMap, nilPtr, nilIface, [0]int{}}
	for _, v := range empty {
		if !isEmpty(v) {
			t.Errorf("isEmpty(%#v) = false", v)
		}
	}
	one := 1
	full := []any{0, false, "x", []any{nil}, map[string]any{"a": nil}, &one, time.Time{}, [1]int{}}
	for _, v := range full {
		if isEmpty(v) {
			t.Errorf("isEmpty(%#v) = true", v)
		}
	}
}

func TestLookupPath(t *testing.T) {
	data := map[string]any{
		"flat.key": 1,
		"labels":   map[string]any{"dev": "jane", "deep": map[string]string{"k": "v"}},
		"text":     "x",
	}
	tests := []struct {
		field  string
		want   any
		wantOK bool
	}{
		{"flat.key", 1, true},
		{"labels.dev", "jane", true},
		{"labels.deep.k", "v", true},
		{"labels", data["labels"], true},
		{"labels.nope", nil, false},
		{"labels.dev.further", nil, false},
		{"text.sub", nil, false},
		{"missing", nil, false},
		{"missing.sub", nil, false},
	}
	for _, tt := range tests {
		got, ok := lookupPath(data, tt.field)
		if ok != tt.wantOK {
			t.Errorf("lookupPath(%q) ok = %v, want %v", tt.field, ok, tt.wantOK)
			continue
		}
		if ok && tt.field != "labels" && got != tt.want {
			t.Errorf("lookupPath(%q) = %v, want %v", tt.field, got, tt.want)
		}
	}
	if v, ok := MapAccessor(nil)("x"); ok || v != nil {
		t.Error("MapAccessor(nil) should report missing")
	}
}

func TestArity_Check(t *testing.T) {
	tests := []struct {
		a    Arity
		n    int
		want string // "" for nil error
	}{
		{Arity{}, 99, ""},
		{Exactly(1), 1, ""},
		{Exactly(1), 2, "requires 1 argument, got 2"},
		{Exactly(2), 1, "requires 2 arguments, got 1"},
		{Exactly(0), 1, "requires 0 arguments, got 1"},
		{AtLeast(1), 0, "requires at least 1 argument, got 0"},
		{AtLeast(2), 5, ""},
		{Between(1, 2), 3, "requires 1 to 2 arguments, got 3"},
		{Between(1, 2), 0, "requires 1 to 2 arguments, got 0"},
		{Between(1, 2), 2, ""},
	}
	for _, tt := range tests {
		err := tt.a.Check("f", tt.n)
		switch {
		case tt.want == "" && err != nil:
			t.Errorf("%+v.Check(%d) = %v, want nil", tt.a, tt.n, err)
		case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
			t.Errorf("%+v.Check(%d) = %v, want %q", tt.a, tt.n, err, tt.want)
		}
	}
}

func TestReadableAs(t *testing.T) {
	v := func(ty ast.ValueType) *ast.Value { return &ast.Value{Type: ty} }
	tests := []struct {
		actual any
		ty     ast.ValueType
		want   bool
	}{
		{nil, ast.ValueString, false},
		{"x", ast.ValueString, true},
		{"x", ast.ValueInteger, false},
		{3, ast.ValueFloat, true},
		{true, ast.ValueBoolean, true},
		{"TRUE", ast.ValueBoolean, true},
		{"yes", ast.ValueBoolean, false},
		{1, ast.ValueBoolean, false},
		{"2026-01-01", ast.ValueDate, true},
		{"never", ast.ValueDate, false},
		{"1h", ast.ValueDuration, true},
		{1, ast.ValueDuration, false},
		{[]any{1}, ast.ValueList, true},
	}
	for _, tt := range tests {
		if got := readableAs(tt.actual, v(tt.ty)); got != tt.want {
			t.Errorf("readableAs(%v, %v) = %v, want %v", tt.actual, tt.ty, got, tt.want)
		}
	}
}

func TestCompareAndEqual_EdgeCases(t *testing.T) {
	str := &ast.Value{Type: ast.ValueString, Str: "abc", Raw: "abc"}
	list := &ast.Value{Type: ast.ValueList, Raw: "[1 2]"}
	dur := &ast.Value{Type: ast.ValueDuration, Duration: time.Hour, Raw: "1h"}
	date := &ast.Value{Type: ast.ValueDate, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Raw: "2026-01-01"}
	boolean := &ast.Value{Type: ast.ValueBoolean, Bool: true, Raw: "true"}
	other := &ast.Value{Type: ast.ValueType(99), Raw: "zz"}

	if equalValues(nil, str) || compareValues(nil, str, token.Gt) {
		t.Error("nil never equals or orders against a string")
	}
	if !equalValues("ABC", str) || !compareValues("abd", str, token.Gt) {
		t.Error("string equality is case-insensitive and ordering is lexical")
	}
	if !equalValues([]any{1, 2}, list) {
		t.Error("a list compares on its string form")
	}
	if !equalValues("1h", dur) || !compareValues(2*time.Hour, dur, token.Gt) || compareValues("x", dur, token.Gt) {
		t.Error("duration comparisons")
	}
	if !compareValues("2026-01-02", date, token.Gt) || !compareValues("2026-01-02", date, token.Neq) ||
		compareValues("x", date, token.Lt) || !compareValues("2026-01-01", date, token.Eq) {
		t.Error("date comparisons")
	}
	if !equalValues("true", boolean) || equalValues("yes", boolean) || equalValues(1, boolean) {
		t.Error("boolean equality reads true/false strings only")
	}
	if !equalValues("zz", other) {
		t.Error("unknown value types compare on raw text")
	}
	if compareOrdered(int64(1), int64(2), token.And) {
		t.Error("a non-relational operator orders nothing")
	}
}

func TestIntOverflowHelpers(t *testing.T) {
	if r, ok := addInt64(math.MaxInt64, 1); ok {
		t.Errorf("addInt64 overflow not detected: %d", r)
	}
	if r, ok := addInt64(math.MinInt64, -1); ok {
		t.Errorf("addInt64 underflow not detected: %d", r)
	}
	if r, ok := addInt64(2, 3); !ok || r != 5 {
		t.Errorf("addInt64(2,3) = %d, %v", r, ok)
	}
	if _, ok := mulInt64(math.MaxInt64, 2); ok {
		t.Error("mulInt64 overflow not detected")
	}
	if _, ok := mulInt64(math.MinInt64, -1); ok {
		t.Error("mulInt64 MinInt64*-1 not detected")
	}
	if r, ok := mulInt64(0, math.MaxInt64); !ok || r != 0 {
		t.Error("mulInt64 by zero")
	}
	if r, ok := mulInt64(-4, 3); !ok || r != -12 {
		t.Errorf("mulInt64(-4,3) = %d, %v", r, ok)
	}
	for _, q := range []string{"9223372036854775807-(-1)", "9223372036854775807*2", "-9223372036854775808-1"} {
		p, err := CompileValue(q, nil)
		if err != nil {
			t.Fatal(err)
		}
		v, err := p.Eval(nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, isFloat := v.(float64); !isFloat {
			t.Errorf("%s = %v (%T), want a float64 on overflow", q, v, v)
		}
	}
}

func TestValueFromAny_Kinds(t *testing.T) {
	tests := []struct {
		in   any
		want ast.ValueType
	}{
		{"s", ast.ValueString}, {true, ast.ValueBoolean}, {int8(1), ast.ValueInteger}, {uint64(1), ast.ValueInteger},
		{cents(1), ast.ValueInteger}, {float32(1), ast.ValueFloat}, {time.Now(), ast.ValueDate}, {time.Hour, ast.ValueDuration},
		{[]any{1}, ast.ValueList}, {[]string{"a"}, ast.ValueList}, {struct{}{}, ast.ValueString},
	}
	for _, tt := range tests {
		got := valueFromAny(tt.in)
		if got == nil || got.Type != tt.want {
			t.Errorf("valueFromAny(%T) = %v, want type %v", tt.in, got, tt.want)
		}
	}
	if valueFromAny(nil) != nil {
		t.Error("valueFromAny(nil) should be nil")
	}
}

func TestCompileCall_RuntimeUnknown(t *testing.T) {
	// A registry assembled without Compile can still be asked to run a call
	// to a function it lacks; that is an error, not a nil result.
	c := compileCall(&ast.FuncCallExpr{Name: "nope"}, FuncRegistry{})
	if _, err := c(func(string) (any, bool) { return nil, false }); !errors.Is(err, errUnknownFunc) {
		t.Errorf("unknown function at runtime: %v", err)
	}
}

func TestBuiltins_NilAndBounds(t *testing.T) {
	nilResult := func(name string, args ...any) {
		t.Helper()
		fn := BuiltinFunctions()[name]
		got, err := fn.Call(args...)
		if err != nil || got != nil {
			t.Errorf("%s(%v) = (%v, %v), want (nil, nil)", name, args, got, err)
		}
	}
	nilResult("lower", nil)
	nilResult("upper", nil)
	nilResult("trim", nil)
	nilResult("len", nil)
	nilResult("contains", nil, "x")
	nilResult("contains", "x", nil)
	nilResult("startsWith", nil, "x")
	nilResult("endsWith", "x", nil)
	nilResult("year", "not a date")
	nilResult("month", nil)
	nilResult("day", nil)
	nilResult("hour", nil)
	nilResult("minute", nil)
	nilResult("second", nil)
	nilResult("weekday", nil)
	nilResult("isBusinessDay", nil)
	nilResult("addDays", nil, 1)
	nilResult("addDays", time.Now(), "many")
	nilResult("addDays", time.Now(), MaxCalendarDays+1)
	nilResult("addDays", time.Now(), float64(-MaxCalendarDays-1))
	nilResult("addDays", time.Now(), math.NaN())
	nilResult("daysAgo", "x")
	nilResult("addBusinessDays", "x", 1)
	nilResult("addBusinessDays", time.Now(), 9e18)
	nilResult("int", struct{}{})
	nilResult("float", struct{}{})
	nilResult("round", 1.5, 400)

	fn := BuiltinFunctions()
	if got, _ := fn["int"].Call(cents(7)); got != int64(7) {
		t.Errorf("int(cents(7)) = %v", got)
	}
	if got, _ := fn["int"].Call(2.9); got != int64(2) {
		t.Errorf("int(2.9) = %v", got)
	}
	if got, _ := fn["float"].Call(uint8(7)); got != 7.0 {
		t.Errorf("float(uint8(7)) = %v", got)
	}
	if got, _ := fn["abs"].Call(int16(-7)); got != int64(7) {
		t.Errorf("abs(int16(-7)) = %v", got)
	}
	if got, _ := fn["abs"].Call("x"); got != int64(0) {
		t.Errorf("abs(x) = %v", got)
	}
	if got, _ := fn["string"].Call(time.Hour); got != "1h0m0s" {
		t.Errorf("string(1h) = %v", got)
	}
	if got, _ := fn["contains"].Call([]string{"A", "b"}, "a"); got != true {
		t.Errorf("contains(list) = %v", got)
	}
	// Holidays with an unparseable entry are ignored rather than matching.
	d := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) // a Friday
	if got, _ := fn["isBusinessDay"].Call(d, []any{"2026-01-02", "nonsense"}); got != false {
		t.Errorf("isBusinessDay with holiday = %v", got)
	}
	if got, _ := fn["addBusinessDays"].Call(d, -1); got.(time.Time).Weekday() != time.Thursday {
		t.Errorf("addBusinessDays back one = %v", got)
	}
}

func TestStructAccessor_Shapes(t *testing.T) {
	type rec struct {
		Name *string `query:"name"`
		Any  any     `query:"any"`
		Skip string  `query:"-"`
		None string
	}
	if _, ok := StructAccessor((*rec)(nil))("name"); ok {
		t.Error("nil pointer struct should report missing")
	}
	if _, ok := StructAccessor(42)("name"); ok {
		t.Error("non-struct should report missing")
	}
	get := StructAccessor(&rec{Any: 3})
	if _, ok := get("name"); ok {
		t.Error("nil pointer field should be missing")
	}
	if v, ok := get("any"); !ok || v != 3 {
		t.Errorf("interface field = %v, %v", v, ok)
	}
	if _, ok := get("Skip"); ok {
		t.Error("skipped tag resolved")
	}
	if _, ok := get("None"); ok {
		t.Error("untagged field resolved")
	}
	if fields := FieldsFromStruct(42); fields != nil {
		t.Error("FieldsFromStruct of a non-struct should be nil")
	}
	if _, err := CompileFor[int]("x=1"); err == nil {
		t.Error("CompileFor over a non-struct should fail")
	}
}
