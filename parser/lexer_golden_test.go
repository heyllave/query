package parser

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

var updateGolden = flag.Bool("update", false, "rewrite the hashes in testdata/lexer_golden.txt from the current lexer")

const goldenPath = "testdata/lexer_golden.txt"

// TestLexer_Golden pins what the lexer, and the parser and cursor built on
// it, produce for several thousand inputs: the shared corpus, the queries of
// every review round, and generated runs of digits, signs, dates, zones,
// quotes, brackets and keywords. Each line of testdata/lexer_golden.txt is a
// hash of goldenEntry (tokens, positions, errors, hints and the cursor
// answer) and the input it was made from, so a change to any of them for any
// input fails here and names the input.
//
// After a deliberate change, regenerate with: go test ./parser -run Golden -update
// To see the full entries, set LEXER_GOLDEN_DUMP to a file path.
func TestLexer_Golden(t *testing.T) {
	lines := readGolden(t)
	var dump strings.Builder
	var out strings.Builder
	changed := 0
	for _, l := range lines {
		entry := goldenEntry(l.input)
		dump.WriteString(entry)
		sum := goldenHash(entry)
		q, _ := json.Marshal(l.input)
		fmt.Fprintf(&out, "%s %s\n", sum, q)
		if sum != l.hash && !*updateGolden {
			changed++
			if changed <= 10 {
				t.Errorf("the lexer's answer for %s changed; it is now:\n%s", q, entry)
			}
		}
	}
	if path := os.Getenv("LEXER_GOLDEN_DUMP"); path != "" {
		if err := os.WriteFile(path, []byte(dump.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if *updateGolden {
		if err := os.WriteFile(goldenPath, []byte(out.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if changed > 10 {
		t.Errorf("%d inputs changed in all", changed)
	}
}

type goldenLine struct{ hash, input string }

func readGolden(t *testing.T) []goldenLine {
	t.Helper()
	f, err := os.Open(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var lines []goldenLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		hash, q, ok := strings.Cut(sc.Text(), " ")
		var s string
		if !ok || json.Unmarshal([]byte(q), &s) != nil {
			t.Fatalf("%s: malformed line %q", goldenPath, sc.Text())
		}
		lines = append(lines, goldenLine{hash, s})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func goldenHash(entry string) string {
	sum := sha256.Sum256([]byte(entry))
	return hex.EncodeToString(sum[:8])
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
