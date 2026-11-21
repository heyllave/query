package parser

import (
	"sort"
	"unicode/utf8"

	"github.com/heyllave/query/parser/internal/chars"
	"github.com/heyllave/query/token"
)

// The lexer is split by where it is in a query, one lex_*.go file each, and
// calls internal/chars, internal/literal and internal/shape for every rule
// that needs no lexer state. CLAUDE.md lists the files.

// lexer tokenizes a query string into a sequence of tokens.
type lexer struct {
	input  string
	pos    int
	start  int
	tokens []token.Token
	errors ErrorList
	// mode is what comes next, and brackets what each '(' still open in
	// field position means. They change only in lex_mode.go.
	mode     mode
	brackets []bracket
	// commaEnds is set while a bare value is read where a ',' separates
	// values instead of being part of one: an IN-list slot, or the operand
	// of a sign inside a call's arguments.
	commaEnds bool
	// strictValues refuses a comparison character inside an unquoted value;
	// see Config.
	strictValues bool
}

// Lex tokenizes the input query string and returns the token stream.
func Lex(input string, maxLength int) ([]token.Token, error) {
	return lexWith(input, Config{MaxLength: maxLength})
}

func lexWith(input string, cfg Config) ([]token.Token, error) {
	if err := checkInput(input, cfg.MaxLength); err != nil {
		return nil, err
	}

	l := lexAll(input, cfg)
	if len(l.errors) > 0 {
		errs := l.errors
		if cfg.ExplicitAnd {
			errs = withMissingAnds(errs, l.tokens)
		}
		return nil, annotate(input, errs, false, cfg)
	}
	return l.tokens, nil
}

// LexValue tokenizes input as a value expression, starting in value mode so a
// leading literal, arithmetic expression, or function call lexes correctly at
// position 0 (where the boolean grammar would expect a field name). It backs
// [ParseValue].
func LexValue(input string, maxLength int) ([]token.Token, error) {
	if err := checkInput(input, maxLength); err != nil {
		return nil, err
	}

	l := &lexer{input: input, mode: modeValue}
	l.run()

	if err := l.errors.errOrNil(); err != nil {
		return nil, annotate(input, err, true, Config{MaxLength: maxLength})
	}
	return l.tokens, nil
}

// withMissingAnds adds, to the errors of a query that did not lex, the
// missing connectives that come before the first of them. Without this a
// query with both mistakes (`a=1 b==2` under ExplicitAnd) reports only the
// lexical one, and the writer who fixes it is handed a second error the first
// answer did not mention. Tokens before the first lexical error are what the
// lexer would have produced anyway, so a missing connective found among them
// is real; anything later is read from a stream the error already disturbed
// and is left out.
func withMissingAnds(errs ErrorList, tokens []token.Token) ErrorList {
	first := errs[0].Position.Offset
	for _, e := range errs[1:] {
		first = min(first, e.Position.Offset)
	}
	p := &parser{tokens: tokens, explicitAnd: true}
	p.parseExpression()
	var out ErrorList
	for _, e := range p.errors {
		if e.Code == CodeMissingAnd && e.Position.Offset < first {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return errs
	}
	out = append(out, errs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position.Offset < out[j].Position.Offset })
	return out
}

// checkInput rejects input that is longer than maxLength characters (when
// maxLength is positive) or is not valid UTF-8. A query is text: a byte that
// is not part of a character cannot name a field or a value, could not be
// carried to a client as JSON, and is reported at its position.
func checkInput(input string, maxLength int) error {
	if n := utf8.RuneCountInString(input); maxLength > 0 && n > maxLength {
		return ErrorList{newError(CodeQueryTooLong, ErrQueryTooLong, token.Position{},
			"query length %d exceeds maximum of %d characters", n, maxLength)}
	}
	if i := invalidByte(input); i >= 0 {
		return ErrorList{newError(CodeUnexpectedChar, ErrSyntax,
			token.Position{Offset: i, Length: 1},
			"byte 0x%02x at position %d is not valid UTF-8", input[i], i)}
	}
	return nil
}

// invalidByte returns the offset of the first byte of s that is not part of
// a UTF-8 character, or -1.
func invalidByte(s string) int {
	if utf8.ValidString(s) {
		return -1
	}
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && w == 1 {
			return i
		}
		i += w
	}
	return -1
}

// lexAll lexes the whole of input in field position, under cfg's
// StrictValues, and returns the lexer with its tokens and errors.
func lexAll(input string, cfg Config) *lexer {
	l := &lexer{input: input, strictValues: cfg.StrictValues}
	l.run()
	return l
}

// run lexes the whole input. Each token starts either in value position,
// right after an operator, or in field position.
func (l *lexer) run() {
	for l.pos < len(l.input) {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}
		l.start = l.pos
		if l.mode.expectsValue() {
			l.lexValue()
			continue
		}
		l.lexFieldPosition()
	}
	l.tokens = append(l.tokens, token.Token{Type: token.EOF, Pos: token.Position{Offset: l.pos}})
}

// emit appends a token of the given text starting at l.start.
func (l *lexer) emit(typ token.Type, value string) {
	l.tokens = append(l.tokens, token.Token{
		Type:  typ,
		Value: value,
		Pos:   token.Position{Offset: l.start, Length: len(value)},
	})
}

// emitArith appends the arithmetic operator at l.pos and moves past it.
func (l *lexer) emitArith(ch byte) {
	l.emit(arithTokenFor(ch), string(ch))
	l.pos++
}

// syntaxError records a syntax error over input[off:off+length]. An error
// that carries a hint is built with newError and added by its caller.
func (l *lexer) syntaxError(code Code, off, length int, format string, args ...any) {
	l.errors.add(newError(code, ErrSyntax, token.Position{Offset: off, Length: length}, format, args...))
}

// unexpectedRune reports the whole rune at l.pos, not its first byte, so the
// message shows what was typed and the span covers it, and moves past it.
// format takes the rune as its one argument.
func (l *lexer) unexpectedRune(format string) {
	_, width := utf8.DecodeRuneInString(l.input[l.pos:])
	l.syntaxError(CodeUnexpectedChar, l.pos, width, format, l.input[l.pos:l.pos+width])
	l.pos += width
}

func (l *lexer) peek(offset int) byte {
	return chars.At(l.input, l.pos+offset)
}

func (l *lexer) skipWhitespace() {
	l.pos = chars.SkipSpace(l.input, l.pos)
}
