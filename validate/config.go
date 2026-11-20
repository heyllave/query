package validate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/heyllave/query/ast"
)

// FieldValueType identifies the data type of a field.
type FieldValueType int

// Field value type constants.
const (
	TypeText     FieldValueType = iota // free-text string
	TypeInteger                        // whole number
	TypeDecimal                        // floating-point number
	TypeBoolean                        // true/false
	TypeDate                           // date (YYYY-MM-DD)
	TypeDatetime                       // date and time
	TypeDuration                       // duration (1d, 4h, etc.)
	TypeList                           // a list of elements, asked about with a selector
)

var fieldValueTypeNames = [...]string{
	TypeText:     "text",
	TypeInteger:  "integer",
	TypeDecimal:  "decimal",
	TypeBoolean:  "boolean",
	TypeDate:     "date",
	TypeDatetime: "datetime",
	TypeDuration: "duration",
	TypeList:     "list",
}

// String returns the name of the field value type.
func (t FieldValueType) String() string {
	if int(t) < len(fieldValueTypeNames) {
		return fieldValueTypeNames[t]
	}
	return fmt.Sprintf("FieldValueType(%d)", t)
}

// UnmarshalJSON reads a field type written either as its name ("decimal") or
// as its number (2), so a field config can come from a client in any language
// without that client knowing the numbering. An unknown name or number is an
// error rather than a silent text field.
func (t *FieldValueType) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		for i, known := range fieldValueTypeNames {
			if strings.EqualFold(name, known) {
				*t = FieldValueType(i)
				return nil
			}
		}
		return fmt.Errorf("unknown field type %q", name)
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("field type must be a name or a number, got %s", data)
	}
	if n < 0 || n >= len(fieldValueTypeNames) {
		return fmt.Errorf("unknown field type %d", n)
	}
	*t = FieldValueType(n)
	return nil
}

// DefaultOps returns the operators that make sense for a field type: the set
// a field gets when its config names none.
func DefaultOps(t FieldValueType) []Op {
	switch t {
	case TypeText:
		return TextOps
	case TypeInteger, TypeDecimal:
		return NumericOps
	case TypeBoolean:
		return BoolOps
	case TypeDate, TypeDatetime:
		return DateOps
	case TypeDuration:
		return DurationOps
	case TypeList:
		return ListOps
	default:
		return nil
	}
}

// Op represents a query operator.
type Op string

// Operator constants.
const (
	OpEq       Op = "="  // equality
	OpNeq      Op = "!=" // not equal
	OpGt       Op = ">"  // greater than
	OpGte      Op = ">=" // greater than or equal
	OpLt       Op = "<"  // less than
	OpLte      Op = "<=" // less than or equal
	OpRange    Op = ".." // inclusive range
	OpWildcard Op = "*"  // wildcard match
	OpPresence Op = "?"  // field exists / has value
)

// TextOps are the operators valid for text fields.
var TextOps = []Op{OpEq, OpNeq, OpWildcard, OpPresence}

// NumericOps are the operators valid for integer and decimal fields.
var NumericOps = []Op{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpRange}

// DateOps are the operators valid for date and datetime fields.
var DateOps = []Op{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpRange}

// BoolOps are the operators valid for boolean fields.
var BoolOps = []Op{OpEq, OpNeq}

// DurationOps are the operators valid for duration fields.
var DurationOps = []Op{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpRange}

// ListOps are the operators valid for a list field itself: only the bare
// name, which asks whether the list is there. Everything else about a list
// is asked with a selector.
var ListOps = []Op{OpPresence}

// DefaultSelectors are the selectors a list field accepts when its config
// names none: one spelling for each question. `@last` is left out because
// it asks what `@first` asks, and `@any(...)` because it is `@(...)`. A host
// that wants them lists them in [FieldConfig.Selectors].
var DefaultSelectors = []string{ast.SelectorFirst, ast.SelectorExists, ast.SelectorAll, ast.SelectorNone}

// FieldConfig declares the name, type, and allowed operations for a query field.
//
// A field of [TypeList] is asked about with a selector (`forks@first`,
// `forks@(name=x)`). Elements declares the fields of one element, which is
// what the condition inside a selector may name; an element field may itself
// be a list. Once a schema declares a list, a selector on any other field is
// an error.
type FieldConfig struct {
	Name       string         // field name
	Type       FieldValueType // data type
	AllowedOps []Op           // allowed operations
	Searchable bool           // included in free-text search
	Nested     bool           // has sub-fields (like labels.*)
	Elements   []FieldConfig  // for TypeList: the fields of one element
	Selectors  []string       // for TypeList: the selectors allowed; nil means DefaultSelectors
	// Values are values the field is known to take, for completion to
	// offer (see the complete package). They are suggestions: validation
	// does not hold a query to them, so the list need not be exhaustive.
	Values []string
}

// AllowsSelector reports whether a list field accepts the selector, named as
// [ast.SelectorExpr] holds it ("first", "all", "" for `@(...)`).
func (f FieldConfig) AllowsSelector(name string) bool {
	for _, allowed := range f.selectors() {
		if allowed == name {
			return true
		}
	}
	return false
}

// SelectorForms lists the selectors a list field accepts as they are
// written: "@first", "@(...)", "@all(...)". It is nil for any other field.
func (f FieldConfig) SelectorForms() []string {
	if f.Type != TypeList {
		return nil
	}
	names := f.selectors()
	forms := make([]string, len(names))
	for i, name := range names {
		forms[i] = ast.SelectorForm(name)
	}
	return forms
}

func (f FieldConfig) selectors() []string {
	if f.Selectors == nil {
		return DefaultSelectors
	}
	return f.Selectors
}

// AllowsOp reports whether the field allows the given operator.
func (f FieldConfig) AllowsOp(op Op) bool {
	for _, allowed := range f.AllowedOps {
		if allowed == op {
			return true
		}
	}
	return false
}

// AstValidator is an extension point for consumers to add custom validation
// rules beyond what the generic validator checks.
type AstValidator interface {
	GetFieldConfig(fieldName string) (FieldConfig, bool)
	ValidateCustomRules(node ast.Expression) error
}
