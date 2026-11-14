package eval

import (
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

var fuzzFields = []validate.FieldConfig{
	{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "n", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "d", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	{Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
	{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "dur", Type: validate.TypeDuration, AllowedOps: validate.DurationOps},
	{Name: "active", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
}

// fuzzRecords are the shapes a record can take: typed values, strings where
// numbers are expected, nested maps, lists of maps and scalars, nils.
var fuzzRecords = []map[string]any{
	{"price": 3.5, "n": 4, "name": "john", "d": time.Now(), "labels": map[string]any{"dev": "x"},
		"tags": []any{map[string]any{"name": "a"}, "b", 3}, "dur": time.Hour, "active": true},
	{"price": "3.5", "n": "four", "name": 7, "d": "yesterday", "labels.dev": "x", "tags": "b", "dur": "2d", "active": "yes"},
	{"price": uint8(3), "n": int16(-4), "name": nil, "d": nil, "tags": []string{}, "dur": 0, "active": 0},
	{},
}

// FuzzCompileMatch checks that any query that compiles can be matched
// against every record shape without panicking, and that the compiled
// program prints to a query that compiles again.
func FuzzCompileMatch(f *testing.F) {
	for _, s := range []string{
		"price>3", "n=5", "name=jo*", "name!=jo*", "len(name)>3", "labels.dev=x",
		"tags@(name=a)", "tags@all(name=a)", "tags@none(name=a)", "tags@first",
		"d>=now()-7d", "dur>1h", "active=true", "lower(name)=john", "NOT price<1",
		"n:1..5", "n IN (-1, 4)", "name IN (jo*, x)", "addBusinessDays(d, 3)>d",
		"coalesce(name, \"x\")=x", `if(active, 1, 0)=1`, "n>=2*abs(price)", "lower(0)=*",
		"name=*", "count(tags)>0", "sum(tags)>0", "string(d)=*2026*", "int(name)>1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		prog, err := Compile(s, fuzzFields, WithMaxLength(0))
		if err != nil {
			return
		}
		for _, rec := range fuzzRecords {
			prog.Match(rec)
		}
		if _, err := Compile(prog.Stringify(), fuzzFields, WithMaxLength(0)); err != nil {
			t.Fatalf("Compile(%q) printed as %q, which does not compile: %v", s, prog.Stringify(), err)
		}
	})
}

// FuzzCompileValue does the same for value expressions.
func FuzzCompileValue(f *testing.F) {
	for _, s := range []string{
		"1+2", "now()-7d", "upper(name)", "(1+2)*3", "1/0", "len(tags)", "[price]*2",
		"9223372036854775807+1", "round(1.5, 400)", "sum(tags)", "avg(tags)", "min(1, 2.5)",
		"string(now())", "int(\"12\")", "float(name)", "first(tags)", "[n]%0",
		"string((A))", "abs(1-8)", "10*-3", "abs(-9223372036854775808)",
		"abs([n]-[price])", "(0%[price])", "2*[price]-[n]", "abs(([n]))", "min(len([name])+1, 9)", "round([price]/3)", "lower(labels.team)", "len(labels.example.com/team)+1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		prog, err := CompileValue(s, fuzzFields, WithMaxLength(0))
		if err != nil {
			return
		}
		for _, rec := range fuzzRecords {
			_, _ = prog.Eval(rec)
		}
		if _, err := CompileValue(prog.Stringify(), fuzzFields, WithMaxLength(0)); err != nil {
			t.Fatalf("CompileValue(%q) printed as %q, which does not compile: %v", s, prog.Stringify(), err)
		}
	})
}
