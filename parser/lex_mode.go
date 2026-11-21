package parser

// mode is what the lexer expects next. Every change of mode is made by one
// of the methods in this file, so each can be read and tested on its own.
type mode uint8

const (
	// modeField: a field, a connective or a punctuation mark.
	modeField mode = iota
	// modeAfterIn: as modeField, right after the keyword IN in field
	// position. A '(' here opens an IN list; anything else is lexed as in
	// modeField (IN was then a field name: IN=1).
	modeAfterIn
	// modeValue: a value, after an operator or in an IN-list slot.
	modeValue
	// modeCallOperand: a value that is the operand of a sign inside a
	// call's arguments, `f(a - 3, b)`, where a ',' ends it.
	modeCallOperand
)

// expectsValue reports whether a value is lexed next.
func (m mode) expectsValue() bool {
	return m == modeValue || m == modeCallOperand
}

// bracket is what a '(' opened in field position means.
type bracket uint8

const (
	bracketGroup bracket = iota // a group of conditions: (a OR b)
	bracketCall                 // a call's arguments: lower(name)
	bracketList                 // an IN list: x IN (a, b)
)

// innermost is the kind of the innermost open bracket, and false when none
// is open.
func (l *lexer) innermost() (bracket, bool) {
	if len(l.brackets) == 0 {
		return 0, false
	}
	return l.brackets[len(l.brackets)-1], true
}

// inList reports whether the innermost open bracket is an IN list.
func (l *lexer) inList() bool {
	b, ok := l.innermost()
	return ok && b == bracketList
}

// openBracket records the '(' at l.pos and returns what it opens: an IN
// list right after IN, a call's arguments right after a name, otherwise a
// group. An IN list is followed by a value.
func (l *lexer) openBracket() bracket {
	kind := bracketGroup
	switch {
	case l.mode == modeAfterIn:
		kind = bracketList
	case l.afterName():
		kind = bracketCall
	}
	l.brackets = append(l.brackets, kind)
	l.mode = modeField
	if kind == bracketList {
		l.mode = modeValue
	}
	return kind
}

// closeBracket forgets the innermost open bracket, if any.
func (l *lexer) closeBracket() {
	if n := len(l.brackets); n > 0 {
		l.brackets = l.brackets[:n-1]
	}
}

// expectValue is the mode after an operator: a value comes next.
func (l *lexer) expectValue() {
	l.mode = modeValue
}

// expectOperand is the mode after a sign that continues a value: inside a
// call's arguments the operand ends at a ',', elsewhere it is a value.
func (l *lexer) expectOperand() {
	l.mode = modeValue
	if b, ok := l.innermost(); ok && b == bracketCall {
		l.mode = modeCallOperand
	}
}

// sawIn records IN in field position: a '(' next opens a list.
func (l *lexer) sawIn() {
	l.mode = modeAfterIn
}

// leaveAfterIn drops modeAfterIn before anything but a '(' is lexed in field
// position.
func (l *lexer) leaveAfterIn() {
	if l.mode == modeAfterIn {
		l.mode = modeField
	}
}

// startValue enters the reading of one value: it notes whether a ',' ends
// it, and returns to field position for whatever follows the value.
func (l *lexer) startValue() {
	l.commaEnds = l.mode == modeCallOperand || l.inList()
	l.mode = modeField
}

// afterComma is the mode after a ',' in field position: the next slot of an
// IN list is a value.
func (l *lexer) afterComma() {
	if l.inList() {
		l.mode = modeValue
	}
}
