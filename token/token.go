package token

import "fmt"

// Position represents a location within a query string.
type Position struct {
	Offset int // byte offset from the start of the query
	Length int // length in bytes of the token
}

// String returns a human-readable representation of the position.
func (p Position) String() string {
	return fmt.Sprintf("offset %d, length %d", p.Offset, p.Length)
}

// Type represents the type of a lexical token.
type Type int

// Token type constants.
const (
	Illegal  Type = iota // unexpected character
	EOF                  // end of input
	Ident                // field names and identifiers
	String               // string values (after operators)
	Integer              // integer values
	Float                // float values (e.g., 50000.50)
	Date                 // date values (2026-01-01)
	Duration             // duration values (1d, 4h, 30m, 2w)
	Boolean              // true, false
	Eq                   // =
	Neq                  // !=
	Gt                   // >
	Gte                  // >=
	Lt                   // <
	Lte                  // <=
	Range                // ..
	Colon                // :
	And                  // AND
	Or                   // OR
	Not                  // NOT
	In                   // IN
	LParen               // (
	RParen               // )
	At                   // @
	Dot                  // .
	Comma                // ,
	Wildcard             // * (within string values)
	Ngt                  // !>  (NOT greater than)
	Ngte                 // !>= (NOT greater than or equal)
	Nlt                  // !<  (NOT less than)
	Nlte                 // !<= (NOT less than or equal)
	Plus                 // + (arithmetic)
	Minus                // - (arithmetic / unary)
	Mul                  // * (arithmetic — distinct from Wildcard)
	Div                  // / (arithmetic)
	Mod                  // % (arithmetic)
	FieldRef             // [field] or ["field"] — a field reference in value position
)

var typeNames = [...]string{
	Illegal:  "ILLEGAL",
	EOF:      "EOF",
	Ident:    "IDENT",
	String:   "STRING",
	Integer:  "INTEGER",
	Float:    "FLOAT",
	Date:     "DATE",
	Duration: "DURATION",
	Boolean:  "BOOLEAN",
	Eq:       "=",
	Neq:      "!=",
	Gt:       ">",
	Gte:      ">=",
	Lt:       "<",
	Lte:      "<=",
	Range:    "..",
	Colon:    ":",
	And:      "AND",
	Or:       "OR",
	Not:      "NOT",
	In:       "IN",
	LParen:   "(",
	RParen:   ")",
	At:       "@",
	Dot:      ".",
	Comma:    ",",
	Wildcard: "*",
	Ngt:      "!>",
	Ngte:     "!>=",
	Nlt:      "!<",
	Nlte:     "!<=",
	Plus:     "+",
	Minus:    "-",
	Mul:      "*",
	Div:      "/",
	Mod:      "%",
	FieldRef: "FIELDREF",
}

// opInfo is the per-token operator metadata. Keeping it in one table means a
// new operator is a single row here (plus its const and its [typeNames] entry),
// not a new case in each of the predicates below. A token with no row is the
// zero opInfo — not an operator — and since Illegal is 0, negatedBase == Illegal
// reads as "not a negated operator".
type opInfo struct {
	symbol      string // source spelling ("!>=")
	comparison  bool   // a comparison operator (drives IsOperator)
	arithmetic  bool   // an arithmetic operator (drives IsArithmetic)
	negatedBase Type   // for a negated comparison (!>): its positive form (>); Illegal otherwise
}

// operatorInfo is indexed by [Type]; opMeta bounds-checks the lookup. Range is a
// comparison token in the grammar but IsOperator excludes it (a range is written
// with ':', not as a binary operator), so it carries a symbol without the
// comparison flag.
var operatorInfo = [...]opInfo{
	Eq:    {symbol: "=", comparison: true},
	Neq:   {symbol: "!=", comparison: true},
	Gt:    {symbol: ">", comparison: true},
	Gte:   {symbol: ">=", comparison: true},
	Lt:    {symbol: "<", comparison: true},
	Lte:   {symbol: "<=", comparison: true},
	Ngt:   {symbol: "!>", comparison: true, negatedBase: Gt},
	Ngte:  {symbol: "!>=", comparison: true, negatedBase: Gte},
	Nlt:   {symbol: "!<", comparison: true, negatedBase: Lt},
	Nlte:  {symbol: "!<=", comparison: true, negatedBase: Lte},
	Colon: {symbol: ":", comparison: true},
	Range: {symbol: ".."},
	Plus:  {symbol: "+", arithmetic: true},
	Minus: {symbol: "-", arithmetic: true},
	Mul:   {symbol: "*", arithmetic: true},
	Div:   {symbol: "/", arithmetic: true},
	Mod:   {symbol: "%", arithmetic: true},
}

// opMeta returns the operator metadata for t, or the zero opInfo for a token
// that is not in the table. The bounds check keeps it safe for every Type.
func opMeta(t Type) opInfo {
	if int(t) >= 0 && int(t) < len(operatorInfo) {
		return operatorInfo[t]
	}
	return opInfo{}
}

// IsArithmetic reports whether the token type is an arithmetic operator.
func (t Type) IsArithmetic() bool {
	return opMeta(t).arithmetic
}

// String returns the human-readable name of the token type.
func (t Type) String() string {
	if int(t) < len(typeNames) {
		return typeNames[t]
	}
	return fmt.Sprintf("Type(%d)", t)
}

// IsOperator reports whether the token type is a comparison operator.
func (t Type) IsOperator() bool {
	return opMeta(t).comparison
}

// IsNegatedOperator reports whether the token type is a negated comparison
// operator (!>, !>=, !<, !<=).
func (t Type) IsNegatedOperator() bool {
	return opMeta(t).negatedBase != Illegal
}

// NegateOperator returns the positive form of a negated comparison operator
// (!> → >, !>= → >=, !< → <, !<= → <=). For non-negated operators it returns
// the input unchanged.
func NegateOperator(t Type) Type {
	if base := opMeta(t).negatedBase; base != Illegal {
		return base
	}
	return t
}

// IsKeyword reports whether the token type is a reserved word that the parser
// also accepts as a field name when it appears in field position (AND, OR,
// NOT, IN, true, false).
func (t Type) IsKeyword() bool {
	switch t { //nolint:exhaustive // only keyword tokens
	case And, Or, Not, In, Boolean:
		return true
	default:
		return false
	}
}

// IsValueEnd reports whether a token type can end a value primary, so that an
// arithmetic operator following it (after whitespace) continues the value:
// `total>10 - 3`.
func (t Type) IsValueEnd() bool {
	switch t { //nolint:exhaustive // only value-ending tokens
	case Integer, Float, Duration, Date, RParen, FieldRef:
		return true
	default:
		return false
	}
}

// IsLogical reports whether the token type is a logical operator (AND, OR).
func (t Type) IsLogical() bool {
	return t == And || t == Or
}

// Token represents a single lexical token with its type, value, and position.
type Token struct {
	Type   Type
	Value  string
	Pos    Position
	Quoted bool // true if a String token came from a "..."-quoted literal
	// Raw is the source text of an unquoted value token before escape
	// sequences were decoded (so `a\(b` keeps its backslash). Empty when the
	// source text and Value are identical.
	Raw string
}

// String returns a debug representation of the token.
func (t Token) String() string {
	if t.Value != "" {
		return fmt.Sprintf("%s(%q)", t.Type, t.Value)
	}
	return t.Type.String()
}

// OperatorSymbol returns the string representation of a comparison or
// arithmetic operator. For a token that has no symbol in the table it returns
// "=", preserving the original fallback.
func OperatorSymbol(op Type) string {
	if s := opMeta(op).symbol; s != "" {
		return s
	}
	return "="
}
