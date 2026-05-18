package eval

import (
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

var testFields = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "year", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "active", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
	{Name: "created_at", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	{Name: "ttl", Type: validate.TypeDuration, AllowedOps: validate.DurationOps, Nested: true},
	{Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
	{Name: "cluster", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "tire_size", Type: validate.TypeText, AllowedOps: append(validate.TextOps, validate.OpPresence)},
}

func TestCompile_Match(t *testing.T) {
	tests := []struct {
		query string
		data  map[string]any
		want  bool
	}{
		// Equality
		{"state=draft", map[string]any{"state": "draft"}, true},
		{"state=draft", map[string]any{"state": "published"}, false},
		{"state=draft", map[string]any{}, false},

		// Not equal
		{"state!=cancelled", map[string]any{"state": "draft"}, true},
		{"state!=cancelled", map[string]any{"state": "cancelled"}, false},

		// Integer comparison
		{"year>2020", map[string]any{"year": 2025}, true},
		{"year>2020", map[string]any{"year": 2020}, false},
		{"year>2020", map[string]any{"year": int64(2025)}, true},
		{"year>=2020", map[string]any{"year": 2020}, true},
		{"year<2025", map[string]any{"year": 2020}, true},
		{"year<=2025", map[string]any{"year": 2025}, true},

		// Decimal
		{"total>=50000", map[string]any{"total": 60000.0}, true},
		{"total>=50000", map[string]any{"total": 100.0}, false},

		// Boolean
		{"active=true", map[string]any{"active": true}, true},
		{"active=true", map[string]any{"active": false}, false},

		// Date
		{"created_at>=2026-01-01", map[string]any{"created_at": time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)}, true},
		{"created_at>=2026-01-01", map[string]any{"created_at": time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)}, false},
		{"created_at>=2026-01-01", map[string]any{"created_at": "2026-06-15"}, true},

		// Wildcard
		{"name=John*", map[string]any{"name": "John Doe"}, true},
		{"name=John*", map[string]any{"name": "Jane Doe"}, false},
		{"name=*ohn", map[string]any{"name": "John"}, true},
		{"name=*testing*", map[string]any{"name": "some testing here"}, true},

		// Presence
		{"tire_size", map[string]any{"tire_size": "225/45R17"}, true},
		{"tire_size", map[string]any{}, false},

		// AND
		{"state=draft AND year>2020", map[string]any{"state": "draft", "year": 2025}, true},
		{"state=draft AND year>2020", map[string]any{"state": "draft", "year": 2019}, false},
		{"state=draft AND year>2020", map[string]any{"state": "published", "year": 2025}, false},

		// OR
		{"state=draft OR state=issued", map[string]any{"state": "draft"}, true},
		{"state=draft OR state=issued", map[string]any{"state": "issued"}, true},
		{"state=draft OR state=issued", map[string]any{"state": "cancelled"}, false},

		// NOT
		{"NOT state=cancelled", map[string]any{"state": "draft"}, true},
		{"NOT state=cancelled", map[string]any{"state": "cancelled"}, false},

		// Grouped
		{"(state=draft OR state=issued) AND total>50000",
			map[string]any{"state": "draft", "total": 60000.0}, true},
		{"(state=draft OR state=issued) AND total>50000",
			map[string]any{"state": "draft", "total": 100.0}, false},

		// Nested fields
		{"labels.dev=jane", map[string]any{"labels.dev": "jane"}, true},
		{"labels.dev=jane", map[string]any{"labels.dev": "john"}, false},

		// Case-insensitive string match
		{"state=Draft", map[string]any{"state": "draft"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := Compile(tt.query, testFields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			got := prog.Match(tt.data)
			if got != tt.want {
				t.Errorf("Match(%v) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestCompile_Range(t *testing.T) {
	prog, err := Compile("created_at:2026-01-01..2026-03-31", testFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		date string
		want bool
	}{
		{"2026-02-15", true},
		{"2026-01-01", true},
		{"2026-03-31", true},
		{"2025-12-31", false},
		{"2026-04-01", false},
	}

	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			d, _ := time.Parse("2006-01-02", tt.date)
			got := prog.Match(map[string]any{"created_at": d})
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompile_MatchFunc(t *testing.T) {
	prog, err := Compile("state=draft", testFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]string{"state": "draft"}
	got := prog.MatchFunc(func(field string) (any, bool) {
		v, ok := data[field]
		return v, ok
	})
	if !got {
		t.Error("expected match")
	}
}

func TestCompile_Fields(t *testing.T) {
	prog, err := Compile("state=draft AND total>50000", testFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	fields := prog.Fields()
	if len(fields) != 2 {
		t.Errorf("got %d fields, want 2", len(fields))
	}
}

func TestCompile_StringRoundTrip(t *testing.T) {
	q := "state=draft AND total>50000"
	prog, err := Compile(q, testFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if prog.String() != q {
		t.Errorf("String: got %q, want %q", prog.String(), q)
	}
	if prog.Stringify() != q {
		t.Errorf("Stringify: got %q, want %q", prog.Stringify(), q)
	}
}

func TestCompile_Restrictions(t *testing.T) {
	textFields := []validate.FieldConfig{
		{Name: "a", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "b", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "c", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}

	tests := []struct {
		name   string
		query  string
		fields []validate.FieldConfig
		opts   []Option
	}{
		{
			name:   "allowed fields excludes total",
			query:  "state=draft AND total>50000",
			fields: testFields,
			opts:   []Option{WithAllowedFields("state")},
		},
		{
			name:   "allowed ops excludes >",
			query:  "year>2020",
			fields: testFields,
			opts:   []Option{WithAllowedOps(validate.OpEq, validate.OpNeq)},
		},
		{
			name:   "max depth exceeded",
			query:  "(a=1 OR b=2) AND c=3",
			fields: textFields,
			opts:   []Option{WithMaxDepth(2)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Compile(tt.query, tt.fields, tt.opts...); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestCompile_Errors(t *testing.T) {
	tests := []struct {
		name, query string
	}{
		{"parse error", "=invalid"},
		{"validation error", "nonexistent=value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Compile(tt.query, testFields); err == nil {
				t.Error("expected error")
			}
		})
	}
}

// --- Function call tests ---

func TestCompile_FuncCall_Builtin(t *testing.T) {
	tests := []struct {
		name  string
		query string
		data  map[string]any
		want  bool
	}{
		{"lower match uppercase", "lower(name)=john", map[string]any{"name": "JOHN"}, true},
		{"lower match mixed", "lower(name)=john", map[string]any{"name": "John"}, true},
		{"lower no match", "lower(name)=john", map[string]any{"name": "Jane"}, false},
		{"upper match lower", "upper(name)=JOHN", map[string]any{"name": "john"}, true},
		{"len greater", "len(name)>3", map[string]any{"name": "John"}, true},
		{"len not greater", "len(name)>3", map[string]any{"name": "Jo"}, false},
		{
			name:  "contains match",
			query: "contains(name, cluster)",
			data:  map[string]any{"name": "demo-cluster-1", "cluster": "cluster"},
			want:  true,
		},
		{
			name:  "contains no match",
			query: "contains(name, cluster)",
			data:  map[string]any{"name": "production", "cluster": "demo"},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, testFields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Match(tt.data); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompile_FuncCall_CustomFunction(t *testing.T) {
	double := Func{
		Name: "double",
		Call: func(args ...any) (any, error) {
			return toInt64(args[0]) * 2, nil
		},
	}
	prog, err := Compile("double(year)>4040", testFields, WithFunctions(double))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tests := []struct {
		name string
		data map[string]any
		want bool
	}{
		{"match", map[string]any{"year": 2025}, true},
		{"no match", map[string]any{"year": 2000}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prog.Match(tt.data); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompile_FuncCall_NoBuiltins(t *testing.T) {
	// With no builtins, lower() isn't registered, so the function call
	// can't resolve — the match should return false.
	prog, err := Compile("lower(name)=john", testFields, WithNoBuiltins())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if prog.Match(map[string]any{"name": "JOHN"}) {
		t.Error("expected no match: lower() not registered")
	}
}

// --- Struct binding tests ---

type testInvoice struct {
	State     string    `query:"state"`
	Total     float64   `query:"total"`
	Year      int       `query:"year"`
	Active    bool      `query:"active"`
	CreatedAt time.Time `query:"created_at"`
	Internal  string    // no query tag — not queryable
}

func TestCompileFor_MatchStruct(t *testing.T) {
	tests := []struct {
		name string
		data testInvoice
		want bool
	}{
		{"match", testInvoice{State: "draft", Total: 60000}, true},
		{"total too low", testInvoice{State: "draft", Total: 100}, false},
	}
	prog, err := CompileFor[testInvoice]("state=draft AND total>50000")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prog.MatchStruct(tt.data); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompileFor_Errors(t *testing.T) {
	tests := []struct {
		name, query string
	}{
		{"type mismatch", "year=notanumber"},
		{"untagged field", "Internal=secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CompileFor[testInvoice](tt.query); err == nil {
				t.Error("expected error")
			}
		})
	}
}
