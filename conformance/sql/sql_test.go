package sql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/heyllave/query"
	"github.com/heyllave/query/eval"
)

// record is one invoice row. Pointers model NULL.
type record struct {
	ID        int      `json:"id"`
	State     string   `json:"state"`
	Customer  *string  `json:"customer"`
	Email     *string  `json:"email"`
	Notes     *string  `json:"notes"`
	Zip       *string  `json:"zip"`
	Country   *string  `json:"country"`
	Total     float64  `json:"total"`
	Qty       int64    `json:"qty"`
	Paid      bool     `json:"paid"`
	CreatedAt string   `json:"created_at"`
	Tags      []string `json:"tags,omitempty"`
}

// testCase is one entry of cases.json.
type testCase struct {
	ID          string `json:"id"`
	Desc        string `json:"desc"`
	Query       string `json:"query"`
	Expect      []int  `json:"expect"`
	ExpectError bool   `json:"expectError"`
	// SkipSQL names a documented, deliberate difference between the
	// evaluator and SQLite for this case (e.g. non-ASCII case folding). The
	// evaluator is still checked against Expect.
	SkipSQL string `json:"skipSQL"`
}

func loadJSON[T any](t *testing.T, path string) T {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return v
}

func (r record) asMap() map[string]any {
	m := map[string]any{
		"id": r.ID, "state": r.State, "total": r.Total, "qty": r.Qty, "paid": r.Paid, "created_at": r.CreatedAt,
	}
	for k, p := range map[string]*string{"customer": r.Customer, "email": r.Email, "notes": r.Notes, "zip": r.Zip, "country": r.Country} {
		if p != nil {
			m[k] = *p
		}
	}
	return m
}

func openDB(t *testing.T, records []record) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(CreateTable()); err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		paid := 0
		if r.Paid {
			paid = 1
		}
		_, err := db.Exec(`INSERT INTO invoices (id, state, customer, email, notes, zip, country, total, qty, paid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, r.State, nullable(r.Customer), nullable(r.Email), nullable(r.Notes), nullable(r.Zip), nullable(r.Country),
			r.Total, r.Qty, paid, r.CreatedAt)
		if err != nil {
			t.Fatalf("insert %d: %v", r.ID, err)
		}
	}
	return db
}

func nullable(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func idsOf(ids []int) string {
	sort.Ints(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// TestCorpus_IsWellFormed validates the test data itself before anything
// runs against it: unique case ids, every expected id a real record, each
// case either expecting rows or an error, every record complete and its
// timestamp in the canonical form the dialect relies on.
func TestCorpus_IsWellFormed(t *testing.T) {
	records := loadJSON[[]record](t, "testdata/records.json")
	cases := loadJSON[[]testCase](t, "testdata/cases.json")
	if len(records) < 10 || len(cases) < 50 {
		t.Fatalf("corpus too small: %d records, %d cases", len(records), len(cases))
	}
	known := map[int]bool{}
	for _, r := range records {
		if known[r.ID] {
			t.Errorf("record id %d is duplicated", r.ID)
		}
		known[r.ID] = true
		ts, err := time.Parse(time.RFC3339, r.CreatedAt)
		if err != nil || ts.Location() != time.UTC || ts.Nanosecond() != 0 || !strings.HasSuffix(r.CreatedAt, "Z") {
			t.Errorf("record %d: created_at %q must be RFC 3339 UTC with no fraction (Z suffix)", r.ID, r.CreatedAt)
		}
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] {
			t.Errorf("case id %q is empty or duplicated", c.ID)
		}
		seen[c.ID] = true
		if c.Query == "" || c.Desc == "" {
			t.Errorf("case %s: query and desc are required", c.ID)
		}
		if c.ExpectError && c.Expect != nil {
			t.Errorf("case %s: expectError and expect are exclusive", c.ID)
		}
		if !c.ExpectError && c.Expect == nil {
			t.Errorf("case %s: needs expect (use [] for no rows) or expectError", c.ID)
		}
		for _, id := range c.Expect {
			if !known[id] {
				t.Errorf("case %s: expected id %d is not a record", c.ID, id)
			}
		}
		if dup := slices.Compact(slices.Clone(c.Expect)); len(dup) != len(c.Expect) {
			t.Errorf("case %s: expect has duplicate ids", c.ID)
		}
	}
}

// TestDifferential runs every case through the evaluator and through SQLite
// and requires both to return the expected ids.
func TestDifferential(t *testing.T) {
	records := loadJSON[[]record](t, "testdata/records.json")
	cases := loadJSON[[]testCase](t, "testdata/cases.json")
	db := openDB(t, records)

	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			expr, err := query.ParseAndValidate(c.Query, Schema)
			if c.ExpectError {
				if err == nil {
					_, err = eval.Compile(c.Query, Schema)
				}
				if err == nil {
					t.Fatalf("%s: expected an error, got none", c.Query)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: %v", c.Query, err)
			}

			// Evaluator.
			prog, err := eval.Compile(c.Query, Schema)
			if err != nil {
				t.Fatalf("%s: compile: %v", c.Query, err)
			}
			var got []int
			for _, r := range records {
				if prog.Match(r.asMap()) {
					got = append(got, r.ID)
				}
			}
			if idsOf(got) != idsOf(slices.Clone(c.Expect)) {
				t.Errorf("%s\n  evaluator: %s\n  expected:  %s", c.Query, idsOf(got), idsOf(slices.Clone(c.Expect)))
			}

			// SQLite.
			if c.SkipSQL != "" {
				t.Logf("SQL skipped: %s", c.SkipSQL)
				return
			}
			where, params, err := Where(expr)
			if err != nil {
				t.Fatalf("%s: lower: %v", c.Query, err)
			}
			rows, err := db.Query("SELECT id FROM invoices WHERE "+where+" ORDER BY id", params...)
			if err != nil {
				t.Fatalf("%s\n  SQL: %s\n  %v", c.Query, where, err)
			}
			defer func() { _ = rows.Close() }()
			var sqlGot []int
			for rows.Next() {
				var id int
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				sqlGot = append(sqlGot, id)
			}
			if idsOf(sqlGot) != idsOf(slices.Clone(c.Expect)) {
				t.Errorf("%s\n  SQL:      %s\n  expected: %s\n  WHERE %s %v", c.Query, idsOf(sqlGot), idsOf(slices.Clone(c.Expect)), where, params)
			}
		})
	}
}

// TestDialect_RejectsUnsupported pins what the dialect refuses rather than
// mistranslates.
func TestDialect_RejectsUnsupported(t *testing.T) {
	for _, q := range []string{"tags@(x=1)", "contains(customer, \"a\")", "created_at>=now()-7d", "nope=1"} {
		expr, err := query.Parse(q)
		if err != nil {
			t.Fatalf("Parse(%q): %v", q, err)
		}
		if _, _, err := Where(expr); err == nil {
			t.Errorf("Where(%q) succeeded, want an error", q)
		}
	}
}
