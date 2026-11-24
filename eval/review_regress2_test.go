package eval

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// Regression tests for the second-pass audit (November 2026). Each case names
// a consistency or robustness gap the first review did not cover and pins the
// corrected behaviour.

var regress2Fields = []validate.FieldConfig{
	{Name: "n", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "created_at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
}

// --- sum() no longer wraps int64 (finding #1) ---

func TestRegress2_SumOverflowPromotesToFloat(t *testing.T) {
	prog, err := CompileValue("sum(tags)", regress2Fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := prog.Eval(map[string]any{"tags": []any{int64(math.MaxInt64), int64(1)}})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	f, ok := got.(float64)
	if !ok {
		t.Fatalf("sum overflow = %v (%T); want a float64, not a wrapped int64", got, got)
	}
	if f <= 0 || f < 9.2e18 {
		t.Errorf("sum overflow = %v; want ~9.223e18 (promoted), not a negative wrap", f)
	}
}

func TestRegress2_SumStaysIntWhenItFits(t *testing.T) {
	prog, _ := CompileValue("sum(tags)", regress2Fields)
	got, err := prog.Eval(map[string]any{"tags": []any{int64(2), int64(3), int64(4)}})
	if err != nil {
		t.Fatal(err)
	}
	if got != int64(9) {
		t.Errorf("sum = %v (%T); want int64(9)", got, got)
	}
}

// --- NaN and ±Inf never leave a computed result (finding #2) ---

func TestRegress2_NonFiniteResultsResolveToNoValue(t *testing.T) {
	cases := []struct {
		expr string
		rec  map[string]any
	}{
		{"[price]+1", map[string]any{"price": math.Inf(1)}},
		{"[price]*[price]", map[string]any{"price": math.Inf(1)}},
		{"[price]-[price]", map[string]any{"price": math.Inf(1)}}, // Inf-Inf = NaN
		{"abs([price])", map[string]any{"price": math.Inf(-1)}},
		{"ceil([price])", map[string]any{"price": math.Inf(1)}},
		{"floor([price])", map[string]any{"price": math.Inf(1)}},
		{"[price]*[price]", map[string]any{"price": 1e308}}, // finite in, +Inf out
		{"[price]+1", map[string]any{"price": math.NaN()}},
	}
	for _, tt := range cases {
		t.Run(tt.expr, func(t *testing.T) {
			prog, err := CompileValue(tt.expr, regress2Fields)
			if err != nil {
				t.Fatalf("compile %q: %v", tt.expr, err)
			}
			got, err := prog.Eval(tt.rec)
			if !errors.Is(err, ErrNoValue) {
				t.Errorf("Eval(%q, %v) = %v, %v; want ErrNoValue (NaN/Inf must not leak)", tt.expr, tt.rec, got, err)
			}
		})
	}
}

func TestRegress2_NonFinitePredicateIsFalse(t *testing.T) {
	// A comparison whose operand computes to a non-finite number folds to false,
	// the same as any other no-value operand.
	prog, err := Compile("n < [price]*[price]", regress2Fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if prog.Match(map[string]any{"n": 1, "price": math.Inf(1)}) {
		t.Error("n < (Inf*Inf) matched; a non-finite operand must make the comparison false")
	}
}

// --- []byte is text, not a list of bytes (finding #4) ---

func TestRegress2_ByteSliceIsText(t *testing.T) {
	cases := []struct {
		name  string
		query string
		rec   map[string]any
		want  bool
	}{
		{"len counts runes not bytes", "len(name)=4", map[string]any{"name": []byte("café")}, true},
		{"not byte count", "len(name)=5", map[string]any{"name": []byte("café")}, false},
		{"equality as string", "name=café", map[string]any{"name": []byte("café")}, true},
		{"contains substring", `contains(name, "af")`, map[string]any{"name": []byte("café")}, true},
		{"wildcard on bytes", "name=caf*", map[string]any{"name": []byte("café")}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, regress2Fields)
			if err != nil {
				t.Fatalf("compile %q: %v", tt.query, err)
			}
			if got := prog.Match(tt.rec); got != tt.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tt.query, tt.rec, got, tt.want)
			}
		})
	}
}

// --- a panicking accessor is contained, not propagated (finding #6) ---

func TestRegress2_PanicInAccessorIsContained(t *testing.T) {
	boom := func(string) (any, bool) { panic("hostile accessor") }

	prog, err := Compile("n=5", regress2Fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// MatchFunc must not let the panic escape; a panicking evaluation is no match.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("MatchFunc propagated a panic: %v", r)
			}
		}()
		if prog.MatchFunc(boom) {
			t.Error("a panicking match counted as a match")
		}
	}()

	vprog, err := CompileValue("[n]+1", regress2Fields)
	if err != nil {
		t.Fatalf("compile value: %v", err)
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("EvalFunc propagated a panic: %v", r)
			}
		}()
		got, err := vprog.EvalFunc(boom)
		if !errors.Is(err, ErrNoValue) {
			t.Errorf("EvalFunc after panic = %v, %v; want ErrNoValue", got, err)
		}
	}()
}

// --- fields promoted through embedded structs resolve (finding #3) ---

type auditMeta struct {
	Dept    string `query:"dept"`
	Level   int    `query:"level"`
	private string `query:"ignored"` //nolint:unused // an unexported promoted tag must be skipped, not panic
}

type auditRec struct {
	auditMeta        // embedded by value
	*auditPtr        // embedded by pointer
	Name      string `query:"name"`
	Dept      string `query:"dept"` // shadows auditMeta.Dept (shallower wins)
}

type auditPtr struct {
	Region string `query:"region"`
}

func TestRegress2_EmbeddedStructFields(t *testing.T) {
	rec := auditRec{
		auditMeta: auditMeta{Dept: "deep", Level: 3},
		auditPtr:  &auditPtr{Region: "emea"},
		Name:      "jane",
		Dept:      "shallow",
	}

	// FieldsFromStruct surfaces promoted fields and the direct one, once each,
	// and never the unexported tagged field.
	names := map[string]bool{}
	for _, fc := range FieldsFromStruct(rec) {
		if names[fc.Name] {
			t.Errorf("FieldsFromStruct returned %q twice", fc.Name)
		}
		names[fc.Name] = true
	}
	for _, want := range []string{"name", "dept", "level", "region"} {
		if !names[want] {
			t.Errorf("FieldsFromStruct missing promoted field %q", want)
		}
	}
	if names["ignored"] {
		t.Error("FieldsFromStruct surfaced an unexported promoted field")
	}

	matches := map[string]bool{
		"level=3":      true,  // promoted by value
		"region=emea":  true,  // promoted by pointer
		"name=jane":    true,  // direct
		"dept=shallow": true,  // the direct field shadows the embedded one
		"dept=deep":    false, // the embedded one is hidden
	}
	for q, want := range matches {
		tp, err := CompileFor[auditRec](q)
		if err != nil {
			t.Fatalf("CompileFor(%q): %v", q, err)
		}
		if got := tp.MatchStruct(rec); got != want {
			t.Errorf("MatchStruct(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestRegress2_NilEmbeddedPointerIsMissing(t *testing.T) {
	// A nil embedded pointer makes its promoted fields missing, not a panic.
	rec := auditRec{Name: "jane"} // auditPtr is nil
	tp, err := CompileFor[auditRec]("region=emea")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("MatchStruct panicked on a nil embedded pointer: %v", r)
			}
		}()
		if tp.MatchStruct(rec) {
			t.Error("region=emea matched with a nil embedded pointer")
		}
	}()
}

// --- reversed literal range matches nothing (finding #5, behaviour pinned) ---

// A range whose bounds are in the wrong order (n:5..1) is not a compile error;
// it reads as n>=5 AND n<=1 and simply matches nothing. This pins that so the
// day it becomes a validation error is a deliberate change, visible here.
func TestRegress2_ReversedRangeMatchesNothing(t *testing.T) {
	prog, err := Compile("n:5..1", regress2Fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, v := range []int{0, 1, 3, 5, 9} {
		if prog.Match(map[string]any{"n": v}) {
			t.Errorf("n:5..1 matched n=%d; a reversed range matches nothing", v)
		}
	}
}

// --- day-literal comparison is on the UTC calendar (timezone, behaviour pinned) ---

// created_at=2026-09-28 reads the record time on the UTC calendar. A timestamp
// late on the 28th in a western zone is already the 29th in UTC, so it does not
// match the 28th. This pins the documented UTC contract.
func TestRegress2_DayLiteralUsesUTCCalendar(t *testing.T) {
	west := time.FixedZone("UTC-5", -5*3600)
	// 2026-09-28T23:00:00-05:00 == 2026-09-29T04:00:00Z.
	rec := map[string]any{"created_at": time.Date(2026, 9, 28, 23, 0, 0, 0, west)}

	eq, _ := Compile("created_at=2026-09-28", regress2Fields)
	if eq.Match(rec) {
		t.Error("a 28th-evening western time matched =2026-09-28; the UTC day is the 29th")
	}
	eq29, _ := Compile("created_at=2026-09-29", regress2Fields)
	if !eq29.Match(rec) {
		t.Error("the same time did not match =2026-09-29; it is the 29th in UTC")
	}
}
