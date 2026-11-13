package validate

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

var optFields = []FieldConfig{
	{Name: "state", Type: TypeText, AllowedOps: TextOps},
	{Name: "secret", Type: TypeText, AllowedOps: []Op{OpEq}},
	{Name: "total", Type: TypeDecimal, AllowedOps: NumericOps},
	{Name: "count", Type: TypeInteger, AllowedOps: NumericOps},
	{Name: "created", Type: TypeDate, AllowedOps: DateOps},
	{Name: "updated", Type: TypeDatetime, AllowedOps: DateOps},
	{Name: "active", Type: TypeBoolean, AllowedOps: BoolOps},
	{Name: "ttl", Type: TypeDuration, AllowedOps: DurationOps},
	{Name: "labels", Type: TypeText, AllowedOps: TextOps, Nested: true},
}

// hook resolves every optFields entry, as a tenant hook would.
type hook struct{}

func (hook) GetFieldConfig(n string) (FieldConfig, bool) {
	for _, f := range optFields {
		if f.Name == n {
			return f, true
		}
	}
	return FieldConfig{}, false
}
func (hook) ValidateCustomRules(ast.Expression) error { return nil }

func mustParse(t *testing.T, q string) ast.Expression {
	t.Helper()
	expr, err := parser.Parse(q, 0)
	if err != nil {
		t.Fatalf("Parse(%q): %v", q, err)
	}
	return expr
}

func TestValidate_Options_Restrictions(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		opts     []Option
		wantKind ErrorKind
		wantOK   bool
	}{
		{"allowed field passes", "state=draft", []Option{WithAllowedFields("state")}, 0, true},
		{"other field is unknown", "total>1", []Option{WithAllowedFields("state")}, ErrFieldNotFound, false},
		{"restriction binds a custom validator", "secret=x", []Option{WithAllowedFields("state"), WithCustomValidator(hook{})}, ErrFieldNotFound, false},
		{"restriction reaches function arguments", "lower(secret)=x", []Option{WithAllowedFields("state")}, ErrFieldNotFound, false},
		{"restriction reaches field refs", "total>[count]", []Option{WithAllowedFields("total")}, ErrFieldNotFound, false},
		{"nested path allowed by its root", "labels.dev=x", []Option{WithAllowedFields("labels")}, 0, true},
		{"nested path blocked without its root", "labels.dev=x", []Option{WithAllowedFields("state")}, ErrFieldNotFound, false},
		{"allowed op passes", "state=draft", []Option{WithAllowedOps(OpEq)}, 0, true},
		{"other op refused", "total>1", []Option{WithAllowedOps(OpEq)}, ErrOperatorNotAllowed, false},
		{"op restriction binds a custom validator", "total>1", []Option{WithAllowedOps(OpEq), WithCustomValidator(hook{})}, ErrOperatorNotAllowed, false},
		{"op restriction through a function", "len(state)>1", []Option{WithAllowedOps(OpEq)}, ErrOperatorNotAllowed, false},
		{"wildcard counts as its own op", "state=dr*", []Option{WithAllowedOps(OpEq)}, ErrOperatorNotAllowed, false},
		{"presence counts as its own op", "state", []Option{WithAllowedOps(OpEq)}, ErrOperatorNotAllowed, false},
		{"range counts as its own op", "total:1..2", []Option{WithAllowedOps(OpEq, OpNeq)}, ErrOperatorNotAllowed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(optFields, tt.opts...).Validate(mustParse(t, tt.query))
			if tt.wantOK {
				if err != nil {
					t.Fatalf("Validate(%q) error = %v, want nil", tt.query, err)
				}
				return
			}
			var list ErrorList
			if !errors.As(err, &list) || len(list) == 0 {
				t.Fatalf("Validate(%q) error = %v, want an ErrorList", tt.query, err)
			}
			if list[0].Kind != tt.wantKind {
				t.Errorf("Validate(%q) kind = %v, want %v (%v)", tt.query, list[0].Kind, tt.wantKind, list[0].Message)
			}
		})
	}
}

func TestValidate_ThroughFunction(t *testing.T) {
	tests := []struct {
		query  string
		wantOK bool
	}{
		{"lower(state)=draft", true},
		{"lower(state)=dr*", true},
		{"len(state)>5", true},       // ordering through a function on a text field
		{"len(state):1..5", true},    // range too
		{"trim(secret)=a*", false},   // secret forbids wildcards
		{"trim(secret)!=a", false},   // and !=
		{"lower(secret)=a", true},    // = is fine
		{"abs(total)>1", true},       // total allows >
		{"abs(count)>1", true},       // count allows >
		{"year(created)>2020", true}, // created allows >
		{"string(active)>a", true},   // boolean: ordering through a function allowed
		{"len(nope)>1", false},       // unknown field inside the call
		{"len(state)>5 AND trim(secret)=x", true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			err := New(optFields).Validate(mustParse(t, tt.query))
			if (err == nil) != tt.wantOK {
				t.Errorf("Validate(%q) error = %v, wantOK %v", tt.query, err, tt.wantOK)
			}
		})
	}
}

func TestValidate_CoercesLiterals(t *testing.T) {
	tests := []struct {
		query    string
		wantType ast.ValueType
		wantStr  string
	}{
		{"state=90210", ast.ValueString, "90210"},
		{"state=00123", ast.ValueString, "00123"},
		{"state=1.5", ast.ValueString, "1.5"},
		{"state=true", ast.ValueString, "true"},
		{"state=2026-01-01", ast.ValueString, "2026-01-01"},
		{"state=1d", ast.ValueString, "1d"},
		{"state=draft", ast.ValueString, "draft"},
		{`created="2026-01-01"`, ast.ValueDate, ""},
		{`updated="2026-01-01T10:00:00Z"`, ast.ValueDate, ""},
		{`updated="2026-01-01 10:00:00"`, ast.ValueDate, ""},
		{"updated=2026-01-01T10:00:00Z", ast.ValueDate, ""},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			expr := mustParse(t, tt.query)
			if err := New(optFields).Validate(expr); err != nil {
				t.Fatalf("Validate(%q): %v", tt.query, err)
			}
			q := expr.(*ast.QualifierExpr)
			if q.Value.Type != tt.wantType {
				t.Errorf("value type = %v, want %v", q.Value.Type, tt.wantType)
			}
			if tt.wantStr != "" && q.Value.Str != tt.wantStr {
				t.Errorf("value str = %q, want %q", q.Value.Str, tt.wantStr)
			}
			if tt.wantType == ast.ValueDate && q.Value.Date.IsZero() {
				t.Error("coerced date is zero")
			}
		})
	}
}

func TestValidate_TypeMismatches(t *testing.T) {
	tests := []struct {
		query string
		want  string // substring of the message
	}{
		{"state=10-3", "arithmetic"},
		{"active=1+1", "arithmetic"},
		{`created="not a date"`, "not compatible"},
		{"count=1.5", "not compatible"},
		{"count=abc", "not compatible"},
		{"active=yes", "not compatible"},
		{"ttl=5", "not compatible"},
		{"total:1..abc", "range end"},
		{"total=now()", ""},   // dynamic: accepted
		{"total>[count]", ""}, // field ref: accepted
		{"total>1+1", ""},     // arithmetic on a number: accepted
		{"created>now()-7d", ""},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			err := New(optFields).Validate(mustParse(t, tt.query))
			if tt.want == "" {
				if err != nil {
					t.Errorf("Validate(%q) error = %v, want nil", tt.query, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate(%q) error = %v, want one containing %q", tt.query, err, tt.want)
			}
		})
	}
}

func TestValidate_ConcurrentUse(t *testing.T) {
	v := New(optFields, WithAllowedOps(OpEq, OpNeq))
	good := mustParse(t, "state=draft AND labels.dev=x")
	bad := mustParse(t, "total>1 AND nope=1")
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := v.Validate(good); err != nil {
				t.Errorf("good query failed: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			var list ErrorList
			if err := v.Validate(bad); !errors.As(err, &list) || len(list) != 2 {
				t.Errorf("bad query errors = %v, want 2", err)
			}
		}()
	}
	wg.Wait()
}

func TestValidate_ValueExpr(t *testing.T) {
	expr, err := parser.ParseValue("upper(state)+[total]", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := New(optFields).Validate(expr); err != nil {
		t.Errorf("value expression over declared fields: %v", err)
	}
	expr, _ = parser.ParseValue("upper(nope)", 0)
	if err := New(optFields).Validate(expr); err == nil {
		t.Error("value expression over an unknown field validated")
	}
	if err := New(optFields).Validate(nil); err != nil {
		t.Errorf("nil expression: %v", err)
	}
}

func TestErrorKind_CodeAndError(t *testing.T) {
	for kind, code := range map[ErrorKind]string{
		ErrFieldNotFound: "fieldNotFound", ErrOperatorNotAllowed: "operatorNotAllowed",
		ErrTypeMismatch: "typeMismatch", ErrCustomRule: "customRule", ErrorKind(99): "customRule",
	} {
		if got := kind.Code(); got != code {
			t.Errorf("%v.Code() = %q, want %q", kind, got, code)
		}
	}
	e := &Error{Message: "boom", Kind: ErrFieldNotFound, Field: "x"}
	if got := e.Error(); !strings.Contains(got, "boom") {
		t.Errorf("Error() = %q", got)
	}
	if got := (ErrorList{}).Error(); got != "no errors" {
		t.Errorf("empty ErrorList.Error() = %q", got)
	}
	if got := (ErrorList{e, e}).Error(); !strings.Contains(got, "; ") {
		t.Errorf("two-error ErrorList.Error() = %q", got)
	}
}
