package ast

import (
	"fmt"
	"strings"
	"time"
)

// FieldPath represents a dotted field path like "labels.dev" as ["labels", "dev"].
type FieldPath []string

// String returns the dotted representation of the field path.
func (fp FieldPath) String() string {
	return strings.Join(fp, ".")
}

// Root returns the first segment of the path.
func (fp FieldPath) Root() string {
	if len(fp) == 0 {
		return ""
	}
	return fp[0]
}

// IsNested reports whether the path has more than one segment.
func (fp FieldPath) IsNested() bool {
	return len(fp) > 1
}

// ValueType identifies the type of a parsed value.
type ValueType int

// Value type constants.
const (
	ValueString   ValueType = iota // plain string
	ValueInteger                   // integer number
	ValueFloat                     // floating-point number
	ValueBoolean                   // true or false
	ValueDate                      // date (YYYY-MM-DD)
	ValueDuration                  // duration (1d, 4h, etc.)
	ValueFunc                      // function call resolved at match time, e.g. now()
	ValueArith                     // arithmetic expression resolved at match time, e.g. 50000*1.1
	ValueList                      // a list of values produced at match time, e.g. a function returning a slice
	ValueFieldRef                  // a field reference in value position, e.g. [base] or ["base-1"]
)

var valueTypeNames = [...]string{
	ValueString:   "string",
	ValueInteger:  "integer",
	ValueFloat:    "float",
	ValueBoolean:  "boolean",
	ValueDate:     "date",
	ValueDuration: "duration",
	ValueFunc:     "function",
	ValueArith:    "arithmetic",
	ValueList:     "list",
	ValueFieldRef: "field",
}

// String returns the name of the value type.
func (v ValueType) String() string {
	if int(v) < len(valueTypeNames) {
		return valueTypeNames[v]
	}
	return fmt.Sprintf("ValueType(%d)", v)
}

// Value represents a typed value in a qualifier expression.
type Value struct {
	Type     ValueType
	Raw      string        // original string from the query
	Str      string        // for string values
	Int      int64         // for integer values
	Float    float64       // for float values
	Bool     bool          // for boolean values
	Date     time.Time     // for date values
	Duration time.Duration // for duration values
	Wildcard bool          // true if the value contains wildcards
	Quoted   bool          // true if the string value came from a "..."-quoted literal
	Func     *FuncCallExpr // for ValueFunc: function call resolved at match time
	Arith    *ArithExpr    // for ValueArith: arithmetic expression resolved at match time
	List     []any         // for ValueList: elements produced at match time
	Field    FieldPath     // for ValueFieldRef: the referenced field path
}

// ArithOp identifies a binary arithmetic operator inside an [ArithExpr]. The
// string representation is also the canonical round-trip form rendered by
// [String].
type ArithOp string

// Arithmetic operator constants.
const (
	ArithAdd ArithOp = "+"
	ArithSub ArithOp = "-"
	ArithMul ArithOp = "*"
	ArithDiv ArithOp = "/"
	ArithMod ArithOp = "%"
)

// String implements [fmt.Stringer]; the symbol is its own canonical form.
func (a ArithOp) String() string { return string(a) }

// ArithExpr is a binary arithmetic expression appearing in value position:
// 50000*1.1, now()-7d, (a+b)*c. Operands are themselves Values, so the tree
// can mix literals, function calls, and nested arithmetic.
type ArithExpr struct {
	Op    ArithOp
	Left  *Value
	Right *Value
}

// Any returns the typed Go value (string, int64, float64, bool, time.Time, or time.Duration).
// For ValueFunc, returns nil — the function call must be resolved by the eval
// engine against a function registry.
func (v Value) Any() any {
	if v.Wildcard {
		return v.Str
	}
	switch v.Type {
	case ValueString:
		return v.Str
	case ValueInteger:
		return v.Int
	case ValueFloat:
		return v.Float
	case ValueBoolean:
		return v.Bool
	case ValueDate:
		return v.Date
	case ValueDuration:
		return v.Duration
	case ValueList:
		return v.List
	case ValueFunc, ValueArith, ValueFieldRef:
		return nil
	default:
		return v.Raw
	}
}
