package parser

import (
	"strings"
	"unicode/utf8"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// Slot is what the grammar expects at a position in a query that is still
// being written.
type Slot int

// The slots a cursor can be in.
const (
	// SlotNone is a position nothing can be said about: the text before it
	// does not read as a query, or what belongs there is punctuation.
	SlotNone Slot = iota
	// SlotField is where a condition starts: a field name, or NOT.
	SlotField
	// SlotKey is a segment after a dot in a field name: labels.<here>.
	SlotKey
	// SlotOperator follows a complete field name: an operator, a selector's
	// '@', or a connective when the bare name is a condition by itself.
	SlotOperator
	// SlotSelector follows the '@' after a field.
	SlotSelector
	// SlotValue is where a value belongs: after an operator, in an IN list,
	// or as the operand of arithmetic.
	SlotValue
	// SlotConnective follows a complete condition: AND, OR, or the ')' that
	// closes the group around it.
	SlotConnective
	// SlotArgument is a fresh argument of a function call.
	SlotArgument
)

var slotNames = [...]string{
	SlotNone:       "none",
	SlotField:      "field",
	SlotKey:        "key",
	SlotOperator:   "operator",
	SlotSelector:   "selector",
	SlotValue:      "value",
	SlotConnective: "connective",
	SlotArgument:   "argument",
}

// String returns the slot's name, the form a bridge carries it in.
func (s Slot) String() string {
	if int(s) >= 0 && int(s) < len(slotNames) {
		return slotNames[s]
	}
	return "none"
}

// Cursor describes a position in a query that is still being written: what
// the grammar expects there and the text typed toward it so far. It is what
// completion is built from; see [CursorAt].
type Cursor struct {
	// Slot is what belongs at the position.
	Slot Slot
	// Word is the text a suggestion replaces: the word the cursor is in or
	// at the end of, and an empty span at the cursor when it is in none.
	// When Quoted or Bracketed it starts at the opening quote or bracket.
	Word token.Position
	// Prefix is the part of the word before the cursor. Inside a quoted
	// string or a bracketed name it is decoded: without the opening quote or
	// bracket and with escapes resolved. An unquoted word is as it was
	// typed, backslashes included.
	Prefix string
	// Quoted reports that the cursor is inside a "..." string still open
	// before it.
	Quoted bool
	// Bracketed reports that the cursor is inside a [...] field name still
	// open before it.
	Bracketed bool
	// Field is the field the slot is about: the one an operator, selector or
	// value belongs to, and for SlotKey the segments before the one being
	// typed. It is nil when the left side is a function.
	Field ast.FieldPath
	// FieldSpan is, for SlotKey, the whole field name written so far, Word
	// included, so a suggestion can rewrite the name in another form.
	FieldSpan token.Position
	// Operator is, for SlotValue, the operator the value follows: a
	// comparison, [token.Colon] for a range, [token.In] inside an IN list.
	Operator token.Type
	// Function is, for SlotArgument, the function being called. For
	// SlotValue it is the function on the left of the comparison, when
	// there is one (`len(name)>` has Field nil and Function "len"): the
	// value is compared with what the function gives, not with a field.
	Function string
	// Arithmetic reports, for SlotValue, that the value is an operand of
	// arithmetic: it follows `+ - * / %` in `total>10 - <here>`. Only a
	// number, a date or a duration can stand there; text would end the
	// arithmetic and leave the sign behind.
	Arithmetic bool
	// Negated reports, for SlotOperator, that NOT follows the field, so
	// only `IN (` can come: `name NOT <here>`.
	Negated bool
	// Lists names the selectors the position is inside, outermost first:
	// for `forks@(items@(<here>` it is forks, then items. Field names at the
	// position are those of one element of the innermost.
	Lists []ast.FieldPath
	// Closable reports that a ')' closing a group or a selector could come
	// at the position.
	Closable bool
}

// CursorAt reports what the grammar expects at byte offset in input. Only
// the text before the offset decides the slot; the text after it only
// extends [Cursor.Word] to the end of the word the cursor is in.
//
// It never fails. A position it cannot place (the text before it does not
// read as a query, the offset is outside the input or splits a character)
// has SlotNone. So has every position of an input longer than
// cfg.MaxLength: a query over the limit will be refused whole, so nothing
// is suggested for it. A host with long queries raises the limit for both.
func CursorAt(input string, offset int, cfg Config) Cursor {
	none := Cursor{Word: token.Position{Offset: offset}}
	if offset < 0 || offset > len(input) {
		none.Word.Offset = 0
		return none
	}
	if checkInput(input, cfg.MaxLength) != nil {
		return none
	}
	prefix := input[:offset]
	if !utf8.ValidString(prefix) {
		return none
	}

	l := &lexer{input: prefix, strictValues: cfg.StrictValues}
	l.run()
	tokens := l.tokens[:len(l.tokens)-1] // drop EOF

	c := Cursor{Word: token.Position{Offset: offset}}
	switch len(l.errors) {
	case 0:
		if n := len(tokens); n > 0 && isWord(tokens[n-1]) && end(tokens[n-1].Pos) == offset {
			last := tokens[n-1]
			tokens = tokens[:n-1]
			c.Word = last.Pos
			c.Prefix = last.Value
			if last.Type == token.Wildcard || last.Raw != "" {
				// A pattern's Value is in pattern form; what was typed is
				// the source text.
				c.Prefix = prefix[last.Pos.Offset:]
			}
		}
	case 1:
		e := l.errors[0]
		if end(e.Position) != offset || (e.Code != CodeUnclosedString && e.Code != CodeUnclosedFieldRef) {
			return none
		}
		if n := len(tokens); n > 0 && tokens[n-1].Pos.Offset >= e.Position.Offset {
			return none
		}
		c.Word = e.Position
		if e.Code == CodeUnclosedString {
			c.Quoted = true
			c.Prefix = openString(prefix, e.Position.Offset)
		} else {
			c.Bracketed = true
			c.Prefix = openFieldRef(prefix, e.Position.Offset)
		}
	default:
		return none
	}

	m := machine{tokens: tokens, explicitAnd: cfg.ExplicitAnd}
	m.run()
	m.place(&c)
	if c.Slot == SlotNone {
		return Cursor{Word: token.Position{Offset: offset}}
	}
	c.Word.Length = wordEnd(input, offset, c, cfg) - c.Word.Offset
	if c.Slot == SlotKey {
		c.FieldSpan.Length = end(c.Word) - c.FieldSpan.Offset
	}
	return c
}

func end(p token.Position) int { return p.Offset + p.Length }

// isWord reports whether a token is text that is typed a character at a
// time, so one that ends at the cursor is still being written. A closed
// quoted string or bracketed name is finished; punctuation is not a word.
func isWord(t token.Token) bool {
	switch t.Type { //nolint:exhaustive // only word tokens
	case token.Ident, token.And, token.Or, token.Not, token.In, token.Boolean,
		token.Integer, token.Float, token.Date, token.Duration, token.Wildcard:
		return true
	case token.String:
		return !t.Quoted
	default:
		return false
	}
}

// openString decodes the string whose opening quote is at start and that
// runs to the end of s.
func openString(s string, start int) string {
	l := &lexer{input: s, pos: start}
	text, _ := l.readQuotedString()
	return text
}

// openFieldRef returns the name inside the bracket at start, which runs to
// the end of s: the decoded text of the quoted form, the trimmed text of the
// bare one.
func openFieldRef(s string, start int) string {
	l := &lexer{input: s, pos: start + 1}
	l.skipWhitespace()
	if l.pos < len(s) && s[l.pos] == '"' {
		text, _ := l.readQuotedString()
		return text
	}
	return strings.TrimSpace(s[l.pos:])
}

// wordEnd returns where the word under the cursor ends in the whole input,
// so a suggestion replaces all of it and not only the part before the
// cursor. A position at the start of a word (nothing typed toward it) is
// left alone: the text after it is the next thing, not this one.
func wordEnd(input string, offset int, c Cursor, cfg Config) int {
	l := &lexer{input: input, strictValues: cfg.StrictValues}
	switch {
	case c.Quoted:
		// The closing quote is this string's only when the whole input has
		// its quotes in pairs; otherwise the next one found opens something
		// later.
		l.run()
		for _, e := range l.errors {
			if e.Code == CodeUnclosedString || e.Code == CodeUnclosedFieldRef {
				return offset
			}
		}
		l = &lexer{input: input, pos: c.Word.Offset}
		if _, ok := l.readQuotedString(); ok {
			return l.pos
		}
		return offset
	case c.Bracketed:
		return bracketEnd(input, offset, c.Word.Offset)
	case c.Word.Length == 0:
		return offset
	case c.Slot != SlotValue:
		return l.identEnd(offset)
	}
	i := offset
	for i < len(input) && l.spaceWidth(i) == 0 && strings.IndexByte(`(),"[]`, input[i]) < 0 {
		i++
	}
	// A word that is the name of a call is replaced with its arguments, so
	// no parenthesis is left behind: name=lo|wer(x).
	if i < len(input) && input[i] == '(' {
		if closed := l.skipCall(i); closed > 0 {
			return closed
		}
	}
	return i
}

// bracketEnd returns where the bracketed name opening at open ends, for a
// cursor at offset inside it: past the closing quote of a quoted name and
// past the ']' when it is there. A name left open ends at the cursor, since
// what follows is then the rest of the query and not part of the name.
func bracketEnd(input string, offset, open int) int {
	l := &lexer{input: input, pos: open + 1}
	l.skipWhitespace()
	end := offset
	if l.pos < len(input) && input[l.pos] == '"' {
		if _, closed := l.readQuotedString(); !closed || l.pos < offset {
			return offset
		}
		end = l.pos
	} else {
		for end < len(input) && (input[end] == '.' || l.identEnd(end) > end) {
			if input[end] == '.' {
				end++
				continue
			}
			end = l.identEnd(end)
		}
	}
	rest := end
	for rest < len(input) && l.spaceWidth(rest) > 0 {
		rest += l.spaceWidth(rest)
	}
	switch {
	case rest < len(input) && input[rest] == ']':
		return rest + 1
	case end == offset:
		return offset
	case rest == len(input), strings.IndexByte("=!<>:)", input[rest]) >= 0:
		// The name ended and its bracket is missing: `["labels.my"=x`. The
		// closing quote is still the name's own.
		return end
	}
	// Anything else after the quote means the quote found belongs to a
	// later string, and the name runs on: `["labels.my AND x="y"`.
	return offset
}

type cursorState int

const (
	stTerm     cursorState = iota // a condition starts here
	stField                       // after a field name
	stDot                         // after a dot in a field name
	stAt                          // after the '@' of a selector
	stIn                          // after IN, before its '('
	stOperand                     // a value belongs here
	stValue                       // after a value
	stDone                        // after a complete condition
	stArg                         // a fresh function argument
	stArgRest                     // inside a function argument
	stFieldNot                    // after a field name and NOT
	stLost                        // the tokens do not read as a query
)

type frameKind int

const (
	frameGroup     frameKind = iota // ( ... )
	frameSelector                   // list@( ... ), @any( @all( @none(
	frameList                       // IN ( ... )
	frameCall                       // fn( ... ) where a condition starts
	frameValueCall                  // fn( ... ) where a value belongs
	frameParen                      // ( ... ) inside a value or an argument
)

// place is what the machine knows about the condition being written.
type place struct {
	field      ast.FieldPath
	fieldStart int
	op         token.Type
	fn         string
	// arith marks that the operand being written follows an arithmetic
	// sign.
	arith bool
	// left is the function on the left of a comparison, when the left side
	// is a call and not a field.
	left string
	// ranged marks that the '..' of a range has been written, so the value
	// after it ends the condition.
	ranged bool
	// call marks a finished function call, which may still turn out to be
	// the left side of a comparison.
	call bool
}

type frame struct {
	kind  frameKind
	saved place
	list  ast.FieldPath
}

// machine follows the tokens before the cursor far enough to know what
// comes next. It is not the parser: it builds nothing and refuses nothing
// the parser would accept, and where the tokens stop making sense it is
// lost and says so, rather than guessing.
type machine struct {
	// explicitAnd is the grammar's switch: without an implicit AND, only a
	// connective can follow a complete condition.
	explicitAnd bool
	tokens      []token.Token
	i           int
	st          cursorState
	at          place
	frames      []frame
}

func (m *machine) run() {
	for m.i = 0; m.i < len(m.tokens) && m.st != stLost; m.i++ {
		m.step(m.tokens[m.i])
	}
}

func (m *machine) next() token.Token {
	if m.i+1 >= len(m.tokens) {
		return token.Token{Type: token.EOF}
	}
	return m.tokens[m.i+1]
}

// adjacent reports whether the next token starts where tok ends.
func (m *machine) adjacent(tok token.Token) bool {
	return m.next().Pos.Offset == end(tok.Pos)
}

func (m *machine) push(kind frameKind) {
	m.frames = append(m.frames, frame{kind: kind, saved: m.at, list: m.at.field})
}

func (m *machine) top() (frameKind, bool) {
	if len(m.frames) == 0 {
		return 0, false
	}
	return m.frames[len(m.frames)-1].kind, true
}

func (m *machine) pop() {
	n := len(m.frames)
	if n == 0 {
		m.st = stLost
		return
	}
	f := m.frames[n-1]
	m.frames = m.frames[:n-1]
	called := m.at.fn
	m.at = f.saved
	if kind, ok := m.top(); ok && (kind == frameCall || kind == frameValueCall) {
		m.st = stArgRest
		return
	}
	switch f.kind {
	case frameValueCall, frameParen:
		m.st = stValue
	case frameCall:
		// A call is a condition by itself or the left side of one.
		m.at = place{call: true, left: called}
		m.st = stDone
	default:
		m.st = stDone
	}
}

// halfRange reports whether the value just read is the first of a range,
// so the '..' and the second are still owed.
func (m *machine) halfRange() bool {
	return m.at.op == token.Colon && !m.at.ranged
}

func (m *machine) field(path ast.FieldPath, tok token.Token) {
	m.at = place{field: path, fieldStart: tok.Pos.Offset}
	m.st = stField
}

func startsTerm(t token.Type) bool {
	switch t { //nolint:exhaustive // only term starters
	case token.Ident, token.LParen, token.Not, token.FieldRef, token.String, token.Boolean, token.In:
		return true
	default:
		return false
	}
}

func isValueToken(t token.Token) bool {
	switch t.Type { //nolint:exhaustive // only value tokens
	case token.String, token.Integer, token.Float, token.Date, token.Duration,
		token.Boolean, token.Wildcard, token.FieldRef:
		return true
	default:
		return false
	}
}

//nolint:gocyclo // one case per state, as the grammar has them
func (m *machine) step(tok token.Token) {
	switch m.st {
	case stTerm:
		m.stepTerm(tok)
	case stField:
		switch {
		case tok.Type == token.Dot:
			m.st = stDot
		case tok.Type == token.At:
			m.st = stAt
		case tok.Type.IsOperator():
			m.at.op = tok.Type
			m.st = stOperand
		case tok.Type == token.In:
			m.st = stIn
		case tok.Type == token.Not && m.next().Type == token.In:
			m.i++
			m.st = stIn
		case tok.Type == token.Not && m.i == len(m.tokens)-1:
			// NOT with nothing after it yet: `name NOT`, where IN may come.
			m.st = stFieldNot
		default:
			m.afterCondition(tok)
		}
	case stFieldNot:
		// Unreachable: the state is only entered on the last token.
		m.st = stLost
	case stDot:
		if tok.Type != token.Ident && !tok.Type.IsKeyword() {
			m.st = stLost
			return
		}
		m.at.field = append(m.at.field[:len(m.at.field):len(m.at.field)], tok.Value)
		m.st = stField
	case stAt:
		m.stepSelector(tok)
	case stIn:
		if tok.Type != token.LParen {
			m.st = stLost
			return
		}
		m.push(frameList)
		m.at.op = token.In
		m.st = stOperand
	case stOperand:
		switch {
		case tok.Type == token.Ident && m.next().Type == token.LParen:
			m.push(frameValueCall)
			m.at = place{fn: tok.Value}
			m.i++
			m.st = stArg
		case tok.Type == token.Ident, isValueToken(tok):
			m.st = stValue
		case tok.Type == token.LParen:
			m.push(frameParen)
		default:
			m.st = stLost
		}
	case stValue:
		kind, framed := m.top()
		switch {
		case tok.Type.IsArithmetic():
			m.at.arith = true
			m.st = stOperand
		case tok.Type == token.Range:
			m.at.ranged = true
			m.at.arith = false
			m.st = stOperand
		case tok.Type == token.Comma && framed && kind == frameList:
			m.at.arith = false
			m.st = stOperand
		case tok.Type == token.RParen:
			m.pop()
		case m.halfRange():
			m.st = stLost
		case framed && (kind == frameList || kind == frameParen):
			m.st = stLost
		default:
			m.afterCondition(tok)
		}
	case stDone:
		switch {
		case tok.Type.IsOperator() && m.at.call:
			// fn(...) as the left side of a comparison.
			m.at = place{op: tok.Type, left: m.at.left}
			m.st = stOperand
		case tok.Type == token.In && m.at.call:
			m.at = place{left: m.at.left}
			m.st = stIn
		default:
			m.afterCondition(tok)
		}
	case stArg, stArgRest:
		switch {
		case tok.Type == token.Comma:
			m.st = stArg
		case tok.Type == token.RParen:
			m.pop()
		case tok.Type == token.Ident && m.next().Type == token.LParen:
			m.push(frameValueCall)
			m.at = place{fn: tok.Value}
			m.i++
			m.st = stArg
		case tok.Type == token.LParen:
			m.push(frameParen)
			m.st = stArgRest
		default:
			m.st = stArgRest
		}
	case stLost:
	}
}

// afterCondition takes the token that follows something complete: a
// connective, the ')' that closes the group around it, or the start of the
// next condition under an implicit AND.
func (m *machine) afterCondition(tok token.Token) {
	switch {
	case tok.Type.IsLogical():
		m.st = stTerm
	case tok.Type == token.RParen:
		m.pop()
	case startsTerm(tok.Type) && !m.explicitAnd:
		m.stepTerm(tok)
	default:
		m.st = stLost
	}
}

func (m *machine) stepTerm(tok token.Token) {
	m.st = stTerm
	namesField := func() bool {
		next := m.next().Type
		return next.IsOperator() || next == token.Dot || next == token.At || next == token.In
	}
	switch tok.Type { //nolint:exhaustive // only tokens that can start a condition
	case token.Not:
		if namesField() {
			m.field(ast.FieldPath{tok.Value}, tok)
		}
	case token.And, token.Or:
		if !namesField() {
			m.st = stLost
			return
		}
		m.field(ast.FieldPath{tok.Value}, tok)
	case token.LParen:
		m.push(frameGroup)
		m.at = place{}
	case token.RParen:
		m.pop()
	case token.Ident:
		if m.next().Type == token.LParen && m.adjacent(tok) {
			m.push(frameCall)
			m.at = place{fn: tok.Value}
			m.i++
			m.st = stArg
			return
		}
		m.field(ast.FieldPath{tok.Value}, tok)
	case token.FieldRef:
		if tok.Quoted {
			m.field(ast.FieldPath{tok.Value}, tok)
			return
		}
		m.field(ast.FieldPath(strings.Split(tok.Value, ".")), tok)
	case token.String:
		if !tok.Quoted {
			m.st = stLost
			return
		}
		m.field(ast.FieldPath{tok.Value}, tok)
	case token.Boolean, token.In:
		m.field(ast.FieldPath{tok.Value}, tok)
	default:
		m.st = stLost
	}
}

func (m *machine) stepSelector(tok token.Token) {
	switch {
	case tok.Type == token.Ident && (tok.Value == ast.SelectorFirst || tok.Value == ast.SelectorLast):
		m.at = place{}
		m.st = stDone
	case tok.Type == token.Ident && m.next().Type == token.LParen &&
		(tok.Value == ast.SelectorAny || tok.Value == ast.SelectorAll || tok.Value == ast.SelectorNone):
		m.i++
		m.push(frameSelector)
		m.at = place{}
		m.st = stTerm
	case tok.Type == token.LParen:
		m.push(frameSelector)
		m.at = place{}
		m.st = stTerm
	default:
		m.st = stLost
	}
}

// place fills in the slot the machine ended in.
func (m *machine) place(c *Cursor) {
	for _, f := range m.frames {
		if f.kind == frameSelector {
			c.Lists = append(c.Lists, f.list)
		}
	}
	kind, framed := m.top()
	closable := framed && (kind == frameGroup || kind == frameSelector)
	inValue := framed && (kind == frameList || kind == frameParen)

	if c.Bracketed {
		// A bracketed name is a field where a condition starts and a field
		// reference where a value belongs; only the first is completed.
		if m.st != stTerm {
			return
		}
		c.Slot = SlotField
		if i := strings.LastIndexByte(c.Prefix, '.'); i >= 0 {
			c.Slot = SlotKey
			c.Field = ast.FieldPath(strings.Split(c.Prefix[:i], "."))
			c.Prefix = c.Prefix[i+1:]
			c.FieldSpan = c.Word
		}
		return
	}
	if c.Quoted && m.st != stOperand {
		// A quoted string is a value; where a condition starts it is a
		// field name or free text, which nothing is known about.
		return
	}

	switch m.st {
	case stTerm:
		c.Slot = SlotField
	case stDot:
		c.Slot = SlotKey
		c.Field = m.at.field
		c.FieldSpan.Offset = m.at.fieldStart
	case stField:
		c.Slot = SlotOperator
		c.Field = m.at.field
		c.Closable = closable
	case stAt:
		c.Slot = SlotSelector
		c.Field = m.at.field
	case stFieldNot:
		c.Slot = SlotOperator
		c.Field = m.at.field
		c.Negated = true
	case stOperand:
		c.Slot = SlotValue
		c.Field = m.at.field
		c.Operator = m.at.op
		c.Function = m.at.left
		c.Arithmetic = m.at.arith
	case stValue, stDone:
		if inValue || (m.st == stValue && m.halfRange()) {
			// Inside an IN list or a parenthesised value only ',' or ')'
			// can follow, and after the first value of a range only '..'.
			return
		}
		c.Slot = SlotConnective
		c.Closable = closable
	case stArg:
		c.Slot = SlotArgument
		c.Function = m.at.fn
	case stIn, stArgRest, stLost:
	}
}
