package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/internal/hintwords"
	"github.com/heyllave/query/internal/nearest"
)

// maxListedFields is the most field names a hint spells out. A longer list
// stops helping and starts hiding the message.
const maxListedFields = 8

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

// bareWordHint is the advice for a word standing alone that is not a field:
// state IS NULL, name contains john, created_at>now() - 7 days. A word on its
// own is a presence test, so a stray word is read as a field name.
func (v *Validator) bareWordHint(name string) string {
	if h, ok := hintwords.SQL(name); ok {
		return h
	}
	// Every word of `name IS NULL` is a bare word, and the first is the one
	// the writer cares about, so it gets the same advice as IS and NULL.
	if v.missingTest {
		return hintwords.Missing
	}
	if unit, ok := hintwords.DurationUnit(name); ok {
		return fmt.Sprintf("a duration is a number joined to a one-letter unit: 7%s", unit)
	}
	if h := v.namedFieldHint(name, "", v.usableFields()); h != "" {
		return h
	}
	return "a word on its own is read as a field that must have a value; " +
		"to use it as text, put it after an operator, in quotes if it has spaces: field=\"two words\""
}

// fieldHint is the advice for a name that is not a field: a correction when
// the validator can name one, otherwise the field list when it is short.
func (v *Validator) fieldHint(name string) string {
	if strings.EqualFold(name, "NOT") {
		h, _ := hintwords.SQL(name)
		return h
	}
	usable := v.usableFields()
	if h := v.namedFieldHint(name, "", usable); h != "" {
		return h
	}
	if len(usable) > 0 && len(usable) <= maxListedFields {
		return "the fields are: " + strings.Join(usable, ", ")
	}
	return ""
}

// argumentHint is the advice for a name passed to the function fn that is
// not a field. A bare word there is always a field, so the likeliest mistake
// after a typo is text written without quotes.
func (v *Validator) argumentHint(name, fn string) string {
	if h := v.namedFieldHint(name, fn, v.usableFields()); h != "" {
		return h
	}
	return fmt.Sprintf("a bare word in a function is a field; to pass text to %s(), put it in quotes: %q", fn, name)
}

// namedFieldHint is the advice that names what was meant: arithmetic written
// without brackets, or the field a typo is closest to. It is "" when the
// validator cannot tell. fn is the function the name was passed to, or "";
// usable is [Validator.usableFields], which the caller may need again.
func (v *Validator) namedFieldHint(name, fn string, usable []string) string {
	if h := arithmeticHint(name, usable, fn); h != "" {
		return h
	}
	near := v.nearField(name, usable)
	switch {
	case near == "":
		return ""
	case fn != "":
		return fmt.Sprintf("did you mean %q? To pass text to %s(), put it in quotes: %q", near, fn, name)
	}
	return fmt.Sprintf("did you mean %q?", near)
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
		case hintwords.IsNumber(part):
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
	case TypeList:
		return fmt.Sprintf("a list is asked about with a selector: %s@first", cfg.Name)
	}
	return ""
}

const arithmeticOperandsHint = "numbers combine with numbers; a date takes + or - a duration; a duration takes + or - a duration, or * or / a number"

// arithmeticResultHint is the advice when arithmetic gives the wrong kind of
// value for a field. The common case is not arithmetic at all in the
// writer's mind: a date or a duration written in a shape the grammar reads
// as a subtraction.
func arithmeticResultHint(cfg FieldConfig, val *ast.Value) string {
	switch cfg.Type { //nolint:exhaustive // only the types arithmetic can be compared with
	case TypeDate, TypeDatetime:
		if strings.Trim(val.Raw, "0123456789-") == "" {
			return "a date is written in full, YYYY-MM-DD: 2026-01-05 rather than 2026-1-5"
		}
		return "a date is YYYY-MM-DD; for a relative date use now()-7d or daysAgo(7)"
	case TypeDuration:
		return "a duration is a number and one unit: 30m, 4h, 7d, 2w"
	case TypeInteger, TypeDecimal:
		return "a number looks like 42, -3 or 1.5"
	}
	return ""
}

func numberHint(raw string) string {
	switch {
	case strings.Contains(raw, ","):
		return "write a number without separators: 1000 rather than 1,000"
	case strings.ContainsAny(raw, "$€£%"):
		return "write the number alone, without a symbol: 5"
	case strings.HasPrefix(raw, "0x") || strings.ContainsAny(raw, "eE") && hintwords.IsNumber(strings.NewReplacer("e", "", "E", "").Replace(raw)):
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

// hasMissingTest reports whether the query holds IS or NULL as a word on its
// own that is not a field, the trace an SQL missing-value test leaves.
func (v *Validator) hasMissingTest(expr ast.Expression) bool {
	found := false
	ast.Walk(expr, func(node ast.Expression) bool {
		if p, ok := node.(*ast.PresenceExpr); ok && !found {
			name := p.Field.String()
			if hintwords.IsMissingTest(name) {
				_, declared := v.resolveField(name)
				found = !declared
			}
		}
		return !found
	})
	return found
}
