package parser

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

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
// the mistake rather than the point.
func annotate(input string, err error, value bool) error {
	var list ErrorList
	if !errors.As(err, &list) {
		return err
	}
	list = mergeRepeats(input, list)
	for i, e := range list {
		if i == maxHinted {
			break
		}
		if e.Hint == "" {
			e.Hint = hintFor(input, e, value)
		}
	}
	return list
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

// parses reports whether q is a valid condition. Hints use it to check a
// rewrite before suggesting it, without going back through annotate.
func parses(q string) bool {
	if len(q) > maxRewriteLength {
		return false
	}
	l := &lexer{input: q}
	l.run()
	if len(l.errors) > 0 {
		return false
	}
	p := &parser{tokens: l.tokens}
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
	hintArithmeticRule = "arithmetic goes on the right of the operator or inside a function: "
	hintArithmeticLeft = hintArithmeticRule + "total>[base]*1.1 or float([base]*1.1)>3"
	hintNotSQL         = "only conditions are allowed here, such as field=value; " +
		"selecting, sorting and limiting are not part of a query"
	hintNoCompareInValue = "a value expression computes a value and cannot compare or combine; " +
		"evaluate this as a condition instead"
	hintDurationForm = "a duration is a number and one unit, with no space: 30m, 4h, 7d, 2w"
)

// sqlWords are words from SQL that have a different spelling here. The hint
// is given when the error sits on or just after one of them.
var sqlWords = map[string]string{
	"LIKE":     "there is no LIKE; use = with * as the wildcard: name=john*",
	"ILIKE":    "there is no ILIKE; = already ignores case, and * is the wildcard: name=john*",
	"BETWEEN":  "a range is written with a colon and two dots: total:1..5",
	"IS":       "to test for a missing value write NOT field; for a present one write the field alone",
	"NULL":     "to test for a missing value write NOT field; for a present one write the field alone",
	"CONTAINS": `to test for part of a text use a wildcard, name=*john*, or contains(name, "john")`,
	"SELECT":   hintNotSQL,
	"FROM":     hintNotSQL,
	"WHERE":    hintNotSQL,
	"ORDER":    hintNotSQL,
	"GROUP":    hintNotSQL,
	"LIMIT":    hintNotSQL,
	"OFFSET":   hintNotSQL,
}

// durationWords are spelled-out units, which a duration literal abbreviates.
var durationWords = map[string]string{
	"minute": "m", "minutes": "m", "min": "m", "mins": "m",
	"hour": "h", "hours": "h", "hr": "h", "hrs": "h",
	"day": "d", "days": "d",
	"week": "w", "weeks": "w",
}

func hintFor(input string, e *Error, value bool) string {
	off := e.Position.Offset
	if off < 0 || off > len(input) {
		return ""
	}
	c := newContext(input, off)

	if h := c.sqlHint(); h != "" && !value {
		return h
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
	case CodeQueryTooLong, CodeInvalidInteger, CodeInvalidFloat, CodeNestingTooDeep:
		return ""
	}
	return ""
}

// context is what the hints read: the text on each side of the error.
type context struct {
	input      string
	off        int
	before     string // input before the error, trailing space removed
	rest       string // input from the error on
	ch         rune   // first character at the error, 0 at the end
	wordBefore string // the word that ends just before the error
	wordAt     string // the word that starts at the error
}

func newContext(input string, off int) context {
	c := context{input: input, off: off, rest: input[off:]}
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

// sqlHint answers when a word borrowed from SQL is used as a keyword at or
// before the error. A field may be called limit or from, so a word followed
// by an operator (limit>5) or part of a dotted name is a field, not SQL.
func (c context) sqlHint() string {
	end := c.off + len(c.wordAt)
	for i := 0; i < end; {
		r, size := utf8.DecodeRuneInString(c.input[i:])
		if !isWordRune(r) {
			i += size
			continue
		}
		word := firstWord(c.input[i:])
		next := strings.TrimLeftFunc(c.input[i+len(word):], unicode.IsSpace)
		usedAsField := next != "" && strings.ContainsRune("=<>!:.@(", rune(next[0]))
		dotted := i > 0 && (c.input[i-1] == '.' || c.input[i-1] == '[' || c.input[i-1] == '"')
		if h, ok := sqlWords[strings.ToUpper(word)]; ok && !usedAsField && !dotted {
			if strings.EqualFold(word, "BETWEEN") {
				if q := c.betweenAsRange(); q != "" {
					return "a range is written with a colon and two dots: " + q
				}
			}
			return h
		}
		i += len(word)
	}
	return ""
}

// fixed is the query with input[start:end] replaced by repl, when that makes
// it a valid condition, and "" otherwise. A hint that can show the writer's
// own query corrected says more than a generic example.
func (c context) fixed(start, end int, repl string) string {
	if start < 0 || end > len(c.input) || start > end {
		return ""
	}
	if q := c.input[:start] + repl + c.input[end:]; parses(q) {
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
	if operand := right[opLen:]; operand != "" && firstWord(operand) == operand && !isNumber(operand) {
		if mirrored, ok := mirrorOperator[right[:opLen]]; ok {
			if rewrite := operand + mirrored + left; parses(rewrite) {
				return "the field goes on the left and arithmetic on the right: " + rewrite
			}
		}
	}
	if rewrite := "float(" + left + ")" + right; parses(rewrite) {
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
		if unit, ok := durationWords[strings.ToLower(c.wordAt)]; ok && endsWithDigit(c.before) {
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

// flipLiteralFirst rewrites `3<total` as `total>3` when the text at the error
// is a literal, an operator and a plain field name; otherwise it is "".
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
	op := tail[:opLen]
	field := firstWord(strings.TrimLeft(tail[opLen:], " \t"))
	if literal == "" || field == "" || (field[0] >= '0' && field[0] <= '9') {
		return ""
	}
	flipped, ok := mirrorOperator[op]
	if !ok || !parses(field+flipped+literal) {
		return ""
	}
	return field + flipped + literal
}

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
	// The error sits where ".." should be; the text since the colon shows
	// what was written instead.
	colon := strings.LastIndexByte(c.input[:c.off], ':')
	if colon < 0 {
		return ""
	}
	field := lastWord(strings.TrimRightFunc(c.input[:colon], unicode.IsSpace))
	written := strings.TrimSpace(c.input[colon+1 : c.off])
	if lo, hi, ok := strings.Cut(written, "-"); ok && isNumber(lo) && isNumber(hi) {
		return fmt.Sprintf("a range uses two dots: %s:%s..%s", field, lo, hi)
	}
	if field != "" && written != "" && !strings.ContainsAny(written, " \t") {
		return fmt.Sprintf(`":" starts a range such as %s:1..5; to compare, write %s=%s`, field, field, written)
	}
	return `":" starts a range such as total:1..5; to compare, use =`
}

func isNumber(s string) bool {
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
	if name, tail, ok := strings.Cut(raw, "*"); ok && name != "" && isNumber(tail) && firstWord(name) == name {
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

// betweenAsRange rewrites `total BETWEEN 1 AND 5` as `total:1..5`, wherever
// it sits in the query, or returns "" when the words do not have that shape
// or the result is not a valid condition.
func (c context) betweenAsRange() string {
	words := strings.Fields(c.input)
	for i, word := range words {
		if !strings.EqualFold(word, "BETWEEN") || i == 0 || i+3 >= len(words) || !strings.EqualFold(words[i+2], "AND") {
			continue
		}
		parts := append([]string{}, words[:i-1]...)
		parts = append(parts, words[i-1]+":"+words[i+1]+".."+words[i+3])
		parts = append(parts, words[i+4:]...)
		if q := strings.Join(parts, " "); parses(q) {
			return q
		}
	}
	return ""
}
