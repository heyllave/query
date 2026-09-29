package bridgejson

import (
	"encoding/json"
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
