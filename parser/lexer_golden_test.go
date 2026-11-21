package parser

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

var (
	updateGolden = flag.Bool("update", false, "rewrite testdata/lexer_golden from the current lexer")
	growGolden   = flag.Int("grow", 0, "add this many generated inputs to testdata/lexer_golden/inputs.txt (implies -update)")
)

const (
	goldenDir    = "testdata/lexer_golden"
	goldenInputs = "testdata/lexer_golden/inputs.txt"
	// goldenShards is how many files the entries are spread over, so each
	// stays small enough for a code review to show its diff.
	goldenShards = 16
)

// TestLexer_Golden pins what the lexer, and the parser and cursor built on
// it, produce for several thousand inputs: the shared corpus, the queries of
// every review round, and generated runs of digits, signs, dates, zones,
// quotes, brackets and keywords.
//
// testdata/lexer_golden/inputs.txt lists the inputs, one JSON string per
// line. The NN.txt files beside it hold, for each input, its goldenEntry in
// full: tokens, positions, errors, hints and the cursor answer. An input's
// file is fixed by a hash of the input, so adding one changes one file. A
// change to the lexer's answer for any input fails here, shows the recorded
// entry next to the new one, and after -update is a readable diff.
//
// After a deliberate change:  go test ./parser -run Golden -update
// To add an input, append it to inputs.txt and run with -update.
// To add generated inputs:    go test ./parser -run Golden -grow 500
func TestLexer_Golden(t *testing.T) {
	inputs := readGoldenInputs(t)
	if *growGolden > 0 {
		inputs = append(inputs, generateGoldenInputs(inputs, *growGolden)...)
		writeGoldenInputs(t, inputs)
	}
	shards := make([]strings.Builder, goldenShards)
	for _, in := range inputs {
		shards[goldenShard(in)].WriteString(goldenEntry(in))
	}
	if *updateGolden || *growGolden > 0 {
		for i := range shards {
			if err := os.WriteFile(goldenShardPath(i), []byte(shards[i].String()), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	changed := 0
	for i := range shards {
		changed += compareGoldenShard(t, i, shards[i].String(), changed)
	}
	if changed > 10 {
		t.Errorf("%d inputs changed in all; run with -update and read the diff", changed)
	}
}

// compareGoldenShard reports each entry of shard i that differs from the
// recorded one, the first few in full, and returns how many differ.
func compareGoldenShard(t *testing.T, i int, got string, before int) int {
	t.Helper()
	b, err := os.ReadFile(goldenShardPath(i))
	if err != nil {
		t.Fatal(err)
	}
	want := string(b)
	if got == want {
		return 0
	}
	recorded := splitGoldenEntries(want)
	current := splitGoldenEntries(got)
	changed := 0
	for q, entry := range current {
		if recorded[q] == entry {
			continue
		}
		changed++
		if before+changed <= 10 {
			t.Errorf("the lexer's answer changed for %s\n%s", q, goldenLineDiff(recorded[q], entry))
		}
	}
	for q := range recorded {
		if _, ok := current[q]; !ok {
			changed++
			t.Errorf("%s: %s is recorded but no longer in inputs.txt; run with -update", goldenShardPath(i), q)
		}
	}
	return changed
}

// goldenLineDiff shows the lines of an entry that changed, the recorded one
// marked - and the current one +. Every entry has the same lines in the
// same order, so they pair up.
func goldenLineDiff(recorded, current string) string {
	if recorded == "" {
		return "(a new input; run with -update)\n" + current
	}
	was := strings.Split(strings.TrimSuffix(recorded, "\n"), "\n")
	now := strings.Split(strings.TrimSuffix(current, "\n"), "\n")
	var b strings.Builder
	for i := 0; i < max(len(was), len(now)); i++ {
		w, n := lineAt(was, i), lineAt(now, i)
		if w != n {
			fmt.Fprintf(&b, "- %s\n+ %s\n", w, n)
		}
	}
	return b.String()
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// splitGoldenEntries maps each entry's "Q ..." line to the whole entry.
func splitGoldenEntries(s string) map[string]string {
	out := map[string]string{}
	var q string
	var cur strings.Builder
	flush := func() {
		if q != "" {
			out[q] = cur.String()
		}
		cur.Reset()
	}
	for _, line := range strings.SplitAfter(s, "\n") {
		if strings.HasPrefix(line, "Q ") {
			flush()
			q = strings.TrimSuffix(line, "\n")
		}
		cur.WriteString(line)
	}
	flush()
	return out
}

// goldenShard is the file an input's entry lives in.
func goldenShard(in string) int {
	sum := sha256.Sum256([]byte(in))
	return int(sum[0]) % goldenShards
}

func goldenShardPath(i int) string {
	return filepath.Join(goldenDir, fmt.Sprintf("%02x.txt", i))
}

func readGoldenInputs(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(goldenInputs)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var inputs []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var s string
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			t.Fatalf("%s: malformed line %q: %v", goldenInputs, sc.Text(), err)
		}
		if seen[s] {
			t.Fatalf("%s: %q is listed twice", goldenInputs, s)
		}
		seen[s] = true
		inputs = append(inputs, s)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return inputs
}

func writeGoldenInputs(t *testing.T, inputs []string) {
	t.Helper()
	var b strings.Builder
	for _, in := range inputs {
		q, _ := json.Marshal(in)
		b.Write(q)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(goldenInputs, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// goldenPieces are what generated inputs are glued together from: names,
// operators, digits, signs, dates, zones, durations, quotes, brackets,
// calls, connectives, escapes and Unicode spaces.
var goldenPieces = []string{
	"a", "x", "labels.team", "f(", "now()", "(", ")", ",", "[", "]", "[a]", `["k-1"]`,
	`"`, `"q"`, `\`, `\*`, "*", "@first", "=", "!=", ">", ">=", "<", "!<=", ":", "..", ".",
	"+", "-", "/", "%", " ", "  ", " ", "0", "1", "10", "1.5", "1.2.3", "2026",
	"2026-11-16", "2026-1", "T10:00:00", "Z", "+02:00", "7d", "30m", "1h", "e",
	"AND", "OR", "NOT", "IN", "in", "and", "true", "é",
}

// generateGoldenInputs returns n new inputs glued from goldenPieces, none
// already in have. The generator is seeded by len(have), so the same
// inputs.txt grown by the same n gives the same inputs.
func generateGoldenInputs(have []string, n int) []string {
	seen := make(map[string]bool, len(have))
	for _, s := range have {
		seen[s] = true
	}
	r := rand.New(rand.NewSource(int64(len(have))))
	var out []string
	for len(out) < n {
		var b strings.Builder
		for k := 2 + r.Intn(8); k > 0; k-- {
			b.WriteString(goldenPieces[r.Intn(len(goldenPieces))])
		}
		if s := b.String(); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// goldenEntry renders everything the lexer decides for one input.
func goldenEntry(in string) string {
	q, _ := json.Marshal(in)
	var b strings.Builder
	fmt.Fprintf(&b, "Q %s\n", q)
	toks, err := lexWith(in, Config{})
	fmt.Fprintf(&b, "  lex     %s\n", goldenTokens(toks, err))
	toks, err = lexWith(in, Config{}.Strict())
	fmt.Fprintf(&b, "  strict  %s\n", goldenTokens(toks, err))
	toks, err = LexValue(in, 0)
	fmt.Fprintf(&b, "  value   %s\n", goldenTokens(toks, err))
	expr, err := ParseWith(in, Config{}.Strict())
	fmt.Fprintf(&b, "  parse   %s\n", goldenParse(expr, err))
	fmt.Fprintf(&b, "  cursor  %+v\n", CursorAt(in, len(in), Config{}))
	return b.String()
}

func goldenTokens(toks []token.Token, err error) string {
	parts := make([]string, 0, len(toks)+1)
	for _, tk := range toks {
		parts = append(parts, fmt.Sprintf("%v:%q@%d+%d", tk.Type, tk.Value, tk.Pos.Offset, tk.Pos.Length))
	}
	if err != nil {
		parts = append(parts, "ERR "+goldenErr(err))
	}
	return strings.Join(parts, " ")
}

func goldenParse(expr ast.Expression, err error) string {
	if err != nil {
		return "ERR " + goldenErr(err)
	}
	return ast.String(expr)
}

func goldenErr(err error) string {
	var list ErrorList
	if !errors.As(err, &list) {
		return err.Error()
	}
	parts := make([]string, 0, len(list))
	for _, e := range list {
		parts = append(parts, fmt.Sprintf("%s@%d+%d %q hint=%q", e.Code, e.Position.Offset, e.Position.Length, e.Message, e.Hint))
	}
	return strings.Join(parts, " | ")
}
