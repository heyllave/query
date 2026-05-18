package validate

import (
	"fmt"

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
)

var fieldValueTypeNames = [...]string{
	TypeText:     "text",
	TypeInteger:  "integer",
	TypeDecimal:  "decimal",
	TypeBoolean:  "boolean",
	TypeDate:     "date",
	TypeDatetime: "datetime",
	TypeDuration: "duration",
}

// String returns the name of the field value type.
func (t FieldValueType) String() string {
	if int(t) < len(fieldValueTypeNames) {
		return fieldValueTypeNames[t]
	}
	return fmt.Sprintf("FieldValueType(%d)", t)
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

// FieldConfig declares the name, type, and allowed operations for a query field.
type FieldConfig struct {
	Name       string         // field name
	Type       FieldValueType // data type
	AllowedOps []Op           // allowed operations
	Searchable bool           // included in free-text search
	Nested     bool           // has sub-fields (like labels.*)
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
