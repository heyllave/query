package parser

import (
	"testing"

	"github.com/heyllave/query/token"
)

// lexerAt is a lexer positioned at the end of prefix, with prefix's tokens,
// as the lexer is just before it reads what follows.
func lexerAt(t *testing.T, prefix string, m mode, brackets ...bracket) *lexer {
	t.Helper()
	l := lexAll(prefix, Config{})
	l.tokens = l.tokens[:len(l.tokens)-1] // drop EOF
	l.pos = len(prefix)
	l.mode = m
	l.brackets = brackets
	return l
}

func TestOpenBracket_WhatAParenOpens(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prefix   string
		mode     mode
		want     bracket
		wantMode mode
	}{
		{"after IN, a list", "x IN ", modeAfterIn, bracketList, modeValue},
		{"right after a name, a call", "lower", modeField, bracketCall, modeField},
		{"after a space, a group", "lower ", modeField, bracketGroup, modeField},
		{"at the start, a group", "", modeField, bracketGroup, modeField},
		{"after AND, a group", "a=1 AND ", modeField, bracketGroup, modeField},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := lexerAt(t, tc.prefix, tc.mode)
			if got := l.openBracket(); got != tc.want || l.mode != tc.wantMode {
				t.Errorf("openBracket() = %v, mode %v; want %v, mode %v", got, l.mode, tc.want, tc.wantMode)
			}
			if len(l.brackets) != 1 || l.brackets[0] != tc.want {
				t.Errorf("brackets = %v, want [%v]", l.brackets, tc.want)
			}
		})
	}
}

func TestCloseBracket_ForgetsTheInnermost(t *testing.T) {
	l := lexerAt(t, "", modeField, bracketList, bracketCall)
	l.closeBracket()
	if !l.inList() {
		t.Error("after closing the call, the list is not the innermost bracket")
	}
	l.closeBracket()
	l.closeBracket() // one more than is open is not an error
	if len(l.brackets) != 0 || l.inList() {
		t.Errorf("brackets = %v after closing everything", l.brackets)
	}
}

func TestExpectOperand_EndsAtACommaOnlyInACall(t *testing.T) {
	for _, tc := range []struct {
		name     string
		brackets []bracket
		want     mode
	}{
		{"in a call", []bracket{bracketCall}, modeCallOperand},
		{"in a group", []bracket{bracketGroup}, modeValue},
		{"in a list", []bracket{bracketList}, modeValue},
		{"outside any bracket", nil, modeValue},
		{"in a group inside a call", []bracket{bracketCall, bracketGroup}, modeValue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := lexerAt(t, "", modeField, tc.brackets...)
			l.expectOperand()
			if l.mode != tc.want {
				t.Errorf("mode = %v, want %v", l.mode, tc.want)
			}
		})
	}
}

func TestStartValue_WhenACommaEndsTheValue(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     mode
		brackets []bracket
		want     bool
	}{
		{"an IN-list slot", modeValue, []bracket{bracketList}, true},
		{"a sign's operand in a call", modeCallOperand, []bracket{bracketCall}, true},
		{"a value after = in a call", modeValue, []bracket{bracketCall}, false},
		{"a value outside brackets", modeValue, nil, false},
		{"a value in a group inside a list", modeValue, []bracket{bracketList, bracketGroup}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := lexerAt(t, "", tc.mode, tc.brackets...)
			l.startValue()
			if l.commaEnds != tc.want || l.mode != modeField {
				t.Errorf("commaEnds = %v, mode %v; want %v, modeField", l.commaEnds, l.mode, tc.want)
			}
		})
	}
}

func TestAfterComma_NextListSlotIsAValue(t *testing.T) {
	inList := lexerAt(t, "", modeField, bracketList)
	inList.afterComma()
	inCall := lexerAt(t, "", modeField, bracketCall)
	inCall.afterComma()
	if inList.mode != modeValue || inCall.mode != modeField {
		t.Errorf("after a comma: in a list %v, in a call %v; want modeValue, modeField", inList.mode, inCall.mode)
	}
}

func TestLeaveAfterIn(t *testing.T) {
	for m, want := range map[mode]mode{modeAfterIn: modeField, modeField: modeField, modeValue: modeValue} {
		l := lexerAt(t, "", m)
		l.leaveAfterIn()
		if l.mode != want {
			t.Errorf("leaveAfterIn from %v = %v, want %v", m, l.mode, want)
		}
	}
}

// An IN inside a call's arguments is a keyword there, not the start of a
// list: the group after the call was lexed as an IN list, so b=1 came out
// as one piece of text.
func TestLex_AnInInsideACallDoesNotOpenAList(t *testing.T) {
	for _, q := range []string{"a=f(x in) (b=1)", "a=g(f(x in)) (b=1)", "a=1+f(in) (b=1)"} {
		toks, err := lexWith(q, Config{})
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		var types []token.Type
		for _, tk := range toks[len(toks)-6:] {
			types = append(types, tk.Type)
		}
		want := []token.Type{token.LParen, token.Ident, token.Eq, token.Integer, token.RParen, token.EOF}
		for i := range want {
			if types[i] != want[i] {
				t.Errorf("%s: the group lexes as %v, want %v", q, types, want)
				break
			}
		}
	}
}
