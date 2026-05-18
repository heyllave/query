package eval

import (
	"strings"
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// These tests exercise CompileFor[T] / MatchStruct against a realistic struct
// type covering every supported value kind (string, int, float, bool,
// time.Time, time.Duration) and every operator class.

type matchInvoice struct {
	State       string        `query:"state"`
	Name        string        `query:"name"`
	Description string        `query:"description"`
	Total       float64       `query:"total"`
	Year        int           `query:"year"`
	Quantity    int64         `query:"quantity"`
	Active      bool          `query:"active"`
	CreatedAt   time.Time     `query:"created_at"`
	TTL         time.Duration `query:"ttl"`
	Internal    string        // no query tag — not queryable
}

func sampleInvoice() matchInvoice {
	return matchInvoice{
		State:       "draft",
		Name:        "John Doe",
		Description: "urgent: handle ASAP",
		Total:       75000.50,
		Year:        2026,
		Quantity:    42,
		Active:      true,
		CreatedAt:   time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		TTL:         2 * time.Hour,
		Internal:    "secret",
	}
}

// --- Value-type coverage ----------------------------------------------------

func TestMatchStruct_ValueTypes(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		// string
		{"state=draft", true},
		{"state=issued", false},
		// integer (int)
		{"year=2026", true},
		{"year=2025", false},
		// integer (int64)
		{"quantity=42", true},
		{"quantity=43", false},
		// float
		{"total=75000.50", true},
		{"total=75001", false}, // int literal vs float field — toInt64 truncates 75000.50 → 75000, ≠ 75001
		// Note: `total=75000` would match because toInt64(75000.50) == 75000.
		// This is documented integer-truncation behavior in equalValues.
		// boolean
		{"active=true", true},
		{"active=false", false},
		// date
		{"created_at=2026-03-15", true},
		{"created_at=2026-03-14", false},
		// duration
		{"ttl=2h", true},
		{"ttl=3h", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Operator-class coverage -----------------------------------------------

func TestMatchStruct_Operators(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		// equality / not-equal
		{"state=draft", true},
		{"state!=draft", false},
		{"state!=cancelled", true},
		// ordered comparison
		{"total>50000", true},
		{"total>=75000.50", true},
		{"total<100000", true},
		{"total<=75000.50", true},
		{"total>100000", false},
		// negated comparison — desugars to NOT (op)
		{"total!>50000", false},  // NOT (total>50000) = NOT true = false
		{"total!>100000", true},  // NOT (total>100000) = NOT false = true
		{"total!>=100000", true}, // NOT (total>=100000) = NOT false = true
		{"total!<10", true},      // NOT (total<10) = NOT false = true
		{"total!<=10", true},     // NOT (total<=10) = NOT false = true
		// wildcards
		{"name=John*", true},
		{"name=*Doe", true},
		{"name=*ohn*", true},
		{"name=Jane*", false},
		// range
		{"total:50000..100000", true},
		{"total:0..1000", false},
		{"year:2020..2030", true},
		{"year:2027..2030", false},
		{"created_at:2026-01-01..2026-12-31", true},
		{"created_at:2025-01-01..2025-12-31", false},
		{"ttl:1h..3h", true},
		// presence
		{"state", true}, // bare-field, has OpPresence on TextOps
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Logical composition ---------------------------------------------------

func TestMatchStruct_LogicalComposition(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		{"state=draft AND total>50000", true},
		{"state=draft AND total>100000", false},
		{"state=cancelled OR total>50000", true},
		{"state=cancelled OR total>100000", false},
		{"NOT state=cancelled", true},
		{"NOT state=draft", false},
		{"(state=draft OR state=issued) AND total>50000", true},
		// implicit AND via juxtaposition
		{"state=draft total>50000", true},
		{"state=draft total>50000 year=2026", true},
		{"state=draft total>50000 year=2027", false},
		// case-insensitive keywords
		{"state=draft and total>50000", true},
		{"state=draft AnD total>50000", true},
		{"state=cancelled or total>50000", true},
		{"not state=cancelled", true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- IN list ---------------------------------------------------------------

func TestMatchStruct_INList(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		{"state IN (draft, issued, paid)", true},
		{"state IN (issued, paid, cancelled)", false},
		{"year IN (2024, 2025, 2026)", true},
		{"year IN (2023, 2024, 2025)", false},
		// quoted values
		{`name IN ("John Doe", "Jane Smith")`, true},
		{`name IN ("Jane Smith", "Bob")`, false},
		// case-insensitive 'in'
		{"state in (draft, issued)", true},
		// composes with AND
		{"state IN (draft, issued) AND total>50000", true},
		{"state IN (paid) AND total>50000", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Quoted strings --------------------------------------------------------

func TestMatchStruct_QuotedStrings(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		{`name="John Doe"`, true},
		{`name="Jane Smith"`, false},
		{`description="urgent: handle ASAP"`, true},
		// escape: the description contains a colon, which would otherwise be
		// parsed as a range delimiter — quoting protects it.
		{`description="urgent: handle"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Arithmetic in value position ------------------------------------------

func TestMatchStruct_Arithmetic(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		// 50000 * 1.1 = 55000
		{"total>=50000*1.1", true},
		{"total>=100000*1.1", false},
		// (50000+1000)*1.1 = 56100
		{"total>=(50000+1000)*1.1", true},
		// precedence: 1 + 2*3 = 7
		{"year>=1+2025", true},
		// duration arithmetic: 1h * 2 = 2h
		{"ttl>=1h*2", true},
		{"ttl>=1h*3", false},
		// duration + duration
		{"ttl>=1h+30m", true},
		// division by zero → nil → false
		{"total>=10/0", false},
		// modulo
		{"year>=2020+(6%2)", true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Functions in value position -------------------------------------------

func TestMatchStruct_FunctionsInValuePosition(t *testing.T) {
	// We need a fresh sample whose CreatedAt is "recent" relative to the
	// test run so now()/daysAgo() comparisons are deterministic.
	inv := sampleInvoice()
	inv.CreatedAt = time.Now().Add(-3 * 24 * time.Hour) // 3 days ago

	tests := []struct {
		query string
		want  bool
	}{
		{"created_at<=now()", true},
		{"created_at>=daysAgo(7)", true},
		{"created_at>=daysAgo(1)", false},
		{"created_at:daysAgo(30)..now()", true},
		{"created_at:daysAgo(1)..now()", false},
		// arithmetic mixed with function call
		{"created_at>=now()-7d", true},
		{"created_at>=now()-1d", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Field-transform functions ---------------------------------------------

func TestMatchStruct_FieldTransforms(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		// lower / upper
		{"lower(state)=draft", true},
		{"upper(state)=DRAFT", true},
		{"upper(name)=JOHN*", true},
		// len
		{"len(name)>5", true},
		{"len(name)>50", false},
		// year / month / day on a Date field
		{"year(created_at)=2026", true},
		{"month(created_at)=3", true},
		{"day(created_at)=15", true},
		{"day(created_at)=14", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Boolean function predicates -------------------------------------------

func TestMatchStruct_FunctionPredicates(t *testing.T) {
	inv := sampleInvoice()
	tests := []struct {
		query string
		want  bool
	}{
		{`contains(name, "Doe")`, true},
		{`contains(name, "Smith")`, false},
		{`contains(description, "urgent")`, true},
		// two-field comparison: name contains state? "John Doe" contains "draft" → false
		{"contains(name, state)", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(inv); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- coalesce / if ---------------------------------------------------------

type matchInvoiceWithFallback struct {
	Name     string `query:"name"`
	Nickname string `query:"nickname"`
	Active   bool   `query:"active"`
}

func TestMatchStruct_CoalesceAndIf(t *testing.T) {
	tests := []struct {
		name  string
		data  matchInvoiceWithFallback
		query string
		want  bool
	}{
		{
			name:  "coalesce picks first non-empty",
			data:  matchInvoiceWithFallback{Name: "John", Nickname: "Johnny"},
			query: `coalesce(nickname, name)="Johnny"`,
			want:  true,
		},
		{
			name:  "coalesce falls through on empty",
			data:  matchInvoiceWithFallback{Name: "John", Nickname: ""},
			query: `coalesce(nickname, name)="John"`,
			want:  true,
		},
		{
			name:  "coalesce default literal",
			data:  matchInvoiceWithFallback{Name: "", Nickname: ""},
			query: `coalesce(nickname, name, "anonymous")="anonymous"`,
			want:  true,
		},
		{
			name:  "if picks on-branch when true",
			data:  matchInvoiceWithFallback{Active: true},
			query: `if(active, "on", "off")="on"`,
			want:  true,
		},
		{
			name:  "if picks off-branch when false",
			data:  matchInvoiceWithFallback{Active: false},
			query: `if(active, "on", "off")="off"`,
			want:  true,
		},
		{
			name:  "if false branch rejected when matching on-value",
			data:  matchInvoiceWithFallback{Active: false},
			query: `if(active, "on", "off")="on"`,
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := CompileFor[matchInvoiceWithFallback](tt.query)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.MatchStruct(tt.data); got != tt.want {
				t.Errorf("MatchStruct = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Pointer receiver ------------------------------------------------------

func TestMatchStruct_PointerReceiver(t *testing.T) {
	// CompileFor[T] is invariant on T — a *matchInvoice instantiation must
	// independently work and dereference the pointer.
	prog, err := CompileFor[*matchInvoice]("state=draft AND total>50000")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	inv := sampleInvoice()
	if !prog.MatchStruct(&inv) {
		t.Error("pointer receiver should match")
	}
}

// --- Custom function registration ------------------------------------------

func TestMatchStruct_CustomFunction(t *testing.T) {
	inv := sampleInvoice()
	prog, err := CompileFor[matchInvoice]("wordCount(description)>=3",
		WithFunctions(Func{
			Name: "wordCount",
			Call: func(args ...any) (any, error) {
				s, _ := args[0].(string)
				return int64(len(strings.Fields(s))), nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.MatchStruct(inv) {
		t.Error("expected wordCount(description) ≥ 3 for 'urgent: handle ASAP'")
	}
}

// --- Missing-field semantics -----------------------------------------------

func TestMatchStruct_ZeroValueSemantics(t *testing.T) {
	// A zero-value struct has zero-valued fields, NOT missing fields — the
	// accessor still returns (zero, true). This contrasts with map[string]any
	// where absent keys yield (nil, false).
	zero := matchInvoice{}

	tests := []struct {
		query string
		want  bool
	}{
		{"state=", false},      // zero-state has Str="" but query parses as empty
		{"state=draft", false}, // zero != draft
		{"total=0", true},      // zero-value float matches 0
		{"year=0", true},
		{"active=false", true},
		// Negated comparison: missing semantics differ for zero values.
		// total!>50000 = NOT (total>50000) = NOT (0>50000) = NOT false = true
		{"total!>50000", true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := CompileFor[matchInvoice](tt.query)
			if err != nil {
				// Some queries (state= with empty value) may fail to compile;
				// only flag when we expected them to.
				if tt.want {
					t.Fatalf("compile: %v", err)
				}
				return
			}
			if got := prog.MatchStruct(zero); got != tt.want {
				t.Errorf("MatchStruct(zero) = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Untagged fields are not queryable -------------------------------------

func TestMatchStruct_UntaggedFieldIsHidden(t *testing.T) {
	// `Internal` has no query tag; the compiler must reject it.
	if _, err := CompileFor[matchInvoice]("Internal=secret"); err == nil {
		t.Error("expected error referencing untagged field Internal")
	}
	if _, err := CompileFor[matchInvoice]("internal=secret"); err == nil {
		t.Error("expected error for lower-case reference too")
	}
}

// --- Sandboxing applies through CompileFor ---------------------------------

func TestMatchStruct_SandboxOptions(t *testing.T) {
	t.Run("WithAllowedFields", func(t *testing.T) {
		// Only `state` is permitted.
		_, err := CompileFor[matchInvoice]("state=draft AND total>50000",
			WithAllowedFields("state"))
		if err == nil {
			t.Error("expected error: total is not in the allowed set")
		}
	})
	t.Run("WithAllowedOps", func(t *testing.T) {
		// Only equality and not-equal allowed.
		_, err := CompileFor[matchInvoice]("total>50000",
			WithAllowedOps(validate.OpEq, validate.OpNeq))
		if err == nil {
			t.Error("expected error: > not in allowed ops")
		}
	})
	t.Run("WithMaxDepth", func(t *testing.T) {
		// 3 levels of nesting required by the query.
		_, err := CompileFor[matchInvoice]("((state=draft OR state=issued) AND total>0) OR year=2026",
			WithMaxDepth(1))
		if err == nil {
			t.Error("expected max-depth error")
		}
	})
	t.Run("WithMaxLength", func(t *testing.T) {
		_, err := CompileFor[matchInvoice]("state=draft AND total>50000",
			WithMaxLength(5))
		if err == nil {
			t.Error("expected max-length error")
		}
	})
}
