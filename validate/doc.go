// Package validate checks a parsed AST against field configurations.
//
// Consumers declare their fields using [FieldConfig] and pass them to
// [New] to create a validator. The validator checks that all fields in the
// query exist, that operators are allowed for each field's type, and that
// value types are compatible.
//
//	fields := []validate.FieldConfig{
//	    {Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
//	    {Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
//	}
//	v := validate.New(fields)
//	if err := v.Validate(expr); err != nil {
//	    // handle validation errors
//	}
//
// Validation also normalizes literals to the declared field type: a
// numeric-looking literal on a text field stays the text it was written as,
// and a quoted RFC 3339 string on a date field becomes a date. Arithmetic on
// a text or boolean field is a type mismatch.
//
// [WithAllowedFields] and [WithAllowedOps] restrict a query beyond the field
// configs. They are enforced after field resolution, so they bind a custom
// validator too, and they reach fields inside function arguments and
// bracketed field references. A comparison whose left side is a function
// (lower(name)=x) is checked against the global allowed operators and, for
// =, !=, wildcard and presence, against each argument field's own list.
//
// Domain-specific rules plug in via the [AstValidator] interface and the
// [WithCustomValidator] option. See the interface docs for semantics.
package validate
