package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heyllave/query/validate"
)

const records = `{"id":1,"state":"draft","total":60000,"name":"Joan","created_at":"2026-11-10","labels":{"team":"payments"},"orders":[{"status":"shipped","price":5}],"tags":["urgent"]}
{"id":2,"state":"Issued","total":100,"name":"bob","created_at":"2025-01-01","labels":{"team":"core"},"orders":[],"tags":[]}
{"id":3,"state":"draft","total":70000,"nickname":"Jo","created_at":"2026-11-14T10:00:00Z","orders":[{"status":"cancelled","price":0}]}
`

// runPipe runs a command over stdin and returns exit code, stdout, stderr.
func runPipe(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	stdout, stderr := tempFiles(t)
	code := runWith(args, strings.NewReader(stdin), stdout, stderr)
	return code, readTemp(t, stdout), readTemp(t, stderr)
}

// ids reads the "id" of each output line, so a test states which records
// came through.
func ids(t *testing.T, out string) string {
	t.Helper()
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		i := strings.Index(line, `"id":`)
		if i < 0 {
			t.Fatalf("line without an id: %s", line)
		}
		got = append(got, line[i+5:i+6])
	}
	return strings.Join(got, ",")
}

func TestFilter_TypesFieldsFromTheRecords(t *testing.T) {
	// Arrange: the queries of the jq comparison, with the records above.
	cases := []struct {
		query string
		want  string
	}{
		{"state=draft AND total>50000", "1,3"},
		{"name=jo*", "1"},
		{"state IN (draft, issued, paid)", "1,2,3"},
		{"created_at:2026-01-01..2026-12-31", "1,3"},
		{"created_at>=now()-30d", "1,3"},
		{"labels.team=payments", "1"},
		{"nickname", "3"},
		{"total>=[id]*30000", "1"},
		{"orders@(status=shipped)", "1"},
		{"orders@all(price>0)", "1,2"},
		{"orders@none(status=cancelled)", "1,2"},
		{"len(name)>3", "1"},
		{`contains(tags, "urgent")`, "1"},
		{`coalesce(nickname, name)="Jo"`, "3"},
		{"missing=1", ""},
		{"NOT state=draft", "2"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			// Act
			code, out, errOut := runPipe(t, records, "filter", tc.query)

			// Assert
			if code != 0 {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if got := ids(t, out); got != tc.want {
				t.Errorf("filter %q passed records %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestFilter_WritesRecordsBackAsRead(t *testing.T) {
	in := "{\"z\": 1, \"a\": 1.50, \"n\": {\"k\": [1, 2]}}\n{\"z\": 2, \"a\": 0}\n"

	code, out, errOut := runPipe(t, in, "filter", "a>1")

	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := "{\"z\":1,\"a\":1.50,\"n\":{\"k\":[1,2]}}\n"; out != want {
		t.Errorf("output %q, want the record compacted with its key order and number spelling %q", out, want)
	}
}

func TestFilter_ArrayInArrayOut(t *testing.T) {
	in := "[{\"z\":1,\"a\":2},\n {\"z\":2,\"a\":0}]\n"

	code, out, _ := runPipe(t, in, "filter", "a>1")
	if code != 0 || out != "[{\"z\":1,\"a\":2}]\n" {
		t.Errorf("array in: exit %d, output %q", code, out)
	}

	code, out, _ = runPipe(t, in, "filter", "--pretty", "a>1")
	if code != 0 || out != "[\n  {\n    \"z\": 1,\n    \"a\": 2\n  }\n]\n" {
		t.Errorf("pretty array: exit %d, output %q", code, out)
	}

	code, out, _ = runPipe(t, in, "filter", "a>5")
	if code != 0 || out != "[]\n" {
		t.Errorf("empty result: exit %d, output %q", code, out)
	}

	code, out, _ = runPipe(t, "", "filter", "a>5")
	if code != 0 || out != "" {
		t.Errorf("empty input: exit %d, output %q", code, out)
	}
}

func TestFilter_UsesTheSchemaWhenGiven(t *testing.T) {
	dir := t.TempDir()
	schema := filepath.Join(dir, "fields.json")
	if err := os.WriteFile(schema, []byte(`{"fields":[{"name":"total","type":"integer","allowed_ops":["="]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runPipe(t, records, "filter", "--schema", schema, "total>5")

	if code != 1 || !strings.Contains(errOut, `operator ">" is not allowed`) {
		t.Errorf("exit %d, stderr %q, want the schema's operator refusal", code, errOut)
	}
}

func TestFilter_ReportsErrors(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"missing condition", records, []string{"filter"}, "missing condition"},
		{"syntax error with its code", records, []string{"filter", "state==x"}, "[unexpectedChar]"},
		{"a record that is not an object", "{\"a\":1}\n5\n", []string{"filter", "a=1"}, "record 2 is not an object"},
		{"invalid JSON", "{\"a\":1}\n{\"a\":\n", []string{"filter", "a=1"}, "record 2: invalid JSON"},
		{"unknown flag", records, []string{"filter", "--nope", "a=1"}, "flag provided but not defined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runPipe(t, tc.stdin, tc.args...)
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr %q, want it to mention %q", errOut, tc.want)
			}
		})
	}
}

func TestEval_WritesAValuePerRecord(t *testing.T) {
	code, out, errOut := runPipe(t, records, "eval", "[total]*2")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := "120000\n200\n140000\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}

	code, out, _ = runPipe(t, records, "eval", "upper(coalesce(nickname, name))")
	if code != 0 || out != "\"JOAN\"\n\"BOB\"\n\"JO\"\n" {
		t.Errorf("strings: exit %d, output %q", code, out)
	}

	code, out, _ = runPipe(t, records, "eval", "--raw", "upper(coalesce(nickname, name))")
	if code != 0 || out != "JOAN\nBOB\nJO\n" {
		t.Errorf("--raw: exit %d, output %q", code, out)
	}

	code, out, _ = runPipe(t, records, "eval", "-r", "[labels.team]")
	if code != 0 || out != "payments\ncore\nnull\n" {
		t.Errorf("a missing value is null: exit %d, output %q", code, out)
	}

	code, _, errOut = runPipe(t, records, "eval", "[total]+")
	if code != 1 || !strings.Contains(errOut, "error") {
		t.Errorf("a broken expression: exit %d, stderr %q", code, errOut)
	}
}

func TestMap_ReshapesEachRecord(t *testing.T) {
	code, out, errOut := runPipe(t, records, "map", `id, total=[total]*2, team=[labels.team], label="x"`)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	want := `{"id":1,"total":120000,"team":"payments","label":"x"}` + "\n" +
		`{"id":2,"total":200,"team":"core","label":"x"}` + "\n" +
		`{"id":3,"total":140000,"team":null,"label":"x"}` + "\n"
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestMap_EntriesMayBeSeparateArguments(t *testing.T) {
	code, out, _ := runPipe(t, records, "map", "id", "n=len(name)")
	if code != 0 || out != "{\"id\":1,\"n\":4}\n{\"id\":2,\"n\":3}\n{\"id\":3,\"n\":null}\n" {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestMap_KeepsNestedPathsAndFunctionArgumentsWhole(t *testing.T) {
	code, out, _ := runPipe(t, `{"a":{"b":1},"xs":[1,2]}`+"\n", "map", "a.b, s=sum([xs]), m=max([a.b], 5)")
	if code != 0 || out != "{\"a.b\":1,\"s\":3,\"m\":5}\n" {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestMap_ReportsErrors(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want string
	}{
		{"an expression without a name", "upper([name])", "needs a name"},
		{"a name without an expression", "x=", "has no expression"},
		{"an empty projection", " , ", "empty projection"},
		{"a syntax error in an entry", "x=[a]+", "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runPipe(t, records, "map", tc.spec)
			if code != 1 || !strings.Contains(errOut, tc.want) {
				t.Errorf("exit %d, stderr %q, want it to mention %q", code, errOut, tc.want)
			}
		})
	}
}

func TestSplitTopLevel(t *testing.T) {
	got := splitTopLevel(`a, b=f(x, y), c="p,q", d=[e,f]`, ',')
	want := []string{"a", " b=f(x, y)", ` c="p,q"`, " d=[e,f]"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("split = %q, want %q", got, want)
	}
}

func TestRecordSchema_TypesFromSamples(t *testing.T) {
	s := &recordSchema{}
	s.samples = map[string]any{
		"n": 1.5, "b": true, "d": "2026-01-01", "t": "x", "o": map[string]any{}, "l": []any{},
	}
	for name, want := range map[string]validate.FieldValueType{
		"n": validate.TypeDecimal, "b": validate.TypeBoolean, "d": validate.TypeDatetime,
		"t": validate.TypeText, "o": validate.TypeText, "l": validate.TypeText, "unseen": validate.TypeText,
	} {
		cfg, ok := s.GetFieldConfig(name)
		if !ok || cfg.Type != want {
			t.Errorf("%s typed %v, want %v", name, cfg.Type, want)
		}
	}
	if cfg, _ := s.GetFieldConfig("o"); !cfg.Nested {
		t.Error("an object is a nested field")
	}
	if cfg, _ := s.GetFieldConfig("unseen"); !cfg.AllowsOp(validate.OpGt) || !cfg.AllowsOp(validate.OpWildcard) {
		t.Error("an unseen field allows every operator")
	}
	if cfg, _ := s.GetFieldConfig("t"); cfg.AllowsOp(validate.OpGt) {
		t.Error("a text field seen in the records keeps the text operators")
	}
}
