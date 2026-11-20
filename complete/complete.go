// Package complete suggests what can be written at a position in a query
// that is still being typed: field names, the operators a field allows, the
// selectors of a list, connectives, and the values a field is known to take.
//
// It is built for a search box. [At] is called on every keystroke with the
// text and the cursor, and answers from the schema alone, so it is fast and
// needs no network. Where the answer lives outside the schema (the values of
// a field, the keys of a nested one) the [Completion] says what is being
// asked for, and the host fetches the candidates from wherever it keeps them
// and passes them to [Completion.Offer], which writes each one the way the
// grammar needs it written.
//
//	c := complete.At(text, cursor, fields, parser.Config{}.Strict())
//	items := c.Items
//	if c.Slot == parser.SlotValue && c.Field == "cluster" {
//		items = c.Offer(clusterNames(c.Prefix)...)
//	}
//	// show items; on accept, replace c.Replace with item.Text
package complete

import (
	"strings"
	"unicode/utf8"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/token"
	"github.com/heyllave/query/validate"
)

// Kind says what an [Item] is, so a client can group or decorate the list.
type Kind string

// The kinds of item.
const (
	KindField    Kind = "field"    // a field name
	KindKey      Kind = "key"      // a key of a nested field, supplied by the host
	KindOperator Kind = "operator" // a comparison operator, a range or an IN list
	KindSelector Kind = "selector" // a selector of a list field
	KindKeyword  Kind = "keyword"  // AND, OR, NOT, or a closing parenthesis
	KindValue    Kind = "value"    // a value
)

// Item is one suggestion.
type Item struct {
	// Label is what to show.
	Label string
	// Text is what to put in place of [Completion.Replace]. It differs from
	// Label when the grammar needs the text written another way: a value
	// with a space in it is quoted, a nested field ends in its dot.
	Text string
	// Kind says what the item is.
	Kind Kind
	// Detail is a short note to show beside the label: a field's type, what
	// an operator does. It may be empty.
	Detail string
}

// Completion is what can be written at a position in a query.
type Completion struct {
	// Slot is what the grammar expects at the position. [parser.SlotNone]
	// means nothing can be suggested there.
	Slot parser.Slot
	// Replace is the span of the query an accepted item's Text goes in
	// place of: the word under the cursor, and an empty span at the cursor
	// when there is none. Offsets are in bytes.
	Replace token.Position
	// Prefix is what has been typed of the thing being completed, decoded:
	// without quotes, and for a key without the field it belongs to. Items
	// are already filtered by it; a host fetching its own candidates
	// filters by it too.
	Prefix string
	// Items are the suggestions the schema can make, best first. It is
	// empty when only the host knows the candidates.
	Items []Item
	// Field is the dotted name of the field the slot is about: the field a
	// value, operator or selector belongs to, and for a key the nested
	// field whose key is being typed. It is empty when there is none.
	Field string
	// Known reports that Field is declared in the schema; Type is then its
	// type. A value slot on an unknown field still names the field, since
	// the host may know it.
	Known bool
	Type  validate.FieldValueType
	// Operator is, for a value, the operator it follows as it is written
	// ("=", ">=", ":" for a range, "IN" inside an IN list).
	Operator string
	// Function is, for an argument, the function being called.
	Function string
	// Lists names the list selectors the position is inside, outermost
	// first. Field is then a field of one element of the innermost.
	Lists []string
	// Quoted reports that the cursor is inside an open quoted string, so
	// Replace starts at its quote and every Text is a whole quoted string.
	Quoted bool
}

// At reports what can be written at byte offset cursor of query, for the
// schema in fields. cfg is the grammar the query will be parsed under: with
// ExplicitAnd a field name is not offered where only a connective can come.
//
// It never fails. A position it cannot place has [parser.SlotNone] and no
// items.
func At(query string, cursor int, fields []validate.FieldConfig, cfg parser.Config) Completion {
	cur := parser.CursorAt(query, cursor, cfg)
	c := Completion{
		Slot:     cur.Slot,
		Replace:  cur.Word,
		Prefix:   cur.Prefix,
		Quoted:   cur.Quoted,
		Function: cur.Function,
	}
	for _, list := range cur.Lists {
		c.Lists = append(c.Lists, list.String())
	}
	scope, inScope := scopeOf(fields, cur.Lists)

	switch cur.Slot {
	case parser.SlotNone:
	case parser.SlotField:
		c.Items = fieldItems(scope, cur.Prefix, true)
		if !cur.Bracketed {
			c.Items = append(c.Items, keywords(cur.Prefix, "NOT")...)
		}
	case parser.SlotArgument:
		c.Items = fieldItems(scope, cur.Prefix, false)
	case parser.SlotKey:
		root, ok := find(scope, cur.Field.Root())
		c.Replace = cur.FieldSpan
		if ok && root.Nested {
			c.Field, c.Known, c.Type = root.Name, true, root.Type
			c.Prefix = strings.Join(append(cur.Field[1:len(cur.Field):len(cur.Field)], cur.Prefix), ".")
			break
		}
		// A dot in a name that is not a nested field's: fields may be
		// declared with dots in their names.
		c.Slot = parser.SlotField
		c.Prefix = cur.Field.String() + "." + cur.Prefix
		c.Items = fieldItems(scope, c.Prefix, true)
	case parser.SlotOperator:
		cfg0, known, _ := resolve(scope, cur.Field)
		c.Field, c.Known, c.Type = cur.Field.String(), known, cfg0.Type
		if known {
			c.Items = operatorItems(cfg0, cur.Prefix)
		}
		if !known || cfg0.AllowsOp(validate.OpPresence) {
			c.Items = append(c.Items, connectives(cur.Prefix, cur.Closable)...)
			if !cfg.ExplicitAnd && cur.Prefix != "" {
				c.Items = append(c.Items, fieldItems(scope, cur.Prefix, true)...)
			}
		}
	case parser.SlotSelector:
		cfg0, known, _ := resolve(scope, cur.Field)
		c.Field, c.Known, c.Type = cur.Field.String(), known, cfg0.Type
		if known {
			c.Items = selectorItems(cfg0, cur.Prefix)
		}
	case parser.SlotValue:
		c.Operator = operatorText(cur.Operator)
		if cur.Field == nil {
			break
		}
		cfg0, known, isKey := resolve(scope, cur.Field)
		c.Field, c.Known, c.Type = cur.Field.String(), known, cfg0.Type
		if known && !isKey {
			c.Items = valueItems(cfg0, cur.Prefix, cur.Quoted)
		}
	case parser.SlotConnective:
		c.Items = connectives(cur.Prefix, cur.Closable)
		if !cfg.ExplicitAnd && cur.Prefix != "" {
			c.Items = append(c.Items, fieldItems(scope, cur.Prefix, true)...)
		}
	}
	if !inScope {
		// Inside a selector on something that is not a declared list, no
		// field name can be vouched for.
		c.Items = withoutKind(c.Items, KindField)
	}
	return c
}

// Offer turns candidates the host supplies into items for this completion,
// each written the way the grammar needs it at this position: a value is
// quoted when it would not read back as itself, a key is joined to its
// field, and a name that is not a plain identifier takes the bracket form.
// The candidates are kept as given, in order; the host has already chosen
// and ranked them. One that cannot be written (it is not valid UTF-8, or is
// an empty key) is left out.
//
// It serves the slots whose candidates live outside the schema: a value, a
// key, and a field or argument when the host adds names of its own. For any
// other slot it returns nil.
func (c Completion) Offer(candidates ...string) []Item {
	var kind Kind
	switch c.Slot { //nolint:exhaustive // only the slots a host can fill
	case parser.SlotValue:
		kind = KindValue
	case parser.SlotKey:
		kind = KindKey
	case parser.SlotField, parser.SlotArgument:
		kind = KindField
	default:
		return nil
	}
	items := make([]Item, 0, len(candidates))
	for _, cand := range candidates {
		if !utf8.ValidString(cand) {
			continue
		}
		var text string
		switch kind { //nolint:exhaustive // the three kinds chosen above
		case KindValue:
			text = valueText(cand, c.Type, c.Known, c.Quoted)
		case KindKey:
			if cand == "" {
				continue
			}
			text = FieldText(c.Field + "." + cand)
		default:
			if cand == "" {
				continue
			}
			text = FieldText(cand)
		}
		items = append(items, Item{Label: cand, Text: text, Kind: kind})
	}
	return items
}

// ValueText writes s as a value that reads back as the text s: bare when
// the grammar reads the bare form as that same text, and in quotes
// otherwise. A value with a space, an operator character or a star in it is
// quoted, and so is one that would read as a number, a date, a duration or
// a boolean (812, 2026-01-01, 7d, true), since a value's type comes from how
// it is written.
func ValueText(s string) string {
	if readsAsText(s) {
		return s
	}
	return ast.StringValue(&ast.Value{Type: ast.ValueString, Raw: s, Str: s, Quoted: true})
}

// FieldText writes a dotted field name the way it is written where a
// condition starts: bare when every segment is a plain identifier, and in
// the bracket form otherwise (["labels.my key"]).
func FieldText(name string) string {
	return ast.String(&ast.PresenceExpr{Field: ast.FieldPath(strings.Split(name, "."))})
}

// readsAsText reports whether s, written bare, is read as the string s
// under the strictest grammar, both after an operator and as an element of
// an IN list, where a comma would end it.
func readsAsText(s string) bool {
	v, ok := bareValue(s)
	return ok && v.Type == ast.ValueString && !v.Wildcard && !v.Quoted && v.Str == s
}

// bareValue reads s as a value written bare: after an operator, as the
// first element of an IN list, and as the last thing in a group. ok is false
// unless all three read it as one and the same value.
func bareValue(s string) (v ast.Value, ok bool) {
	cfg := parser.Config{}.Strict()
	expr, err := parser.ParseWith("f="+s, cfg)
	if err != nil {
		return v, false
	}
	q, isQualifier := expr.(*ast.QualifierExpr)
	if !isQualifier || q.FieldFunc != nil || q.Field.String() != "f" || q.Operator != token.Eq || q.EndValue != nil {
		return v, false
	}
	list, err := parser.ParseWith("f IN ("+s+", z)", cfg)
	if err != nil {
		return v, false
	}
	elems := ast.Qualifiers(list)
	if len(elems) != 2 || elems[0].Value.Type != q.Value.Type || elems[0].Value.Raw != q.Value.Raw || elems[0].Value.Str != q.Value.Str {
		return v, false
	}
	// Before a ')' too: a trailing backslash would escape it.
	group, err := parser.ParseWith("(f="+s+")", cfg)
	if err != nil {
		return v, false
	}
	if in := ast.Qualifiers(group); len(in) != 1 || in[0].Value.Type != q.Value.Type || in[0].Value.Str != q.Value.Str {
		return v, false
	}
	return q.Value, true
}

// valueText writes a candidate value for a field of type t. A text field,
// or one nothing is known about, takes the candidate as text. Any other
// type takes it as written when it reads as one literal, so 812 stays a
// number on a numeric field; a candidate that does not is given as text.
func valueText(s string, t validate.FieldValueType, known, quoted bool) string {
	if quoted {
		return ast.StringValue(&ast.Value{Type: ast.ValueString, Raw: s, Str: s, Quoted: true})
	}
	if known && t != validate.TypeText && readsAsLiteral(s) {
		return s
	}
	return ValueText(s)
}

// readsAsLiteral reports whether s, written bare, is one literal that is
// not text: a number, a date, a duration or a boolean.
func readsAsLiteral(s string) bool {
	v, ok := bareValue(s)
	if !ok {
		return false
	}
	switch v.Type { //nolint:exhaustive // only plain literals
	case ast.ValueInteger, ast.ValueFloat, ast.ValueBoolean, ast.ValueDate, ast.ValueDuration:
		return v.Raw == s
	default:
		return false
	}
}

// scopeOf returns the fields that can be named at a position inside the
// given list selectors: the schema itself outside any, the elements of the
// innermost list inside. ok is false when one of them is not a declared
// list, so nothing is known about what its elements hold.
func scopeOf(fields []validate.FieldConfig, lists []ast.FieldPath) (scope []validate.FieldConfig, ok bool) {
	scope = fields
	for _, list := range lists {
		cfg, found := find(scope, list.String())
		if !found || cfg.Type != validate.TypeList {
			return nil, false
		}
		scope = cfg.Elements
	}
	return scope, true
}

func find(fields []validate.FieldConfig, name string) (validate.FieldConfig, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return validate.FieldConfig{}, false
}

// resolve finds the config a field path is held to, as the validator does:
// the field of that name, or the nested field its first segment names.
// isKey reports the second case.
func resolve(fields []validate.FieldConfig, path ast.FieldPath) (cfg validate.FieldConfig, known, isKey bool) {
	if len(path) == 0 {
		return validate.FieldConfig{}, false, false
	}
	if cfg, ok := find(fields, path.String()); ok {
		return cfg, true, false
	}
	if cfg, ok := find(fields, path.Root()); ok && cfg.Nested && len(path) > 1 {
		return cfg, true, true
	}
	return validate.FieldConfig{}, false, false
}

// matches orders names by how well they fit what was typed: those that
// start with it first, then those that only contain it. Case is ignored.
// The order within each group is the order given.
func matches(names []string, prefix string) []int {
	if prefix == "" {
		idx := make([]int, len(names))
		for i := range names {
			idx[i] = i
		}
		return idx
	}
	p := strings.ToLower(prefix)
	var starts, contains []int
	for i, name := range names {
		switch n := strings.ToLower(name); {
		case strings.HasPrefix(n, p):
			starts = append(starts, i)
		case strings.Contains(n, p):
			contains = append(contains, i)
		}
	}
	return append(starts, contains...)
}

// fieldItems lists the fields that fit what was typed, in schema order.
// With dot set, a nested field's text ends in the dot its key follows.
func fieldItems(fields []validate.FieldConfig, prefix string, dot bool) []Item {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	var items []Item
	for _, i := range matches(names, prefix) {
		f := fields[i]
		if f.Name == "" {
			continue
		}
		text := FieldText(f.Name)
		if dot && f.Nested && text == f.Name {
			text += "."
		}
		items = append(items, Item{Label: f.Name, Text: text, Kind: KindField, Detail: f.Type.String()})
	}
	return items
}

var opOrder = []struct {
	op     validate.Op
	text   string
	detail string
}{
	{validate.OpEq, "=", "is"},
	{validate.OpNeq, "!=", "is not"},
	{validate.OpGt, ">", "greater than"},
	{validate.OpGte, ">=", "at least"},
	{validate.OpLt, "<", "less than"},
	{validate.OpLte, "<=", "at most"},
	{validate.OpRange, ":", "range, from..to"},
}

// operatorItems lists what can follow a field: its comparison operators in
// a fixed order, IN lists when it can be compared for equality, and its
// selectors when it is a list.
func operatorItems(cfg validate.FieldConfig, prefix string) []Item {
	var items []Item
	add := func(label, text, detail string, kind Kind) {
		if strings.HasPrefix(strings.ToLower(label), strings.ToLower(prefix)) {
			items = append(items, Item{Label: label, Text: text, Kind: kind, Detail: detail})
		}
	}
	for _, o := range opOrder {
		if cfg.AllowsOp(o.op) {
			add(o.text, o.text, o.detail, KindOperator)
		}
	}
	if cfg.AllowsOp(validate.OpEq) {
		add("IN (...)", "IN (", "is one of", KindOperator)
	}
	if cfg.AllowsOp(validate.OpNeq) {
		add("NOT IN (...)", "NOT IN (", "is none of", KindOperator)
	}
	for _, form := range cfg.SelectorForms() {
		add(form, strings.TrimSuffix(form, "...)"), selectorDetail(form), KindSelector)
	}
	return items
}

// selectorItems lists a list field's selectors where the '@' is already
// written, so each text is what follows it.
func selectorItems(cfg validate.FieldConfig, prefix string) []Item {
	var items []Item
	for _, form := range cfg.SelectorForms() {
		text := strings.TrimSuffix(strings.TrimPrefix(form, "@"), "...)")
		if !strings.HasPrefix(text, prefix) {
			continue
		}
		items = append(items, Item{Label: form, Text: text, Kind: KindSelector, Detail: selectorDetail(form)})
	}
	return items
}

func selectorDetail(form string) string {
	switch form {
	case ast.SelectorForm(ast.SelectorFirst), ast.SelectorForm(ast.SelectorLast):
		return "has an element"
	case ast.SelectorForm(ast.SelectorExists), ast.SelectorForm(ast.SelectorAny):
		return "an element matches"
	case ast.SelectorForm(ast.SelectorAll):
		return "every element matches"
	case ast.SelectorForm(ast.SelectorNone):
		return "no element matches"
	default:
		return ""
	}
}

// valueItems lists the values the schema knows a field takes: both
// booleans, and whatever its config lists under Values.
func valueItems(cfg validate.FieldConfig, prefix string, quoted bool) []Item {
	values := cfg.Values
	if cfg.Type == validate.TypeBoolean && len(values) == 0 {
		values = []string{"true", "false"}
	}
	var items []Item
	for _, i := range matches(values, prefix) {
		v := values[i]
		if !utf8.ValidString(v) {
			continue
		}
		items = append(items, Item{Label: v, Text: valueText(v, cfg.Type, true, quoted), Kind: KindValue})
	}
	return items
}

func connectives(prefix string, closable bool) []Item {
	items := keywords(prefix, "AND", "OR")
	if closable && prefix == "" {
		items = append(items, Item{Label: ")", Text: ")", Kind: KindKeyword})
	}
	return items
}

func keywords(prefix string, words ...string) []Item {
	var items []Item
	for _, w := range words {
		if strings.HasPrefix(w, strings.ToUpper(prefix)) {
			items = append(items, Item{Label: w, Text: w, Kind: KindKeyword})
		}
	}
	return items
}

func withoutKind(items []Item, kind Kind) []Item {
	out := items[:0]
	for _, it := range items {
		if it.Kind != kind {
			out = append(out, it)
		}
	}
	return out
}

func operatorText(op token.Type) string {
	switch op { //nolint:exhaustive // the operators a value can follow
	case token.In:
		return "IN"
	case token.Illegal:
		return ""
	default:
		return token.OperatorSymbol(op)
	}
}
