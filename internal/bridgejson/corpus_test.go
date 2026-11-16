package bridgejson

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// corpusCase is one entry of conformance/corpus.json, the file every client
// runs. Here it runs through the envelope functions the WASM and C bridges
// are thin wrappers over, so the contract foreign clients see is checked by
// plain `go test` on every platform.
type corpusCase struct {
	ID           string          `json:"id"`
	Op           string          `json:"op"`
	Query        string          `json:"query"`
	FieldSet     string          `json:"fieldSet"`
	Fields       json.RawMessage `json:"fields"`
	Record       json.RawMessage `json:"record"`
	ExpectError  bool            `json:"expectError"`
	ExpectCode   string          `json:"expectCode"`
	ExpectAst    string          `json:"expectAst"`
	ExpectString string          `json:"expectString"`
	ExpectMatch  *bool           `json:"expectMatch"`
	ExpectValue  *float64        `json:"expectValue"`
	Tolerance    *float64        `json:"tolerance"`
}

// wire sends an envelope through JSON, as every bridge does, so the
// assertions see what a foreign client would.
func wire(t *testing.T, env Envelope) map[string]any {
	t.Helper()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("envelope does not marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("envelope does not unmarshal: %v", err)
	}
	return out
}

func TestCorpus_ThroughBridgeEnvelopes(t *testing.T) {
	data, err := os.ReadFile("../../conformance/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		FieldSets map[string]json.RawMessage `json:"fieldSets"`
		Cases     []corpusCase               `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) < 135 {
		t.Fatalf("corpus has %d cases, expected at least 135 (stale?)", len(corpus.Cases))
	}

	for _, tc := range corpus.Cases {
		t.Run(tc.Op+"/"+tc.ID, func(t *testing.T) {
			fields := string(tc.Fields)
			if tc.FieldSet != "" {
				fields = string(corpus.FieldSets[tc.FieldSet])
			}
			record := "{}"
			if len(tc.Record) > 0 {
				record = string(tc.Record)
			}

			var got map[string]any
			switch tc.Op {
			case "parse":
				got = wire(t, Guard(func() Envelope { return Parse(tc.Query, DefaultMaxLength) }))
			case "parseAndValidate":
				got = wire(t, Guard(func() Envelope { return ParseAndValidate(tc.Query, fields, DefaultMaxLength) }))
			case "stringify":
				parsed := wire(t, Parse(tc.Query, DefaultMaxLength))
				tree, err := json.Marshal(parsed["result"])
				if err != nil {
					t.Fatal(err)
				}
				got = wire(t, Guard(func() Envelope { return Stringify(string(tree), DefaultMaxLength) }))
			case "match":
				got = wire(t, Guard(func() Envelope { return Match(tc.Query, fields, record, DefaultMaxLength) }))
			case "eval":
				got = wire(t, Guard(func() Envelope { return Eval(tc.Query, fields, record, DefaultMaxLength) }))
			default:
				t.Fatalf("unknown op %q", tc.Op)
			}

			msg, _ := got["error"].(string)
			if strings.HasPrefix(msg, "internal error") {
				t.Fatalf("bridge panicked: %s", msg)
			}
			if tc.ExpectError {
				if msg == "" {
					t.Fatalf("expected an error, got %v", got)
				}
				if tc.ExpectCode != "" && firstCode(got) != tc.ExpectCode {
					t.Fatalf("code = %q, want %q", firstCode(got), tc.ExpectCode)
				}
				return
			}
			if msg != "" {
				t.Fatalf("unexpected error: %s", msg)
			}
			if tc.ExpectAst != "" {
				node, _ := got["result"].(map[string]any)
				if node["type"] != tc.ExpectAst {
					t.Fatalf("ast type = %v, want %q", node["type"], tc.ExpectAst)
				}
			}
			if tc.ExpectString != "" && got["result"] != tc.ExpectString {
				t.Fatalf("string = %v, want %q", got["result"], tc.ExpectString)
			}
			if tc.ExpectMatch != nil && got["result"] != *tc.ExpectMatch {
				t.Fatalf("match = %v, want %v", got["result"], *tc.ExpectMatch)
			}
			if tc.ExpectValue != nil {
				tol := 1e-9
				if tc.Tolerance != nil {
					tol = *tc.Tolerance
				}
				v, ok := got["result"].(float64)
				if !ok || math.Abs(v-*tc.ExpectValue) > tol {
					t.Fatalf("value = %v, want %v (±%v)", got["result"], *tc.ExpectValue, tol)
				}
			}
		})
	}
}

func firstCode(env map[string]any) string {
	for _, key := range []string{"errors", "validationErrors"} {
		list, _ := env[key].([]any)
		if len(list) == 0 {
			continue
		}
		entry, _ := list[0].(map[string]any)
		code, _ := entry["code"].(string)
		return code
	}
	return ""
}

func TestBridge_MalformedInputsNeverPanic(t *testing.T) {
	junk := []string{"", "null", "[]", "{", `"x"`, "1", `{"type":"binary"}`, `{"type":"qualifier","op":"="}`,
		`{"type":"group"}`, `{"type":"unary"}`, `{"type":"selector"}`, `{"type":"nope"}`,
		`{"type":"qualifier","field":["a"],"op":"..","value":{"type":"number","raw":"x","value":"x"}}`}

	for _, in := range junk {
		t.Run(in, func(t *testing.T) {
			calls := map[string]func() Envelope{
				"validate":         func() Envelope { return Validate(in, in, DefaultMaxLength) },
				"stringify":        func() Envelope { return Stringify(in, DefaultMaxLength) },
				"parseAndValidate": func() Envelope { return ParseAndValidate("a=1", in, DefaultMaxLength) },
				"match":            func() Envelope { return Match("a=1", in, in, DefaultMaxLength) },
				"matchRecord":      func() Envelope { return Match("a=1", `[{"name":"a","type":"decimal"}]`, in, DefaultMaxLength) },
				"eval":             func() Envelope { return Eval("[a]+1", `[{"name":"a","type":"decimal"}]`, in, DefaultMaxLength) },
			}
			for name, call := range calls {
				got := wire(t, Guard(call))
				if msg, _ := got["error"].(string); strings.HasPrefix(msg, "internal error") {
					t.Errorf("%s panicked on %q: %s", name, in, msg)
				}
			}
		})
	}
}

func TestBridge_FailuresCarryCodes(t *testing.T) {
	fields := `[{"name":"total","type":"decimal"}]`

	tests := []struct {
		name string
		env  Envelope
		key  string
	}{
		{"parse", Parse("total>", DefaultMaxLength), "errors"},
		{"parseAndValidate syntax", ParseAndValidate("total>", fields, DefaultMaxLength), "errors"},
		{"parseAndValidate unknown field", ParseAndValidate("nope=1", fields, DefaultMaxLength), "validationErrors"},
		{"match syntax", Match("total>", fields, "{}", DefaultMaxLength), "errors"},
		{"match unknown field", Match("nope=1", fields, "{}", DefaultMaxLength), "validationErrors"},
		{"eval unknown field", Eval("[nope]+1", fields, "{}", DefaultMaxLength), "validationErrors"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wire(t, tt.env)
			if got["error"] == nil {
				t.Fatalf("expected an error, got %v", got)
			}
			if firstCode(map[string]any{tt.key: got[tt.key]}) == "" {
				t.Fatalf("no coded entry under %q in %v", tt.key, got)
			}
		})
	}
}

func TestParseFields_Shapes(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"numbered type", `[{"Name":"total","Type":2,"AllowedOps":["=",">"]}]`, ""},
		{"named type", `[{"name":"total","type":"decimal","allowedOps":["=",">"]}]`, ""},
		{"named type any case", `[{"name":"at","type":"DateTime"}]`, ""},
		{"empty list", `[]`, ""},
		{"unknown type name", `[{"name":"a","type":"number"}]`, "unknown field type"},
		{"type out of range", `[{"name":"a","type":42}]`, "unknown field type"},
		{"negative type", `[{"name":"a","type":-1}]`, "unknown field type"},
		{"type of the wrong kind", `[{"name":"a","type":true}]`, "field type must be"},
		{"no name", `[{"type":"text"}]`, "has no name"},
		{"repeated name", `[{"name":"a"},{"name":"a"}]`, "declared twice"},
		{"unknown operator", `[{"name":"a","allowedOps":["=="]}]`, "unknown operator"},
		{"not a list", `{"name":"a"}`, "invalid fields config"},
		{"not JSON", `[`, "invalid fields config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFields(tt.in)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseFields_DefaultOperators(t *testing.T) {
	fields := `[{"name":"total","type":"decimal"},{"name":"name","type":"text"},{"name":"none","type":"text","allowedOps":[]}]`

	tests := []struct {
		query string
		want  bool
	}{
		{"total>5", true},
		{"total:1..20", true},
		{"name=a*", true},
		{"name>a", false},
		{"total=1*", false},
		{"none=a", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := wire(t, ParseAndValidate(tt.query, fields, DefaultMaxLength))

			if ok := got["error"] == nil; ok != tt.want {
				t.Fatalf("accepted = %v, want %v (%v)", ok, tt.want, got["error"])
			}
		})
	}
}

func TestBridge_FailuresCarryHints(t *testing.T) {
	fields := `[{"name":"total","type":"decimal"},{"name":"state","type":"text","allowedOps":["="]}]`

	tests := []struct {
		name string
		env  Envelope
		key  string
		hint string
	}{
		{"syntax", Parse("state==draft", DefaultMaxLength), "errors", "single ="},
		{"arithmetic on the left", Match("[total]*2>3", fields, "{}", DefaultMaxLength), "errors", "float([total]*2)>3"},
		{"unknown field", ParseAndValidate("totl>5", fields, DefaultMaxLength), "validationErrors", `did you mean "total"?`},
		{"refused operator", Match("state=a*", fields, "{}", DefaultMaxLength), "validationErrors", "it allows: ="},
		{"value expression", Eval("total*2", fields, "{}", DefaultMaxLength), "errors", "[total]*2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := wire(t, tt.env)

			// Assert
			list, _ := got[tt.key].([]any)
			if len(list) == 0 {
				t.Fatalf("no entries under %q in %v", tt.key, got)
			}
			entry, _ := list[0].(map[string]any)
			hint, _ := entry["hint"].(string)
			if !strings.Contains(hint, tt.hint) {
				t.Errorf("hint = %q, want one containing %q", hint, tt.hint)
			}
			if message, _ := got["error"].(string); !strings.Contains(message, tt.hint) {
				t.Errorf("error = %q, want the hint in the joined message too", message)
			}
		})
	}
}

func TestBridge_NoHintKeyWhenThereIsNothingToSuggest(t *testing.T) {
	// Arrange: ten fields, none close to the name asked for.
	fields := `[{"name":"alpha","type":"text"},{"name":"bravo","type":"text"},{"name":"charlie","type":"text"},
		{"name":"delta","type":"text"},{"name":"echo","type":"text"},{"name":"foxtrot","type":"text"},
		{"name":"golf","type":"text"},{"name":"hotel","type":"text"},{"name":"india","type":"text"}]`

	// Act
	got := wire(t, ParseAndValidate("zzzzzz=1", fields, DefaultMaxLength))

	// Assert
	list, _ := got["validationErrors"].([]any)
	if len(list) != 1 {
		t.Fatalf("validationErrors = %v, want one entry", got)
	}
	if _, present := list[0].(map[string]any)["hint"]; present {
		t.Errorf("entry has a hint key with nothing to suggest: %v", list[0])
	}
}

// TestParseWith_StrictOptions pins the two switches a bridge call may set: off unless
// asked for, and reported with the same coded envelope as any parse failure.
func TestParseWith_StrictOptions(t *testing.T) {
	const fields = `[{"name":"a","type":"integer"},{"name":"b","type":"integer"}]`
	strict := Options{MaxLength: DefaultMaxLength, ExplicitAnd: true, StrictValues: true}

	code := func(t *testing.T, env Envelope) string {
		t.Helper()
		errs, _ := wire(t, env)["errors"].([]any)
		if len(errs) == 0 {
			return ""
		}
		first, _ := errs[0].(map[string]any)
		c, _ := first["code"].(string)
		return c
	}

	tests := []struct {
		name string
		env  Envelope
		want string
	}{
		{"parse, default grammar", Parse("a=1 b=2", DefaultMaxLength), ""},
		{"parse, explicit AND", ParseWith("a=1 b=2", Options{MaxLength: DefaultMaxLength, ExplicitAnd: true}), "missingAnd"},
		{"parse, operator in a value by default", Parse("a=1+b=2", DefaultMaxLength), ""},
		{"parse, strict values", ParseWith("a=1+b=2", Options{MaxLength: DefaultMaxLength, StrictValues: true}), "unexpectedChar"},
		{"parse, strict accepts a plain query", ParseWith("a=1 AND b=2", strict), ""},
		{"parseAndValidate", ParseAndValidateWith("a=1 b=2", fields, strict), "missingAnd"},
		{"match", MatchWith("a=1 b=2", fields, `{"a":1,"b":2}`, strict), "missingAnd"},
		{"match accepts a plain query", MatchWith("a=1 AND b=2", fields, `{"a":1,"b":2}`, strict), ""},
		{"length limit still applies", ParseWith("a=1 AND b=2", Options{MaxLength: 3, ExplicitAnd: true}), "queryTooLong"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := code(t, tc.env); got != tc.want {
				t.Errorf("first code = %q, want %q (envelope %v)", got, tc.want, tc.env)
			}
		})
	}
}
