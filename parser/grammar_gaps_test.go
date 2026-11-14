package parser

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/heyllave/query/ast"
)

// TestParse_Reprint pins how inputs that used to be misread now parse, by the
// canonical text ast.String prints for them.
func TestParse_Reprint(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		// NOT IN used to read as `x AND NOT=a`.
		{"not in", "x NOT IN (a, b)", "x!=a AND x!=b"},
		{"not in lower case", "x not in (a)", "x!=a"},
		{"not in under or", "y OR x NOT IN (a, b)", "y OR x!=a AND x!=b"},
		{"negated not in", "NOT x NOT IN (a)", "NOT x!=a"},
		{"not without in is a negation", "x NOT y", "x AND NOT y"},
		{"not as a field before in", "not IN (a)", `["not"]=a`},
		// IN on a function used to be "reserved word IN cannot name a function".
		{"function in", "lower(name) IN (ab, cd)", "lower(name)=ab OR lower(name)=cd"},
		{"function not in", "len(name) NOT IN (1, 2)", "len(name)!=1 AND len(name)!=2"},
		// A term followed by a group used to be a call to the term.
		{"term then group", "urgent (state=open OR state=new)", "urgent AND (state=open OR state=new)"},
		{"call needs no space", "urgent(state)", "urgent(state)"},
		{"call then group", "f(a) (b)", "f(a) AND (b)"},
		{"in list may follow a space", "x IN (a)", "x=a"},
		// A Unicode space separates terms like an ASCII one.
		{"no-break space between terms", "a b", "a AND b"},
		{"ideographic space between terms", "a　b", "a AND b"},
		{"no-break space ends a value", "name=x n=1", "name=x AND n=1"},
		{"no-break space in a quoted name stays quoted", "[\"a b\"]=1", "[\"a b\"]=1"},
		{"no-break space in a quoted value stays quoted", "name=\"a b\"", "name=\"a b\""},
		// A dotted digit run is an address or version, not a number.
		{"cidr is one string", "ip=10.0.0.1/24", `ip="10.0.0.1/24"`},
		{"float still divides", "n>1.5/2", "n>1.5/2"},
		// Arithmetic in arguments and signed operands.
		{"argument opening with a bracketed field", "n>abs([a]-[b])", "n>abs(([a]-[b]))"},
		{"argument opening with a call", "n>min(len(a)+1, 9)", "n>min((len(a)+1), 9)"},
		{"field-side argument arithmetic", "round([a]/3)=2", "round(([a]/3))=2"},
		{"arithmetic argument", "d>addDays(d, 1-8)", "d>addDays(d, (1-8))"},
		{"spaced arithmetic argument", "d>addDays(d, 2 * 3 + 1)", "d>addDays(d, (2*3+1))"},
		{"arithmetic argument in field position", "addDays(d, 1-8)>d", "addDays(d, (1-8))>d"},
		{"signed argument", "d>addDays(d, -7)", "d>addDays(d, -7)"},
		{"signed operand after operator", "n>10*-3", "n>10*(-3)"},
		{"double minus", "n>10--3", "n>10-(-3)"},
		{"bracketed field argument in value position", "n>1+abs([cpu-limit])", "n>1+abs(cpu-limit)"},
		{"quoted bracketed field argument", `n>abs(["cpu limit"])`, `n>abs(["cpu limit"])`},
		// A string argument written as a group must not print as a field.
		{"grouped string argument", "string((draft))=x", `string("draft")=x`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			expr, err := Parse(tt.query, 256)

			// Assert
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.query, err)
			}
			got := ast.String(expr)
			if got != tt.want {
				t.Errorf("Parse(%q) prints %q, want %q", tt.query, got, tt.want)
			}
			again, err := Parse(got, 0)
			if err != nil {
				t.Fatalf("reparse %q: %v", got, err)
			}
			if s := ast.String(again); s != got {
				t.Errorf("%q reprints as %q", got, s)
			}
		})
	}
}

func TestParse_GapErrors(t *testing.T) {
	tests := []struct {
		name  string
		query string
		code  Code
	}{
		{"not in without a list", "x NOT IN y", CodeExpectedInList},
		{"empty not in", "x NOT IN ()", CodeEmptyInList},
		{"in cannot name a function", "in (a)", CodeExpectedField},
		{"compound duration is not a literal", "d>now()+1h30m", CodeUnexpected},
		{"duration out of range", "ttl>99999999999999999999d", CodeInvalidDuration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := Parse(tt.query, 256)

			// Assert
			errs := Errors(err)
			if len(errs) == 0 {
				t.Fatalf("Parse(%q) error = %v, want code %q", tt.query, err, tt.code)
			}
			if errs[0].Code != tt.code {
				t.Errorf("Parse(%q) code = %q (%s), want %q", tt.query, errs[0].Code, errs[0].Message, tt.code)
			}
		})
	}
}

// A failed term is reported once, not once per attempt to continue past it.
func TestParse_ErrorReportedOnce(t *testing.T) {
	for _, q := range []string{"in(a)", "1.2.3", "=x y"} {
		t.Run(q, func(t *testing.T) {
			_, err := Parse(q, 256)
			seen := map[string]bool{}
			for _, e := range Errors(err) {
				if seen[e.Error()] {
					t.Errorf("Parse(%q) reports %q twice", q, e.Error())
				}
				seen[e.Error()] = true
			}
			if len(seen) == 0 {
				t.Fatalf("Parse(%q) error = %v, want a parse error", q, err)
			}
		})
	}
}

// The parser recurses once per nesting level; MaxNesting keeps a query with
// the length limit disabled from exhausting the stack.
func TestParse_NestingBounded(t *testing.T) {
	shapes := map[string]func(n int) string{
		"groups":       func(n int) string { return strings.Repeat("(", n) + "a" + strings.Repeat(")", n) },
		"negations":    func(n int) string { return strings.Repeat("NOT ", n) + "a" },
		"value groups": func(n int) string { return "n=" + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) },
		"calls":        func(n int) string { return strings.Repeat("f(", n) + "a" + strings.Repeat(")", n) },
		"value calls":  func(n int) string { return "x=" + strings.Repeat("f(", n) + "a" + strings.Repeat(")", n) },
		"selectors":    func(n int) string { return strings.Repeat("a@(", n) + "a" + strings.Repeat(")", n) },
	}
	for name, build := range shapes {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(build(MaxNesting), 0); err != nil {
				t.Errorf("%d levels: %v, want nil", MaxNesting, err)
			}
			for _, n := range []int{MaxNesting + 1, 100_000} {
				_, err := Parse(build(n), 0)
				errs := Errors(err)
				if len(errs) != 1 || errs[0].Code != CodeNestingTooDeep {
					t.Errorf("%d levels: error = %v, want one %q", n, err, CodeNestingTooDeep)
				}
			}
		})
	}
}

func TestParseDuration_Bounds(t *testing.T) {
	tests := []struct {
		in     string
		want   time.Duration
		wantOK bool
	}{
		{"106751d", 106751 * 24 * time.Hour, true},
		{"106752d", 0, false}, // one day past what time.Duration holds
		{"99999999999999999999w", 0, false},
		{"0m", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDuration(tt.in)
			if (err == nil) != tt.wantOK || got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, %v; want %v, ok=%v", tt.in, got, err, tt.want, tt.wantOK)
			}
			var pe *Error
			if err != nil && (!errors.As(err, &pe) || pe.Code != CodeInvalidDuration) {
				t.Errorf("ParseDuration(%q) error = %v, want code %q", tt.in, err, CodeInvalidDuration)
			}
		})
	}
}
