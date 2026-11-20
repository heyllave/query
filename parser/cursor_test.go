package parser

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// cursorCase is a query with a '|' where the cursor is, and what CursorAt
// must say about that position. word is the text of Cursor.Word.
type cursorCase struct {
	query    string
	slot     Slot
	prefix   string
	word     string
	field    string
	op       token.Type
	lists    string // dotted names joined with a space
	fn       string
	closable bool
	quoted   bool
	bracket  bool
}

// split removes the '|' cursor marker from a marked query.
func split(t *testing.T, marked string) (query string, offset int) {
	t.Helper()
	offset = strings.IndexByte(marked, '|')
	if offset < 0 || strings.Count(marked, "|") != 1 {
		t.Fatalf("case %q needs exactly one '|'", marked)
	}
	return marked[:offset] + marked[offset+1:], offset
}

func checkCursor(t *testing.T, tc cursorCase, cfg Config) {
	t.Helper()
	query, offset := split(t, tc.query)
	c := CursorAt(query, offset, cfg)
	if c.Slot != tc.slot {
		t.Fatalf("slot = %s, want %s", c.Slot, tc.slot)
	}
	if c.Prefix != tc.prefix {
		t.Errorf("prefix = %q, want %q", c.Prefix, tc.prefix)
	}
	if got := query[c.Word.Offset : c.Word.Offset+c.Word.Length]; got != tc.word {
		t.Errorf("word = %q, want %q", got, tc.word)
	}
	if got := c.Field.String(); got != tc.field {
		t.Errorf("field = %q, want %q", got, tc.field)
	}
	if c.Operator != tc.op {
		t.Errorf("operator = %s, want %s", c.Operator, tc.op)
	}
	var lists []string
	for _, l := range c.Lists {
		lists = append(lists, l.String())
	}
	if got := strings.Join(lists, " "); got != tc.lists {
		t.Errorf("lists = %q, want %q", got, tc.lists)
	}
	if c.Function != tc.fn {
		t.Errorf("function = %q, want %q", c.Function, tc.fn)
	}
	if c.Closable != tc.closable {
		t.Errorf("closable = %v, want %v", c.Closable, tc.closable)
	}
	if c.Quoted != tc.quoted {
		t.Errorf("quoted = %v, want %v", c.Quoted, tc.quoted)
	}
	if c.Bracketed != tc.bracket {
		t.Errorf("bracketed = %v, want %v", c.Bracketed, tc.bracket)
	}
}

func TestCursorAt_Slots(t *testing.T) {
	cases := []cursorCase{
		// Where a condition starts.
		{query: "|", slot: SlotField},
		{query: "   |", slot: SlotField},
		{query: "na|", slot: SlotField, prefix: "na", word: "na"},
		{query: "name|", slot: SlotField, prefix: "name", word: "name"},
		{query: "a=1 AND |", slot: SlotField},
		{query: "a=1 and na|", slot: SlotField, prefix: "na", word: "na"},
		{query: "a=1 OR |", slot: SlotField},
		{query: "NOT |", slot: SlotField},
		{query: "NOT na|", slot: SlotField, prefix: "na", word: "na"},
		{query: "a=1 AND NOT |", slot: SlotField},
		{query: "(|", slot: SlotField},
		{query: "((na|", slot: SlotField, prefix: "na", word: "na"},
		{query: "a=1 OR (b=2 AND |", slot: SlotField},
		{query: "NO|", slot: SlotField, prefix: "NO", word: "NO"},
		{query: "NOT|", slot: SlotField, prefix: "NOT", word: "NOT"},
		{query: "tru|", slot: SlotField, prefix: "tru", word: "tru"},
		{query: "café|", slot: SlotField, prefix: "café", word: "café"},
		{query: "10|", slot: SlotField, prefix: "10", word: "10"},

		// After a complete field name.
		{query: "name |", slot: SlotOperator, field: "name"},
		{query: "labels.team |", slot: SlotOperator, field: "labels.team"},
		{query: "[labels.team]|", slot: SlotOperator, field: "labels.team"},
		{query: `["my field"]|`, slot: SlotOperator, field: "my field"},
		{query: `"my field" |`, slot: SlotOperator, field: "my field"},
		{query: "(name |", slot: SlotOperator, field: "name", closable: true},
		{query: "name I|", slot: SlotOperator, field: "name", prefix: "I", word: "I"},
		{query: "not=1 AND in |", slot: SlotOperator, field: "in"},
		{query: "true |", slot: SlotOperator, field: "true"},

		// A key after a dot.
		{query: "labels.|", slot: SlotKey, field: "labels"},
		{query: "labels.te|", slot: SlotKey, field: "labels", prefix: "te", word: "te"},
		{query: "labels.app.kubernetes.io/na|", slot: SlotKey, field: "labels.app.kubernetes", prefix: "io/na", word: "io/na"},
		{query: "a=1 AND labels.in|", slot: SlotKey, field: "labels", prefix: "in", word: "in"},
		{query: "not.|", slot: SlotKey, field: "not"},

		// Selectors.
		{query: "forks@|", slot: SlotSelector, field: "forks"},
		{query: "forks@al|", slot: SlotSelector, field: "forks", prefix: "al", word: "al"},
		{query: "forks@first|", slot: SlotSelector, field: "forks", prefix: "first", word: "first"},
		{query: "forks@first |", slot: SlotConnective},
		{query: "forks@(|", slot: SlotField, lists: "forks"},
		{query: "forks@any(|", slot: SlotField, lists: "forks"},
		{query: "forks@all(na|", slot: SlotField, lists: "forks", prefix: "na", word: "na"},
		{query: "forks@none(kind=|", slot: SlotValue, lists: "forks", field: "kind", op: token.Eq},
		{query: "forks@(kind=x |", slot: SlotConnective, lists: "forks", closable: true},
		{query: "forks@(kind=x) |", slot: SlotConnective},
		{query: "forks@(items@(|", slot: SlotField, lists: "forks items"},
		{query: "forks@(items@(n=1) AND |", slot: SlotField, lists: "forks"},
		{query: "a.b@(c.d@all(e |", slot: SlotOperator, lists: "a.b c.d", field: "e", closable: true},
		{query: "NOT forks@(|", slot: SlotField, lists: "forks"},

		// Values.
		{query: "name=|", slot: SlotValue, field: "name", op: token.Eq},
		{query: "name = |", slot: SlotValue, field: "name", op: token.Eq},
		{query: "name!=|", slot: SlotValue, field: "name", op: token.Neq},
		{query: "total>=|", slot: SlotValue, field: "total", op: token.Gte},
		{query: "total!<|", slot: SlotValue, field: "total", op: token.Nlt},
		{query: "name=dr|", slot: SlotValue, field: "name", op: token.Eq, prefix: "dr", word: "dr"},
		{query: "name=pr-10|", slot: SlotValue, field: "name", op: token.Eq, prefix: "pr-10", word: "pr-10"},
		{query: "name=dr*|", slot: SlotValue, field: "name", op: token.Eq, prefix: "dr*", word: "dr*"},
		{query: "total>10|", slot: SlotValue, field: "total", op: token.Gt, prefix: "10", word: "10"},
		{query: "ttl>7d|", slot: SlotValue, field: "ttl", op: token.Gt, prefix: "7d", word: "7d"},
		{query: "created>2026-01-0|", slot: SlotValue, field: "created", op: token.Gt, prefix: "2026-01-0", word: "2026-01-0"},
		{query: "ready=tr|", slot: SlotValue, field: "ready", op: token.Eq, prefix: "tr", word: "tr"},
		{query: "labels.team=|", slot: SlotValue, field: "labels.team", op: token.Eq},
		{query: "[labels.team]=pl|", slot: SlotValue, field: "labels.team", op: token.Eq, prefix: "pl", word: "pl"},
		{query: "total:|", slot: SlotValue, field: "total", op: token.Colon},
		{query: "total:1..|", slot: SlotValue, field: "total", op: token.Colon},
		{query: "total:1..5|", slot: SlotValue, field: "total", op: token.Colon, prefix: "5", word: "5"},
		{query: "a=1 AND b=|", slot: SlotValue, field: "b", op: token.Eq},
		{query: "(a=1 OR b=|", slot: SlotValue, field: "b", op: token.Eq},
		{query: "lower(name)=|", slot: SlotValue, op: token.Eq, fn: "lower"},
		{query: "lower(name)=jo|", slot: SlotValue, op: token.Eq, prefix: "jo", word: "jo", fn: "lower"},
		{query: "len(lower(name))>|", slot: SlotValue, op: token.Gt, fn: "len"},
		{query: "a=1 AND len(name):1..|", slot: SlotValue, op: token.Colon, fn: "len"},
		{query: "name=x AND total>|", slot: SlotValue, field: "total", op: token.Gt},

		// Quoted values.
		{query: `name="|`, slot: SlotValue, field: "name", op: token.Eq, word: `"`, quoted: true},
		{query: `name="fo|`, slot: SlotValue, field: "name", op: token.Eq, prefix: "fo", word: `"fo`, quoted: true},
		{query: `name="a b|`, slot: SlotValue, field: "name", op: token.Eq, prefix: "a b", word: `"a b`, quoted: true},
		{query: `name="a\"b|`, slot: SlotValue, field: "name", op: token.Eq, prefix: `a"b`, word: `"a\"b`, quoted: true},
		{query: `name="fo|o bar"`, slot: SlotValue, field: "name", op: token.Eq, prefix: "fo", word: `"foo bar"`, quoted: true},
		{query: `name="fo|o bar" AND x="y"`, slot: SlotValue, field: "name", op: token.Eq, prefix: "fo", word: `"foo bar"`, quoted: true},
		{query: `name="fo| AND x="y"`, slot: SlotValue, field: "name", op: token.Eq, prefix: "fo", word: `"fo`, quoted: true},
		{query: `name IN ("a", "b|`, slot: SlotValue, field: "name", op: token.In, prefix: "b", word: `"b`, quoted: true},

		// IN lists.
		{query: "name IN (|", slot: SlotValue, field: "name", op: token.In},
		{query: "name in (a, |", slot: SlotValue, field: "name", op: token.In},
		{query: "name IN (a, b|", slot: SlotValue, field: "name", op: token.In, prefix: "b", word: "b"},
		{query: "name NOT IN (|", slot: SlotValue, field: "name", op: token.In},
		{query: "name IN (a, b) |", slot: SlotConnective},
		{query: "(name IN (a) |", slot: SlotConnective, closable: true},
		{query: "lower(name) IN (|", slot: SlotValue, op: token.In, fn: "lower"},
		{query: "lower(name) IN (a, |", slot: SlotValue, op: token.In, fn: "lower"},

		// After a complete condition.
		{query: "a=1 |", slot: SlotConnective},
		{query: "a=1 AN|", slot: SlotConnective, prefix: "AN", word: "AN"},
		{query: "a=1 na|", slot: SlotConnective, prefix: "na", word: "na"},
		{query: `a="x"|`, slot: SlotConnective},
		{query: `a="x" |`, slot: SlotConnective},
		{query: "a=[b] |", slot: SlotConnective},
		{query: "(a=1 |", slot: SlotConnective, closable: true},
		{query: "(a=1) |", slot: SlotConnective},
		{query: "((a=1) |", slot: SlotConnective, closable: true},
		{query: "a:1..5 |", slot: SlotConnective},
		{query: "a>10+5 |", slot: SlotConnective},
		{query: "a>(1+2)*3 |", slot: SlotConnective},
		{query: "lower(name) |", slot: SlotConnective},
		{query: "contains(tags, \"x\") |", slot: SlotConnective},
		{query: "created>now() |", slot: SlotConnective},

		// Function arguments.
		{query: "lower(|", slot: SlotArgument, fn: "lower"},
		{query: "lower(na|", slot: SlotArgument, fn: "lower", prefix: "na", word: "na"},
		{query: "contains(tags, |", slot: SlotArgument, fn: "contains"},
		{query: "f(g(|", slot: SlotArgument, fn: "g"},
		{query: "f(g(a), |", slot: SlotArgument, fn: "f"},
		{query: "created>addDays(|", slot: SlotArgument, fn: "addDays"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			checkCursor(t, tc, Config{})
			checkCursor(t, tc, Config{}.Strict())
		})
	}
}

func TestCursorAt_WordReachesTheEndOfTheWordUnderTheCursor(t *testing.T) {
	cases := []cursorCase{
		{query: "na|me=x", slot: SlotField, prefix: "na", word: "name"},
		{query: "a=1 AND na|me", slot: SlotField, prefix: "na", word: "name"},
		{query: "name=dra|ft AND ttl>1d", slot: SlotValue, field: "name", op: token.Eq, prefix: "dra", word: "draft"},
		{query: "name IN (dra|ft, issued)", slot: SlotValue, field: "name", op: token.In, prefix: "dra", word: "draft"},
		{query: "(name=dra|ft)", slot: SlotValue, field: "name", op: token.Eq, prefix: "dra", word: "draft"},
		{query: "labels.te|am=x", slot: SlotKey, field: "labels", prefix: "te", word: "team"},
		{query: "forks@al|l(x=1)", slot: SlotSelector, field: "forks", prefix: "al", word: "all"},
		{query: "a=1 AN|D b=2", slot: SlotConnective, prefix: "AN", word: "AND"},
		// A word that names a call goes with its arguments.
		{query: "name=lo|wer(x)", slot: SlotValue, field: "name", op: token.Eq, prefix: "lo", word: "lower(x)"},
		{query: "name=lo|wer(trim(x)) AND b=1", slot: SlotValue, field: "name", op: token.Eq, prefix: "lo", word: "lower(trim(x))"},
		{query: "name=lo|wer(x", slot: SlotValue, field: "name", op: token.Eq, prefix: "lo", word: "lower"},
		// Nothing typed toward the slot: the text after the cursor is the
		// next thing and is left alone.
		{query: "|name=x", slot: SlotField},
		{query: "a=1 AND |name=x", slot: SlotField},
		{query: "name=|draft", slot: SlotValue, field: "name", op: token.Eq},
		{query: "name |=draft", slot: SlotOperator, field: "name"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) { checkCursor(t, tc, Config{}) })
	}
}

func TestCursorAt_BracketedFieldNames(t *testing.T) {
	cases := []cursorCase{
		{query: "[|", slot: SlotField, word: "[", bracket: true},
		{query: "[lab|", slot: SlotField, prefix: "lab", word: "[lab", bracket: true},
		{query: `["my fi|`, slot: SlotField, prefix: "my fi", word: `["my fi`, bracket: true},
		{query: "[labels.te|", slot: SlotKey, field: "labels", prefix: "te", word: "[labels.te", bracket: true},
		{query: `["labels.my k|`, slot: SlotKey, field: "labels", prefix: "my k", word: `["labels.my k`, bracket: true},
		{query: `["labels.my k|ey"]=x`, slot: SlotKey, field: "labels", prefix: "my k", word: `["labels.my key"]`, bracket: true},
		{query: "a=1 AND [lab|", slot: SlotField, prefix: "lab", word: "[lab", bracket: true},
		// The name's own closing quote and bracket go with it, whichever of
		// them is there.
		{query: `["labels.my|"`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my"`, bracket: true},
		{query: `["labels.my|"=x`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my"`, bracket: true},
		{query: `["labels.my|" ]=x`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my" ]`, bracket: true},
		{query: "[labels.te|am]=x", slot: SlotKey, field: "labels", prefix: "te", word: "[labels.team]", bracket: true},
		{query: "[labels.te|am=x", slot: SlotKey, field: "labels", prefix: "te", word: "[labels.team", bracket: true},
		{query: "[lab|els", slot: SlotField, prefix: "lab", word: "[labels", bracket: true},
		// A quote further on belongs to a later string, so the name ends
		// at the cursor.
		{query: `["labels.my| AND x="y"`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my`, bracket: true},
		// With its quote closed and a connective after it, the quote is the
		// name's own.
		{query: `["labels.my|" AND name="y"`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my"`, bracket: true},
		{query: `["labels.my| key" OR a=1`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my key"`, bracket: true},
		{query: `(["labels.my|")`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my"`, bracket: true},
		{query: `["labels.my| AND x="y`, slot: SlotKey, field: "labels", prefix: "my", word: `["labels.my`, bracket: true},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) { checkCursor(t, tc, Config{}) })
	}
	t.Run("the whole name is the span of a key", func(t *testing.T) {
		query, offset := split(t, "a=1 AND labels.app.te|am=x")
		c := CursorAt(query, offset, Config{})
		if got := query[c.FieldSpan.Offset : c.FieldSpan.Offset+c.FieldSpan.Length]; got != "labels.app.team" {
			t.Fatalf("field span = %q, want %q", got, "labels.app.team")
		}
	})
}

// What follows a complete condition depends on the grammar: with an
// implicit AND the next condition may start, without one only a connective
// can come.
func TestCursorAt_TheGrammarDecidesWhatFollowsACondition(t *testing.T) {
	strict := Config{}.Strict()
	for _, tc := range []struct {
		query   string
		lenient Slot
		strict  Slot
	}{
		{"a b c |", SlotOperator, SlotNone},
		{"name=x NOT |", SlotField, SlotNone},
		{"name=x (|", SlotField, SlotNone},
		{"name=x b|", SlotConnective, SlotConnective},
		{"name=x b |", SlotOperator, SlotNone},
		{"name=x b=|", SlotValue, SlotNone},
		{"(a=1) b|", SlotConnective, SlotConnective},
		{"forks@first (|", SlotField, SlotNone},
		{"lower(name) x=|", SlotValue, SlotNone},
		// With a connective both grammars agree.
		{"name=x AND NOT |", SlotField, SlotField},
		{"name=x AND (|", SlotField, SlotField},
		{"name=x OR b=|", SlotValue, SlotValue},
	} {
		query, offset := split(t, tc.query)
		if got := CursorAt(query, offset, Config{}).Slot; got != tc.lenient {
			t.Errorf("%s under the default grammar: slot = %s, want %s", tc.query, got, tc.lenient)
		}
		if got := CursorAt(query, offset, strict).Slot; got != tc.strict {
			t.Errorf("%s under the strict grammar: slot = %s, want %s", tc.query, got, tc.strict)
		}
	}
}

func TestCursorAt_AnOperandOfArithmetic(t *testing.T) {
	for marked, want := range map[string]bool{
		"total>10+|":           false, // one word so far, "10+": a sign with nothing after it is not arithmetic yet
		"total>10 - 3|":        true,
		"total>10 - 3|0":       true,
		"total>10 - |":         true,
		"total>10 -|":          true,
		"total>10 *|":          true,
		"created>now() - |":    true,
		`name="a" + |`:         true,
		"total>10*2+|":         true,
		"total>(1+|":           true,
		"created>now()-|":      true,
		"total>10+5|":          true,
		"total>|":              false,
		"total>10 AND cpu>|":   false,
		"total>10+5 AND cpu>|": false,
		"total:1+1..|":         false,
		"total:1..2+3|":        true,
		"total IN (1+1, |":     false,
		"total IN (1, 2+3|":    true,
	} {
		query, offset := split(t, marked)
		c := CursorAt(query, offset, Config{})
		if c.Slot == SlotNone && !want {
			continue
		}
		if c.Slot != SlotValue || c.Arithmetic != want {
			t.Errorf("%s: slot %s arithmetic %v, want a value with arithmetic %v", marked, c.Slot, c.Arithmetic, want)
		}
	}
}

// A date still being typed is one word, though short of its ten characters
// it lexes as digits and minus signs.
func TestCursorAt_ADateBeingTypedIsOneWord(t *testing.T) {
	cases := []cursorCase{
		{query: "day=2026-01-0|", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-01-0", word: "2026-01-0"},
		{query: "day=2026-0|", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-0", word: "2026-0"},
		{query: "day=2026-01-|", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-01-", word: "2026-01-"},
		{query: "day=2026-|", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-", word: "2026-"},
		{query: "day=2026-01-01|", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-01-01", word: "2026-01-01"},
		{query: "day>=2026-1-5|", slot: SlotValue, field: "day", op: token.Gte, prefix: "2026-1-5", word: "2026-1-5"},
		{query: "day:2026-01-01..2026-02-|", slot: SlotValue, field: "day", op: token.Colon, prefix: "2026-02-", word: "2026-02-"},
		{query: "day IN (2026-01-01, 2026-0|", slot: SlotValue, field: "day", op: token.In, prefix: "2026-0", word: "2026-0"},
		{query: "a=1 AND day=2026-01-0|", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-01-0", word: "2026-01-0"},
		// The rest of the date after the cursor goes with it.
		{query: "day=2026-0|1-15 AND a=1", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-0", word: "2026-01-15"},
		{query: "day=2026-01-0|)", slot: SlotValue, field: "day", op: token.Eq, prefix: "2026-01-0", word: "2026-01-0"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			checkCursor(t, tc, Config{})
			checkCursor(t, tc, Config{}.Strict())
		})
	}
	// Other arithmetic is still read operand by operand.
	query, offset := split(t, "n>10-3|")
	if c := CursorAt(query, offset, Config{}); c.Prefix != "3" || !c.Arithmetic {
		t.Errorf("n>10-3: prefix %q arithmetic %v, want the operand 3", c.Prefix, c.Arithmetic)
	}
	query, offset = split(t, "n>1+2026-0|")
	if c := CursorAt(query, offset, Config{}); c.Prefix != "0" {
		t.Errorf("n>1+2026-0: prefix %q, want the operand 0", c.Prefix)
	}
}

// The word under the cursor ends where its token does, so what follows it
// is left as it was.
func TestCursorAt_AValueWordStopsAtWhatFollowsIt(t *testing.T) {
	for marked, want := range map[string]string{
		"count>4|*2 AND ready=true":  "4",
		"count>4|0*2":                "40",
		"day>2026-01-01|-7d":         "2026-01-01",
		"day>2026-01-0|1-7d":         "2026-01-01",
		"total:100|..500":            "100",
		"total:10|0..500":            "100",
		"total:100..50|0 AND a=1":    "500",
		"n>10+5|*2":                  "5",
		"n>now()-7|d+1":              "7d",
		"name=pr-1|0-3 AND a=1":      "pr-10-3",
		"name=a|/b":                  "a/b",
		"name=ch|eck*":               "check*",
		"ip=10.0|.0.1/24":            "10.0.0.1/24",
		"name IN (a|b, cd)":          "ab",
		"labels.exa|mple.com/team=x": "example.com/team",
		"labels.a|.b.c AND d=1":      "a.b.c",
		"a=1 AND labels.te|am.x/y=z": "team.x/y",
	} {
		query, offset := split(t, marked)
		c := CursorAt(query, offset, Config{})
		if got := query[c.Word.Offset : c.Word.Offset+c.Word.Length]; got != want {
			t.Errorf("%s: word = %q, want %q", marked, got, want)
		}
	}
}

func TestCursorAt_AfterAFieldAndNot(t *testing.T) {
	for _, tc := range []struct {
		query  string
		prefix string
	}{
		{"name NOT |", ""},
		{"name NOT I|", "I"},
		{"a=1 AND labels.team not |", ""},
		{"(name NOT in|", "in"},
	} {
		query, offset := split(t, tc.query)
		for _, cfg := range []Config{{}, Config{}.Strict()} {
			c := CursorAt(query, offset, cfg)
			if c.Slot != SlotOperator || !c.Negated || c.Prefix != tc.prefix || len(c.Field) == 0 {
				t.Errorf("%s: slot %s negated %v prefix %q field %q", tc.query, c.Slot, c.Negated, c.Prefix, c.Field)
			}
		}
	}
	// Once IN is written the list follows as usual, and a plain operator
	// slot is not negated.
	query, offset := split(t, "name NOT IN (|")
	if c := CursorAt(query, offset, Config{}); c.Slot != SlotValue || c.Negated {
		t.Errorf("name NOT IN (: slot %s negated %v", c.Slot, c.Negated)
	}
	query, offset = split(t, "name |")
	if c := CursorAt(query, offset, Config{}); c.Negated {
		t.Error("name: a plain operator slot is marked negated")
	}
}

func TestCursorAt_NothingToSay(t *testing.T) {
	for _, marked := range []string{
		// The text before the cursor does not read as a query.
		"a==|", "a=1 )|", "a=1 ) |", "AND |", "a=1 AND OR |", "#|", "a=1 AND #x |",
		"a=1 @|", "@|", ".|", "a..|", "(a=1)@|",
		// Only punctuation belongs here.
		"total:1 |", "total:1 AND |", "name IN |", "name IN (a |", "name IN (a b|", "f(a |", "f(a, b |", "a>(1 |",
		// A field reference where a value belongs is not completed.
		"a=[b|", `a=["b|`,
		// A quoted string where a condition starts is free text.
		`"fo|`, `a=1 AND "fo|`,
		// A selector on something that is not a field.
		"forks@bogus(|",
	} {
		t.Run(marked, func(t *testing.T) {
			query, offset := split(t, marked)
			c := CursorAt(query, offset, Config{})
			if c.Slot != SlotNone {
				t.Fatalf("slot = %s, want none (field %q, prefix %q)", c.Slot, c.Field, c.Prefix)
			}
			if c.Word != (token.Position{Offset: offset}) {
				t.Errorf("word = %+v, want an empty span at the cursor", c.Word)
			}
		})
	}
}

func TestCursorAt_RefusesABadPosition(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		offset int
		cfg    Config
	}{
		{"negative offset", "name=x", -1, Config{}},
		{"offset past the end", "name=x", 7, Config{}},
		{"offset inside a character", "café", 4, Config{}},
		{"invalid UTF-8 before the cursor", "na\xffme", 5, Config{}},
		{"invalid UTF-8 after the cursor", "name\xff", 2, Config{}},
		{"longer than the limit", "name=abcdef", 4, Config{MaxLength: 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := CursorAt(tc.input, tc.offset, tc.cfg)
			if c.Slot != SlotNone {
				t.Fatalf("slot = %s, want none", c.Slot)
			}
			if c.Word.Offset < 0 || c.Word.Offset > len(tc.input) || c.Word.Length != 0 {
				t.Errorf("word = %+v, want an empty span inside the input", c.Word)
			}
		})
	}
	t.Run("the limit counts characters, not bytes", func(t *testing.T) {
		if c := CursorAt("café=x", 5, Config{MaxLength: 6}); c.Slot != SlotOperator && c.Slot != SlotField {
			t.Fatalf("slot = %s for a query at the limit", c.Slot)
		}
	})
}

func TestCursorAt_StrictValuesDecidesWhatReads(t *testing.T) {
	// a=1+b=2 is one text value by default and an error under StrictValues.
	query, offset := split(t, "a=1+b=2|")
	if c := CursorAt(query, offset, Config{}); c.Slot != SlotValue {
		t.Errorf("default grammar: slot = %s, want value", c.Slot)
	}
	if c := CursorAt(query, offset, Config{StrictValues: true}); c.Slot != SlotNone {
		t.Errorf("strict values: slot = %s, want none", c.Slot)
	}
}

func TestSlot_String(t *testing.T) {
	want := map[Slot]string{
		SlotNone: "none", SlotField: "field", SlotKey: "key", SlotOperator: "operator",
		SlotSelector: "selector", SlotValue: "value", SlotConnective: "connective",
		SlotArgument: "argument", Slot(99): "none", Slot(-1): "none",
	}
	for slot, name := range want {
		if got := slot.String(); got != name {
			t.Errorf("Slot(%d).String() = %q, want %q", int(slot), got, name)
		}
	}
}

// checkCursorInvariants holds a Cursor to what every caller relies on,
// whatever the input.
func checkCursorInvariants(t *testing.T, input string, offset int, c Cursor) {
	t.Helper()
	w := c.Word
	if w.Offset < 0 || w.Length < 0 || w.Offset+w.Length > len(input) {
		t.Fatalf("CursorAt(%q, %d): word %+v is outside the input", input, offset, w)
	}
	if offset >= 0 && offset <= len(input) && (offset < w.Offset || offset > w.Offset+w.Length) {
		t.Fatalf("CursorAt(%q, %d): word %+v does not hold the cursor", input, offset, w)
	}
	if !utf8.ValidString(input[w.Offset:w.Offset+w.Length]) && utf8.ValidString(input) {
		t.Fatalf("CursorAt(%q, %d): word %+v splits a character", input, offset, w)
	}
	if c.Slot == SlotNone {
		if w.Length != 0 || c.Prefix != "" || c.Field != nil || c.Lists != nil || c.Quoted || c.Bracketed || c.Negated || c.Arithmetic || c.Function != "" {
			t.Fatalf("CursorAt(%q, %d): a position with no slot carries %+v", input, offset, c)
		}
		return
	}
	if !utf8.ValidString(c.Prefix) {
		t.Fatalf("CursorAt(%q, %d): prefix %q is not valid UTF-8", input, offset, c.Prefix)
	}
	if c.Slot == SlotKey {
		f := c.FieldSpan
		if f.Offset < 0 || f.Offset > w.Offset || f.Offset+f.Length != w.Offset+w.Length {
			t.Fatalf("CursorAt(%q, %d): field span %+v does not end with the word %+v", input, offset, f, w)
		}
		if len(c.Field) == 0 {
			t.Fatalf("CursorAt(%q, %d): a key with no field before it", input, offset)
		}
	}
	if c.Quoted && c.Slot != SlotValue {
		t.Fatalf("CursorAt(%q, %d): quoted in slot %s", input, offset, c.Slot)
	}
	if c.Bracketed && c.Slot != SlotField && c.Slot != SlotKey {
		t.Fatalf("CursorAt(%q, %d): bracketed in slot %s", input, offset, c.Slot)
	}
}

// TestCursorAt_EveryPositionOfTheCorpus runs every byte offset of every
// query in the shared corpus, valid or not, through the invariants.
func TestCursorAt_EveryPositionOfTheCorpus(t *testing.T) {
	for _, q := range corpusQueries(t) {
		for offset := -1; offset <= len(q)+1; offset++ {
			for _, cfg := range []Config{{}, Config{}.Strict()} {
				checkCursorInvariants(t, q, offset, CursorAt(q, offset, cfg))
			}
		}
	}
}

// TestCursorAt_AgreesWithTheParser holds the machine to the parser: in a
// query the parser accepts, the end of every token is a position where
// something is expected, so it must have a slot. A query that calls a
// function is left out, since inside an argument only punctuation follows.
func TestCursorAt_AgreesWithTheParser(t *testing.T) {
	checked := 0
	for _, q := range corpusQueries(t) {
		expr, err := ParseWith(q, Config{})
		if err != nil || len(ast.Calls(expr)) > 0 {
			continue
		}
		tokens, err := Lex(q, 0)
		if err != nil {
			t.Fatalf("Lex(%q): %v", q, err)
		}
		for _, tok := range tokens {
			at := tok.Pos.Offset + tok.Pos.Length
			if l := (&lexer{input: q[:at]}); func() bool { l.run(); return len(l.errors) > 0 }() {
				continue // the text up to here does not lex by itself
			}
			checked++
			if c := CursorAt(q, at, Config{}); c.Slot == SlotNone {
				t.Errorf("CursorAt(%q, %d) has no slot after %q", q, at, q[:at])
			}
		}
		// A complete condition can always be followed by a connective, or
		// is still a word being typed.
		switch c := CursorAt(q, len(q), Config{}); c.Slot {
		case SlotConnective, SlotOperator, SlotValue, SlotField, SlotKey, SlotSelector:
		default:
			t.Errorf("CursorAt(%q, end) = %s", q, c.Slot)
		}
	}
	if checked < 200 {
		t.Fatalf("only %d positions checked; the corpus did not load as expected", checked)
	}
}

func corpusQueries(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("../conformance/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Query string `json:"query"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range corpus.Cases {
		if c.Query != "" {
			out = append(out, c.Query)
		}
	}
	if len(out) < 100 {
		t.Fatalf("corpus has %d queries, expected over 100", len(out))
	}
	return out
}

// FuzzCursorAt checks that no input and no offset makes CursorAt panic or
// break an invariant, and that it answers the same thing twice.
func FuzzCursorAt(f *testing.F) {
	for _, s := range []string{
		"", "na", "name=", `name="fo`, "labels.te", "forks@(kind=", "a IN (b, ", "f(g(a), ",
		"[lab", `["labels.my k`, "a=1 AND NOT (b>2 OR ", "a:1..", "a>10 - ", "café=", "a=1)",
		"a@first@(", "NOT NOT", "a=\\", "IN(", "true.false@all(in=", "a=\"x\\", "[\"a\\",
		"x=a b", "　", "a=*b*", "a=1+b=2", "f((", "a>(1+", "A>0 %A(A)A<=0",
	} {
		f.Add(s, len(s))
		f.Add(s, len(s)/2)
	}
	f.Fuzz(func(t *testing.T, s string, offset int) {
		for _, cfg := range []Config{{}, Config{}.Strict(), {MaxLength: 16}} {
			c := CursorAt(s, offset, cfg)
			checkCursorInvariants(t, s, offset, c)
			again := CursorAt(s, offset, cfg)
			if c.Slot != again.Slot || c.Word != again.Word || c.Prefix != again.Prefix {
				t.Fatalf("CursorAt(%q, %d) answered %+v, then %+v", s, offset, c, again)
			}
		}
	})
}
