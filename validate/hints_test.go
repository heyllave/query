package validate

import (
	"errors"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

var hintFields = []FieldConfig{
	{Name: "name", Type: TypeText, AllowedOps: TextOps},
	{Name: "state", Type: TypeText, AllowedOps: []Op{OpEq, OpNeq}},
	{Name: "total", Type: TypeDecimal, AllowedOps: NumericOps},
	{Name: "base", Type: TypeDecimal, AllowedOps: NumericOps},
	{Name: "replicas", Type: TypeInteger, AllowedOps: NumericOps},
	{Name: "created_at", Type: TypeDatetime, AllowedOps: DateOps},
	{Name: "active", Type: TypeBoolean, AllowedOps: BoolOps},
	{Name: "ttl", Type: TypeDuration, AllowedOps: DurationOps},
	{Name: "tags", Type: TypeText, AllowedOps: TextOps},
	{Name: "labels", Type: TypeText, AllowedOps: TextOps, Nested: true},
}

func validationErrors(t *testing.T, v *Validator, query string) ErrorList {
	t.Helper()
	expr, err := parser.Parse(query, 0)
	if err != nil {
		t.Fatalf("Parse(%q): %v", query, err)
	}
	var list ErrorList
	if err := v.Validate(expr); err != nil && !errors.As(err, &list) {
		t.Fatalf("Validate(%q) error is not an ErrorList: %v", query, err)
	}
	return list
}

func TestValidate_Hints(t *testing.T) {
	tests := []struct {
		name  string
		query string
		kind  ErrorKind
		hint  string
	}{
		// Unknown fields.
		{"swapped letters", "anme=x", ErrFieldNotFound, `did you mean "name"?`},
		{"missing letter", "totl>5", ErrFieldNotFound, `did you mean "total"?`},
		{"wrong case", "Total>5", ErrFieldNotFound, `did you mean "total"?`},
		{"typo in a nested name", "lables.team=x", ErrFieldNotFound, `did you mean "labels.team"?`},
		{"typo in a bracketed field", "total>[bsae]", ErrFieldNotFound, `did you mean "base"?`},
		{"subtraction read as a name", "total-base>3", ErrFieldNotFound, "[total]-[base]"},
		{"division read as a name", "total/2>3", ErrFieldNotFound, "[total]/2"},

		// Names inside functions.
		{"text passed bare", "contains(tags, urgent)", ErrFieldNotFound, `put it in quotes: "urgent"`},
		{"typo inside a function", "lower(anme)=x", ErrFieldNotFound, `did you mean "name"?`},
		{"arithmetic inside a function", "abs(total-base)>3", ErrFieldNotFound, "abs([total]-[base])"},

		// Stray words.
		{"is null", "name IS NULL", ErrFieldNotFound, "NOT field"},
		{"order by", "name=x ORDER BY total", ErrFieldNotFound, "only conditions"},
		{"contains as a word", "name contains john", ErrFieldNotFound, "name=*john*"},
		{"not after the field", "name NOT=x", ErrFieldNotFound, "NOT goes before"},
		{"spelled-out unit", "created_at>now() - 7 days", ErrFieldNotFound, "7d"},
		{"unquoted second word", "name=john smith", ErrFieldNotFound, "in quotes if it has spaces"},

		// Values that do not fit.
		{"field without brackets", "total>base", ErrTypeMismatch, "[base]"},
		{"word for a number", "total=abc", ErrTypeMismatch, "42, -3 or 1.5"},
		{"thousands separator", "total>1,000", ErrTypeMismatch, "without separators"},
		{"currency", "total>$5", ErrTypeMismatch, "without a symbol"},
		{"fraction for an integer", "replicas>2.5", ErrTypeMismatch, "whole numbers"},
		{"word for a boolean", "active=yes", ErrTypeMismatch, "true or false"},
		{"word for a date", "created_at>yesterday", ErrTypeMismatch, "now()-7d"},
		{"compound duration", "ttl>1h30m", ErrTypeMismatch, "90m rather than 1h30m"},
		{"fractional duration", "ttl>1.5h", ErrTypeMismatch, "90m rather than 1.5h"},
		{"word for a duration", "ttl=soon", ErrTypeMismatch, "30m, 4h, 7d, 2w"},
		{"chained comparison", "total>5>3", ErrTypeMismatch, "AND or OR"},

		// Operators the field does not allow.
		{"ordering on equality-only", "state>a", ErrOperatorNotAllowed, "state allows: =, !="},
		{"wildcard on equality-only", "state=dr*", ErrOperatorNotAllowed, "* pattern; it allows: =, !="},
		{"range on equality-only", "state:a..b", ErrOperatorNotAllowed, "range; it allows: =, !="},
		{"field alone", "state", ErrOperatorNotAllowed, "on its own tests that it has a value"},
		{"ordering on text", "name>a", ErrOperatorNotAllowed, "a * pattern, the field alone"},
	}
	v := New(hintFields)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			list := validationErrors(t, v, tt.query)

			// Assert
			for _, e := range list {
				if e.Kind == tt.kind && strings.Contains(e.Hint, tt.hint) {
					return
				}
			}
			t.Errorf("Validate(%q) = %v, want a %s error with a hint containing %q", tt.query, list, tt.kind.Code(), tt.hint)
		})
	}
}

func TestValidate_HintListsFieldsWhenThereAreFew(t *testing.T) {
	// Arrange
	v := New(hintFields[:3])

	// Act
	list := validationErrors(t, v, "unrelated=1")

	// Assert
	if len(list) != 1 || list[0].Hint != "the fields are: name, state, total" {
		t.Errorf("hint = %v, want the three field names", list)
	}
}

func TestValidate_NoHintWhenNothingIsClose(t *testing.T) {
	// Arrange
	v := New(hintFields)

	// Act
	list := validationErrors(t, v, "unrelated=1")

	// Assert
	if len(list) != 1 || list[0].Hint != "" {
		t.Errorf("hint = %v, want none: no field is close and the list is long", list)
	}
}

// A hint must not name a field the caller is not allowed to use, or a
// restriction could be read back out of the error messages.
func TestValidate_HintsDoNotRevealRestrictedFields(t *testing.T) {
	fields := []FieldConfig{
		{Name: "name", Type: TypeText, AllowedOps: TextOps},
		{Name: "salary", Type: TypeDecimal, AllowedOps: NumericOps},
	}
	tests := []struct {
		name  string
		v     *Validator
		query string
	}{
		{"typo of a field outside WithAllowedFields", New(fields, WithAllowedFields("name")), "salry>5"},
		{"the hidden field itself", New(fields, WithAllowedFields("name")), "salary>5"},
		{"hidden field named as a value", New(fields, WithAllowedFields("name")), "name=salary"},
		{"typo inside a function", New(fields, WithAllowedFields("name")), "abs(salry)>5"},
		{"field denied by a custom validator", New(fields, WithCustomValidator(denying{"salary"})), "salry>5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			list := validationErrors(t, tt.v, tt.query)

			// Assert
			for _, e := range list {
				if strings.Contains(e.Hint, "salary") {
					t.Errorf("Validate(%q) hint names the restricted field: %q", tt.query, e.Hint)
				}
			}
		})
	}
}

// denying is a custom validator that hides the named fields and accepts the
// rest as text.
type denying []string

func (d denying) GetFieldConfig(name string) (FieldConfig, bool) {
	for _, hidden := range d {
		if name == hidden {
			return FieldConfig{}, false
		}
	}
	if name == "name" || name == "salary" {
		return FieldConfig{Name: name, Type: TypeText, AllowedOps: TextOps}, true
	}
	return FieldConfig{}, false
}

func (denying) ValidateCustomRules(ast.Expression) error { return nil }

func TestValidate_OperatorHintFollowsWithAllowedOps(t *testing.T) {
	// Arrange: total allows every numeric operator, the caller only equality.
	v := New(hintFields, WithAllowedOps(OpEq, OpNeq))

	// Act
	list := validationErrors(t, v, "total>5")

	// Assert
	if len(list) != 1 || !strings.HasSuffix(list[0].Hint, "total allows: =, !=") {
		t.Errorf("hint = %v, want only the operators WithAllowedOps leaves", list)
	}
}

func TestValidate_FunctionHints(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		query   string
		hint    string
	}{
		{"near miss", []string{"lower", "now"}, "lowr(name)=x", `did you mean "lower"?`},
		{"short list", []string{"lower", "now"}, "upper(name)=X", "the functions allowed here are: lower, now"},
		{"none allowed", nil, "upper(name)=X", "no functions are allowed here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			v := New(hintFields, WithAllowedFunctions(tt.allowed...))

			// Act
			list := validationErrors(t, v, tt.query)

			// Assert
			for _, e := range list {
				if e.Kind == ErrFunctionNotAllowed && e.Hint == tt.hint {
					return
				}
			}
			t.Errorf("Validate(%q) = %v, want functionNotAllowed with hint %q", tt.query, list, tt.hint)
		})
	}
}

func TestError_ErrorIncludesHint(t *testing.T) {
	// Arrange
	list := validationErrors(t, New(hintFields), "totl>5")

	// Act
	text := list.Error()

	// Assert
	if want := `position 0: unknown field "totl" (did you mean "total"?)`; text != want {
		t.Errorf("Error() = %q, want %q", text, want)
	}
}

func TestValidate_HintsAreBoundedOnManyErrors(t *testing.T) {
	// Arrange: twenty unknown names, each one edit from a real field.
	query := strings.TrimSuffix(strings.Repeat("totl>1 AND ", 20), " AND ")

	// Act
	list := validationErrors(t, New(hintFields), query)

	// Assert
	if len(list) != 20 {
		t.Fatalf("got %d errors, want 20", len(list))
	}
	hinted := 0
	for _, e := range list {
		if e.Hint != "" {
			hinted++
		}
	}
	if hinted != maxHinted {
		t.Errorf("%d errors carry a hint, want %d", hinted, maxHinted)
	}
}

// Every error of an SQL missing-value test carries the advice for it, the
// first word included, whether or not that word is a declared field.
func TestValidate_MissingTestHintOnEveryWord(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"declared field that allows presence", "name IS NULL"},
		{"declared field that does not", "total IS NULL"},
		{"undeclared field", "nickname IS NULL"},
		{"is not null", "nickname IS NOT NULL"},
		{"lower case", "nickname is null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			list := validationErrors(t, New(hintFields), tt.query)

			// Assert
			if len(list) == 0 {
				t.Fatalf("Validate(%q) passed, want errors", tt.query)
			}
			for _, e := range list {
				if !strings.Contains(e.Hint, "NOT field") {
					t.Errorf("Validate(%q): %s has hint %q, want the missing-value advice", tt.query, e.Message, e.Hint)
				}
			}
		})
	}
}

// A field that is really called is or null is a field, not SQL.
func TestValidate_FieldsNamedLikeSQLAreNotAMissingTest(t *testing.T) {
	// Arrange
	fields := []FieldConfig{
		{Name: "null", Type: TypeText, AllowedOps: TextOps},
		{Name: "name", Type: TypeText, AllowedOps: TextOps},
	}

	// Act
	list := validationErrors(t, New(fields), "null AND anme")

	// Assert
	if len(list) != 1 || !strings.Contains(list[0].Hint, `did you mean "name"?`) {
		t.Errorf("errors = %v, want one typo hint for anme", list)
	}
}

// A stray word gets advice about stray words, never the field list, however
// that list is worded.
func TestValidate_BareWordFallbackIsNotTheFieldList(t *testing.T) {
	// Arrange: few enough fields that an unknown field in a comparison lists them.
	v := New(hintFields[:3])

	// Act
	compared := validationErrors(t, v, "zzzzzz=1")
	alone := validationErrors(t, v, "name=x zzzzzz")

	// Assert
	if len(compared) != 1 || compared[0].Hint != "the fields are: name, state, total" {
		t.Errorf("comparison hint = %v, want the field list", compared)
	}
	if len(alone) != 1 || !strings.Contains(alone[0].Hint, "a word on its own") {
		t.Errorf("stray word hint = %v, want the stray-word advice", alone)
	}
	if strings.Contains(alone[0].Hint, "zzzzzz") {
		t.Errorf("stray word hint echoes the word as an example: %q", alone[0].Hint)
	}
}

// counting records how many times the validator asks it about a field.
type counting struct{ calls *int }

func (c counting) GetFieldConfig(name string) (FieldConfig, bool) {
	*c.calls++
	for _, f := range hintFields {
		if f.Name == name {
			return f, true
		}
	}
	return FieldConfig{}, false
}

func (counting) ValidateCustomRules(ast.Expression) error { return nil }

// A hint is built once per error. With a custom validator each build asks
// about every declared field, so building it twice doubles the calls.
func TestValidate_HintIsBuiltOncePerError(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"stray word", "name=x zzzzzz"},
		{"name in a function", "lower(zzzzzz)=x"},
		{"field in a comparison", "zzzzzz=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			calls := 0
			v := New(hintFields, WithCustomValidator(counting{&calls}))
			expr, err := parser.Parse(tt.query, 0)
			if err != nil {
				t.Fatal(err)
			}
			baseline := 0
			_ = New(hintFields, WithCustomValidator(counting{&baseline})).Validate(mustParseHint(t, "name=x"))

			// Act
			_ = v.Validate(expr)

			// Assert: the lookups a valid query makes, the failed lookup, and
			// one pass over the declared fields for the hint.
			if limit := baseline + 2 + len(hintFields); calls > limit {
				t.Errorf("GetFieldConfig called %d times, want at most %d", calls, limit)
			}
		})
	}
}

func mustParseHint(t *testing.T, query string) ast.Expression {
	t.Helper()
	expr, err := parser.Parse(query, 0)
	if err != nil {
		t.Fatalf("Parse(%q): %v", query, err)
	}
	return expr
}
