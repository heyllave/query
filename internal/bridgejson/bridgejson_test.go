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
	back, err := JSONToAST(string(data))
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
			expr, err := JSONToAST(node)
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
	expr, err := JSONToAST(node)
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
		if _, err := JSONToAST(node); err == nil {
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
			expr, err := JSONToAST(tc.node)
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
	validated := Validate(tree, fields)
	printed := Stringify(tree)

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
	sum := "1" + strings.Repeat("+1", DefaultMaxLength)
	cases := []struct {
		name string
		node string
	}{
		{"a string value", `{"type":"qualifier","op":"=","field":["f"],"value":{"type":"string","raw":"` + long + `"}}`},
		{"a string value's decoded content",
			`{"type":"qualifier","op":"=","field":["f"],"value":{"type":"string","raw":"a","value":"` + long + `"}}`},
		{"an arithmetic value", `{"type":"qualifier","op":"=","field":["f"],"value":{"type":"arith","raw":"` + sum + `"}}`},
		{"an end value", `{"type":"qualifier","op":"..","field":["f"],"value":{"type":"integer","raw":"1"},` +
			`"endValue":{"type":"arith","raw":"` + sum + `"}}`},
		{"a field function", `{"type":"qualifier","op":"=","fieldFunc":"lower(` + long + `)","value":{"type":"integer","raw":"1"}}`},
		{"a field name", `{"type":"presence","field":["` + long + `"]}`},
		{"the whole tree", `{"type":"presence","field":["f"]}` + strings.Repeat(" ", maxASTJSONLength)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := JSONToAST(tc.node)

			if got := parser.Errors(err); len(got) != 1 || got[0].Code != parser.CodeQueryTooLong {
				t.Fatalf("JSONToAST error = %v, want the %s failure", err, parser.CodeQueryTooLong)
			}
			for name, env := range map[string]Envelope{"validate": Validate(tc.node, `[]`), "stringify": Stringify(tc.node)} {
				if code := firstCode(wire(t, env)); code != string(parser.CodeQueryTooLong) {
					t.Errorf("%s answered %v, want code %s", name, env, parser.CodeQueryTooLong)
				}
			}
		})
	}
}

func TestJSONToAST_AcceptsTheTreeOfAQueryAtTheLengthLimit(t *testing.T) {
	// An IN list lowers to one qualifier per element, the largest tree a query
	// of a given length produces.
	field := strings.Repeat("f", 120)
	q := field + " IN (1" + strings.Repeat(",1", (DefaultMaxLength-len(field)-7)/2) + ")"
	expr, err := parser.Parse(q, DefaultMaxLength)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	data, err := json.MarshalIndent(AstToJSON(expr), "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := JSONToAST(string(data)); err != nil {
		t.Errorf("JSONToAST refused a %d-byte tree of a %d-byte query: %v", len(data), len(q), err)
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
			if _, err := JSONToAST(node); err == nil {
				t.Fatalf("JSONToAST accepted %s", node)
			}
		})
	}
}
