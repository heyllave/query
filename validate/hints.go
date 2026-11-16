package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/internal/nearest"
)

// maxListedFields is the most field names a hint spells out. A longer list
// stops helping and starts hiding the message.
const maxListedFields = 8

// sqlFieldWords are words from SQL that end up read as field names, with what
// to write instead.
var sqlFieldWords = map[string]string{
	"IS":       "to test for a missing value write NOT field; for a present one write the field alone",
	"NULL":     "to test for a missing value write NOT field; for a present one write the field alone",
	"LIKE":     "there is no LIKE; use = with * as the wildcard: name=john*",
	"ILIKE":    "there is no ILIKE; = already ignores case, and * is the wildcard: name=john*",
	"BETWEEN":  "a range is written with a colon and two dots: total:1..5",
	"CONTAINS": `to test for part of a text use a wildcard, name=*john*, or contains(name, "john")`,
	"NOT":      "NOT goes before a condition: NOT state=draft; to compare, use state!=draft",
	"SELECT":   hintNotSQL,
	"FROM":     hintNotSQL,
	"WHERE":    hintNotSQL,
	"ORDER":    hintNotSQL,
	"BY":       hintNotSQL,
	"GROUP":    hintNotSQL,
	"LIMIT":    hintNotSQL,
	"OFFSET":   hintNotSQL,
}

const hintNotSQL = "only conditions are allowed here, such as field=value; " +
	"selecting, sorting and limiting are not part of a query"

// usableFields lists the declared fields this validator would accept, sorted.
// Hints name fields only from this list, so a field hidden by
// [WithAllowedFields] or by a custom validator is never suggested.
func (v *Validator) usableFields() []string {
	names := make([]string, 0, len(v.fields))
	for name := range v.fields {
		if _, ok := v.resolveField(name); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// durationWords are spelled-out units, which a duration literal abbreviates.
var durationWords = map[string]string{
	"minute": "m", "minutes": "m", "min": "m", "mins": "m",
	"hour": "h", "hours": "h", "hr": "h", "hrs": "h",
	"day": "d", "days": "d",
	"week": "w", "weeks": "w",
}

// bareWordHint is the advice for a word standing alone that is not a field:
// state IS NULL, name contains john, created_at>now() - 7 days. A word on its
// own is a presence test, so a stray word is read as a field name.
func (v *Validator) bareWordHint(name string) string {
	if h, ok := sqlFieldWords[strings.ToUpper(name)]; ok {
		return h
	}
	if unit, ok := durationWords[strings.ToLower(name)]; ok {
		return fmt.Sprintf("a duration is a number joined to a one-letter unit: 7%s", unit)
	}
	if h := v.fieldHint(name, ""); h != "" && !strings.HasPrefix(h, "the fields are") {
		return h
	}
	return fmt.Sprintf("a word on its own is read as a field that must have a value; "+
		"to use it as text, attach it to a comparison and quote text that has spaces: name=%q", name)
}

// fieldHint is the advice for a name that is not a field. fn is the function
// whose argument it was, or "".
func (v *Validator) fieldHint(name, fn string) string {
	if fn == "" && strings.EqualFold(name, "NOT") {
		return sqlFieldWords["NOT"]
	}
	usable := v.usableFields()
	if h := arithmeticHint(name, usable, fn); h != "" {
		return h
	}
	if near := v.nearField(name, usable); near != "" {
		if fn != "" {
			return fmt.Sprintf("did you mean %q? To pass text to %s(), put it in quotes: %q", near, fn, name)
		}
		return fmt.Sprintf("did you mean %q?", near)
	}
	if fn != "" {
		return fmt.Sprintf("a bare word in a function is a field; to pass text to %s(), put it in quotes: %q", fn, name)
	}
	if n := len(usable); n > 0 && n <= maxListedFields {
		return "the fields are: " + strings.Join(usable, ", ")
	}
	return ""
}

// nearField suggests the usable field name closest to name. For a dotted
// name it corrects the first segment and keeps the rest (lables.team).
func (v *Validator) nearField(name string, usable []string) string {
	if near := nearest.Match(name, usable); near != "" {
		return near
	}
	head, rest, dotted := strings.Cut(name, ".")
	if !dotted {
		return ""
	}
	var nested []string
	for _, u := range usable {
		if v.fields[u].Nested {
			nested = append(nested, u)
		}
	}
	if near := nearest.Match(head, nested); near != "" {
		return near + "." + rest
	}
	return ""
}

// arithmeticHint recognises a name such as total-base or total/3: field
// names may hold - and /, so arithmetic written without brackets reads as one
// name.
func arithmeticHint(name string, usable []string, fn string) string {
	known := make(map[string]bool, len(usable))
	for _, u := range usable {
		known[u] = true
	}
	var b strings.Builder
	parts, fields, start := 0, 0, 0
	for i := 0; i <= len(name); i++ {
		if i < len(name) && name[i] != '-' && name[i] != '/' {
			continue
		}
		part := name[start:i]
		switch {
		case known[part]:
			b.WriteString("[" + part + "]")
			fields++
		case isPlainNumber(part):
			b.WriteString(part)
		default:
			return ""
		}
		if i < len(name) {
			b.WriteByte(name[i])
		}
		parts++
		start = i + 1
	}
	if parts < 2 || fields == 0 {
		return ""
	}
	if fn != "" {
		return fmt.Sprintf("names may contain - and /, so this reads as one field; for arithmetic write the fields in brackets: %s(%s)", fn, b.String())
	}
	return fmt.Sprintf("names may contain - and /, so this reads as one field; arithmetic goes on the right of the operator or inside a function, with fields in brackets: %s", b.String())
}

func isPlainNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

// opHint is the advice when a field does not allow an operator: what it does
// allow.
func (v *Validator) opHint(name string, op Op) string {
	cfg, ok := v.resolveField(name)
	if !ok {
		return ""
	}
	var allowed []string
	for _, candidate := range []Op{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpRange, OpWildcard, OpPresence} {
		if cfg.AllowsOp(candidate) && v.opGloballyAllowed(candidate) {
			allowed = append(allowed, describeOp(candidate))
		}
	}
	list := "no operators"
	if len(allowed) > 0 {
		list = strings.Join(allowed, ", ")
	}
	switch op { //nolint:exhaustive // the comparison operators share the default
	case OpPresence:
		return fmt.Sprintf("a field on its own tests that it has a value, which %s does not allow; it allows: %s", name, list)
	case OpWildcard:
		return fmt.Sprintf("%s cannot be matched with a * pattern; it allows: %s", name, list)
	case OpRange:
		return fmt.Sprintf("%s cannot be tested with a range; it allows: %s", name, list)
	}
	return fmt.Sprintf("%s allows: %s", name, list)
}

func describeOp(op Op) string {
	switch op { //nolint:exhaustive // the comparison operators are their own symbol
	case OpRange:
		return "a range (field:1..5)"
	case OpWildcard:
		return "a * pattern"
	case OpPresence:
		return "the field alone"
	}
	return string(op)
}

// typeHint is the advice when a literal does not fit the field's type.
func (v *Validator) typeHint(cfg FieldConfig, val *ast.Value) string {
	raw := val.Raw
	if val.Type == ast.ValueString && !val.Quoted {
		if _, ok := v.resolveField(raw); ok {
			return fmt.Sprintf("to compare with the field %s, write it in brackets: [%s]", raw, raw)
		}
		if strings.ContainsAny(raw, "<>=") {
			return "a value cannot hold a second comparison; join two conditions with AND or OR"
		}
	}
	switch cfg.Type {
	case TypeBoolean:
		return "a boolean is true or false"
	case TypeInteger:
		if val.Type == ast.ValueFloat {
			return fmt.Sprintf("%s holds whole numbers", cfg.Name)
		}
		return numberHint(raw)
	case TypeDecimal:
		return numberHint(raw)
	case TypeDate, TypeDatetime:
		return "a date is YYYY-MM-DD, optionally with a time: 2026-01-31T10:00:00Z; for a relative date use now()-7d or daysAgo(7)"
	case TypeDuration:
		switch units, fraction := durationShape(raw); {
		case units > 1:
			return "a duration has one unit: write 90m rather than 1h30m"
		case units == 1 && fraction:
			return "a duration is a whole number and one unit: write 90m rather than 1.5h"
		}
		return "a duration is a number and one unit: 30m, 4h, 7d, 2w"
	case TypeText:
		return "put the text in quotes"
	}
	return ""
}

func numberHint(raw string) string {
	switch {
	case strings.Contains(raw, ","):
		return "write a number without separators: 1000 rather than 1,000"
	case strings.ContainsAny(raw, "$€£%"):
		return "write the number alone, without a symbol: 5"
	case strings.HasPrefix(raw, "0x") || strings.ContainsAny(raw, "eE") && isPlainNumber(strings.NewReplacer("e", "", "E", "").Replace(raw)):
		return "write a number in plain decimal form: 1000"
	}
	return "a number looks like 42, -3 or 1.5"
}

// durationShape counts the number-then-unit pairs in raw (1h30m has two) and
// reports whether it is written with a decimal point, when raw holds nothing
// but digits, dots and units. Anything else, such as a word, counts as zero.
func durationShape(raw string) (units int, fraction bool) {
	for i, r := range raw {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			fraction = true
		case (r == 'm' || r == 'h' || r == 'd' || r == 'w') && i > 0 && raw[i-1] >= '0' && raw[i-1] <= '9':
			units++
		default:
			return 0, false
		}
	}
	return units, fraction
}

// functionHint lists the functions [WithAllowedFunctions] admits, when the
// list is short enough to read.
func (v *Validator) functionHint(name string) string {
	names := make([]string, 0, len(v.allowedFuncs))
	for allowed := range v.allowedFuncs {
		names = append(names, allowed)
	}
	sort.Strings(names)
	if near := nearest.Match(name, names); near != "" {
		return fmt.Sprintf("did you mean %q?", near)
	}
	switch n := len(names); {
	case n == 0:
		return "no functions are allowed here"
	case n <= maxListedFields:
		return "the functions allowed here are: " + strings.Join(names, ", ")
	}
	return ""
}
