package bridgejson

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/golden.jsonl from the current behaviour")

// goldenEntry is what one query means: how it prints back and the tree a
// client receives, or the stable codes of the errors it fails with. English
// messages are left out on purpose; they may be reworded.
type goldenEntry struct {
	Query string   `json:"query"`
	Print string   `json:"print,omitempty"`
	AST   *AST     `json:"ast,omitempty"`
	Codes []string `json:"codes,omitempty"`
}

func goldenFor(query string) goldenEntry {
	entry := goldenEntry{Query: query}
	env := Parse(query, DefaultMaxLength)
	if tree, ok := env["result"].(*AST); ok {
		entry.AST = tree
		tree, err := json.Marshal(tree)
		if err != nil {
			panic(err)
		}
		entry.Print, _ = Stringify(string(tree))["result"].(string)
		return entry
	}
	list, _ := env["errors"].([]map[string]any)
	for _, e := range list {
		code, _ := e["code"].(string)
		entry.Codes = append(entry.Codes, code)
	}
	return entry
}

// TestGolden_LanguageIsStable pins what every query in testdata/queries.txt
// parses to. A change to the grammar, to the canonical printed form, to the
// JSON tree clients receive or to an error code shows up here as a diff, so
// none of them can change by accident. An intended change is recorded with
//
//	go test ./internal/bridgejson -run TestGolden -update
//
// and reviewed as the diff of testdata/golden.jsonl: every changed line is a
// query that now means something else to a caller who already depends on it.
func TestGolden_LanguageIsStable(t *testing.T) {
	queries := readLines(t, "testdata/queries.txt")
	if len(queries) < 300 {
		t.Fatalf("testdata/queries.txt has %d queries, expected at least 300 (truncated?)", len(queries))
	}

	var current bytes.Buffer
	enc := json.NewEncoder(&current)
	enc.SetEscapeHTML(false)
	for _, q := range queries {
		if err := enc.Encode(goldenFor(q)); err != nil {
			t.Fatal(err)
		}
	}

	if *update {
		if err := os.WriteFile("testdata/golden.jsonl", current.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want := readLines(t, "testdata/golden.jsonl")
	got := strings.Split(strings.TrimSuffix(current.String(), "\n"), "\n")
	if len(want) != len(got) {
		t.Fatalf("golden.jsonl has %d entries for %d queries; rerun with -update and review the diff", len(want), len(got))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("query %q changed meaning\n was: %s\n now: %s", queries[i], want[i], got[i])
		}
	}
}

// TestGolden_PrintedFormIsAFixedPoint checks the promise the golden file
// records: the printed form of a query parses to a tree that prints the same.
func TestGolden_PrintedFormIsAFixedPoint(t *testing.T) {
	for _, q := range readLines(t, "testdata/queries.txt") {
		first := goldenFor(q)
		if first.AST == nil {
			continue
		}
		t.Run(q, func(t *testing.T) {
			second := goldenFor(first.Print)

			if second.Print != first.Print {
				t.Errorf("%q prints as %q, which prints as %q", q, first.Print, second.Print)
			}
		})
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if sc.Text() != "" {
			lines = append(lines, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}
