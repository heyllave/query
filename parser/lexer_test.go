package parser

import (
	"testing"

	"github.com/heyllave/query/token"
)

func TestLex_SimpleOperators(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   []token.Type
		values []string
	}{
		{"equality", "state=draft",
			[]token.Type{token.Ident, token.Eq, token.String, token.EOF},
			[]string{"state", "=", "draft", ""}},
		{"not equal", "state!=cancelled",
			[]token.Type{token.Ident, token.Neq, token.String, token.EOF},
			[]string{"state", "!=", "cancelled", ""}},
		{"greater than", "year>2020",
			[]token.Type{token.Ident, token.Gt, token.Integer, token.EOF},
			[]string{"year", ">", "2020", ""}},
		{"gte", "total>=50000",
			[]token.Type{token.Ident, token.Gte, token.Integer, token.EOF},
			[]string{"total", ">=", "50000", ""}},
		{"less than", "year<2025",
			[]token.Type{token.Ident, token.Lt, token.Integer, token.EOF},
			[]string{"year", "<", "2025", ""}},
		{"lte", "total<=99999",
			[]token.Type{token.Ident, token.Lte, token.Integer, token.EOF},
			[]string{"total", "<=", "99999", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := Lex(tt.input, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertTokenTypes(t, tokens, tt.want)
			assertTokenValues(t, tokens, tt.values)
		})
	}
}

func TestLex_Keywords(t *testing.T) {
	tokens, err := Lex("a=1 AND b=2 OR NOT c=3", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []token.Type{
		token.Ident, token.Eq, token.Integer, token.And,
		token.Ident, token.Eq, token.Integer, token.Or,
		token.Not, token.Ident, token.Eq, token.Integer, token.EOF,
	}
	assertTokenTypes(t, tokens, want)
}

func TestLex_Parentheses(t *testing.T) {
	tokens, err := Lex("(a=1 OR b=2)", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []token.Type{token.LParen, token.Ident, token.Eq, token.Integer, token.Or, token.Ident, token.Eq, token.Integer, token.RParen, token.EOF}
	assertTokenTypes(t, tokens, want)
}

func TestLex_Wildcards(t *testing.T) {
	tests := []struct {
		name  string
		input string
		value string
	}{
		{"prefix", "name=John*", "John*"},
		{"suffix", "name=*ohn", "*ohn"},
		{"contains", "name=*oh*", "*oh*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := Lex(tt.input, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tokens[2].Type != token.Wildcard {
				t.Errorf("got %v, want Wildcard", tokens[2].Type)
			}
			if tokens[2].Value != tt.value {
				t.Errorf("got %q, want %q", tokens[2].Value, tt.value)
			}
		})
	}
}

func TestLex_InvalidWildcard(t *testing.T) {
	_, err := Lex("name=a*b*c", 0)
	if err == nil {
		t.Fatal("expected error for invalid wildcard")
	}
}

func TestLex_ValueTypes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		typ   token.Type
		value string
	}{
		{"date", "created_at>=2026-01-01", token.Date, "2026-01-01"},
		{"duration_d", "ttl>1d", token.Duration, "1d"},
		{"duration_h", "ttl>4h", token.Duration, "4h"},
		{"duration_m", "ttl>30m", token.Duration, "30m"},
		{"duration_w", "ttl>2w", token.Duration, "2w"},
		{"boolean_true", "active=true", token.Boolean, "true"},
		{"boolean_false", "active=false", token.Boolean, "false"},
		{"float", "total>=50000.50", token.Float, "50000.50"},
		{"negative_int", "offset>=-10", token.Integer, "-10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := Lex(tt.input, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tokens[2].Type != tt.typ {
				t.Errorf("got %v, want %v", tokens[2].Type, tt.typ)
			}
			if tokens[2].Value != tt.value {
				t.Errorf("got %q, want %q", tokens[2].Value, tt.value)
			}
		})
	}
}

func TestLex_DottedField(t *testing.T) {
	tokens, err := Lex("labels.dev=jane", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []token.Type{token.Ident, token.Dot, token.Ident, token.Eq, token.String, token.EOF}
	assertTokenTypes(t, tokens, want)
}

func TestLex_Escape(t *testing.T) {
	tokens, err := Lex(`name=hello\*world`, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[2].Type != token.String {
		t.Errorf("got %v, want String", tokens[2].Type)
	}
	if tokens[2].Value != "hello*world" {
		t.Errorf("got %q, want %q", tokens[2].Value, "hello*world")
	}
}

func TestLex_ColonRange(t *testing.T) {
	tokens, err := Lex("created_at:2026-01-01..2026-03-31", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []token.Type{token.Ident, token.Colon, token.Date, token.Range, token.Date, token.EOF}
	assertTokenTypes(t, tokens, want)
}

func TestLex_Empty(t *testing.T) {
	tokens, err := Lex("", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Type != token.EOF {
		t.Errorf("expected single EOF, got %v", tokens)
	}
}

func TestLex_TooLong(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	_, err := Lex(string(long), 256)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLex_IllegalChar(t *testing.T) {
	_, err := Lex("state=draft & active=true", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLex_Position(t *testing.T) {
	tokens, err := Lex("a=1 AND b=2", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[3].Pos.Offset != 4 {
		t.Errorf("AND offset: got %d, want 4", tokens[3].Pos.Offset)
	}
	if tokens[3].Pos.Length != 3 {
		t.Errorf("AND length: got %d, want 3", tokens[3].Pos.Length)
	}
}

func assertTokenTypes(t *testing.T, got []token.Token, want []token.Type) {
	t.Helper()
	if len(got) != len(want) {
		types := make([]token.Type, len(got))
		for i, tok := range got {
			types[i] = tok.Type
		}
		t.Fatalf("got %d tokens %v, want %d %v", len(got), types, len(want), want)
	}
	for i, tok := range got {
		if tok.Type != want[i] {
			t.Errorf("token[%d]: got %v, want %v", i, tok.Type, want[i])
		}
	}
}

func assertTokenValues(t *testing.T, got []token.Token, want []string) {
	t.Helper()
	for i, tok := range got {
		if i < len(want) && tok.Value != want[i] {
			t.Errorf("token[%d]: got value %q, want %q", i, tok.Value, want[i])
		}
	}
}
