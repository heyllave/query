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
// bracketed field references.
//
// A field's AllowedOps bounds every way a query can read it. A comparison
// whose left side is a function (lower(name)=x) is checked against the
// global allowed operators and against each argument field's own list;
// ordering a function's result over a text field (len(name)>5) is allowed
// when the field allows wildcards. A field read on the value side
// (name=[other], total>len(other)) is checked against the mirrored operator.
// A function used as a predicate (startsWith(name, "a")) needs the wildcard
// operator on each text field it reads and equality on any other, globally
// and per field. A field that allows only = can therefore be tested for
// equality and nothing else.
//
// The validator accepts any function name unless [WithAllowedFunctions] lists
// the permitted ones: which functions exist is up to whoever evaluates or
// translates the tree. A consumer that generates SQL or another language from
// a validated tree should list exactly the functions it can translate.
//
// Domain-specific rules plug in via the [AstValidator] interface and the
// [WithCustomValidator] option. See the interface docs for semantics.
package validate
