package validate

import (
	"errors"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

// listFields is a record with a list of forks, each of which has a list of
// environment variables.
var listFields = []FieldConfig{
	{Name: "name", Type: TypeText, AllowedOps: TextOps},
	{Name: "total", Type: TypeInteger, AllowedOps: NumericOps},
	{Name: "forks", Type: TypeList, AllowedOps: ListOps, Elements: []FieldConfig{
		{Name: "name", Type: TypeText, AllowedOps: TextOps},
		{Name: "replicas", Type: TypeInteger, AllowedOps: NumericOps},
		{Name: "env", Type: TypeList, AllowedOps: ListOps, Elements: []FieldConfig{
			{Name: "key", Type: TypeText, AllowedOps: TextOps},
		}},
	}},
	{Name: "tags", Type: TypeList, AllowedOps: ListOps, Selectors: []string{ast.SelectorFirst, ast.SelectorLast, ast.SelectorAny}},
}

func validateList(t *testing.T, q string) error {
	t.Helper()
	expr, err := parser.Parse(q, 0)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	return New(listFields).Validate(expr)
}

func TestValidate_ListFieldAcceptsItsSelectors(t *testing.T) {
	for _, q := range []string{
		"forks@first",
		"forks@(name=a)",
		"forks@all(replicas>2)",
		"forks@none(name=a* OR replicas:1..3)",
		"NOT forks@first AND name=x",
		"forks",
		// An element field may be a list too.
		"forks@(name=a AND env@(key=DEBUG))",
		"forks@(env@first)",
		// A list that names its own selectors.
		"tags@last",
		"tags@any(total>1)",
	} {
		// Act
		err := validateList(t, q)

		// Assert
		if err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}
}

func TestValidate_ListFieldRefusals(t *testing.T) {
	tests := []struct {
		q, message, hint string
		kind             ErrorKind
	}{
		{"forks@last", `selector @last is not allowed for field "forks"`, "to test that the list has an element write forks@first", ErrOperatorNotAllowed},
		{"forks@any(name=a)", `selector @any(...) is not allowed for field "forks"`, "write it without the word: forks@(...)", ErrOperatorNotAllowed},
		{"tags@all(total>1)", `selector @all(...) is not allowed for field "tags"`, "tags allows: @first, @last, @any(...)", ErrOperatorNotAllowed},
		{"name@(x=1)", `"name" is not a list field`, "a selector applies to a list field: forks, tags", ErrOperatorNotAllowed},
		{"name@first", `"name" is not a list field`, "a selector applies to a list field: forks, tags", ErrOperatorNotAllowed},
		{"forks@(bogus=1)", `unknown field "bogus" on "forks"`, "", ErrFieldNotFound},
		// The record's own fields are out of scope inside a selector.
		{"forks@(total>1)", `unknown field "total" on "forks"`, "", ErrFieldNotFound},
		{"forks@(env@(bogus=1))", `unknown field "bogus" on "env"`, "", ErrFieldNotFound},
		{"forks@(name@first)", `"name" is not a list field`, "a selector applies to a list field: env", ErrOperatorNotAllowed},
		{"forks@(replicas=many)", `field "replicas"`, "", ErrTypeMismatch},
		{"forks=x", `field "forks"`, "", ErrOperatorNotAllowed},
		{"nope@first", `unknown field "nope"`, "", ErrFieldNotFound},
	}
	for _, tt := range tests {
		// Act
		err := validateList(t, tt.q)

		// Assert
		var list ErrorList
		if !errors.As(err, &list) || len(list) != 1 {
			t.Errorf("%q: got %v, want one error", tt.q, err)
			continue
		}
		e := list[0]
		if e.Kind != tt.kind || !strings.Contains(e.Message, tt.message) {
			t.Errorf("%q: got %v (kind %v), want %q", tt.q, e.Message, e.Kind, tt.message)
		}
		if tt.hint != "" && e.Hint != tt.hint {
			t.Errorf("%q: hint %q, want %q", tt.q, e.Hint, tt.hint)
		}
	}
}

// A schema that declares no list keeps the rule it always had: the base only
// has to exist, and the condition is checked against the record's fields.
func TestValidate_SelectorsWithoutADeclaredList(t *testing.T) {
	fields := []FieldConfig{
		{Name: "items", Type: TypeText, AllowedOps: TextOps},
		{Name: "sku", Type: TypeText, AllowedOps: TextOps},
	}
	for _, q := range []string{"items@first", "items@last", "items@any(sku=a)", "items@(sku=a)"} {
		// Act
		expr, err := parser.Parse(q, 0)
		if err == nil {
			err = New(fields).Validate(expr)
		}

		// Assert
		if err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}
}

func TestFieldConfig_SelectorForms(t *testing.T) {
	// Act
	defaults := strings.Join(listFields[2].SelectorForms(), " ")
	named := strings.Join(listFields[3].SelectorForms(), " ")

	// Assert
	if defaults != "@first @(...) @all(...) @none(...)" {
		t.Errorf("defaults: %q", defaults)
	}
	if named != "@first @last @any(...)" {
		t.Errorf("named: %q", named)
	}
	if listFields[0].SelectorForms() != nil {
		t.Errorf("a text field has selector forms")
	}
}
