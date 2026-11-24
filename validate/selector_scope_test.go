package validate

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

// These tests pin the restriction contract inside a list selector's element
// condition (see the WithAllowedFields / WithCustomValidator docs): operator and
// function restrictions cascade into the element condition, while field-level
// restrictions (allowedFields, a custom validator) govern the record's own
// fields, NOT the element fields, which come from FieldConfig.Elements. An
// element-scope that starts governing fields differently is a deliberate change,
// visible here.

func validateWith(t *testing.T, q string, opts ...Option) error {
	t.Helper()
	expr, err := parser.Parse(q, 0)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	return New(listFields, opts...).Validate(expr)
}

func TestSelectorScope_AllowedFieldsGovernsTopLevelNotElements(t *testing.T) {
	opt := WithAllowedFields("forks", "name")

	// Top-level restriction still bites: a disallowed top-level field is refused.
	if err := validateWith(t, "total>5", opt); err == nil {
		t.Error("total>5 accepted; total is not in WithAllowedFields")
	}

	// Element fields of an allowed list are governed by Elements, not
	// allowedFields: `replicas` is not (and cannot be) named in allowedFields,
	// yet the selector condition on it is valid because the list is allowed.
	for _, q := range []string{"forks@(name=a)", "forks@(replicas>2)", "forks@all(replicas:1..3)"} {
		if err := validateWith(t, q, opt); err != nil {
			t.Errorf("%q rejected under WithAllowedFields(forks,name): %v; element fields come from Elements", q, err)
		}
	}
	// A nonexistent element field is still refused (existence from Elements).
	if err := validateWith(t, "forks@(bogus=1)", opt); err == nil {
		t.Error("forks@(bogus=1) accepted; bogus is not an element field")
	}
}

func TestSelectorScope_OpsAndFunctionsCascade(t *testing.T) {
	// Operator restriction reaches inside the selector.
	if err := validateWith(t, "forks@(replicas>2)", WithAllowedOps(OpEq)); err == nil {
		t.Error("forks@(replicas>2) accepted under WithAllowedOps(OpEq); ops must cascade into selectors")
	}
	if err := validateWith(t, "forks@(name=a)", WithAllowedOps(OpEq)); err != nil {
		t.Errorf("forks@(name=a) rejected under WithAllowedOps(OpEq): %v", err)
	}
	// Function restriction reaches inside the selector, at any depth.
	if err := validateWith(t, "forks@(upper(name)=a)", WithAllowedFunctions("lower")); err == nil {
		t.Error("forks@(upper(name)=a) accepted; function restriction must cascade into selectors")
	}
	if err := validateWith(t, "forks@(env@(upper(key)=X))", WithAllowedFunctions("lower")); err == nil {
		t.Error("nested forks@(env@(upper(key)=X)) accepted; function restriction must cascade into nested selectors")
	}
}

// scopeCV resolves only the top-level fields it is told about and denies
// everything else, standing in for a tenant hook over the record schema.
type scopeCV struct{ allow map[string]FieldConfig }

func (c scopeCV) GetFieldConfig(name string) (FieldConfig, bool) {
	cfg, ok := c.allow[name]
	return cfg, ok
}
func (scopeCV) ValidateCustomRules(ast.Expression) error { return nil }

func TestSelectorScope_CustomValidatorGovernsTopLevelNotElements(t *testing.T) {
	cv := scopeCV{allow: map[string]FieldConfig{
		"name":  {Name: "name", Type: TypeText, AllowedOps: TextOps},
		"forks": listFields[2], // the forks list, with its Elements
	}}
	opt := WithCustomValidator(cv)

	// The hook governs top-level fields: `total` is denied though it is declared
	// statically.
	if err := validateWith(t, "total>5", opt); err == nil {
		t.Error("total>5 accepted; the custom validator does not resolve total")
	}
	// Element fields resolve from the list's Elements, not the hook: the hook
	// returns (_, false) for `replicas`, yet the selector condition is valid.
	if err := validateWith(t, "forks@(replicas>2)", opt); err != nil {
		t.Errorf("forks@(replicas>2) rejected under a custom validator: %v; element fields come from Elements", err)
	}
	if err := validateWith(t, "forks@(bogus=1)", opt); err == nil {
		t.Error("forks@(bogus=1) accepted; bogus is not an element field")
	}
}
