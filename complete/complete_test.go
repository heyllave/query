package complete

import (
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/token"
	"github.com/heyllave/query/validate"
)

// schema is shaped like a real host's: plain fields of each type, a field
// with known values, a nested field, a list whose elements hold another
// list, and names that need the bracket form.
func schema() []validate.FieldConfig {
	return []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "managedBy", Type: validate.TypeText, AllowedOps: []validate.Op{validate.OpEq, validate.OpNeq},
			Values: []string{"user", "resource plugin", "812", "a*b", "true"}},
		{Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
		{Name: "replicas", Type: validate.TypeInteger, AllowedOps: validate.NumericOps, Values: []string{"1", "42"}},
		{Name: "ttl", Type: validate.TypeDuration, AllowedOps: validate.DurationOps, Values: []string{"7d", "1h"}},
		{Name: "createdAt", Type: validate.TypeDatetime, AllowedOps: validate.DateOps, Values: []string{"2026-01-01"}},
		{Name: "ready", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
		{Name: "forks", Type: validate.TypeList, AllowedOps: validate.ListOps, Elements: []validate.FieldConfig{
			{Name: "baselineKind", Type: validate.TypeText, AllowedOps: validate.TextOps, Values: []string{"Deployment", "Rollout"}},
			{Name: "baselineName", Type: validate.TypeText, AllowedOps: validate.TextOps, Values: []string{"web", "api server"}},
			{Name: "ports", Type: validate.TypeList, AllowedOps: validate.ListOps, Selectors: ast.Selectors(), Elements: []validate.FieldConfig{
				{Name: "number", Type: validate.TypeInteger, AllowedOps: validate.NumericOps, Values: []string{"80", "443"}},
			}},
		}},
		{Name: "owner email", Type: validate.TypeText, AllowedOps: []validate.Op{validate.OpEq}, Values: []string{"a@b.com"}},
		{Name: "not", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
	}
}

var strict = parser.Config{}.Strict()

// at runs completion on a query that marks the cursor with '|'.
func at(t *testing.T, marked string, cfg parser.Config) (Completion, string) {
	t.Helper()
	offset := strings.IndexByte(marked, '|')
	if offset < 0 || strings.Count(marked, "|") != 1 {
		t.Fatalf("case %q needs exactly one '|'", marked)
	}
	query := marked[:offset] + marked[offset+1:]
	return At(query, offset, schema(), cfg), query
}

func texts(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}

func labels(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

func TestAt_Items(t *testing.T) {
	allFields := []string{"name", "managedBy", "labels.", "replicas", "ttl", "createdAt", "ready", "forks", `["owner email"]`, `["not"]`}
	cases := []struct {
		query string
		slot  parser.Slot
		want  []string // the Text of each item, in order
	}{
		// Field names.
		{"|", parser.SlotField, append(append([]string{}, allFields...), "NOT")},
		{"na|", parser.SlotField, []string{"name", "managedBy", "baselineName"}[:2]},
		{"re|", parser.SlotField, []string{"replicas", "ready", "createdAt"}},
		{"RE|", parser.SlotField, []string{"replicas", "ready", "createdAt"}},
		{"zzz|", parser.SlotField, nil},
		{"own|", parser.SlotField, []string{`["owner email"]`}},
		{"no|", parser.SlotField, []string{`["not"]`, "NOT"}},
		{"name=x AND |", parser.SlotField, append(append([]string{}, allFields...), "NOT")},
		{"name=x AND NOT t|", parser.SlotField, []string{"ttl", "createdAt", `["not"]`}},
		{"(l|", parser.SlotField, []string{"labels.", "replicas", "ttl", `["owner email"]`}},
		{"[own|", parser.SlotField, []string{`["owner email"]`}},
		{`["owner e|`, parser.SlotField, []string{`["owner email"]`}},

		// Inside a selector the fields are those of one element.
		{"forks@(|", parser.SlotField, []string{"baselineKind", "baselineName", "ports", "NOT"}},
		{"forks@all(baselineN|", parser.SlotField, []string{"baselineName"}},
		{"forks@(ports@(|", parser.SlotField, []string{"number", "NOT"}},
		{"forks@(ports@(number=80) AND |", parser.SlotField, []string{"baselineKind", "baselineName", "ports", "NOT"}},
		{"forks@(name|", parser.SlotField, []string{"baselineName"}},
		// A selector on something that is not a declared list: no field
		// can be vouched for.
		{"name@(|", parser.SlotField, []string{"NOT"}},
		{"bogus@(na|", parser.SlotField, nil},
		{"forks@(bogus@(|", parser.SlotField, []string{"NOT"}},

		// What follows a field.
		{"name |", parser.SlotOperator, []string{"=", "!=", "IN (", "NOT IN (", "AND", "OR"}},
		{"managedBy |", parser.SlotOperator, []string{"=", "!=", "IN (", "NOT IN ("}},
		{"replicas |", parser.SlotOperator, []string{"=", "!=", ">", ">=", "<", "<=", ":", "IN (", "NOT IN ("}},
		{"ready |", parser.SlotOperator, []string{"=", "!=", "IN (", "NOT IN ("}},
		{`["owner email"] |`, parser.SlotOperator, []string{"=", "IN ("}},
		{"forks |", parser.SlotOperator, []string{"@first", "@(", "@all(", "@none(", "AND", "OR"}},
		{"forks@(ports |", parser.SlotOperator, []string{"@first", "@last", "@(", "@any(", "@all(", "@none(", "AND", "OR", ")"}},
		{"(name |", parser.SlotOperator, []string{"=", "!=", "IN (", "NOT IN (", "AND", "OR", ")"}},
		{"labels.team |", parser.SlotOperator, []string{"=", "!=", "IN (", "NOT IN (", "AND", "OR"}},
		{"replicas >|", parser.SlotValue, []string{"1", "42"}},
		{"name I|", parser.SlotOperator, []string{"IN ("}},
		{"name N|", parser.SlotOperator, []string{"NOT IN ("}},
		{"name A|", parser.SlotOperator, []string{"AND"}},
		{"name o|", parser.SlotOperator, []string{"OR"}},
		{"bogus |", parser.SlotOperator, []string{"AND", "OR"}},
		{"forks@(baselineKind |", parser.SlotOperator, []string{"=", "!=", "IN (", "NOT IN (", "AND", "OR", ")"}},

		// Selectors after the '@'.
		{"forks@|", parser.SlotSelector, []string{"first", "(", "all(", "none("}},
		{"forks@a|", parser.SlotSelector, []string{"all("}},
		{"forks@(ports@|", parser.SlotSelector, []string{"first", "last", "(", "any(", "all(", "none("}},
		{"forks@(ports@a|", parser.SlotSelector, []string{"any(", "all("}},
		{"name@|", parser.SlotSelector, nil},
		{"bogus@|", parser.SlotSelector, nil},

		// Values the schema knows.
		{"managedBy=|", parser.SlotValue, []string{"user", `"resource plugin"`, `"812"`, `"a*b"`, `"true"`}},
		{"managedBy!=re|", parser.SlotValue, []string{`"resource plugin"`}},
		{"managedBy=US|", parser.SlotValue, []string{"user"}},
		{"managedBy=plug|", parser.SlotValue, []string{`"resource plugin"`}},
		{`managedBy="|`, parser.SlotValue, []string{`"user"`, `"resource plugin"`, `"812"`, `"a*b"`, `"true"`}},
		{`managedBy="res|`, parser.SlotValue, []string{`"resource plugin"`}},
		{"managedBy IN (|", parser.SlotValue, []string{"user", `"resource plugin"`, `"812"`, `"a*b"`, `"true"`}},
		{"managedBy IN (user, 8|", parser.SlotValue, []string{`"812"`}},
		{"ready=|", parser.SlotValue, []string{"true", "false"}},
		{"ready=f|", parser.SlotValue, []string{"false"}},
		{"replicas>=|", parser.SlotValue, []string{"1", "42"}},
		{"replicas:1..|", parser.SlotValue, []string{"1", "42"}},
		{"ttl<|", parser.SlotValue, []string{"7d", "1h"}},
		{"createdAt>|", parser.SlotValue, []string{"2026-01-01"}},
		{"forks@(baselineKind=|", parser.SlotValue, []string{"Deployment", "Rollout"}},
		{"forks@(baselineName=|", parser.SlotValue, []string{"web", `"api server"`}},
		{"forks@(ports@all(number>|", parser.SlotValue, []string{"80", "443"}},
		{`["owner email"]=|`, parser.SlotValue, []string{"a@b.com"}},
		// Values only the host knows.
		{"name=|", parser.SlotValue, nil},
		{"labels.team=|", parser.SlotValue, nil},
		{"bogus=|", parser.SlotValue, nil},
		{"lower(name)=|", parser.SlotValue, nil},
		// Inside a selector the outer field of the same name is not meant.
		{"forks@(managedBy=|", parser.SlotValue, nil},

		// After a complete condition.
		{"name=x |", parser.SlotConnective, []string{"AND", "OR"}},
		{"name=x a|", parser.SlotConnective, []string{"AND"}},
		{"name=x o|", parser.SlotConnective, []string{"OR"}},
		{"(name=x |", parser.SlotConnective, []string{"AND", "OR", ")"}},
		{"forks@(baselineKind=Rollout |", parser.SlotConnective, []string{"AND", "OR", ")"}},
		{"forks@first |", parser.SlotConnective, []string{"AND", "OR"}},
		{"(name=x a|", parser.SlotConnective, []string{"AND"}},

		// Keys, which only the host knows.
		{"labels.|", parser.SlotKey, nil},
		{"labels.te|", parser.SlotKey, nil},

		// Function arguments are fields, written without a trailing dot.
		{"lower(|", parser.SlotArgument, []string{"name", "managedBy", "labels", "replicas", "ttl", "createdAt", "ready", "forks", `["owner email"]`, `["not"]`}},
		{"lower(na|", parser.SlotArgument, []string{"name", "managedBy"}},

		// Nothing to say.
		{"name==|", parser.SlotNone, nil},
		{"name IN (a |", parser.SlotNone, nil},
		{"replicas:1 |", parser.SlotNone, nil},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			c, _ := at(t, tc.query, strict)
			if c.Slot != tc.slot {
				t.Fatalf("slot = %s, want %s", c.Slot, tc.slot)
			}
			if got := texts(c.Items); !reflect.DeepEqual(got, tc.want) && (len(got) > 0 || len(tc.want) > 0) {
				t.Errorf("items =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

func TestAt_DescribesTheSlot(t *testing.T) {
	cases := []struct {
		query    string
		field    string
		known    bool
		typ      validate.FieldValueType
		operator string
		prefix   string
		replace  string
		lists    []string
		function string
		quoted   bool
	}{
		{query: "managedBy=us|", field: "managedBy", known: true, typ: validate.TypeText, operator: "=", prefix: "us", replace: "us"},
		{query: "replicas>=4|", field: "replicas", known: true, typ: validate.TypeInteger, operator: ">=", prefix: "4", replace: "4"},
		{query: "replicas!<4|", field: "replicas", known: true, typ: validate.TypeInteger, operator: "!<", prefix: "4", replace: "4"},
		{query: "replicas:1..|", field: "replicas", known: true, typ: validate.TypeInteger, operator: ":"},
		{query: "name IN (a, b|", field: "name", known: true, typ: validate.TypeText, operator: "IN", prefix: "b", replace: "b"},
		{query: `name="a b|" AND ready=true`, field: "name", known: true, typ: validate.TypeText, operator: "=", prefix: "a b", replace: `"a b"`, quoted: true},
		{query: "labels.team=pl|", field: "labels.team", known: true, typ: validate.TypeText, operator: "=", prefix: "pl", replace: "pl"},
		{query: "bogus=x|", field: "bogus", operator: "=", prefix: "x", replace: "x"},
		{query: "lower(name)=x|", operator: "=", prefix: "x", replace: "x", function: "lower"},
		{query: "forks@(ports@(number=4|", field: "number", known: true, typ: validate.TypeInteger, operator: "=", prefix: "4", replace: "4", lists: []string{"forks", "ports"}},
		{query: "labels.|", field: "labels", known: true, typ: validate.TypeText, replace: "labels."},
		{query: "name=x AND labels.app.kubernetes.io/na|me=y", field: "labels", known: true, typ: validate.TypeText, prefix: "app.kubernetes.io/na", replace: "labels.app.kubernetes.io/name"},
		{query: `["labels.my k|`, field: "labels", known: true, typ: validate.TypeText, prefix: "my k", replace: `["labels.my k`},
		{query: "forks |", field: "forks", known: true, typ: validate.TypeList},
		{query: "forks@al|", field: "forks", known: true, typ: validate.TypeList, prefix: "al", replace: "al"},
		{query: "contains(labels, na|", function: "contains", prefix: "na", replace: "na"},
		{query: "na|me=x", prefix: "na", replace: "name"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			c, query := at(t, tc.query, strict)
			if c.Field != tc.field || c.Known != tc.known || c.Type != tc.typ {
				t.Errorf("field = %q known=%v type=%s, want %q known=%v type=%s", c.Field, c.Known, c.Type, tc.field, tc.known, tc.typ)
			}
			if c.Operator != tc.operator {
				t.Errorf("operator = %q, want %q", c.Operator, tc.operator)
			}
			if c.Prefix != tc.prefix {
				t.Errorf("prefix = %q, want %q", c.Prefix, tc.prefix)
			}
			if got := query[c.Replace.Offset : c.Replace.Offset+c.Replace.Length]; got != tc.replace {
				t.Errorf("replace = %q, want %q", got, tc.replace)
			}
			if !reflect.DeepEqual(c.Lists, tc.lists) {
				t.Errorf("lists = %q, want %q", c.Lists, tc.lists)
			}
			if c.Function != tc.function {
				t.Errorf("function = %q, want %q", c.Function, tc.function)
			}
			if c.Quoted != tc.quoted {
				t.Errorf("quoted = %v, want %v", c.Quoted, tc.quoted)
			}
		})
	}
}

func TestAt_ItemsCarryTheirKindAndDetail(t *testing.T) {
	c, _ := at(t, "(forks |", strict)
	want := []Item{
		{Label: "@first", Text: "@first", Kind: KindSelector, Detail: "has an element"},
		{Label: "@(...)", Text: "@(", Kind: KindSelector, Detail: "an element matches"},
		{Label: "@all(...)", Text: "@all(", Kind: KindSelector, Detail: "every element matches"},
		{Label: "@none(...)", Text: "@none(", Kind: KindSelector, Detail: "no element matches"},
		{Label: "AND", Text: "AND", Kind: KindKeyword},
		{Label: "OR", Text: "OR", Kind: KindKeyword},
		{Label: ")", Text: ")", Kind: KindKeyword},
	}
	if !reflect.DeepEqual(c.Items, want) {
		t.Fatalf("items = %+v\nwant    %+v", c.Items, want)
	}

	c, _ = at(t, "l|", strict)
	if got, want := c.Items[0], (Item{Label: "labels", Text: "labels.", Kind: KindField, Detail: "text"}); got != want {
		t.Errorf("nested field item = %+v, want %+v", got, want)
	}
	c, _ = at(t, "replicas |", strict)
	if got, want := c.Items[6], (Item{Label: ":", Text: ":", Kind: KindOperator, Detail: "range, from..to"}); got != want {
		t.Errorf("range item = %+v, want %+v", got, want)
	}
	c, _ = at(t, "managedBy=re|", strict)
	if got, want := c.Items[0], (Item{Label: "resource plugin", Text: `"resource plugin"`, Kind: KindValue}); got != want {
		t.Errorf("value item = %+v, want %+v", got, want)
	}
}

func TestAt_ImplicitAndOffersFieldsOnlyWhenTheGrammarHasIt(t *testing.T) {
	for _, tc := range []struct {
		query   string
		lenient []string
		strict  []string
	}{
		{"name=x re|", []string{"replicas", "ready", "createdAt"}, nil},
		{"name=x a|", []string{"AND", "name", "managedBy", "labels.", "replicas", "createdAt", "ready", `["owner email"]`}, []string{"AND"}},
		{"name r|", []string{"replicas", "ready", "createdAt", "forks", `["owner email"]`}, nil},
		// With nothing typed the list would be every field after every
		// condition, so the connectives stand alone.
		{"name=x |", []string{"AND", "OR"}, []string{"AND", "OR"}},
		// A field that cannot stand alone is not followed by another.
		{"managedBy r|", nil, nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			c, _ := at(t, tc.query, parser.Config{})
			if got := texts(c.Items); !reflect.DeepEqual(got, tc.lenient) && (len(got) > 0 || len(tc.lenient) > 0) {
				t.Errorf("default grammar: items = %q, want %q", got, tc.lenient)
			}
			c, _ = at(t, tc.query, strict)
			if got := texts(c.Items); !reflect.DeepEqual(got, tc.strict) && (len(got) > 0 || len(tc.strict) > 0) {
				t.Errorf("strict grammar: items = %q, want %q", got, tc.strict)
			}
		})
	}
}

func TestAt_ANameWithADotThatIsNotNested(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "spec.cluster", Type: validate.TypeText, AllowedOps: validate.TextOps, Values: []string{"prod"}},
		{Name: "spec.ttl", Type: validate.TypeDuration, AllowedOps: validate.DurationOps},
		{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	c := At("a=1 AND spec.c", 14, fields, strict)
	if c.Slot != parser.SlotField || c.Prefix != "spec.c" {
		t.Fatalf("slot = %s prefix = %q, want field and %q", c.Slot, c.Prefix, "spec.c")
	}
	if got, want := texts(c.Items), []string{"spec.cluster"}; !reflect.DeepEqual(got, want) {
		t.Errorf("items = %q, want %q", got, want)
	}
	if c.Replace != (token.Position{Offset: 8, Length: 6}) {
		t.Errorf("replace = %+v, want the whole dotted name", c.Replace)
	}
	c = At("spec.cluster=", 13, fields, strict)
	if got, want := texts(c.Items), []string{"prod"}; !reflect.DeepEqual(got, want) || c.Field != "spec.cluster" {
		t.Errorf("value of a dotted field: field %q items %q, want %q", c.Field, got, want)
	}
}

func TestAt_NeverFailsOnABadPosition(t *testing.T) {
	for _, tc := range []struct {
		name   string
		query  string
		cursor int
		cfg    parser.Config
	}{
		{"negative cursor", "name=x", -5, strict},
		{"cursor past the end", "name=x", 99, strict},
		{"cursor inside a character", "café=x", 4, strict},
		{"invalid UTF-8", "na\xffme=", 6, strict},
		{"over the length limit", strings.Repeat("a", 300) + "=", 301, parser.Config{MaxLength: 256}},
		{"no schema", "name=", 5, strict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fields []validate.FieldConfig
			if tc.name != "no schema" {
				fields = schema()
			}
			c := At(tc.query, tc.cursor, fields, tc.cfg)
			if tc.name == "no schema" {
				if c.Slot != parser.SlotValue || c.Known || len(c.Items) != 0 {
					t.Fatalf("without a schema: %+v", c)
				}
				return
			}
			if c.Slot != parser.SlotNone || len(c.Items) != 0 || c.Replace.Length != 0 {
				t.Fatalf("completion = %+v, want nothing", c)
			}
		})
	}
}

func TestCompletion_Offer(t *testing.T) {
	t.Run("values of a text field are written as text", func(t *testing.T) {
		c, _ := at(t, "name=|", strict)
		items := c.Offer("web", "api server", "812", "7d", "true", "2026-01-01", "a*b", `say "hi"`, "a=b", "pr-10-3", "10-3", "", "x)", "a,b")
		want := []string{"web", `"api server"`, `"812"`, `"7d"`, `"true"`, `"2026-01-01"`, `"a*b"`, `"say \"hi\""`, `"a=b"`, "pr-10-3", `"10-3"`, `""`, `"x)"`, `"a,b"`}
		if got := texts(items); !reflect.DeepEqual(got, want) {
			t.Fatalf("texts = %q\nwant    %q", got, want)
		}
		if got := labels(items)[1]; got != "api server" {
			t.Errorf("label = %q, want the candidate as given", got)
		}
		for _, it := range items {
			if it.Kind != KindValue {
				t.Errorf("kind of %q = %s, want value", it.Label, it.Kind)
			}
		}
	})
	t.Run("values of a typed field keep the form of their type", func(t *testing.T) {
		c, _ := at(t, "replicas>|", strict)
		// What an integer field would refuse is left out: a fraction, a
		// word, arithmetic.
		if got, want := texts(c.Offer("3", "1.5", "many", "10-3", "-4", "007")), []string{"3", "-4", "007"}; !reflect.DeepEqual(got, want) {
			t.Errorf("integer field: %q, want %q", got, want)
		}
		c, _ = at(t, "ttl>|", strict)
		if got, want := texts(c.Offer("7d", "30m")), []string{"7d", "30m"}; !reflect.DeepEqual(got, want) {
			t.Errorf("duration field: %q, want %q", got, want)
		}
		c, _ = at(t, "createdAt>|", strict)
		if got, want := texts(c.Offer("2026-01-01", "2026-01-01T10:00:00Z")), []string{"2026-01-01", "2026-01-01T10:00:00Z"}; !reflect.DeepEqual(got, want) {
			t.Errorf("datetime field: %q, want %q", got, want)
		}
	})
	t.Run("a field nothing is known about takes text", func(t *testing.T) {
		c, _ := at(t, "bogus=|", strict)
		if got, want := texts(c.Offer("812", "x y")), []string{`"812"`, `"x y"`}; !reflect.DeepEqual(got, want) {
			t.Errorf("texts = %q, want %q", got, want)
		}
	})
	t.Run("inside an open quote every value is a whole quoted string", func(t *testing.T) {
		c, query := at(t, `name="we|`, strict)
		items := c.Offer("web", "api server")
		if got, want := texts(items), []string{`"web"`, `"api server"`}; !reflect.DeepEqual(got, want) {
			t.Fatalf("texts = %q, want %q", got, want)
		}
		if got := apply(query, c, items[0]); got != `name="web"` {
			t.Errorf("applied = %q", got)
		}
	})
	t.Run("keys are joined to their field", func(t *testing.T) {
		c, query := at(t, "name=x AND labels.te|", strict)
		items := c.Offer("team", "app.kubernetes.io/name", "my key", "1st", "a..b", "", "\xff")
		want := []string{"labels.team", "labels.app.kubernetes.io/name", `["labels.my key"]`, `["labels.1st"]`, `["labels.a..b"]`}
		if got := texts(items); !reflect.DeepEqual(got, want) {
			t.Fatalf("texts = %q\nwant    %q", got, want)
		}
		if items[0].Kind != KindKey || items[0].Label != "team" {
			t.Errorf("item = %+v", items[0])
		}
		if got := apply(query, c, items[2]); got != `name=x AND ["labels.my key"]` {
			t.Errorf("applied = %q", got)
		}
	})
	t.Run("field names the host adds take the form they need", func(t *testing.T) {
		c, _ := at(t, "|", strict)
		if got, want := texts(c.Offer("extra", "with space", "or", "")), []string{"extra", `["with space"]`, `["or"]`}; !reflect.DeepEqual(got, want) {
			t.Errorf("texts = %q, want %q", got, want)
		}
	})
	t.Run("candidates that are not valid UTF-8 are left out", func(t *testing.T) {
		c, _ := at(t, "name=|", strict)
		if got, want := texts(c.Offer("ok", "bad\xff", "fine")), []string{"ok", "fine"}; !reflect.DeepEqual(got, want) {
			t.Errorf("texts = %q, want %q", got, want)
		}
	})
	t.Run("a slot the schema fills takes no candidates", func(t *testing.T) {
		for _, q := range []string{"name |", "forks@|", "name=x |", "name==|"} {
			if c, _ := at(t, q, strict); c.Offer("x") != nil {
				t.Errorf("%q (%s): Offer returned items", q, c.Slot)
			}
		}
	})
	t.Run("no candidates is an empty list, not nil", func(t *testing.T) {
		c, _ := at(t, "name=|", strict)
		if items := c.Offer(); items == nil || len(items) != 0 {
			t.Errorf("Offer() = %#v", items)
		}
	})
}

func TestValueText(t *testing.T) {
	for in, want := range map[string]string{
		"draft":         "draft",
		"pr-10-3":       "pr-10-3",
		"v1.2.3":        "v1.2.3",
		"a@b.com":       "a@b.com",
		"example.com/x": "example.com/x",
		"café":          "café",
		"":              `""`,
		"two words":     `"two words"`,
		"812":           `"812"`,
		"-5":            `"-5"`,
		"1.5":           `"1.5"`,
		"7d":            `"7d"`,
		"true":          `"true"`,
		"2026-01-01":    `"2026-01-01"`,
		"10-3":          `"10-3"`,
		"a*":            `"a*"`,
		"a=b":           `"a=b"`,
		"a>b":           `"a>b"`,
		`a"b`:           `"a\"b"`,
		`a\b`:           `a\b`,
		`a\`:            `"a\\"`,
		"a,b":           `"a,b"`,
		"(x)":           `"(x)"`,
		"[x]":           `"[x]"`,
		"a\tb":          `"a\tb"`,
		"a\nb":          `"a\nb"`,
		"a b":           "\"a b\"",
		"1..5":          `"1..5"`,
		"now()":         `"now()"`,
	} {
		if got := ValueText(in); got != want {
			t.Errorf("ValueText(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestFieldText(t *testing.T) {
	for in, want := range map[string]string{
		"name":                          "name",
		"labels.team":                   "labels.team",
		"labels.app.kubernetes.io/name": "labels.app.kubernetes.io/name",
		"cpu-limit":                     "cpu-limit",
		"my field":                      `["my field"]`,
		"labels.my key":                 `["labels.my key"]`,
		"not":                           `["not"]`,
		"AND":                           `["AND"]`,
		"true":                          `["true"]`,
		"labels.in":                     "labels.in",
		"1st":                           `["1st"]`,
		"a..b":                          `["a..b"]`,
		"":                              `[""]`,
		`a"b`:                           `["a\"b"]`,
	} {
		if got := FieldText(in); got != want {
			t.Errorf("FieldText(%q) = %s, want %s", in, got, want)
		}
	}
}

// apply puts an accepted item into the query, as an editor would.
func apply(query string, c Completion, it Item) string {
	return query[:c.Replace.Offset] + it.Text + query[c.Replace.Offset+c.Replace.Length:]
}

// TestAt_AcceptingSuggestionsWritesValidQueries writes queries the way a
// person using only the suggestions would: at each step it takes one of the
// items offered at the end of the text, chosen at random, and whenever the
// result stands as a whole query it must parse under the strict grammar and
// validate against the schema. A suggestion that is wrong for its position,
// or written in a form the grammar reads differently, fails here.
func TestAt_AcceptingSuggestionsWritesValidQueries(t *testing.T) {
	fields := schema()
	validator := validate.New(fields)
	keys := []string{"team", "app.kubernetes.io/name", "my key"}
	hostValues := []string{"web", "api server", "812", "a*b", `say "hi"`, "10-3"}

	complete, steps := 0, 0
	for seed := int64(1); seed <= 400; seed++ {
		rng := rand.New(rand.NewSource(seed))
		query := ""
		var owesRangeEnd bool
		for step := 0; step < 40; step++ {
			c := At(query, len(query), fields, strict)
			if c.Slot == parser.SlotConnective && c.Prefix == "" && len(c.Lists) == 0 && !hasItem(c.Items, ")") {
				complete++
				expr, err := parser.ParseWith(query, strict)
				if err != nil {
					t.Fatalf("seed %d: %q does not parse: %v", seed, query, err)
				}
				if err := validator.Validate(expr); err != nil {
					t.Fatalf("seed %d: %q does not validate: %v", seed, query, err)
				}
			}
			items := c.Items
			switch {
			case c.Slot == parser.SlotKey:
				items = c.Offer(keys...)
			case c.Slot == parser.SlotValue && len(items) == 0:
				items = c.Offer(hostValues...)
			}
			// IN lists are separated by commas, which are not suggested;
			// they have a test of their own.
			items = withoutIn(items)
			if len(items) == 0 {
				t.Fatalf("seed %d: nothing offered after %q (slot %s)", seed, query, c.Slot)
			}
			it := items[rng.Intn(len(items))]
			query = apply(query, c, it)
			steps++
			switch {
			case it.Text == ":":
				owesRangeEnd = true
			case it.Kind == KindValue && owesRangeEnd:
				owesRangeEnd = false
				query += ".."
			case it.Kind == KindOperator, strings.HasSuffix(it.Text, "("), strings.HasSuffix(it.Text, "."):
				// The next thing follows directly.
			default:
				query += " "
			}
		}
	}
	if complete < 1000 || steps < 10000 {
		t.Fatalf("the walk checked %d whole queries in %d steps; it is not exercising much", complete, steps)
	}
}

func hasItem(items []Item, text string) bool {
	for _, it := range items {
		if it.Text == text {
			return true
		}
	}
	return false
}

func withoutIn(items []Item) []Item {
	var out []Item
	for _, it := range items {
		if !strings.Contains(it.Text, "IN (") {
			out = append(out, it)
		}
	}
	return out
}

func TestAt_InListsBuiltFromSuggestions(t *testing.T) {
	fields := schema()
	validator := validate.New(fields)
	for _, field := range []string{"managedBy", "replicas", "ttl", "createdAt", "ready"} {
		for _, form := range []string{"IN (", "NOT IN ("} {
			query := field + " "
			c := At(query, len(query), fields, strict)
			if !hasItem(c.Items, form) {
				t.Fatalf("%q: %q is not offered: %q", query, form, texts(c.Items))
			}
			query += form
			for i := 0; i < 3; i++ {
				c = At(query, len(query), fields, strict)
				if c.Slot != parser.SlotValue || c.Operator != "IN" || len(c.Items) == 0 {
					t.Fatalf("%q: slot %s operator %q items %q", query, c.Slot, c.Operator, texts(c.Items))
				}
				query = apply(query, c, c.Items[i%len(c.Items)]) + ", "
			}
			query = strings.TrimSuffix(query, ", ") + ")"
			expr, err := parser.ParseWith(query, strict)
			if err != nil {
				t.Fatalf("%q does not parse: %v", query, err)
			}
			if err := validator.Validate(expr); err != nil {
				t.Fatalf("%q does not validate: %v", query, err)
			}
		}
	}
}

// checkCompletion holds a Completion to what a client relies on, whatever
// the input.
func checkCompletion(t *testing.T, query string, cursor int, c Completion) {
	t.Helper()
	r := c.Replace
	if r.Offset < 0 || r.Length < 0 || r.Offset+r.Length > len(query) {
		t.Fatalf("At(%q, %d): replace %+v is outside the query", query, cursor, r)
	}
	if c.Slot == parser.SlotNone {
		if len(c.Items) != 0 || r.Length != 0 {
			t.Fatalf("At(%q, %d): no slot, but %+v", query, cursor, c)
		}
		return
	}
	if cursor < r.Offset || cursor > r.Offset+r.Length {
		t.Fatalf("At(%q, %d): replace %+v does not hold the cursor", query, cursor, r)
	}
	if !utf8.ValidString(c.Prefix) || !utf8.ValidString(c.Field) {
		t.Fatalf("At(%q, %d): prefix %q or field %q is not valid UTF-8", query, cursor, c.Prefix, c.Field)
	}
	for _, it := range c.Items {
		if it.Text == "" || it.Label == "" || it.Kind == "" {
			t.Fatalf("At(%q, %d): incomplete item %+v", query, cursor, it)
		}
		if !utf8.ValidString(it.Text) {
			t.Fatalf("At(%q, %d): item text %q is not valid UTF-8", query, cursor, it.Text)
		}
		if out := apply(query, c, it); utf8.ValidString(query) && !utf8.ValidString(out) {
			t.Fatalf("At(%q, %d): accepting %q splits a character: %q", query, cursor, it.Text, out)
		}
	}
}

// FuzzAt checks that no query and no cursor makes completion panic or hand
// back something a client cannot apply.
func FuzzAt(f *testing.F) {
	for _, s := range []string{
		"", "na", "name ", "name=", `managedBy="re`, "labels.te", "forks@(baselineKind=", "forks@(ports@a",
		"managedBy IN (user, ", "lower(na", "[own", `["labels.my k`, "name=x AND NOT (ready=", "replicas:1..",
		"not.", "true ", "café=", "forks@first ", "name=x a", "a=1)", "a==", "x=a b",
	} {
		f.Add(s, len(s))
		f.Add(s, len(s)/2)
	}
	fields := schema()
	f.Fuzz(func(t *testing.T, s string, cursor int) {
		for _, cfg := range []parser.Config{{}, strict} {
			c := At(s, cursor, fields, cfg)
			checkCompletion(t, s, cursor, c)
			c.Offer("x", "a b", "", "812")
		}
	})
}

// FuzzValueText checks the promise the host relies on when it offers its
// own values: whatever the text, the written form reads back as that text.
func FuzzValueText(f *testing.F) {
	for _, s := range []string{
		"", "draft", "two words", "812", "7d", "true", "2026-01-01", "10-3", "a*", `a"b`, `a\`, `\`, "a=b",
		"(x)", "[x]", "a,b", "1..5", "now()", "a\nb", "a b", "café", "!", "..", "-", "+1", "*", `\*`,
		"AND", "a AND b", "x)", "@", "a@first", "\x00", "\x7f",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		inQuote, _ := place{quoted: true}.write(s)
		for _, text := range []string{ValueText(s), inQuote} {
			expr, err := parser.ParseWith("f="+text, strict)
			if err != nil {
				t.Fatalf("ValueText(%q) = %s, which does not parse: %v", s, text, err)
			}
			q, ok := expr.(*ast.QualifierExpr)
			if !ok || q.Value.Type != ast.ValueString || q.Value.Wildcard || q.Value.Str != s {
				t.Fatalf("ValueText(%q) = %s, which reads back as %s", s, text, ast.String(expr))
			}
			// The same form must hold before the ')' of a group.
			group, err := parser.ParseWith("(f="+text+")", strict)
			if err != nil {
				t.Fatalf("ValueText(%q) = %s, which does not parse in a group: %v", s, text, err)
			}
			if inner := ast.Qualifiers(group); len(inner) != 1 || inner[0].Value.Str != s || inner[0].Value.Wildcard {
				t.Fatalf("ValueText(%q) = %s, which reads back in a group as %s", s, text, ast.String(group))
			}
			// And inside an IN list.
			in, err := parser.ParseWith("f IN ("+text+", z)", strict)
			if err != nil {
				t.Fatalf("ValueText(%q) = %s, which does not parse in an IN list: %v", s, text, err)
			}
			first := ast.Qualifiers(in)[0]
			if first.Value.Type != ast.ValueString || first.Value.Wildcard || first.Value.Str != s {
				t.Fatalf("ValueText(%q) = %s, which reads back in an IN list as %s", s, text, ast.String(in))
			}
		}
	})
}

// FuzzFieldText checks that a name written by FieldText reads back as the
// same name, as a field and not as anything else.
func FuzzFieldText(f *testing.F) {
	for _, s := range []string{
		"name", "labels.team", "my field", "not", "AND", "true", "in", "1st", "a..b", ".a", "a.", `a"b`, `a\`,
		"labels.app.kubernetes.io/name", "café", "a b.c d", "[x]", "a=b", "a b", "x.not", "or.x",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) || s == "" {
			return
		}
		text := FieldText(s)
		expr, err := parser.ParseWith(text, strict)
		if err != nil {
			t.Fatalf("FieldText(%q) = %s, which does not parse: %v", s, text, err)
		}
		p, ok := expr.(*ast.PresenceExpr)
		if !ok || p.Field.String() != s {
			t.Fatalf("FieldText(%q) = %s, which reads back as %s", s, text, ast.String(expr))
		}
	})
}

// A value compared with a function's result is not known to be text, so a
// number offered there stays a number: len(name)>"5" would compare as text
// and answer differently.
func TestCompletion_OfferAfterAFunctionKeepsLiterals(t *testing.T) {
	c, query := at(t, "len(name)>|", strict)
	if c.Slot != parser.SlotValue || c.Function != "len" || c.Field != "" {
		t.Fatalf("completion = %+v", c)
	}
	// len gives a number, so only numbers are offered.
	items := c.Offer("5", "10", "1.5", "7d", "two words", "abc")
	if got, want := texts(items), []string{"5", "10", "1.5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts = %q, want %q", got, want)
	}

	// The accepted query answers as the one a person would type.
	fields := schema()
	record := map[string]any{"name": "abcdefg"}
	for i, typed := range []string{"len(name)>5", "len(name)>10"} {
		accepted := apply(query, c, items[i])
		if accepted != typed {
			t.Fatalf("accepted = %q, want %q", accepted, typed)
		}
		got, err := eval.Compile(accepted, fields)
		if err != nil {
			t.Fatal(err)
		}
		want, err := eval.Compile(typed, fields)
		if err != nil {
			t.Fatal(err)
		}
		if got.Match(record) != want.Match(record) {
			t.Errorf("%s matches %v, the typed form %v", accepted, got.Match(record), want.Match(record))
		}
	}
	if prog, _ := eval.Compile("len(name)>10", fields); prog.Match(record) {
		t.Error("len(name)>10 matches a seven-letter name")
	}

	// In an IN list after a function, and inside an open quote, the same
	// rules hold.
	c, _ = at(t, "len(name) IN (1, |", strict)
	if got := texts(c.Offer("5")); !reflect.DeepEqual(got, []string{"5"}) {
		t.Errorf("in a list after a function: %q", got)
	}
	c, _ = at(t, `lower(name)="a|`, strict)
	if got := texts(c.Offer("5")); !reflect.DeepEqual(got, []string{`"5"`}) {
		t.Errorf("inside a quote after a function: %q", got)
	}
	// A function that gives text takes text: 812 bare would be compared as
	// a number and miss the name "812".
	for _, q := range []string{"lower(name)=|", "trim(name)=|", "upper(name) IN (a, |"} {
		c, _ = at(t, q, strict)
		if got, want := texts(c.Offer("812", "1.50", "web", "two words", "7d")), []string{`"812"`, `"1.50"`, "web", `"two words"`, `"7d"`}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %q, want %q", q, got, want)
		}
	}
	accepted := `lower(name)="812"`
	prog, err := eval.Compile(accepted, fields)
	if err != nil || !prog.Match(map[string]any{"name": "812"}) {
		t.Errorf("%s does not match the name 812 (%v)", accepted, err)
	}
	// A function that gives a date takes a date, bare or in quotes.
	c, _ = at(t, "addDays(createdAt, 1)>|", strict)
	if got, want := texts(c.Offer("2026-01-01", "soon", "5")), []string{"2026-01-01"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after a date function: %q, want %q", got, want)
	}
	// One whose result is not known keeps a literal as written and quotes
	// only what must be text.
	c, _ = at(t, "coalesce(name, description)=|", strict)
	if got, want := texts(c.Offer("5", "two words", "abc")), []string{"5", `"two words"`, "abc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after a function of unknown result: %q, want %q", got, want)
	}
	// A field nothing is known about is still taken as text.
	c, _ = at(t, "bogus>|", strict)
	if got := texts(c.Offer("5")); !reflect.DeepEqual(got, []string{`"5"`}) {
		t.Errorf("after an unknown field: %q", got)
	}
}

// Accepting an item never writes twice what the query already has after the
// cursor, and never leaves half of what it replaced.
func TestAt_AcceptingAnItemInTheMiddleOfAQuery(t *testing.T) {
	keys := []string{"my key", "team"}
	for _, tc := range []struct {
		query string // with the cursor
		pick  string // the label to accept, from the items or from keys
		want  string
	}{
		// A nested field's dot, when the dot is already there.
		{"labels|.foo=1", "labels", "labels.foo=1"},
		{"lab|.foo=1", "labels", "labels.foo=1"},
		{"lab|=1", "labels", "labels=1"},
		{"lab| AND name=x", "labels", "labels AND name=x"},
		{"name=x AND lab|", "labels", "name=x AND labels."},
		{"name=x AND lab|els.team=y", "labels", "name=x AND labels.team=y"},
		// A selector's parenthesis, when the parenthesis is already there.
		{"forks@al|(baselineKind=x)", "@all(...)", "forks@all(baselineKind=x)"},
		{"forks@|(baselineKind=x)", "@none(...)", "forks@none(baselineKind=x)"},
		{"forks @|(baselineKind=x)", "@all(...)", "forks @all(baselineKind=x)"},
		{"forks@al|", "@all(...)", "forks@all("},
		// An IN list's parenthesis.
		{"name |(a, b)", "IN (...)", "name IN(a, b)"},
		// A bracketed name with its quote closed and its bracket missing.
		{`["labels.my|"`, "my key", `["labels.my key"]`},
		{`["labels.my|"=x`, "my key", `["labels.my key"]=x`},
		{`["labels.my|"]=x`, "my key", `["labels.my key"]=x`},
		{`["labels.te|`, "team", "labels.team"},
		// A value that is the name of a call goes with its arguments.
		{"managedBy=us|er(x)", "user", "managedBy=user"},
		{"managedBy=us|er(x) AND ready=true", "user", "managedBy=user AND ready=true"},
		{"managedBy=us|er", "user", "managedBy=user"},
	} {
		c, query := at(t, tc.query, strict)
		items := c.Items
		if c.Slot == parser.SlotKey {
			items = c.Offer(keys...)
		}
		var picked *Item
		for i := range items {
			if items[i].Label == tc.pick {
				picked = &items[i]
			}
		}
		if picked == nil {
			t.Errorf("%s: %q is not offered: %q", tc.query, tc.pick, labels(items))
			continue
		}
		if got := apply(query, c, *picked); got != tc.want {
			t.Errorf("%s: accepting %q gives %q, want %q", tc.query, tc.pick, got, tc.want)
		}
	}
	// Each of the results that is a whole query parses.
	for _, q := range []string{"labels.foo=1", "forks@all(baselineKind=x)", `["labels.my key"]=x`, "managedBy=user AND ready=true", "name IN(a, b)"} {
		if _, err := parser.ParseWith(q, strict); err != nil {
			t.Errorf("%s does not parse: %v", q, err)
		}
	}
}

func TestAt_AfterAFieldAndNot(t *testing.T) {
	for _, tc := range []struct {
		query   string
		strict  []string
		lenient []string
	}{
		{"name NOT |", []string{"IN ("}, []string{"IN ("}},
		{"name NOT I|", []string{"IN ("}, []string{"IN (", "replicas", `["owner email"]`}},
		{"name NOT in|", []string{"IN ("}, []string{"IN ("}},
		{"name NOT re|", nil, []string{"replicas", "ready", "createdAt"}},
		// A field that cannot be negated has no NOT IN.
		{`["owner email"] NOT |`, nil, nil},
		{"bogus NOT |", []string{"IN ("}, []string{"IN ("}},
	} {
		c, query := at(t, tc.query, strict)
		if got := texts(c.Items); !reflect.DeepEqual(got, tc.strict) && (len(got) > 0 || len(tc.strict) > 0) {
			t.Errorf("%s under the strict grammar: %q, want %q", tc.query, got, tc.strict)
		}
		if len(c.Items) > 0 && c.Items[0].Text == "IN (" {
			if got := apply(query, c, c.Items[0]); !strings.HasSuffix(got, "NOT IN (") {
				t.Errorf("%s: accepting IN gives %q", tc.query, got)
			}
		}
		c, _ = at(t, tc.query, parser.Config{})
		if got := texts(c.Items); !reflect.DeepEqual(got, tc.lenient) && (len(got) > 0 || len(tc.lenient) > 0) {
			t.Errorf("%s under the default grammar: %q, want %q", tc.query, got, tc.lenient)
		}
	}
}

// Under the strict grammar only a connective follows a complete condition,
// so nothing is offered where the default grammar would start the next one.
func TestAt_StrictOffersNothingWhereOnlyAConnectiveCanCome(t *testing.T) {
	for _, q := range []string{"name=x NOT |", "name=x (|", "name=x ready=|", "forks@first (|"} {
		if c, _ := at(t, q, strict); c.Slot != parser.SlotNone || len(c.Items) != 0 {
			t.Errorf("%s under the strict grammar: slot %s items %q", q, c.Slot, texts(c.Items))
		}
		if c, _ := at(t, q, parser.Config{}); c.Slot == parser.SlotNone {
			t.Errorf("%s under the default grammar: nothing offered", q)
		}
	}
}

// TestAt_AcceptingAnItemNeverAddsAParseError takes every query of the shared
// corpus, puts the cursor at every position in turn, and accepts every item
// offered there. The result may still be unfinished, but up to the end of
// the accepted text it must not have more parse errors than the query had:
// a suggestion is never what breaks it. Host candidates go through Offer at the slots that take them.
func TestAt_AcceptingAnItemNeverAddsAParseError(t *testing.T) {
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
	fields := append(schema(),
		validate.FieldConfig{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps, Values: []string{"draft", "in review"}},
		validate.FieldConfig{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		validate.FieldConfig{Name: "created_at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
	)
	candidates := []string{"web", "two words", "812", "a*b", "7d"}
	// errorsBefore counts the parse errors that start before limit. An error
	// at or after the accepted text says only that more is needed or that
	// what followed no longer fits, which is not the suggestion breaking
	// what was written.
	errorsBefore := func(q string, limit int, cfg parser.Config) int {
		_, err := parser.ParseWith(q, cfg)
		n := 0
		for _, e := range parser.Errors(err) {
			if e.Position.Offset < limit {
				n++
			}
		}
		return n
	}

	accepted := 0
	for _, tc := range corpus.Cases {
		q := tc.Query
		if q == "" || !utf8.ValidString(q) {
			continue
		}
		for _, cfg := range []parser.Config{{}, strict} {
			for cursor := 0; cursor <= len(q); cursor++ {
				if !utf8.ValidString(q[:cursor]) {
					continue
				}
				c := At(q, cursor, fields, cfg)
				// Text typed in front of other text runs into it, whoever
				// types it, so only a position where the replaced span is
				// followed by a break is held to this: the end, a space or
				// a closing parenthesis.
				if rest := q[c.Replace.Offset+c.Replace.Length:]; rest != "" && rest[0] != ' ' && rest[0] != ')' {
					continue
				}
				items := append([]Item{}, c.Items...)
				items = append(items, c.Offer(candidates...)...)
				for _, it := range items {
					accepted++
					out := apply(q, c, it)
					end := c.Replace.Offset + len(it.Text)
					before := errorsBefore(q, c.Replace.Offset+c.Replace.Length, cfg)
					if after := errorsBefore(out, end, cfg); after > before {
						t.Errorf("%q at %d (%s, strict=%v): accepting %q gives %q, with %d parse errors up to the accepted text where there were %d",
							q, cursor, c.Slot, cfg.ExplicitAnd, it.Text, out, after, before)
					}
				}
			}
		}
	}
	if accepted < 5000 {
		t.Fatalf("only %d items were accepted; the corpus did not load as expected", accepted)
	}
}

// After an arithmetic sign only a number, a date or a duration can stand:
// text would end the arithmetic and leave the sign behind.
func TestCompletion_OfferForAnOperandOfArithmetic(t *testing.T) {
	c, query := at(t, "replicas>10 - 3|", strict)
	if c.Slot != parser.SlotValue || !c.Arithmetic {
		t.Fatalf("completion = %+v", c)
	}
	// replicas is a number, so the operand is one too.
	items := c.Offer("web", "5", "two words", "7d", "2026-01-01", "1.5", "true", ".5", "5.")
	if got, want := texts(items), []string{"5", "1.5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts = %q, want %q", got, want)
	}
	if got := apply(query, c, items[0]); got != "replicas>10 - 5" {
		t.Errorf("applied = %q", got)
	}
	// The schema's own values are held to the same rule.
	c, _ = at(t, "replicas>10 - 4|", strict)
	if got, want := texts(c.Items), []string{"42"}; !reflect.DeepEqual(got, want) {
		t.Errorf("schema values = %q, want %q", got, want)
	}
	// The field's type and the sign decide the kind: a date takes a
	// duration after + or -, a duration takes a duration after + or - and
	// a number after * or /.
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"createdAt>now() - |", []string{"7d", "1h"}},
		{"createdAt>2026-01-01 + |", []string{"7d", "1h"}},
		{"createdAt>now() * |", nil},
		{"ttl>1h + |", []string{"7d", "1h"}},
		{"ttl>1h * |", []string{"5", "1.5"}},
		{"ttl>1h % |", nil},
		{"replicas>10 - |", []string{"5", "1.5"}},
		{"replicas>10 -|", []string{"5", "1.5"}},
		{"bogus>10 - |", []string{"5", "7d", "1h", "2026-01-01", "1.5"}},
		{"len(name)>10 - |", []string{"5", "1.5"}},
	} {
		c, _ := at(t, tc.query, strict)
		if c.Slot != parser.SlotValue || !c.Arithmetic {
			t.Errorf("%s: slot %s arithmetic %v", tc.query, c.Slot, c.Arithmetic)
			continue
		}
		got := texts(c.Offer("web", "5", "7d", "1h", "2026-01-01", "1.5", "true", "two words"))
		if !reflect.DeepEqual(got, tc.want) && (len(got) > 0 || len(tc.want) > 0) {
			t.Errorf("%s: %q, want %q", tc.query, got, tc.want)
		}
	}
	// A plain value slot is not marked.
	if c, _ := at(t, "replicas>|", strict); c.Arithmetic {
		t.Error("replicas>: marked as an operand of arithmetic")
	}
}

// Nothing is offered that the field would refuse, from the schema's own
// list or from the host, in a quote or out of one.
func TestAt_OffersOnlyWhatTheFieldAccepts(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "ready", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
		{Name: "count", Type: validate.TypeInteger, AllowedOps: validate.NumericOps, Values: []string{"3", "1.5", "many", "42"}},
		{Name: "ttl", Type: validate.TypeDuration, AllowedOps: validate.DurationOps, Values: []string{"7d", "1h30m", "90m"}},
		{Name: "day", Type: validate.TypeDate, AllowedOps: validate.DateOps, Values: []string{"2026-01-01", "soon", "2026-1-5"}},
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps, Values: []string{"web", "812", "two words"}},
	}
	validator := validate.New(fields)
	for _, tc := range []struct {
		query string // with the cursor
		want  []string
		host  []string // what Offer keeps of 3, abc, true, 7d, 2026-01-01 and "two words"
	}{
		{"ready=|", []string{"true", "false"}, []string{"true"}},
		{`ready="|`, []string{"true", "false"}, []string{"true"}},
		{`ready="t|`, []string{"true"}, []string{"true"}},
		{"count=|", []string{"3", "42"}, []string{"3"}},
		{`count="|`, []string{"3", "42"}, []string{"3"}},
		{"count IN (1, |", []string{"3", "42"}, []string{"3"}},
		{"ttl<=|", []string{"7d", "90m"}, []string{"7d"}},
		{`ttl<="|`, []string{"7d", "90m"}, []string{"7d"}},
		// A date without its zeros is not one literal bare, so it goes in
		// quotes, where a date field takes it.
		{"day>|", []string{"2026-01-01", `"2026-1-5"`}, []string{"2026-01-01"}},
		{`day>"|`, []string{`"2026-01-01"`, `"2026-1-5"`}, []string{`"2026-01-01"`}},
		{"name=|", []string{"web", `"812"`, `"two words"`}, []string{`"3"`, "abc", `"true"`, `"7d"`, `"2026-01-01"`, `"two words"`}},
		{`name="|`, []string{`"web"`, `"812"`, `"two words"`}, []string{`"3"`, `"abc"`, `"true"`, `"7d"`, `"2026-01-01"`, `"two words"`}},
	} {
		offset := strings.IndexByte(tc.query, '|')
		query := tc.query[:offset] + tc.query[offset+1:]
		c := At(query, offset, fields, strict)
		if got := texts(c.Items); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: items %q, want %q", tc.query, got, tc.want)
		}
		host := c.Offer("3", "abc", "true", "7d", "2026-01-01", "two words")
		if got := texts(host); !reflect.DeepEqual(got, tc.host) {
			t.Errorf("%s: Offer keeps %q, want %q", tc.query, got, tc.host)
		}
		// Whatever is offered, once accepted, parses and validates.
		for _, it := range append(append([]Item{}, c.Items...), host...) {
			out := apply(query, c, it)
			if strings.Contains(out, " IN (") {
				out += ")"
			}
			expr, err := parser.ParseWith(out, strict)
			if err != nil {
				t.Errorf("%s: accepting %q gives %q, which does not parse: %v", tc.query, it.Text, out, err)
				continue
			}
			if err := validator.Validate(expr); err != nil {
				t.Errorf("%s: accepting %q gives %q, which the field refuses: %v", tc.query, it.Text, out, err)
			}
		}
	}
}

// Accepting a value leaves what follows it as it was: an operator, the dots
// of a range, the rest of a key.
func TestAt_AcceptingAValueKeepsWhatFollows(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "count", Type: validate.TypeInteger, AllowedOps: validate.NumericOps, Values: []string{"2", "40"}},
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps, Values: []string{"100", "250"}},
		{Name: "day", Type: validate.TypeDate, AllowedOps: validate.DateOps, Values: []string{"2026-01-01", "2026-01-15", "2026-02-01"}},
		{Name: "ready", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
		{Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	for _, tc := range []struct {
		query, pick, want string
	}{
		{"count>4|*2 AND ready=true", "2", "count>2*2 AND ready=true"},
		{"count>4|0*2", "40", "count>40*2"},
		{"day>2026-01-01|-7d", "2026-01-15", "day>2026-01-15-7d"},
		{"total:100|..500", "250", "total:250..500"},
		{"total:1..10|0 AND ready=true", "250", "total:1..250 AND ready=true"},
		// A date being typed is one word, with or without its last digits.
		{"day=2026-01-0|", "2026-01-01", "day=2026-01-01"},
		{"day=2026-0|", "2026-02-01", "day=2026-02-01"},
		{"day=2026-01-|", "2026-01-15", "day=2026-01-15"},
		{"day=2026-0|1-15 AND ready=true", "2026-01-01", "day=2026-01-01 AND ready=true"},
		{"day IN (2026-01-01, 2026-0|", "2026-02-01", "day IN (2026-01-01, 2026-02-01"},
	} {
		offset := strings.IndexByte(tc.query, '|')
		query := tc.query[:offset] + tc.query[offset+1:]
		c := At(query, offset, fields, strict)
		// Through Offer, so the candidate does not have to start with what
		// is already typed.
		offered := c.Offer(tc.pick)
		if len(offered) != 1 {
			t.Errorf("%s: %q is not offered (slot %s, prefix %q)", tc.query, tc.pick, c.Slot, c.Prefix)
			continue
		}
		picked := &offered[0]
		if got := apply(query, c, *picked); got != tc.want {
			t.Errorf("%s: accepting %q gives %q, want %q", tc.query, tc.pick, got, tc.want)
		}
	}
	// A key is replaced whole, dots and all.
	for _, tc := range []struct{ query, want string }{
		{"labels.exa|mple.com/team=x", `["labels.my key"]=x`},
		{"labels.a|.b.c AND name=x", `["labels.my key"] AND name=x`},
		{`["labels.my|" AND name="y"`, `["labels.my key"] AND name="y"`},
		{`["labels.my|"`, `["labels.my key"]`},
	} {
		offset := strings.IndexByte(tc.query, '|')
		query := tc.query[:offset] + tc.query[offset+1:]
		c := At(query, offset, fields, strict)
		items := c.Offer("my key")
		if c.Slot != parser.SlotKey || len(items) != 1 {
			t.Errorf("%s: slot %s, offered %q", tc.query, c.Slot, texts(items))
			continue
		}
		got := apply(query, c, items[0])
		if got != tc.want {
			t.Errorf("%s: accepting the key gives %q, want %q", tc.query, got, tc.want)
		}
		if _, err := parser.ParseWith(got, strict); err != nil {
			t.Errorf("%s: %q does not parse: %v", tc.query, got, err)
		}
	}
}
