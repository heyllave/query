package bridgejson

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

const completeFields = `[
	{"name": "name", "type": "text"},
	{"name": "status", "type": "text", "values": ["ready", "not ready"]},
	{"name": "replicas", "type": "integer"},
	{"name": "labels", "type": "text", "nested": true},
	{"name": "forks", "type": "list", "elements": [{"name": "kind", "type": "text", "values": ["Rollout"]}]}
]`

func completeResult(t *testing.T, env Envelope) map[string]any {
	t.Helper()
	got := wire(t, env)
	if msg, _ := got["error"].(string); msg != "" {
		t.Fatalf("unexpected error: %s", msg)
	}
	c, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("result = %v, want an object", got["result"])
	}
	return c
}

func TestComplete_Shape(t *testing.T) {
	c := completeResult(t, Complete(`forks@(kind=`, completeFields, 12, false, Options{}))
	want := map[string]any{
		"slot":     "value",
		"replace":  map[string]any{"offset": 12.0, "length": 0.0},
		"prefix":   "",
		"items":    []any{map[string]any{"label": "Rollout", "text": "Rollout", "kind": "value"}},
		"field":    "kind",
		"type":     "text",
		"operator": "=",
		"lists":    []any{"forks"},
	}
	gotJSON, _ := json.Marshal(c)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("completion =\n  %s\nwant\n  %s", gotJSON, wantJSON)
	}

	t.Run("items is an array even when empty", func(t *testing.T) {
		c := completeResult(t, Complete("name=", completeFields, 5, false, Options{}))
		if items, ok := c["items"].([]any); !ok || len(items) != 0 {
			t.Fatalf("items = %#v, want an empty array", c["items"])
		}
	})
	t.Run("a field that is not declared carries no type", func(t *testing.T) {
		c := completeResult(t, Complete("bogus=", completeFields, 6, false, Options{}))
		if _, has := c["type"]; has || c["field"] != "bogus" {
			t.Fatalf("completion = %v", c)
		}
	})
	t.Run("an operator item carries its detail", func(t *testing.T) {
		c := completeResult(t, Complete("replicas ", completeFields, 9, false, Options{}))
		first, _ := c["items"].([]any)[0].(map[string]any)
		if first["text"] != "=" || first["kind"] != "operator" || first["detail"] != "is" {
			t.Fatalf("first item = %v", first)
		}
	})
	t.Run("an open quote is reported", func(t *testing.T) {
		c := completeResult(t, Complete(`status="no`, completeFields, 10, false, Options{}))
		if c["quoted"] != true {
			t.Fatalf("completion = %v", c)
		}
	})
	t.Run("a function argument names its function", func(t *testing.T) {
		c := completeResult(t, Complete("lower(na", completeFields, 8, false, Options{}))
		if c["slot"] != "argument" || c["function"] != "lower" {
			t.Fatalf("completion = %v", c)
		}
	})
}

func TestComplete_OptionsDecideTheGrammar(t *testing.T) {
	query := "name=x re"
	lenient := completeResult(t, Complete(query, completeFields, len(query), false, Options{}))
	if got := wireTexts(lenient["items"]); len(got) != 1 || got[0] != "replicas" {
		t.Errorf("default grammar: items = %q, want replicas", got)
	}
	strict := completeResult(t, Complete(query, completeFields, len(query), false, Options{}.Strict()))
	if got := wireTexts(strict["items"]); len(got) != 0 {
		t.Errorf("strict grammar: items = %q, want none", got)
	}
	long := completeResult(t, Complete(query, completeFields, len(query), false, Options{MaxLength: 4}))
	if long["slot"] != "none" {
		t.Errorf("over the length limit: slot = %v, want none", long["slot"])
	}
}

func TestComplete_RefusesWhatIsTheCallersMistake(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		fields string
		cursor int
		utf16  bool
		want   string
	}{
		{"negative cursor", "name=x", completeFields, -1, false, "cursor -1 is not a position in the query: it is counted in bytes, from 0 to 6"},
		{"cursor past the end", "name=x", completeFields, 7, false, "cursor 7 is not a position in the query"},
		{"cursor inside a character", "café=x", completeFields, 4, false, "cannot split a character"},
		{"negative cursor in UTF-16", "name=x", completeFields, -1, true, "counted in UTF-16 code units, from 0 to 6"},
		{"cursor past the end in UTF-16", "🚀=x", completeFields, 5, true, "counted in UTF-16 code units, from 0 to 4"},
		{"cursor inside a surrogate pair", "🚀=x", completeFields, 1, true, "cannot split a character"},
		{"query that is not UTF-8", "na\xffme", completeFields, 2, false, "query is not valid UTF-8"},
		{"fields that are not JSON", "name=", "{", 5, false, "invalid fields config"},
		{"a field with no name", "name=", `[{"type": "text"}]`, 5, false, "has no name"},
		{"an unknown field type", "name=", `[{"name": "a", "type": "blob"}]`, 5, false, "unknown field type"},
		{"an unknown selector", "name=", `[{"name": "a", "type": "list", "selectors": ["some"]}]`, 5, false, "unknown selector"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wire(t, Guard(func() Envelope { return Complete(tc.query, tc.fields, tc.cursor, tc.utf16, Options{}) }))
			msg, _ := got["error"].(string)
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("error = %q, want it to contain %q", msg, tc.want)
			}
			if _, has := got["result"]; has {
				t.Fatalf("a refused call answered with a result: %v", got)
			}
		})
	}
}

// TestComplete_UTF16AgreesWithBytes checks every position of queries with
// characters of one, two, three and four bytes: the answer in UTF-16 code
// units is the answer in bytes, converted, and the span it gives cuts the
// JavaScript string at the same characters.
func TestComplete_UTF16AgreesWithBytes(t *testing.T) {
	for _, query := range []string{
		`name="café 🚀" AND st`, "名前=x AND rep", `status="né🚀`, "🚀🚀 ", "labels.té", `name=é AND forks@(kind=R`,
	} {
		units := utf16.Encode([]rune(query))
		for at := 0; at <= len(query); at++ {
			if !utf8.ValidString(query[:at]) {
				continue
			}
			inBytes := completeResult(t, Complete(query, completeFields, at, false, Options{}))
			cursor16 := len(utf16.Encode([]rune(query[:at])))
			in16 := completeResult(t, Complete(query, completeFields, cursor16, true, Options{}))

			b, _ := inBytes["replace"].(map[string]any)
			u, _ := in16["replace"].(map[string]any)
			bo, bl := int(b["offset"].(float64)), int(b["length"].(float64))
			uo, ul := int(u["offset"].(float64)), int(u["length"].(float64))
			if uo < 0 || uo+ul > len(units) {
				t.Fatalf("%q at %d: UTF-16 span %d+%d is outside the string", query, at, uo, ul)
			}
			if got, want := string(utf16.Decode(units[uo:uo+ul])), query[bo:bo+bl]; got != want {
				t.Fatalf("%q at %d: UTF-16 span covers %q, byte span covers %q", query, at, got, want)
			}
			delete(inBytes, "replace")
			delete(in16, "replace")
			a, _ := json.Marshal(inBytes)
			c, _ := json.Marshal(in16)
			if string(a) != string(c) {
				t.Fatalf("%q at %d: bytes answered %s, UTF-16 answered %s", query, at, a, c)
			}
		}
	}
}

func TestOffer(t *testing.T) {
	cases := []struct {
		name       string
		completion string
		candidates string
		want       []string // texts
		wantErr    string
	}{
		{name: "text values", completion: `{"slot":"value","field":"name","type":"text"}`, candidates: `["web","api server","812"]`, want: []string{"web", `"api server"`, `"812"`}},
		{name: "numeric values", completion: `{"slot":"value","field":"replicas","type":"integer"}`, candidates: `["3","many"]`, want: []string{"3"}},
		{name: "values of a field that is not declared", completion: `{"slot":"value","field":"bogus"}`, candidates: `["3"]`, want: []string{`"3"`}},
		{name: "values inside an open quote", completion: `{"slot":"value","field":"name","type":"text","quoted":true}`, candidates: `["web"]`, want: []string{`"web"`}},
		{name: "keys", completion: `{"slot":"key","field":"labels"}`, candidates: `["team","my key",""]`, want: []string{"labels.team", `["labels.my key"]`}},
		{name: "fields", completion: `{"slot":"field"}`, candidates: `["extra","two words"]`, want: []string{"extra", `["two words"]`}},
		{name: "no candidates", completion: `{"slot":"value"}`, candidates: `[]`, want: []string{}},
		{name: "a slot the schema fills", completion: `{"slot":"operator","field":"name"}`, candidates: `["x"]`, want: []string{}},
		{name: "the whole object Complete returned", completion: `{"slot":"value","replace":{"offset":5,"length":0},"prefix":"","items":[],"field":"name","type":"text","operator":"="}`, candidates: `["a b"]`, want: []string{`"a b"`}},

		{name: "completion that is not JSON", completion: `{`, candidates: `[]`, wantErr: "invalid completion"},
		{name: "completion that is null", completion: `null`, candidates: `[]`, wantErr: `unknown slot ""`},
		{name: "unknown slot", completion: `{"slot":"bogus"}`, candidates: `[]`, wantErr: `unknown slot "bogus"`},
		{name: "unknown type", completion: `{"slot":"value","type":"blob"}`, candidates: `[]`, wantErr: `unknown field type "blob"`},
		{name: "key with no field", completion: `{"slot":"key"}`, candidates: `["a"]`, wantErr: "a key slot names no field"},
		{name: "candidates that are not an array", completion: `{"slot":"value"}`, candidates: `"x"`, wantErr: "candidates must be an array of strings"},
		{name: "candidates that are null", completion: `{"slot":"value"}`, candidates: `null`, wantErr: "candidates must be an array of strings"},
		{name: "candidates that are not strings", completion: `{"slot":"value"}`, candidates: `[1, 2]`, wantErr: "candidates must be an array of strings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wire(t, Guard(func() Envelope { return Offer(tc.completion, tc.candidates) }))
			msg, _ := got["error"].(string)
			if tc.wantErr != "" {
				if !strings.Contains(msg, tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", msg, tc.wantErr)
				}
				return
			}
			if msg != "" {
				t.Fatalf("unexpected error: %s", msg)
			}
			list, ok := got["result"].([]any)
			if !ok {
				t.Fatalf("result = %#v, want an array", got["result"])
			}
			if texts := wireTexts(list); strings.Join(texts, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("texts = %q, want %q", texts, tc.want)
			}
		})
	}
}

func TestUTF16Offsets(t *testing.T) {
	s := "aé名🚀z" // 1, 2, 3 and 4 bytes; the rocket is two code units
	if got := UTF16Len(s); got != 6 {
		t.Fatalf("UTF16Len = %d, want 6", got)
	}
	for units, want := range map[int]int{0: 0, 1: 1, 2: 3, 3: 6, 5: 10, 6: 11} {
		if got, ok := utf16ToByte(s, units); !ok || got != want {
			t.Errorf("utf16ToByte(%d) = %d, %v; want %d", units, got, ok, want)
		}
	}
	for _, units := range []int{-1, 4, 7, 100} {
		if got, ok := utf16ToByte(s, units); ok {
			t.Errorf("utf16ToByte(%d) = %d, want it refused", units, got)
		}
	}
	if got, ok := utf16ToByte("", 0); !ok || got != 0 {
		t.Errorf("utf16ToByte of the empty string = %d, %v", got, ok)
	}
}

// FuzzBridgeComplete checks that no query, cursor or unit makes the bridge
// panic, and that an answer always marshals and keeps its span inside the
// query.
func FuzzBridgeComplete(f *testing.F) {
	for _, s := range []string{"", "na", "status=", `status="no`, "labels.te", "forks@(kind=", "café 🚀 st", "a\xffb"} {
		f.Add(s, len(s), false)
		f.Add(s, len(s)/2, true)
	}
	f.Fuzz(func(t *testing.T, s string, cursor int, inUTF16 bool) {
		env := Guard(func() Envelope { return Complete(s, completeFields, cursor, inUTF16, Options{}) })
		data, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("envelope does not marshal: %v", err)
		}
		if msg, _ := env["error"].(string); strings.HasPrefix(msg, "internal error") {
			t.Fatalf("Complete(%q, %d) panicked: %s", s, cursor, msg)
		}
		c, ok := env["result"].(Completion)
		if !ok {
			return
		}
		limit := len(s)
		if inUTF16 {
			limit = UTF16Len(s)
		}
		if c.Replace.Offset < 0 || c.Replace.Length < 0 || c.Replace.Offset+c.Replace.Length > limit {
			t.Fatalf("Complete(%q, %d): span %+v is outside the query", s, cursor, c.Replace)
		}
		if cursor < c.Replace.Offset || cursor > c.Replace.Offset+c.Replace.Length {
			t.Fatalf("Complete(%q, %d): span %+v does not hold the cursor", s, cursor, c.Replace)
		}
		if env := Guard(func() Envelope { return Offer(string(data[len(`{"result":`):len(data)-1]), `["x","a b",""]`) }); env["error"] != nil {
			t.Fatalf("Offer refused what Complete returned: %v", env["error"])
		}
	})
}
