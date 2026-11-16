package parser

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/heyllave/query/internal/hintwords"
	"github.com/heyllave/query/internal/nearest"
	"github.com/heyllave/query/token"
)

// annotate gives each error that has none a hint: what the writer most likely
// meant and how to write it. Codes and positions say what is wrong and where;
// the hint says what to do about it. It reads the query text around the
// error, so one place holds everything the parser knows about common
// mistakes, and no error site has to guess at intent.
//
// value is true for a value expression (ParseValue), where a comparison is
// the mistake rather than the point. cfg is the grammar the query was parsed
// under: a rewrite is only suggested if the caller would accept it typed
// back, so it has to fit cfg.MaxLength and parse under the same strict
// options.
func annotate(input string, err error, value bool, cfg Config) error {
	var list ErrorList
	if !errors.As(err, &list) {
		return err
	}
	list = mergeRepeats(input, list)
	q := &query{input: input, value: value, maxLength: cfg.MaxLength, cfg: cfg}
	for i, e := range list {
		if i == maxHinted {
			break
		}
		if e.Hint == "" {
			e.Hint = hintFor(q, e)
		}
	}
	return list
}

// query is what every hint of one failed parse shares: the text, the mode,
// the caller's limit, and the words of the text, which are scanned once
// however many errors ask for them.
type query struct {
	input     string
	value     bool
	maxLength int
	cfg       Config
	words     []word
	scanned   bool
}

// word is one bare word of the query, outside quotes and brackets.
type word struct {
	start, end int
	// field is true when the word is used as a name: followed by an operator,
	// a dot, a selector or a call, or itself a later part of a dotted name.
	field bool
}

// bareWords returns the words of the query that are not inside a quoted
// string or a bracketed field name, where text is data rather than syntax.
func (q *query) bareWords() []word {
	if q.scanned {
		return q.words
	}
	q.scanned = true
	in := q.input
	for i := 0; i < len(in); {
		r, size := utf8.DecodeRuneInString(in[i:])
		switch {
		case r == '"':
			i = skipQuoted(in, i)
		case r == '[':
			i = skipBracketed(in, i)
		case isWordRune(r):
			text := firstWord(in[i:])
			end := i + len(text)
			next := strings.TrimLeftFunc(in[end:], unicode.IsSpace)
			used := next != "" && strings.ContainsRune("=<>!:.@(", rune(next[0]))
			dotted := i > 0 && in[i-1] == '.'
			q.words = append(q.words, word{start: i, end: end, field: used || dotted})
			i = end
		default:
			i += size
		}
	}
	return q.words
}

// skipQuoted returns the index just past the string that opens at in[open],
// or len(in) when it never closes.
func skipQuoted(in string, open int) int {
	for i := open + 1; i < len(in); i++ {
		switch in[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(in)
}

// skipBracketed returns the index just past the bracketed name that opens at
// in[open], or len(in) when it never closes.
func skipBracketed(in string, open int) int {
	for i := open + 1; i < len(in); i++ {
		switch in[i] {
		case '"':
			i = skipQuoted(in, i) - 1
		case ']':
			return i + 1
		}
	}
	return len(in)
}

// parses reports whether rewrite is a valid condition the caller would
// accept: within the caller's length limit and the rewrite bound.
func (q *query) parses(rewrite string) bool {
	if len(rewrite) > maxRewriteLength {
		return false
	}
	if q.maxLength > 0 && utf8.RuneCountInString(rewrite) > q.maxLength {
		return false
	}
	return parses(rewrite, q.cfg)
}

// A hint reads the query around its error, and some re-parse a corrected
// copy, so each costs time in proportion to the query. Bounding how many
// errors get one, and how long a query may be re-parsed, keeps a failing
// query linear in its length when the caller has lifted the length limit.
const (
	maxHinted        = 8
	maxRewriteLength = 4096
)

// mergeRepeats folds a run of the same unexpected character into one error
// that spans the run, so && is reported once, as "&&", rather than once per
// character.
func mergeRepeats(input string, list ErrorList) ErrorList {
	out := make(ErrorList, 0, len(list))
	for _, e := range list {
		if n := len(out); n > 0 {
			prev := out[n-1]
			end := prev.Position.Offset + prev.Position.Length
			if prev.Code == CodeUnexpectedChar && e.Code == CodeUnexpectedChar && e.Position.Length == 1 &&
				e.Position.Offset == end && end < len(input) && prev.Position.Length > 0 &&
				input[e.Position.Offset] == input[prev.Position.Offset] && input[e.Position.Offset] < utf8.RuneSelf {
				prev.Position.Length++
				prev.Message = fmt.Sprintf("unexpected %q", input[prev.Position.Offset:end+1])
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// parses reports whether q is a valid condition under cfg's strict options.
// Hints use it to check a rewrite before suggesting it, without going back
// through annotate.
func parses(q string, cfg Config) bool {
	l := &lexer{input: q, strictValues: cfg.StrictValues}
	l.run()
	if len(l.errors) > 0 {
		return false
	}
	p := &parser{tokens: l.tokens, explicitAnd: cfg.ExplicitAnd}
	p.parseExpression()
	return len(p.errors) == 0 && p.peek().Type == token.EOF
}

// describe names a token the way a person would: "*" rather than *("*"),
// end of query rather than EOF.
func describe(tok token.Token) string {
	switch tok.Type { //nolint:exhaustive // every other token is named by its text
	case token.EOF:
		return "the end of the query"
	case token.FieldRef:
		return fmt.Sprintf("%q", "["+tok.Value+"]")
	}
	if tok.Value != "" {
		return fmt.Sprintf("%q", tok.Value)
	}
	return fmt.Sprintf("%q", tok.Type.String())
}

const (
	hintArithmeticRule   = "arithmetic goes on the right of the operator or inside a function: "
	hintArithmeticLeft   = hintArithmeticRule + "total>[base]*1.1 or float([base]*1.1)>3"
	hintNoCompareInValue = "a value expression computes a value and cannot compare or combine; " +
		"evaluate this as a condition instead"
	hintDurationForm = "a duration is a number and one unit, with no space: 30m, 4h, 7d, 2w"
)

// sqlCodes are the failures a word borrowed from SQL produces: something
// unexpected, or a field or value missing where the SQL keyword stood.
var sqlCodes = map[Code]bool{
	CodeUnexpected: true, CodeUnexpectedChar: true, CodeExpectedField: true, CodeExpectedValue: true,
}

func hintFor(q *query, e *Error) string {
	off := e.Position.Offset
	if off < 0 || off > len(q.input) {
		return ""
	}
	c := newContext(q, off)
	value := q.value

	if !value && sqlCodes[e.Code] {
		if h := c.sqlHint(); h != "" {
			return h
		}
	}

	switch e.Code {
	case CodeUnexpectedChar:
		return c.unexpectedChar(value)
	case CodeUnexpected:
		return c.unexpected(value)
	case CodeExpectedField:
		return c.expectedField()
	case CodeExpectedValue:
		return c.expectedValue(value)
	case CodeExpectedRange:
		return c.expectedRange()
	case CodeUnclosedParen:
		return c.unclosedParen()
	case CodeUnclosedString:
		return `add the closing quote; write a quote inside the text as \"`
	case CodeUnclosedFieldRef:
		if strings.EqualFold(c.wordBefore, "IN") {
			return "an IN list uses parentheses: state IN (draft, issued)"
		}
		return `add the closing "]"`
	case CodeEmptyFieldRef:
		return "put a field name between the brackets: [total]"
	case CodeExpectedInList:
		return "put the values in parentheses, separated by commas: state IN (draft, issued)"
	case CodeUnclosedIn:
		if c.atEnd() {
			return `add the closing ")"`
		}
		return "separate the values with commas: state IN (draft, issued)"
	case CodeEmptyInList:
		return "list at least one value: state IN (draft)"
	case CodeInvalidWildcard:
		return c.invalidWildcard(e.Position.Length)
	case CodeInvalidDate:
		return "a date is YYYY-MM-DD, optionally with a time: 2026-01-31 or 2026-01-31T10:00:00Z"
	case CodeInvalidDuration:
		return hintDurationForm
	case CodeExpectedSelector:
		return c.expectedSelector()
	case CodeUnclosedFuncArgs:
		if c.atEnd() {
			return `add the closing ")"`
		}
		if c.ch == '=' || c.ch == '<' || c.ch == '>' || c.ch == '!' {
			return "close the call before comparing: lower(name)=john"
		}
		return `separate arguments with commas; an argument is a field, a number or quoted text`
	case CodeMissingAnd:
		return c.missingAnd()
	case CodeQueryTooLong, CodeInvalidInteger, CodeInvalidFloat, CodeNestingTooDeep:
		return ""
	}
	return ""
}

// context is what the hints read: the text on each side of the error.
type context struct {
	*query
	off        int
	before     string // input before the error, trailing space removed
	rest       string // input from the error on
	ch         rune   // first character at the error, 0 at the end
	wordBefore string // the word that ends just before the error
	wordAt     string // the word that starts at the error
}

func newContext(q *query, off int) context {
	input := q.input
	c := context{query: q, off: off, rest: input[off:]}
	c.before = strings.TrimRightFunc(input[:off], unicode.IsSpace)
	if c.rest != "" {
		c.ch, _ = utf8.DecodeRuneInString(c.rest)
	}
	c.wordBefore = lastWord(c.before)
	c.wordAt = firstWord(c.rest)
	return c
}

func (c context) atEnd() bool { return strings.TrimSpace(c.rest) == "" }

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func lastWord(s string) string {
	end := len(s)
	start := end
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:start])
		if !isWordRune(r) {
			break
		}
		start -= size
	}
	return s[start:end]
}

func firstWord(s string) string {
	end := 0
	for end < len(s) {
		r, size := utf8.DecodeRuneInString(s[end:])
		if !isWordRune(r) {
			break
		}
		end += size
	}
	return s[:end]
}

func isArithmeticRune(r rune) bool {
	return r == '+' || r == '-' || r == '*' || r == '/' || r == '%'
}

func isConnective(word string) bool {
	return strings.EqualFold(word, "AND") || strings.EqualFold(word, "OR") || strings.EqualFold(word, "NOT")
}

// sqlHint answers when a word borrowed from SQL is used as a keyword in the
// clause that failed: at the error, or before it with no AND or OR between.
// A field may be called limit or from, so a word used as a name (limit>5,
// labels.order) is a field, not SQL; and text inside quotes is data.
func (c context) sqlHint() string {
	words := c.bareWords()
	last := len(words) - 1
	for last >= 0 && words[last].start > c.off {
		last--
	}
	for i := last; i >= 0; i-- {
		w := words[i]
		text := c.input[w.start:w.end]
		if strings.EqualFold(text, "AND") || strings.EqualFold(text, "OR") {
			if w.start < c.off && !c.betweenBefore(words, i) {
				return ""
			}
			continue
		}
		if w.field || isConnective(text) {
			continue
		}
		hint, ok := hintwords.SQL(text)
		if !ok {
			continue
		}
		if strings.EqualFold(text, "BETWEEN") {
			if q := c.betweenAsRange(w); q != "" {
				return "a range is written with a colon and two dots: " + q
			}
		}
		return hint
	}
	return ""
}

// betweenBefore reports whether the AND at words[i] is the one inside
// BETWEEN lo AND hi rather than a connective that ends the clause.
func (c context) betweenBefore(words []word, i int) bool {
	for j := i - 1; j >= 0 && j >= i-2; j-- {
		if strings.EqualFold(c.input[words[j].start:words[j].end], "BETWEEN") {
			return true
		}
	}
	return false
}

// fixed is the query with input[start:end] replaced by repl, when that makes
// it a valid condition, and "" otherwise. A hint that can show the writer's
// own query corrected says more than a generic example.
func (c context) fixed(start, end int, repl string) string {
	if start < 0 || end > len(c.input) || start > end {
		return ""
	}
	if q := c.input[:start] + repl + c.input[end:]; c.parses(q) {
		return q
	}
	return ""
}

// connective is the advice for & and |: the run of that character, and the
// space around it, becomes the word.
func (c context) connective(word, example string) string {
	start := len(c.before)
	end := c.off
	for end < len(c.input) && rune(c.input[end]) == c.ch {
		end++
	}
	for end < len(c.input) && (c.input[end] == ' ' || c.input[end] == '\t') {
		end++
	}
	if q := c.fixed(start, end, " "+word+" "); q != "" {
		return fmt.Sprintf("join conditions with %s: %s", word, q)
	}
	return fmt.Sprintf("join conditions with %s: %s", word, example)
}

func (c context) unexpectedChar(value bool) string {
	switch c.ch {
	case '&':
		return c.connective("AND", "state=draft AND total>5")
	case '|':
		return c.connective("OR", "state=draft OR state=issued")
	case '~':
		return "there is no ~ operator; match part of a text with a wildcard: name=*john*"
	case '^':
		return "there is no ^ operator; use pow(2, 3)"
	case ']':
		return `this "]" has no matching "["; groups and IN lists use parentheses`
	case '!':
		if strings.HasPrefix(strings.TrimLeftFunc(c.rest[1:], unicode.IsSpace), "=") && !strings.HasPrefix(c.rest[1:], "=") {
			return "write != with no space"
		}
		startsTerm := c.off == 0 || c.input[c.off-1] == ' ' || c.input[c.off-1] == '('
		if q := c.fixed(c.off, c.off+1, "NOT "); q != "" && startsTerm {
			return "negate a condition with NOT: " + q
		}
		return "negate a condition with NOT, or compare with !=: NOT state=draft, state!=draft"
	case '=', '<', '>':
		return c.doubledOperator()
	}
	if isArithmeticRune(c.ch) {
		if strings.HasPrefix(c.rest, "--") || (c.ch == '-' && strings.HasSuffix(c.input[:c.off], "-")) {
			return "comments are not supported"
		}
		if value {
			return "an operator needs a value on each side: [base]*1.1"
		}
		return c.arithmeticLeft()
	}
	return ""
}

// arithmeticLeft is the advice for arithmetic written left of the operator.
// When the query is a single comparison it shows that comparison rewritten,
// total*2>3 as float([total]*2)>3; otherwise it states the rule.
func (c context) arithmeticLeft() string {
	// The rewrite below drops spaces, which is only safe when none of them is
	// inside a quoted string.
	if strings.ContainsRune(c.input, '"') {
		return hintArithmeticLeft
	}
	for _, word := range strings.Fields(c.input) {
		if isConnective(word) {
			return hintArithmeticLeft
		}
	}
	cmp := strings.IndexAny(c.input[c.off:], "=<>!")
	if cmp < 0 {
		return hintArithmeticLeft
	}
	cmp += c.off
	left := strings.Join(strings.Fields(c.input[:cmp]), "")
	right := strings.Join(strings.Fields(c.input[cmp:]), "")
	if strings.HasPrefix(left, "(") && strings.HasSuffix(left, ")") {
		left = left[1 : len(left)-1]
	}
	// A leading bare name is a field, which arithmetic writes in brackets; a
	// call or a bracketed field is already an operand.
	if op := strings.IndexAny(left, "+-*/%"); op > 0 && !strings.ContainsAny(left[:op], "([") {
		left = "[" + left[:op] + "]" + left[op:]
	}
	// A bare word on the right is most likely the field being compared, so
	// the comparison is turned around: now()-7d<created_at.
	opLen := 0
	for opLen < len(right) && opLen < 2 && strings.ContainsRune("=<>!", rune(right[opLen])) {
		opLen++
	}
	if operand := right[opLen:]; isFieldName(operand) {
		if mirrored, ok := mirrorOperator[right[:opLen]]; ok {
			if rewrite := operand + mirrored + left; c.parses(rewrite) {
				return "the field goes on the left and arithmetic on the right: " + rewrite
			}
		}
	}
	if rewrite := "float(" + left + ")" + right; c.parses(rewrite) {
		return hintArithmeticRule + rewrite
	}
	return hintArithmeticLeft
}

// mirrorOperator is each comparison as read from the other side: a<b is b>a.
var mirrorOperator = map[string]string{"=": "=", "!=": "!=", "<": ">", "<=": ">=", ">": "<", ">=": "<="}

// doubledOperator handles a second operator character where the value should
// start: ==, =>, =<, <>, > =.
func (c context) doubledOperator() string {
	raw := c.input[:c.off]
	spaced := raw != c.before
	prev := byte(0)
	if c.before != "" {
		prev = c.before[len(c.before)-1]
	}
	// with is the hint followed by the writer's query with both characters
	// replaced by op, when that query is valid.
	with := func(hint, op string) string {
		if q := c.fixed(len(c.before)-1, c.off+1, op); q != "" {
			return hint + ": " + q
		}
		return hint
	}
	switch {
	case prev == '=' && c.ch == '=':
		return with("equality is a single =", "=")
	case prev == '=' && c.ch == '>':
		return with(`"greater than or equal" is written >=`, ">=")
	case prev == '=' && c.ch == '<':
		return with(`"less than or equal" is written <=`, "<=")
	case (prev == '<' && c.ch == '>') || (prev == '>' && c.ch == '<'):
		return with(`"not equal" is written !=`, "!=")
	case spaced && (prev == '>' || prev == '<' || prev == '!') && c.ch == '=':
		return with(fmt.Sprintf("write %c= with no space", prev), string(prev)+"=")
	}
	return fmt.Sprintf("to compare with text that starts with %q, put the value in quotes", string(c.ch))
}

// missingAnd is the hint for two conditions with no connective, under
// ExplicitAnd. A bracket directly after a name is a keyed lookup from another
// language, and joining it with AND would produce exactly the misreading the
// option exists to refuse, so it gets its own advice.
func (c context) missingAnd() string {
	if c.ch == '[' && c.off > 0 && c.before == c.input[:c.off] && isWordRune(rune(c.input[c.off-1])) {
		end := skipBracketed(c.input, c.off)
		key := strings.Trim(c.input[c.off+1:max(end-1, c.off+1)], `"`)
		name := lastWord(c.before)
		if key != "" && name != "" && !strings.ContainsAny(key, " \t\"[]") {
			dotted := name + "." + key
			if q := c.input[:c.off-len(name)] + dotted + c.input[end:]; c.parses(q) {
				return fmt.Sprintf("a key is written after a dot: %s", q)
			}
			return fmt.Sprintf("a key is written after a dot: %s", dotted)
		}
		return "a key is written after a dot: labels.team"
	}
	return `join two conditions with AND or OR: a=1 AND b=2; quote text that has a space: name="John Doe"`
}

func (c context) unexpected(value bool) string {
	if value {
		switch {
		case c.ch == '=' || c.ch == '<' || c.ch == '>' || c.ch == '!' || c.ch == ':':
			return hintNoCompareInValue
		case isConnective(c.wordAt) || strings.EqualFold(c.wordAt, "IN"):
			return hintNoCompareInValue
		case strings.EqualFold(c.wordAt, "mod"):
			return "the remainder operator is %: 10%3"
		}
		if unit, ok := hintwords.DurationUnit(c.wordAt); ok && endsWithDigit(c.before) {
			return fmt.Sprintf("write the unit as one letter, joined to the number: %s%s", lastWord(c.before), unit)
		}
		if unicode.IsLetter(c.ch) && endsWithDigit(c.before) {
			return hintDurationForm
		}
		if c.ch == ')' {
			return `this ")" has no matching "("`
		}
		return "a value expression is one value; join parts with an operator such as + or *"
	}
	switch {
	case isArithmeticRune(c.ch):
		return c.arithmeticLeft()
	case c.ch == ')':
		return `this ")" has no matching "("`
	case c.ch == '.':
		return "a dotted name has one dot between parts: labels.team"
	case c.ch == ',':
		return "conditions are joined with AND or OR, not commas; a list of values goes in IN (a, b)"
	}
	return "a condition ended before this; join two conditions with AND or OR"
}

func endsWithDigit(s string) bool {
	return s != "" && s[len(s)-1] >= '0' && s[len(s)-1] <= '9'
}

func (c context) expectedField() string {
	if strings.HasSuffix(c.before, ".") {
		if c.ch >= '0' && c.ch <= '9' {
			return "list elements are reached with a selector, not an index: tags@first or tags@(name=x)"
		}
		return "a dotted name needs a part after the dot: labels.team"
	}
	if c.atEnd() {
		switch {
		case c.before == "":
			return "write a condition such as state=draft"
		case isConnective(c.wordBefore):
			return fmt.Sprintf("a condition is missing after %s", strings.ToUpper(c.wordBefore))
		case strings.HasSuffix(c.before, "("):
			return `put a condition after "("`
		}
		return "a condition is missing here"
	}
	switch {
	case c.ch == ')':
		return "put a condition inside the parentheses: (state=draft OR state=issued)"
	case c.ch == '=' || c.ch == '<' || c.ch == '>' || c.ch == '!' || c.ch == ':':
		if strings.EqualFold(c.wordBefore, "AND") || strings.EqualFold(c.wordBefore, "OR") {
			return "each comparison needs its own field: total>=1 AND total<=5, or the range total:1..5"
		}
		return "a field name is missing before the operator: state=draft"
	case strings.EqualFold(c.wordAt, "AND") || strings.EqualFold(c.wordAt, "OR"):
		return fmt.Sprintf("a condition is missing before %s", strings.ToUpper(c.wordAt))
	case c.ch == '.':
		return "a name cannot start with a dot: labels.team"
	}
	if flipped := c.flipLiteralFirst(); flipped != "" {
		return "the field goes on the left: " + flipped
	}
	if (c.ch >= '0' && c.ch <= '9') || c.ch == '"' {
		return "a comparison starts with a field, then the operator, then the value: total>3"
	}
	return ""
}

// flipLiteralFirst rewrites `3<total` as `total>3` when the rest of the
// query is exactly a literal, an operator and a field name, dotted or not.
// With anything after the field (3<total AND 4<base) the rewrite would drop
// it, so there is none.
func (c context) flipLiteralFirst() string {
	i := 0
	for i < len(c.rest) && !strings.ContainsRune("=<>! \t", rune(c.rest[i])) {
		i++
	}
	literal := c.rest[:i]
	tail := strings.TrimLeft(c.rest[i:], " \t")
	opLen := 0
	for opLen < len(tail) && opLen < 2 && strings.ContainsRune("=<>!", rune(tail[opLen])) {
		opLen++
	}
	field := strings.TrimSpace(tail[opLen:])
	flipped, ok := mirrorOperator[tail[:opLen]]
	if !ok || literal == "" || !isFieldName(field) {
		return ""
	}
	if rewrite := c.input[:c.off] + field + flipped + literal; c.parses(rewrite) {
		return rewrite
	}
	return ""
}

// isFieldName reports whether s is one bare field name: letters, digits and
// underscores, with dots, slashes and hyphens between parts, as in
// labels.example.com/team. A number is not a name.
func isFieldName(s string) bool {
	if s == "" || hintwords.IsNumber(s) || !isWordStart(rune(s[0])) {
		return false
	}
	for _, r := range s {
		if !isWordRune(r) && r != '.' && r != '/' && r != '-' {
			return false
		}
	}
	last := s[len(s)-1]
	return last != '.' && last != '/' && last != '-'
}

func isWordStart(r rune) bool { return unicode.IsLetter(r) || r == '_' }

func (c context) expectedValue(value bool) string {
	if strings.HasSuffix(c.before, "..") || strings.HasPrefix(c.rest, "..") {
		return "a range needs both ends: total:1..5; for one side use >= or <="
	}
	if value {
		if c.before == "" {
			return "write a value such as [base]*1.1 or now()-7d"
		}
		return "an operator needs a value on each side: [base]*1.1"
	}
	if c.before != "" && isArithmeticRune(rune(c.before[len(c.before)-1])) {
		return "an operator needs a value on each side: total>[base]*1.1"
	}
	if c.atEnd() {
		return "add a value after the operator; to test that a field has a value, write the field alone"
	}
	if c.ch == ')' {
		return "add a value before the closing parenthesis"
	}
	return ""
}

func (c context) expectedRange() string {
	const general = `":" starts a range such as total:1..5; to compare, use =`
	// The error sits where ".." should be; the text since the colon shows
	// what was written instead.
	colon := strings.LastIndexByte(c.input[:c.off], ':')
	if colon < 0 {
		return general
	}
	field := fieldBefore(c.input[:colon])
	written := strings.TrimSpace(c.input[colon+1 : c.off])
	if field == "" || written == "" || strings.ContainsAny(written, " \t") {
		return general
	}
	if lo, hi, ok := strings.Cut(written, "-"); ok && hintwords.IsNumber(lo) && hintwords.IsNumber(hi) {
		return fmt.Sprintf("a range uses two dots: %s:%s..%s", field, lo, hi)
	}
	return fmt.Sprintf(`":" starts a range such as %s:1..5; to compare, write %s=%s`, field, field, written)
}

// fieldBefore returns the field name that ends s, as written: total,
// labels.team, [total] or ["billing address"]. It is "" when s does not end
// in a name.
func fieldBefore(s string) string {
	s = strings.TrimRightFunc(s, unicode.IsSpace)
	if strings.HasSuffix(s, "]") {
		if open := strings.LastIndexByte(s, '['); open >= 0 && skipBracketed(s, open) == len(s) {
			return s[open:]
		}
		return ""
	}
	start := len(s)
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:start])
		if !isWordRune(r) && r != '.' && r != '/' && r != '-' {
			break
		}
		start -= size
	}
	if name := s[start:]; isFieldName(name) {
		return name
	}
	return ""
}

func (c context) unclosedParen() string {
	switch {
	case c.atEnd():
		return `add the closing ")"`
	case isArithmeticRune(c.ch):
		return c.arithmeticLeft()
	case c.ch == ',':
		return "a list of values goes after IN: state IN (draft, issued), state NOT IN (draft, issued)"
	}
	return `close the group with ")" before this, or join conditions with AND or OR`
}

func (c context) invalidWildcard(length int) string {
	end := min(c.off+length, len(c.input))
	raw := c.input[c.off:end]
	if strings.Contains(raw, "**") {
		return "there is no ** operator; use pow(2, 3)"
	}
	if name, tail, ok := strings.Cut(raw, "*"); ok && name != "" && hintwords.IsNumber(tail) && firstWord(name) == name {
		return fmt.Sprintf("to multiply the field %s, write it in brackets: [%s]*%s", name, name, tail)
	}
	return `a star matches any text and may only start or end the value: jo*, *son, *oh*; write a literal star as \*`
}

var selectors = []string{"first", "last", "any", "all", "none"}

func (c context) expectedSelector() string {
	word := strings.ToLower(c.wordAt)
	switch word {
	case "":
		return "name what to ask of the list: tags@first, tags@(name=x), tags@all(name=x), tags@none(name=x)"
	case "any", "all", "none":
		return fmt.Sprintf("%s needs a condition in parentheses: tags@%s(name=x)", word, word)
	case "some", "exists":
		return "at least one element is tags@any(name=x), or the short form tags@(name=x)"
	case "every", "each":
		return "every element is tags@all(name=x)"
	case "no", "not":
		return "no element is tags@none(name=x)"
	case "count", "len", "size", "length":
		return "count a list with a function: count(tags)>2"
	}
	if near := nearest.Match(word, selectors); near != "" {
		return fmt.Sprintf("did you mean @%s?", near)
	}
	return "a selector is one of first, last, any(...), all(...) or none(...)"
}

// betweenAsRange rewrites `total BETWEEN 1 AND 5` as `total:1..5` for the
// BETWEEN at between, leaving the rest of the query as written. It returns ""
// when the words around it do not have that shape or the result is not a
// valid condition.
func (c context) betweenAsRange(between word) string {
	field := fieldBefore(c.input[:between.start])
	if field == "" {
		return ""
	}
	fieldStart := strings.LastIndex(c.input[:between.start], field)
	lo, afterLo := nextRun(c.input, between.end)
	and, afterAnd := nextRun(c.input, afterLo)
	hi, afterHi := nextRun(c.input, afterAnd)
	if lo == "" || hi == "" || !strings.EqualFold(and, "AND") {
		return ""
	}
	return c.fixed(fieldStart, afterHi, field+":"+lo+".."+hi)
}

// nextRun returns the run of non-space characters that starts at or after
// from, and the index just past it.
func nextRun(s string, from int) (run string, end int) {
	start := from
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end = start
	for end < len(s) && s[end] != ' ' && s[end] != '\t' {
		end++
	}
	return s[start:end], end
}
