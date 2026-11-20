package bridgejson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// roundTrip sends a parsed query through the JSON contract and back, the path
// every bridge client's stringify takes.
func roundTrip(t *testing.T, q string) string {
	t.Helper()
	expr, err := parser.Parse(q, 256)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	data, err := json.Marshal(AstToJSON(expr))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back, err := JSONToAST(string(data), DefaultMaxLength)
	if err != nil {
		t.Fatalf("JSONToAST(%s): %v", data, err)
	}
	return ast.String(back)
}

func TestJSONToAST_RoundTripsWhatTheParserRead(t *testing.T) {
	cases := []string{
		`cliente.nombre="Ana Pérez"`,
		`state="and"`,
		`state="true"`,
		`state="42"`,
		`state=foo*`,
		`total:100..500`,
		`fecha>now()-7d`,
		`total>=[base]*2`,
		`fecha>2024-01-02`,
		`d>5h`,
		`x=-3`,
		`x=1.5`,
		`flag=true`,
		`tags@any(x=1)`,
		`items@first`,
		`items@(x=1)`,
		`(a=1 OR b=2) AND NOT c=3`,
		// A function as a predicate, alone and beside a qualifier, and a
		// function on the left of a comparison.
		`contains(tags, "x")`,
		`contains(tags, "x") AND b>1`,
		`NOT startsWith(name, "a")`,
		`lower(name)=john`,
		`items@any(contains(tags, "x"))`,
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			if got := roundTrip(t, q); got != q {
				t.Errorf("round trip = %q, want %q", got, q)
			}
		})
	}
}

func TestJSONToAST_QuotesAStringThatWouldNotReadBackBare(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want string
	}{
		{"a space", `{"type":"string","raw":"Ana Pérez"}`, `f="Ana Pérez"`},
		{"a keyword, which a value position reads as a word", `{"type":"string","raw":"OR"}`, `f=OR`},
		{"a boolean word", `{"type":"string","raw":"true"}`, `f="true"`},
		{"a number", `{"type":"string","raw":"500"}`, `f="500"`},
		{"a literal star", `{"type":"string","raw":"a*b"}`, `f="a*b"`},
		{"an embedded quote", `{"type":"string","raw":"say \"hi\""}`, `f="say \"hi\""`},
		{"a plain word", `{"type":"string","raw":"abierta"}`, `f=abierta`},
		{"a pattern", `{"type":"string","raw":"hid*","wildcard":true}`, `f=hid*`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := `{"type":"qualifier","op":"=","field":["f"],"value":` + tc.val + `}`
			expr, err := JSONToAST(node, DefaultMaxLength)
			if err != nil {
				t.Fatalf("JSONToAST: %v", err)
			}
			got := ast.String(expr)
			if got != tc.want {
				t.Fatalf("String = %q, want %q", got, tc.want)
			}
			reparsed, err := parser.Parse(got, 256)
			if err != nil {
				t.Fatalf("output %q does not parse: %v", got, err)
			}
			q, ok := reparsed.(*ast.QualifierExpr)
			if !ok {
				t.Fatalf("output %q parses as %T, want one qualifier", got, reparsed)
			}
			var in Val
			if err := json.Unmarshal([]byte(tc.val), &in); err != nil {
				t.Fatal(err)
			}
			if q.Value.Str != in.Raw || q.Value.Wildcard != in.Wildcard {
				t.Errorf("reads back as %q (wildcard %v), want %q (wildcard %v)",
					q.Value.Str, q.Value.Wildcard, in.Raw, in.Wildcard)
			}
		})
	}
}

func TestJSONToAST_BuildsARangeFromItsEndValue(t *testing.T) {
	node := `{"type":"qualifier","op":"..","field":["total"],` +
		`"value":{"type":"integer","raw":"100"},"endValue":{"type":"float","raw":"500.5"}}`
	expr, err := JSONToAST(node, DefaultMaxLength)
	if err != nil {
		t.Fatalf("JSONToAST: %v", err)
	}
	if got := ast.String(expr); got != "total:100..500.5" {
		t.Errorf("String = %q, want total:100..500.5", got)
	}
}

func TestJSONToAST_RefusesAValueWhoseTextIsNotItsType(t *testing.T) {
	for _, val := range []string{
		`{"type":"integer","raw":"abc"}`,
		`{"type":"date","raw":"yesterday"}`,
		`{"type":"boolean","raw":"1"}`,
	} {
		node := `{"type":"qualifier","op":"=","field":["f"],"value":` + val + `}`
		if _, err := JSONToAST(node, DefaultMaxLength); err == nil {
			t.Errorf("JSONToAST(%s) succeeded, want an error", val)
		}
	}
}

func TestValidationErrors_CodesEachFailureWithItsField(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "state", Type: validate.TypeText, AllowedOps: []validate.Op{validate.OpEq}},
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: []validate.Op{validate.OpEq, validate.OpGt}},
	}
	expr, err := parser.Parse(`nope=1 AND state>2 AND total=abc`, 256)
	if err != nil {
		t.Fatal(err)
	}
	got := ValidationErrors(validate.New(fields).Validate(expr))
	want := []struct{ code, field, op string }{
		{"fieldNotFound", "nope", ""},
		{"operatorNotAllowed", "state", ">"},
		{"typeMismatch", "total", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d errors %v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i]["code"] != w.code || got[i]["field"] != w.field || got[i]["op"] != w.op {
			t.Errorf("error %d = %v, want code %s field %s op %q", i, got[i], w.code, w.field, w.op)
		}
	}
}

func TestValidationErrors_IsEmptyForAnotherError(t *testing.T) {
	_, err := parser.Parse(`=x`, 256)
	if list := ValidationErrors(err); len(list) != 0 {
		t.Errorf("ValidationErrors(parse error) = %v, want none", list)
	}
}

func TestJSONToAST_RefusesTextThePrinterWouldWriteUnread(t *testing.T) {
	const (
		orders = `{"type":"presence","field":["orders"]}`
		inner  = `{"type":"qualifier","op":"=","field":["x"],"value":{"type":"integer","raw":"1"}}`
		value  = `{"type":"integer","raw":"1"}`
	)
	cases := []struct {
		name string
		node string
	}{
		{"a selector that carries a second term",
			`{"type":"selector","selector":"first OR secret=a*","base":` + orders + `}`},
		{"a selector the language does not have",
			`{"type":"selector","selector":"every","base":` + orders + `,"inner":` + inner + `}`},
		{"@first with an inner expression",
			`{"type":"selector","selector":"first","base":` + orders + `,"inner":` + inner + `}`},
		{"@any without an inner expression",
			`{"type":"selector","selector":"any","base":` + orders + `}`},
		{"an anonymous selector without an inner expression",
			`{"type":"selector","selector":"","base":` + orders + `}`},
		{"an operator that carries a second term",
			`{"type":"qualifier","op":"=1 OR secret=","field":["x"],"value":` + value + `}`},
		{"a missing operator",
			`{"type":"qualifier","field":["x"],"value":` + value + `}`},
		{"a range operator without an end",
			`{"type":"qualifier","op":"..","field":["x"],"value":` + value + `}`},
		{"a logical operator the language does not have",
			`{"type":"binary","op":"XOR","left":` + inner + `,"right":` + inner + `}`},
		{"a list value, whose raw text is printed as written",
			`{"type":"qualifier","op":"=","field":["x"],"value":{"type":"list","raw":"1 OR secret=a*"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := JSONToAST(tc.node, DefaultMaxLength)
			if err == nil {
				t.Fatalf("JSONToAST succeeded and prints %q, want an error", ast.String(expr))
			}
		})
	}
}

func TestValidate_ATreeCannotPrintAFieldItDidNotValidate(t *testing.T) {
	// Arrange: only `orders` is declared; the selector name tries to add a
	// term on `secret` that validation never sees as a field.
	fields := `[{"name":"orders","type":"text"}]`
	tree := `{"type":"selector","selector":"first OR secret=a*",` +
		`"base":{"type":"presence","field":["orders"]}}`

	// Act
	validated := Validate(tree, fields, DefaultMaxLength)
	printed := Stringify(tree, DefaultMaxLength)

	// Assert
	if validated["valid"] == true {
		t.Errorf("Validate accepted the tree: %v", validated)
	}
	if printed["error"] == nil {
		t.Errorf("Stringify printed %q, want an error", printed["result"])
	}
}

func TestJSONToAST_HoldsATreeToTheQueryLengthLimit(t *testing.T) {
	long := strings.Repeat("a", DefaultMaxLength+1)
	cases := []struct {
		name string
		node string
	}{
		{"a string value", `{"type":"qualifier","op":"=","field":["f"],"value":{"type":"string","raw":"` + long + `"}}`},
		{"a string value's decoded content",
			`{"type":"qualifier","op":"=","field":["f"],"value":{"type":"string","raw":"a","value":"` + long + `"}}`},
		{"a field name", `{"type":"presence","field":["` + long + `"]}`},
		{"a field name in segments", `{"type":"presence","field":["` + strings.Repeat(`a","`, DefaultMaxLength/2) + `a"]}`},
		{"the whole tree", `{"type":"presence","field":["f"]}` + strings.Repeat(" ", maxASTJSONLength)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := JSONToAST(tc.node, DefaultMaxLength)

			if got := parser.Errors(err); len(got) != 1 || got[0].Code != parser.CodeQueryTooLong {
				t.Fatalf("JSONToAST error = %v, want the %s failure", err, parser.CodeQueryTooLong)
			}
			for name, env := range map[string]Envelope{"validate": Validate(tc.node, `[]`, DefaultMaxLength), "stringify": Stringify(tc.node, DefaultMaxLength)} {
				if code := firstCode(wire(t, env)); code != string(parser.CodeQueryTooLong) {
					t.Errorf("%s answered %v, want code %s", name, env, parser.CodeQueryTooLong)
				}
			}
		})
	}
}

func TestJSONToAST_AcceptsTheTreeOfAQueryAtTheLengthLimit(t *testing.T) {
	field := strings.Repeat("f", 120)
	args := "0" + strings.Repeat(",0", (DefaultMaxLength-4)/2)
	for name, q := range map[string]string{
		// An IN list lowers to one qualifier per element, the largest tree a
		// query of a given length produces.
		"an IN list": field + " IN (1" + strings.Repeat(",1", (DefaultMaxLength-len(field)-7)/2) + ")",
		// The printer writes a call longer than its source (a space after
		// each comma), so the tree carries more text than the query held.
		"a predicate call":    "f(" + args[:DefaultMaxLength-3] + ")",
		"a field function":    "f(" + args[:DefaultMaxLength-5] + ")=1",
		"an arithmetic value": "x=" + strings.Repeat("1+", (DefaultMaxLength-3)/2) + "1",
		"a field name":        strings.Repeat("a", DefaultMaxLength),
		"a string value":      "x=" + strings.Repeat("a", DefaultMaxLength-2),
	} {
		t.Run(name, func(t *testing.T) {
			if len(q) > DefaultMaxLength {
				t.Fatalf("test query is %d characters", len(q))
			}
			expr, err := parser.Parse(q, DefaultMaxLength)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			data, err := json.MarshalIndent(AstToJSON(expr), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			back, err := JSONToAST(string(data), DefaultMaxLength)
			if err != nil {
				t.Fatalf("JSONToAST refused a %d-byte tree of a %d-character query: %v", len(data), len(q), err)
			}
			if got, want := ast.String(back), ast.String(expr); got != want {
				t.Errorf("tree prints %q, query prints %q", got, want)
			}
		})
	}
}

// A tree is held to the limit its query was parsed under, not to the
// default: validate and stringify take the same maxLength as parse.
func TestJSONToAST_HoldsATreeToTheCallersLimit(t *testing.T) {
	q := "x=" + strings.Repeat("a", DefaultMaxLength+10)
	env := Parse(q, 2*DefaultMaxLength)
	tree, err := json.Marshal(env["result"])
	if err != nil || env["error"] != nil {
		t.Fatalf("Parse(%d chars, limit %d) = %v, %v", len(q), 2*DefaultMaxLength, env, err)
	}
	if code := firstCode(wire(t, Validate(string(tree), `[]`, DefaultMaxLength))); code != string(parser.CodeQueryTooLong) {
		t.Errorf("Validate under the default limit answered %q, want %s", code, parser.CodeQueryTooLong)
	}
	if code := firstCode(wire(t, Stringify(string(tree), DefaultMaxLength))); code != string(parser.CodeQueryTooLong) {
		t.Errorf("Stringify under the default limit answered %q, want %s", code, parser.CodeQueryTooLong)
	}
	for _, limit := range []int{2 * DefaultMaxLength, 0} {
		if env := Validate(string(tree), `[{"name":"x","type":"text"}]`, limit); env["valid"] != true {
			t.Errorf("Validate under limit %d answered %v", limit, env)
		}
		if env := Stringify(string(tree), limit); env["result"] != q {
			t.Errorf("Stringify under limit %d answered %v", limit, env)
		}
	}
}

// A selector binds to the one term before its '@'. A tree whose selector hangs
// off a bare OR (or off nothing) would print as a query that groups
// differently, so the bridge refuses it instead of printing it.
func TestJSONToAST_RefusesASelectorWhoseBaseIsNotASingleTerm(t *testing.T) {
	qual := func(field string) string {
		return `{"type":"qualifier","field":"` + field + `","operator":"=","value":{"type":"number","raw":"1","value":1}}`
	}
	for name, node := range map[string]string{
		"binary base": `{"type":"selector","selector":"first","base":{"type":"binary","op":"OR","left":` + qual("a") + `,"right":` + qual("b") + `}}`,
		"no base":     `{"type":"selector","selector":"first"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := JSONToAST(node, DefaultMaxLength); err == nil {
				t.Fatalf("JSONToAST accepted %s", node)
			}
		})
	}
}

func TestAstToJSON_CarriesAPredicateCallAndItsFields(t *testing.T) {
	expr, err := parser.Parse(`contains(lower(labels.team), "pay") AND b>1`, 0)
	if err != nil {
		t.Fatal(err)
	}
	tree := AstToJSON(expr)
	if tree.Left == nil || tree.Left.Type != "call" {
		t.Fatalf("left node = %+v, want a call", tree.Left)
	}
	if got, want := tree.Left.Call, `contains(lower(labels.team), "pay")`; got != want {
		t.Errorf("call = %q, want %q", got, want)
	}
	if got := tree.Left.Fields; len(got) != 1 || got[0][0] != "labels" || got[0][1] != "team" {
		t.Errorf("fields = %v, want [[labels team]]", got)
	}

	q, err := parser.Parse(`len(name)>3`, 0)
	if err != nil {
		t.Fatal(err)
	}
	node := AstToJSON(q)
	if node.FieldFunc != "len(name)" || len(node.Fields) != 1 || node.Fields[0][0] != "name" {
		t.Errorf("fieldFunc node = %+v, want len(name) reading name", node)
	}
}

func TestJSONToAST_RefusesACallThatIsNotOne(t *testing.T) {
	for _, data := range []string{
		`{"type":"call","call":"a=1"}`,
		`{"type":"call","call":""}`,
		`{"type":"call","call":"contains("}`,
	} {
		if _, err := JSONToAST(data, DefaultMaxLength); err == nil {
			t.Errorf("JSONToAST(%s) accepted a call that is not a function call", data)
		}
	}
}

func TestParse_LengthLimitCountsCharacters(t *testing.T) {
	// 10 characters, 20 bytes: a limit of 10 admits it, a limit of 9 does not.
	q := "a=" + "ééééééé" + "é"
	if env := Parse(q, 10); env["error"] != nil {
		t.Errorf("Parse under a 10-character limit failed: %v", env["error"])
	}
	env := Parse(q, 9)
	if env["error"] == nil {
		t.Fatal("Parse under a 9-character limit succeeded")
	}
	if msg, _ := env["error"].(string); msg != "position 0: query length 10 exceeds maximum of 9 characters" {
		t.Errorf("message = %q", msg)
	}
}

func TestEntryPoints_HonourMaxLength(t *testing.T) {
	q := "state=abcdefghijklmnopqrstuvwxyz"
	fields := `[{"Name":"state","Type":"text"}]`
	if env := ParseAndValidate(q, fields, 8); env["error"] == nil {
		t.Error("ParseAndValidate ignored maxLength")
	}
	if env := ParseAndValidate(q, fields, 0); env["error"] != nil {
		t.Errorf("ParseAndValidate with the limit disabled failed: %v", env["error"])
	}
	if env := Match(q, fields, `{"state":"x"}`, 8); env["error"] == nil {
		t.Error("Match ignored maxLength")
	}
	if env := Match(q, fields, `{"state":"x"}`, 0); env["error"] != nil {
		t.Errorf("Match with the limit disabled failed: %v", env["error"])
	}
	if env := Eval("[state]", fields, `{"state":"x"}`, 3); env["error"] == nil {
		t.Error("Eval ignored maxLength")
	}
	if env := Eval("[state]", fields, `{"state":"x"}`, 0); env["error"] != nil {
		t.Errorf("Eval with the limit disabled failed: %v", env["error"])
	}
}

// A date written short on a date field becomes a date, and the tree that
// reaches a client must still print and validate: the value carries the
// date in full, not the text it was written as.
func TestShortDate_SurvivesTheBridgeRoundTrip(t *testing.T) {
	fields := `[{"name":"createdAt","type":"datetime"},{"name":"n","type":"integer"},{"name":"name","type":"text"}]`
	for _, tc := range []struct{ query, printed string }{
		{"createdAt=2026-11-5", "createdAt=2026-11-05"},
		{`createdAt="2026-11-5"`, "createdAt=2026-11-05"},
		{"createdAt:2026-1-1..2026-1-9", "createdAt:2026-01-01..2026-01-09"},
		{"createdAt IN (2026-1-1, 2026-2-1)", "createdAt=2026-01-01 OR createdAt=2026-02-01"},
		// A subtraction written out stays one through the round trip.
		{"n=(2026-11)", "n=2026 - (11)"},
		{"n=2026 - 11", "n=2026 - (11)"},
	} {
		parsed := wire(t, ParseAndValidate(tc.query, fields, DefaultMaxLength))
		if msg, _ := parsed["error"].(string); msg != "" {
			t.Fatalf("%s: %s", tc.query, msg)
		}
		tree, err := json.Marshal(parsed["result"])
		if err != nil {
			t.Fatal(err)
		}
		printed := wire(t, Stringify(string(tree), DefaultMaxLength))
		if printed["result"] != tc.printed {
			t.Errorf("%s: prints as %v (%v), want %s", tc.query, printed["result"], printed["error"], tc.printed)
		}
		if again := wire(t, Validate(string(tree), fields, DefaultMaxLength)); again["valid"] != true {
			t.Errorf("%s: its tree does not validate: %v", tc.query, again)
		}
		// The printed form means the same as what was written.
		reparsed := wire(t, ParseAndValidate(tc.printed, fields, DefaultMaxLength))
		if msg, _ := reparsed["error"].(string); msg != "" {
			t.Errorf("%s: its printed form %s is refused: %s", tc.query, tc.printed, msg)
		}
	}
	// Without a schema the run prints as it was written, so it is still a
	// date written short when it is read again.
	parsed := wire(t, Parse("createdAt=2026-11-5", DefaultMaxLength))
	tree, _ := json.Marshal(parsed["result"])
	if printed := wire(t, Stringify(string(tree), DefaultMaxLength)); printed["result"] != "createdAt=2026-11-5" {
		t.Errorf("unvalidated tree prints as %v, want createdAt=2026-11-5", printed)
	}
	if again := wire(t, Validate(string(tree), fields, DefaultMaxLength)); again["valid"] != true {
		t.Errorf("unvalidated tree does not validate: %v", again)
	}
}
