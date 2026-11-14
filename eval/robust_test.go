package eval

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// Robustness tests: whatever a caller registers or hands in as a record, a
// compiled program answers or reports an error. It never panics and never
// gives different answers under concurrency.

var robustFields = []validate.FieldConfig{
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "count", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "ready", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
	{Name: "created_at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
	{Name: "ttl", Type: validate.TypeDuration, AllowedOps: validate.DurationOps},
	{Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
}

var robustQueries = []string{
	"name=a", "name!=a", "name=a*", "name=*a*", `name IN (a, b)`, "name NOT IN (a, b)",
	"total>1", "total:1..5", "total>=[count]*2", "count=3", "count>-1", "ready=true",
	"created_at>=2026-01-01", "created_at>now()-7d", "ttl>1h", "labels.team=core",
	"labels.example.com/team=core", "lower(name)=a", "len(name)>2", `contains(name, "a")`,
	"abs(total)>1", "round(total)=2", "year(created_at)=2026", "NOT name=a", "name=a OR total>1",
	`startsWith(labels.team, "c")`, "addDays(created_at, 1)>now()", "min(total, count)>0",
}

func TestMatch_HostileRecordValues(t *testing.T) {
	type odd struct{ X int }
	var nilPtr *odd
	var nilMap map[string]any
	cyclic := map[string]any{}
	cyclic["team"] = cyclic
	cyclic["labels"] = cyclic

	values := map[string]any{
		"nil":             nil,
		"nil pointer":     nilPtr,
		"nil map":         nilMap,
		"struct":          odd{1},
		"pointer":         &odd{1},
		"func":            func() {},
		"chan":            make(chan int),
		"slice":           []any{1, "a", nil},
		"typed slice":     []string{"a"},
		"bytes":           []byte("a"),
		"NaN":             math.NaN(),
		"+Inf":            math.Inf(1),
		"-Inf":            math.Inf(-1),
		"max uint64":      uint64(math.MaxUint64),
		"min int64":       int64(math.MinInt64),
		"complex":         complex(1, 2),
		"zero time":       time.Time{},
		"far time":        time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
		"huge duration":   time.Duration(math.MaxInt64),
		"empty string":    "",
		"invalid utf8":    "\xff\xfe",
		"long string":     strings.Repeat("a", 1<<16),
		"cyclic map":      cyclic,
		"map of non-any":  map[string]int{"team": 1},
		"map of any keys": map[any]any{"team": "core", 1: 2},
		"error value":     errors.New("x"),
	}

	for _, q := range robustQueries {
		prog, err := Compile(q, robustFields)
		if err != nil {
			t.Fatalf("Compile(%q): %v", q, err)
		}
		for name, v := range values {
			t.Run(q+"/"+name, func(t *testing.T) {
				record := map[string]any{
					"name": v, "total": v, "count": v, "ready": v,
					"created_at": v, "ttl": v, "labels": v,
				}

				first := prog.Match(record)
				second := prog.Match(record)

				if first != second && !strings.Contains(q, "now()") {
					t.Errorf("Match is not deterministic: %v then %v", first, second)
				}
			})
		}
	}
}

func TestEval_HostileRecordValues(t *testing.T) {
	exprs := []string{"[total]+1", "abs([total]-[count])", "min(len([name])+1, 9)", "[total]/[count]", "[total]*[total]", "abs([total])", "round([total]/3)",
		"len([name])", "upper([name])", "[created_at]+1d", "[count]%2", "min([total], [count])"}
	values := []any{nil, math.NaN(), math.Inf(1), int64(math.MinInt64), uint64(math.MaxUint64), "", "x",
		[]any{}, map[string]any{}, struct{}{}, time.Time{}, true, 0, 0.0, float32(1.5), int8(-1)}

	for _, src := range exprs {
		prog, err := CompileValue(src, robustFields)
		if err != nil {
			t.Fatalf("CompileValue(%q): %v", src, err)
		}
		for _, v := range values {
			t.Run(src, func(t *testing.T) {
				record := map[string]any{"name": v, "total": v, "count": v, "created_at": v}

				_, _ = prog.Eval(record) // any answer or error; a panic fails the test
			})
		}
	}
}

func TestWithFunctions_PanicIsContained(t *testing.T) {
	boom := Func{Name: "boom", Call: func(...any) (any, error) { panic("kaboom") }}
	index := Func{Name: "first", Call: func(args ...any) (any, error) { return args[3], nil }}
	empty := Func{Name: "empty"}
	opts := WithFunctions(boom, index, empty)

	tests := []struct {
		query string
	}{
		{"boom(name)=a"},
		{"name=boom()"},
		{"first(name)=a"},
		{"empty(name)=a"},
		{"lower(boom(name))=a"},
		{"NOT boom(name)=a OR name=a"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := Compile(tt.query, robustFields, opts)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}

			prog.Match(map[string]any{"name": "a"}) // must return, not panic
		})
	}

	t.Run("Eval reports an error", func(t *testing.T) {
		prog, err := CompileValue("boom([name])", robustFields, opts)
		if err != nil {
			t.Fatalf("CompileValue: %v", err)
		}

		_, err = prog.Eval(map[string]any{"name": "a"})

		if !errors.Is(err, ErrNoValue) {
			t.Errorf("Eval error = %v, want ErrNoValue", err)
		}
	})
}

func TestProgram_ConcurrentMatch(t *testing.T) {
	progs := make([]*Program, len(robustQueries))
	for i, q := range robustQueries {
		prog, err := Compile(q, robustFields)
		if err != nil {
			t.Fatalf("Compile(%q): %v", q, err)
		}
		progs[i] = prog
	}
	record := map[string]any{
		"name": "alpha", "total": 3.5, "count": 3, "ready": true,
		"created_at": time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), "ttl": 2 * time.Hour,
		"labels": map[string]any{"team": "core", "example.com/team": "core"},
	}
	want := make([]bool, len(progs))
	for i, p := range progs {
		want[i] = p.Match(record)
	}

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				for i, p := range progs {
					if got := p.Match(record); got != want[i] {
						t.Errorf("%q: concurrent Match = %v, want %v", robustQueries[i], got, want[i])
						return
					}
				}
			}
		}()
	}
	wg.Wait()
}
