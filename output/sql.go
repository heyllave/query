package output

import "github.com/heyllave/query/token"

// SQL-generation helpers. They translate the language's own operators and
// wildcard patterns into their SQL spellings. They live here, in the output
// layer, rather than in the backend-agnostic ast package: ast holds the tree,
// and each backend (SQL here, another elsewhere) owns its own rendering.

// SQLOperator returns the SQL equivalent of a comparison operator token.
// For wildcards it returns "LIKE", or "NOT LIKE" when the operator is !=.
func SQLOperator(op token.Type, wildcard bool) string {
	if wildcard {
		if op == token.Neq {
			return "NOT LIKE"
		}
		return "LIKE"
	}
	switch op { //nolint:exhaustive // only comparison operators
	case token.Eq:
		return "="
	case token.Neq:
		return "!="
	case token.Gt:
		return ">"
	case token.Gte:
		return ">="
	case token.Lt:
		return "<"
	case token.Lte:
		return "<="
	default:
		return "="
	}
}

// WildcardToLike converts a query wildcard pattern to a SQL LIKE pattern.
// A literal %, _ or backslash in the pattern is escaped with a backslash, so
// the clause that uses the result must declare it: LIKE ? ESCAPE '\'. Without
// the ESCAPE clause a database with no default escape character (SQLite,
// Oracle, SQL Server) reads those characters as wildcards again.
//
//	"John*"    → "John%"
//	"*yota"    → "%yota"
//	"*test*"   → "%test%"
//	"a\\*b*"   → "a*b%"  (an escaped star is a literal star)
func WildcardToLike(pattern string) string {
	var buf []byte
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			// Pattern form: `\*` is a literal star, `\\` a literal backslash,
			// which LIKE needs escaped under ESCAPE '\'.
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				buf = append(buf, '*')
				i++
				continue
			}
			if i+1 < len(pattern) && pattern[i+1] == '\\' {
				i++
			}
			buf = append(buf, '\\', '\\')
		case '*':
			buf = append(buf, '%')
		case '%', '_':
			buf = append(buf, '\\', pattern[i])
		default:
			buf = append(buf, pattern[i])
		}
	}
	return string(buf)
}
